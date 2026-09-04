package ir

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

func Migrate(raw []byte) ([]byte, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var doc any
	if err := d.Decode(&doc); err != nil {
		return nil, fmt.Errorf("project file is not valid JSON: %w", err)
	}
	obj, ok := doc.(map[string]any)
	if !ok {
		return nil, errors.New("project file must be a JSON object")
	}
	version, ok := obj["version"].(json.Number)
	if !ok {
		return nil, errors.New("project file has no numeric 'version' field")
	}
	n, err := version.Float64()
	if err != nil {
		return nil, errors.New("project file has no numeric 'version' field")
	}
	switch {
	case n == Version:
		return raw, nil
	case n > Version:
		return nil, fmt.Errorf("project file is version %s but this Togen only understands up to %d", version, Version)
	default:
		return nil, fmt.Errorf("no migration from version %s", version)
	}
}
