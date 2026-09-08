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

	"github.com/mooncitizen/togen/internal/catalogue"
	"github.com/mooncitizen/togen/internal/cost"
	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/workspace"
)

const (
	kebabPattern    = ir.KebabPattern
	envKeyPattern   = `^[A-Z][A-Z0-9_]*$`
	iconPathPattern = `^\.\.?/.+$`
)

func Project() map[string]any {
	entries := catalogue.Embedded().All()
	nodeVariants := make([]any, 0, len(entries))
	for _, e := range entries {
		t := ir.NodeType(e.ID)
		ps := e.Properties
		if props, ok := ir.PropsFor(t); ok {
			ps = propsSchema(props)
		}
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
				"type":       map[string]any{"const": e.ID, "description": "Kind of thing this node is"},
				"name":       described(kebab(32), "Human name used for resource names and outputs"),
				"properties": described(ps, fmt.Sprintf("Settings for the %s", e.ID)),
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
	keyedBy(fields, "usage", kebabPattern, "Usage of one node by name, or of the network")
	return doc
}

func Views() map[string]any {
	return map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"$id":                  "https://togen.dev/schema/views.schema.json",
		"title":                "Togen views",
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"version", "views"},
		"properties": map[string]any{
			"version": map[string]any{"type": "integer", "minimum": 1, "description": "File format version"},
			"views":   map[string]any{"type": "array", "items": viewSchema(), "description": "The named diagrams of the project"},
		},
	}
}

// if/then/else rather than oneOf, so a wrong value is reported as the one
// thing it is not, instead of as a whole alternative failing.
func viewSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"id", "name", "nodes"},
		"properties": map[string]any{
			"id":   described(kebab(32), "Stable identifier the layout is keyed by"),
			"name": described(nonEmptyString(), "Title shown in the studio and on an export"),
			"nodes": map[string]any{
				"type":        []string{"string", "array"},
				"if":          map[string]any{"type": "string"},
				"then":        map[string]any{"const": "*"},
				"else":        map[string]any{"items": nonEmptyString()},
				"description": `Ids of the nodes the view shows, or "*" for every node`,
			},
		},
	}
}

func Simulation() map[string]any {
	return map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"$id":                  "https://togen.dev/schema/simulation.schema.json",
		"title":                "Togen simulation",
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"version", "sources"},
		"properties": map[string]any{
			"version": map[string]any{"type": "integer", "minimum": 1, "description": "File format version"},
			"sources": map[string]any{"type": "array", "items": sourceSchema(), "description": "Where load enters the system"},
			"bursts":  map[string]any{"type": "array", "items": burstSchema(), "description": "Multipliers laid over a source for a while"},
			"edges": map[string]any{
				"type":                 "object",
				"description":          "Calls an edge makes per request at its source node, when it is not 1",
				"additionalProperties": map[string]any{"type": "number", "minimum": 0},
			},
		},
	}
}

func sourceSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"id", "name", "target", "rate"},
		"properties": map[string]any{
			"id":     described(kebab(32), "Stable identifier a burst points at"),
			"name":   described(nonEmptyString(), "Who this load is, shown in the studio"),
			"target": described(nonEmptyString(), "Id of the gateway, service or function the load enters at"),
			"rate": map[string]any{
				"type":        "string",
				"pattern":     cost.RatePattern,
				"description": "Requests this source makes, such as 800/min",
			},
			"bytesPerRequest": map[string]any{"type": "number", "minimum": 0, "description": "Average response size, the only source of derived egress"},
		},
	}
}

func burstSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"id", "name", "source", "multiplier", "minutes", "timesPerMonth"},
		"properties": map[string]any{
			"id":            described(kebab(32), "Stable identifier the studio selects by"),
			"name":          described(nonEmptyString(), "Name of the scenario"),
			"source":        described(nonEmptyString(), "Id of the source this multiplies"),
			"multiplier":    map[string]any{"type": "number", "minimum": 1, "description": "How many times the baseline the burst reaches"},
			"minutes":       map[string]any{"type": "number", "exclusiveMinimum": 0, "description": "How long one occurrence lasts"},
			"timesPerMonth": map[string]any{"type": "number", "exclusiveMinimum": 0, "description": "How often it happens in a month"},
		},
	}
}

// Version 1 keeps nodes and viewport at the top; version 2 has them per view.
func Layout() map[string]any {
	position := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"x", "y"},
		"properties": map[string]any{
			"x": number("Canvas x of the node's top left corner"),
			"y": number("Canvas y of the node's top left corner"),
		},
	}
	nodes := map[string]any{"type": "object", "additionalProperties": position, "description": "Where each node sits, by id"}
	viewport := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"x", "y", "zoom"},
		"description":          "Pan and zoom of the canvas",
		"properties": map[string]any{
			"x":    number("Canvas x at the left edge of the window"),
			"y":    number("Canvas y at the top edge of the window"),
			"zoom": map[string]any{"type": "number", "exclusiveMinimum": 0, "description": "Scale, 1 being actual size"},
		},
	}
	view := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           map[string]any{"nodes": nodes, "viewport": viewport},
	}
	return map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"$id":                  "https://togen.dev/schema/layout.schema.json",
		"title":                "Togen layout",
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"version"},
		"properties": map[string]any{
			"version":  map[string]any{"type": "integer", "minimum": 1, "description": "File format version"},
			"nodes":    described(copied(nodes), "Version 1 only: positions of the one drawing"),
			"viewport": described(copied(viewport), "Version 1 only: the window onto the one drawing"),
			"views": map[string]any{
				"type":                 "object",
				"additionalProperties": view,
				"description":          "Version 2: a layout per view, by view id",
			},
		},
	}
}

func number(text string) map[string]any {
	return map[string]any{"type": "number", "description": text}
}

func copied(schema map[string]any) map[string]any {
	out := make(map[string]any, len(schema))
	for k, v := range schema {
		out[k] = v
	}
	return out
}

// patternProperties rather than propertyNames: an unknown key is then reported against
// the block that holds it and names the key, where a propertyNames failure carries neither.
func keyedBy(fields map[string]any, name, keys, text string) {
	sub, ok := fields[name].(map[string]any)
	if !ok {
		return
	}
	entry, ok := sub["additionalProperties"].(map[string]any)
	if !ok {
		return
	}
	entry["additionalProperties"] = false
	props, _ := entry["properties"].(map[string]any)
	if shape, ok := props["shape"].(map[string]any); ok {
		shape["enum"] = anyValues(catalogue.Shapes)
	}
	if icon, ok := props["icon"].(map[string]any); ok {
		// The enum lets an editor complete the bundled ids; the pattern admits a file of the user's own.
		delete(icon, "type")
		icon["anyOf"] = []any{
			enumOf(catalogue.Embedded().Icons()),
			map[string]any{"type": "string", "pattern": iconPathPattern},
		}
	}
	fields[name] = map[string]any{
		"type":                 "object",
		"description":          text,
		"patternProperties":    map[string]any{keys: entry},
		"additionalProperties": false,
	}
}

// What each provider offers, for the studio's palette and for anyone reading the schemas.
// The node properties come from the props structs where there are any, so a reader sees one
// document rather than two halves.
func Catalogue() map[string]any {
	c := catalogue.Embedded()
	providers := map[string]any{}
	for _, p := range c.Providers() {
		scheme, _ := c.Scheme(p)
		entries := make([]any, 0, len(c.For(p)))
		for _, e := range c.For(p) {
			props := e.Properties
			if structProps, ok := ir.PropsFor(ir.NodeType(e.ID)); ok {
				props = propsSchema(structProps)
			}
			entry := map[string]any{
				"id":          e.ID,
				"label":       e.Label,
				"group":       e.Group,
				"description": e.Description,
				"resource":    e.Resource,
				"tier":        string(e.Tier),
				"style":       toMap(e.Style),
				"properties":  props,
			}
			if len(e.Aliases) > 0 {
				entry["aliases"] = anyValues(e.Aliases)
			}
			entries = append(entries, entry)
		}
		provider := map[string]any{"accent": scheme.Accent, "network": toMap(scheme.Network), "entries": entries}
		if scheme.ResourceGroup != nil {
			provider["resourceGroup"] = toMap(*scheme.ResourceGroup)
		}
		providers[p] = provider
	}
	return map[string]any{"providers": providers}
}

func Styles() map[string]any {
	return toMap(catalogue.Embedded().Schemes())
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

func Regions() map[string]any {
	out := make(map[string]any, len(ir.Providers))
	for _, p := range ir.Providers {
		out[string(p)] = ir.Regions[p]
	}
	return out
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
		{"views.schema.json", Views()},
		{"simulation.schema.json", Simulation()},
		{"layout.schema.json", Layout()},
		{"relations.json", Relations()},
		{"engines.json", Engines()},
		{"styles.json", Styles()},
		{"catalogue.json", Catalogue()},
		{"regions.json", Regions()},
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
	reflector := &jsonschema.Reflector{DoNotReference: true, ExpandedStruct: true, Mapper: mapped}
	doc := toMap(reflector.Reflect(value))
	delete(doc, "$schema")
	// The reflector derives an $id from the module path. A nested $id resets the
	// base URI inside the document and has no use here, so it goes.
	delete(doc, "$id")
	doc["additionalProperties"] = false
	return doc
}

// The reflector sees a rate as a string; the grammar is the cost package's.
func mapped(t reflect.Type) *jsonschema.Schema {
	if t == reflect.TypeFor[cost.Rate]() {
		return &jsonschema.Schema{Type: "string", Pattern: cost.RatePattern}
	}
	return nil
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
