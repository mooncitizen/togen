package schema

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/invopop/jsonschema"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/workspace"
)

const (
	kebabPattern  = `^[a-z][a-z0-9]*(-[a-z0-9]+)*$`
	envKeyPattern = `^[A-Z][A-Z0-9_]*$`
)

func Project() map[string]any {
	nodeVariants := make([]any, 0, len(ir.NodeTypes))
	for _, t := range ir.NodeTypes {
		props, _ := ir.PropsFor(t)
		ps := propsSchema(props)
		required := []string{"id", "type", "name"}
		if _, ok := ps["required"]; ok {
			required = append(required, "properties")
		}
		nodeVariants = append(nodeVariants, map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required":             required,
			"properties": map[string]any{
				"id":         described(nonEmptyString(), "Stable identifier edges point at"),
				"type":       map[string]any{"const": string(t), "description": "Kind of thing this node is"},
				"name":       described(kebab(32), "Human name used for resource names and outputs"),
				"properties": described(ps, fmt.Sprintf("Settings for the %s", t)),
			},
		})
	}
	return map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"$id":                  "https://togen.dev/schema/project.schema.json",
		"title":                "Togen project",
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"version", "name", "provider", "region", "environment", "nodes", "edges"},
		"properties": map[string]any{
			"version":     map[string]any{"const": ir.Version},
			"name":        kebab(32),
			"provider":    enumOf(ir.Providers),
			"region":      nonEmptyString(),
			"environment": kebab(16),
			"nodes":       map[string]any{"type": "array", "items": map[string]any{"oneOf": nodeVariants}},
			"edges":       map[string]any{"type": "array", "items": edgeSchema()},
		},
	}
}

func Config() map[string]any {
	doc := reflected(&workspace.Config{})
	doc["$schema"] = "https://json-schema.org/draft/2020-12/schema"
	doc["$id"] = "https://togen.dev/schema/togen.schema.json"
	doc["title"] = "Togen configuration"
	// A missing key means the default, so nothing here is required.
	delete(doc, "required")

	fields, _ := doc["properties"].(map[string]any)
	style, _ := fields["style"].(map[string]any)
	styleFields, _ := style["properties"].(map[string]any)
	theme, _ := styleFields["theme"].(map[string]any)
	theme["enum"] = anyValues(workspace.Themes)
	theme["default"] = workspace.DefaultTheme
	keyedBy(styleFields, "kinds", oneOfPattern(ir.NodeTypes), "Style for every node of a type")
	keyedBy(styleFields, "nodes", kebabPattern, "Style for one node by name")
	return doc
}

// patternProperties rather than propertyNames: an unknown key is then reported against
// the block that holds it and names the key, where a propertyNames failure carries neither.
func keyedBy(fields map[string]any, name, keys, text string) {
	sub, ok := fields[name].(map[string]any)
	if !ok {
		return
	}
	style, ok := sub["additionalProperties"].(map[string]any)
	if !ok {
		return
	}
	style["additionalProperties"] = false
	if shape, ok := style["properties"].(map[string]any)["shape"].(map[string]any); ok {
		shape["enum"] = anyValues(workspace.Shapes)
	}
	fields[name] = map[string]any{
		"type":                 "object",
		"description":          text,
		"patternProperties":    map[string]any{keys: style},
		"additionalProperties": false,
	}
}

func oneOfPattern[T ~string](values []T) string {
	names := make([]string, len(values))
	for i, v := range values {
		names[i] = string(v)
	}
	return "^(" + strings.Join(names, "|") + ")$"
}

func Relations() map[string]any {
	rules := ir.RelationRules()
	rel := make(map[string]any, len(ir.Relations))
	for _, r := range ir.Relations {
		rule := rules[r]
		rel[string(r)] = map[string]any{"from": anyValues(rule.From), "to": anyValues(rule.To)}
	}
	return map[string]any{"relations": rel}
}

func Engines() map[string]any {
	out := map[string]any{}
	for _, p := range ir.Providers {
		table, ok := ir.Engines[p]
		if !ok {
			continue
		}
		engines := make(map[string]any, len(table))
		for _, e := range ir.EngineTypes {
			info, ok := table[e]
			if !ok {
				continue
			}
			engines[string(e)] = map[string]any{"port": info.Port, "versions": info.Versions}
		}
		out[string(p)] = engines
	}
	return out
}

func Write(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	files := []struct {
		name string
		doc  map[string]any
	}{
		{"project.schema.json", Project()},
		{"togen.schema.json", Config()},
		{"relations.json", Relations()},
		{"engines.json", Engines()},
	}
	for _, f := range files {
		b, err := json.MarshalIndent(f.doc, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, f.name), append(b, '\n'), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func edgeSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"id", "from", "to", "relation"},
		"properties": map[string]any{
			"id":       described(nonEmptyString(), "Stable identifier for this edge"),
			"from":     described(nonEmptyString(), "Id of the node the edge starts at"),
			"to":       described(nonEmptyString(), "Id of the node the edge ends at"),
			"relation": described(enumOf(ir.Relations), "What the two nodes do to each other"),
			"properties": map[string]any{
				"type":                 "object",
				"description":          "Settings for this edge",
				"additionalProperties": false,
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"pattern":     `^/`,
						"default":     "/",
						"description": "Path the gateway routes to the target",
					},
					"methods": map[string]any{
						"type":        "array",
						"minItems":    1,
						"items":       enumOf(ir.Methods),
						"default":     []any{string(ir.MethodAny)},
						"description": "HTTP methods the route accepts",
					},
				},
				"default": map[string]any{},
			},
		},
	}
}

// invopop cannot see the values behind a named string type, so the enums are injected by field type.
var enumsByType = map[reflect.Type][]any{
	reflect.TypeFor[ir.Size]():    anyValues(ir.Sizes),
	reflect.TypeFor[ir.Runtime](): anyValues(ir.Runtimes),
	reflect.TypeFor[ir.Engine]():  anyValues(ir.EngineTypes),
}

func reflected(value any) map[string]any {
	reflector := &jsonschema.Reflector{DoNotReference: true, ExpandedStruct: true}
	doc := toMap(reflector.Reflect(value))
	delete(doc, "$schema")
	// The reflector derives an $id from the module path. A nested $id resets the
	// base URI inside the document and has no use here, so it goes.
	delete(doc, "$id")
	doc["additionalProperties"] = false
	return doc
}

func propsSchema(props any) map[string]any {
	doc := reflected(props)
	fields, _ := doc["properties"].(map[string]any)
	t := reflect.TypeOf(props).Elem()
	for i := range t.NumField() {
		f := t.Field(i)
		name := jsonName(f)
		if name == "env" {
			env := envSchema()
			if sub, ok := fields[name].(map[string]any); ok {
				env["description"] = sub["description"]
			}
			fields[name] = env
			continue
		}
		sub, ok := fields[name].(map[string]any)
		if !ok {
			continue
		}
		if values, ok := enumsByType[f.Type]; ok {
			sub["enum"] = values
		}
	}
	return doc
}

func envSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": map[string]any{"type": "string"},
		"propertyNames":        map[string]any{"pattern": envKeyPattern},
		"default":              map[string]any{},
	}
}

func kebab(max int) map[string]any {
	return map[string]any{"type": "string", "minLength": 1, "maxLength": max, "pattern": kebabPattern}
}

func nonEmptyString() map[string]any {
	return map[string]any{"type": "string", "minLength": 1}
}

func described(schema map[string]any, text string) map[string]any {
	schema["description"] = text
	return schema
}

func enumOf[T ~string](values []T) map[string]any {
	return map[string]any{"type": "string", "enum": anyValues(values)}
}

func anyValues[T ~string](values []T) []any {
	out := make([]any, len(values))
	for i, v := range values {
		out[i] = string(v)
	}
	return out
}

func jsonName(f reflect.StructField) string {
	name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
	if name == "" {
		return f.Name
	}
	return name
}

func toMap(v any) map[string]any {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var m map[string]any
	if err := d.Decode(&m); err != nil {
		panic(err)
	}
	return m
}
