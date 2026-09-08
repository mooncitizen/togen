package workspace

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"

	"github.com/mooncitizen/togen/internal/catalogue"
	"github.com/mooncitizen/togen/internal/cost"
	"github.com/mooncitizen/togen/internal/ir"
)

const (
	configSchemaURL = "https://togen.dev/schema/togen.schema.json"

	DefaultTheme = "dark"
	LegacyNote   = "togen/togen.json is deprecated; run togen init --migrate to move it to togen.yml"
	iconMessage  = "must be a bundled icon id or a relative path"
)

var Themes = []string{DefaultTheme, "light", "system"}

// Written by just generate: go:embed cannot reach schema/ from here.
//
//go:embed togen.schema.json
var configSchemaJSON []byte

var configSchema = &compiledSchema{url: configSchemaURL, raw: configSchemaJSON}

type Config struct {
	Version int        `json:"version" yaml:"version" jsonschema:"minimum=1,description=File format version"`
	Targets []string   `json:"targets" yaml:"targets" jsonschema:"minItems=1,description=Code generators to run"`
	OutDir  string     `json:"outDir"  yaml:"outDir"  jsonschema:"minLength=1,description=Directory the generated code is written to"`
	Style   *Style     `json:"style,omitempty" yaml:"style,omitempty" jsonschema:"description=How the studio draws the diagram"`
	Usage   cost.Usage `json:"usage,omitempty" yaml:"usage,omitempty" jsonschema:"description=Expected monthly usage per node, for togen cost"`
}

type Style struct {
	Theme string                    `json:"theme,omitempty" yaml:"theme,omitempty" jsonschema:"description=Colour scheme the studio opens with"`
	Kinds map[ir.NodeType]NodeStyle `json:"kinds,omitempty" yaml:"kinds,omitempty"`
	Nodes map[string]NodeStyle      `json:"nodes,omitempty" yaml:"nodes,omitempty"`
}

type NodeStyle struct {
	Color string          `json:"color,omitempty" yaml:"color,omitempty" jsonschema:"pattern=^#[0-9a-fA-F]{6}$,description=Fill colour as #rrggbb"`
	Icon  string          `json:"icon,omitempty"  yaml:"icon,omitempty"  jsonschema:"description=A bundled icon id such as aws/rds or a path relative to togen.yml"`
	Shape catalogue.Shape `json:"shape,omitempty" yaml:"shape,omitempty" jsonschema:"description=Outline the node is drawn with"`
}

func DefaultConfig() Config {
	return Config{Version: ir.Version, Targets: []string{"hcl"}, OutDir: "infra", Style: &Style{Theme: DefaultTheme}}
}

// The second return is the deprecation note for the legacy file, empty when there is none.
func LoadConfig(cwd string) (Config, string, error) {
	switch {
	case Exists(ConfigPath(cwd)):
		config, err := loadYAMLConfig(cwd)
		return config, "", err
	case Exists(LegacyConfigPath(cwd)):
		config, err := loadLegacyConfig(cwd)
		return config, LegacyNote, err
	}
	return DefaultConfig(), "", nil
}

func loadYAMLConfig(cwd string) (Config, error) {
	shown := shownPath(cwd, ConfigPath(cwd))
	raw, err := os.ReadFile(ConfigPath(cwd))
	if err != nil {
		return Config{}, &Error{fmt.Sprintf("%s could not be read: %v", shown, err)}
	}
	var doc any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return Config{}, ir.Errors{{Path: shown, Message: fmt.Sprintf("not valid YAML: %v", err)}}
	}
	if doc == nil {
		return DefaultConfig(), nil
	}
	fields, ok := stringKeys(doc).(map[string]any)
	if !ok {
		return Config{}, &Error{fmt.Sprintf("%s must be a mapping of settings", shown)}
	}
	if errs := validateConfig(fields); len(errs) > 0 {
		return Config{}, errs
	}

	config := DefaultConfig()
	if err := yaml.Unmarshal(raw, &config); err != nil {
		return Config{}, &Error{fmt.Sprintf("%s: %s", shown, err)}
	}
	if config.Version > ir.Version {
		return Config{}, &Error{versionMessage(shown, config.Version, ir.Version)}
	}
	if config.Style == nil {
		config.Style = &Style{}
	}
	if config.Style.Theme == "" {
		config.Style.Theme = DefaultTheme
	}
	return config, nil
}

func loadLegacyConfig(cwd string) (Config, error) {
	shown := shownPath(cwd, LegacyConfigPath(cwd))
	raw, err := ReadJSONFile(LegacyConfigPath(cwd), cwd)
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
		return Config{}, &Error{versionMessage(shown, config.Version, ir.Version)}
	}
	if len(config.Targets) == 0 {
		return Config{}, &Error{fmt.Sprintf("%s has no targets", shown)}
	}
	return config, nil
}

func versionMessage(shown string, version, newest int) string {
	return fmt.Sprintf("%s is version %d but this Togen only understands up to %d", shown, version, newest)
}

func validateConfig(fields map[string]any) ir.Errors {
	instance, err := jsonValue(fields)
	if err != nil {
		return ir.Errors{{Message: err.Error()}}
	}
	errs := configSchema.check(instance, ConfigName)
	for i := range errs {
		errs[i] = configError(errs[i])
	}
	return errs
}

// An icon is an anyOf of the bundled ids and a path, and the schema's word for
// failing both says nothing a person can act on; a rate's pattern is as opaque.
func configError(err ir.ValidationError) ir.ValidationError {
	parts := strings.Split(err.Path, ".")
	switch {
	case len(parts) == 4 && parts[0] == "style" && parts[3] == "icon":
		err.Message = iconMessage
	case len(parts) == 3 && parts[0] == "usage" && slices.Contains(rateKeys, parts[2]):
		err.Message = cost.RateHelp
	}
	return err
}

var rateKeys = []string{"requests", "invocations", "messages"}

// The validator wants what a JSON decode produces, and YAML hands back ints and
// timestamps, so the parsed document goes through JSON on its way in.
func jsonValue(fields map[string]any) (any, error) {
	raw, err := json.Marshal(fields)
	if err != nil {
		return nil, err
	}
	return jsonschema.UnmarshalJSON(bytes.NewReader(raw))
}

// YAML allows any scalar as a key; JSON and the validator do not.
func stringKeys(value any) any {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			out[key] = stringKeys(item)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			out[fmt.Sprint(key)] = stringKeys(item)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = stringKeys(item)
		}
		return out
	}
	return value
}
