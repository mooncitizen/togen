package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/mooncitizen/togen/internal/ir"
)

type Error struct{ Message string }

func (e *Error) Error() string { return e.Message }

type Config struct {
	Version int      `json:"version"`
	Targets []string `json:"targets"`
	OutDir  string   `json:"outDir"`
}

func DefaultConfig() Config {
	return Config{Version: 1, Targets: []string{"hcl"}, OutDir: "infra"}
}

func ReadJSONFile(path, cwd string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, &Error{fmt.Sprintf("%s not found. Run 'togen init' first.", shownPath(cwd, path))}
	}
	if !json.Valid(raw) {
		return nil, &Error{fmt.Sprintf("%s is not valid JSON", shownPath(cwd, path))}
	}
	return raw, nil
}

func WriteJSONFile(path string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return WriteRaw(path, append(raw, '\n'))
}

// Written via a sibling temp file and a rename, so a reader never sees a
// truncated file: a plain WriteFile is visible half-written to anyone
// reading concurrently, and the canvas writes this constantly.
func WriteRaw(path string, raw []byte) error {
	dir := filepath.Dir(path)
	stem := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	tmp, err := os.CreateTemp(dir, "."+stem+"-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer func() { _ = os.Remove(name) }()

	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(name, 0o644); err != nil {
		return err
	}
	return os.Rename(name, path)
}

func LoadConfig(cwd string) (Config, error) {
	shown := shownPath(cwd, ConfigPath(cwd))
	raw, err := ReadJSONFile(ConfigPath(cwd), cwd)
	if err != nil {
		return Config{}, err
	}
	config := DefaultConfig()
	if err := json.Unmarshal(raw, &config); err != nil {
		var mismatch *json.UnmarshalTypeError
		if errors.As(err, &mismatch) {
			return Config{}, &Error{fmt.Sprintf("%s: %s must be a %s", shown, mismatch.Field, jsonTypeName(mismatch.Type))}
		}
		return Config{}, &Error{fmt.Sprintf("%s: %s", shown, err)}
	}
	if config.Version > ir.Version {
		return Config{}, &Error{fmt.Sprintf("%s is version %d but this Togen only understands up to %d", shown, config.Version, ir.Version)}
	}
	if len(config.Targets) == 0 {
		return Config{}, &Error{fmt.Sprintf("%s has no targets", shown)}
	}
	return config, nil
}

func jsonTypeName(t reflect.Type) string {
	switch t.Kind() {
	case reflect.String:
		return "string"
	case reflect.Bool:
		return "boolean"
	case reflect.Slice, reflect.Array:
		return "array"
	case reflect.Map, reflect.Struct:
		return "object"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return "number"
	}
	return t.String()
}

func Exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}
