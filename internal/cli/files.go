package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"slices"

	"github.com/mooncitizen/togen/internal/ir"
)

type CliError struct{ Message string }

func (e *CliError) Error() string { return e.Message }

type Config struct {
	Version int      `json:"version"`
	Targets []string `json:"targets"`
	OutDir  string   `json:"outDir"`
}

var DefaultConfig = Config{Version: 1, Targets: []string{"hcl"}, OutDir: "infra"}

func defaultConfig() Config {
	c := DefaultConfig
	c.Targets = slices.Clone(DefaultConfig.Targets)
	return c
}

func readJSONFile(path, cwd string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, &CliError{fmt.Sprintf("%s not found. Run 'togen init' first.", shownPath(cwd, path))}
	}
	if !json.Valid(raw) {
		return nil, &CliError{fmt.Sprintf("%s is not valid JSON", shownPath(cwd, path))}
	}
	return raw, nil
}

func writeJSONFile(path string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o644)
}

func LoadProject(cwd string) (*ir.Project, ir.Errors, error) {
	raw, err := readJSONFile(projectPath(cwd), cwd)
	if err != nil {
		return nil, nil, err
	}
	migrated, err := ir.Migrate(raw)
	if err != nil {
		return nil, nil, &CliError{err.Error()}
	}
	project, errs := ir.ValidateProject(migrated)
	if len(errs) > 0 {
		return nil, errs, nil
	}
	return project, nil, nil
}

func LoadConfig(cwd string) (Config, error) {
	shown := shownPath(cwd, configPath(cwd))
	raw, err := readJSONFile(configPath(cwd), cwd)
	if err != nil {
		return Config{}, err
	}
	config := defaultConfig()
	if err := json.Unmarshal(raw, &config); err != nil {
		var mismatch *json.UnmarshalTypeError
		if errors.As(err, &mismatch) {
			return Config{}, &CliError{fmt.Sprintf("%s: %s must be a %s", shown, mismatch.Field, jsonTypeName(mismatch.Type))}
		}
		return Config{}, &CliError{fmt.Sprintf("%s: %s", shown, err)}
	}
	if config.Version > ir.Version {
		return Config{}, &CliError{fmt.Sprintf("%s is version %d but this Togen only understands up to %d", shown, config.Version, ir.Version)}
	}
	if len(config.Targets) == 0 {
		return Config{}, &CliError{fmt.Sprintf("%s has no targets", shown)}
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

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}
