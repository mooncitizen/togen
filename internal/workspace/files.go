package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

type Error struct{ Message string }

func (e *Error) Error() string { return e.Message }

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
	raw, err := Marshal(value)
	if err != nil {
		return err
	}
	return WriteRaw(path, raw)
}

// The bytes every file under togen/ is written with.
func Marshal(value any) ([]byte, error) {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
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
