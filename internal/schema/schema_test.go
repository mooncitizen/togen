package schema

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommittedSchemaIsCurrent(t *testing.T) {
	dir := t.TempDir()
	if err := Write(dir); err != nil {
		t.Fatal(err)
	}
	committed := map[string][]string{
		"project.schema.json": {
			filepath.Join("..", "..", "schema", "project.schema.json"),
			filepath.Join("..", "ir", "project.schema.json"),
		},
		"togen.schema.json": {
			filepath.Join("..", "..", "schema", "togen.schema.json"),
			filepath.Join("..", "workspace", "togen.schema.json"),
		},
		"relations.json": {filepath.Join("..", "..", "schema", "relations.json")},
		"engines.json":   {filepath.Join("..", "..", "schema", "engines.json")},
	}
	for name, paths := range committed {
		want, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range paths {
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("%s: %v (run just generate)", path, err)
			}
			if !bytes.Equal(want, got) {
				t.Fatalf("%s is stale, run just generate", path)
			}
		}
	}
}

func TestProjectSchemaShape(t *testing.T) {
	doc := Project()
	props, ok := doc["properties"].(map[string]any)
	if !ok {
		t.Fatal("no properties")
	}
	if _, ok := props["nodes"]; !ok {
		t.Fatal("no nodes")
	}
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{
		`"const":"database"`,
		`"default":8080`,
		`"^[a-z][a-z0-9]*(-[a-z0-9]+)*$"`,
		`"additionalProperties":false`,
		`"enum":["small","medium","large"]`,
		`"enum":["postgres","mysql"]`,
		`"minimum":20`,
		`"^[A-Z][A-Z0-9_]*$"`,
	} {
		if !strings.Contains(string(b), needle) {
			t.Errorf("schema lacks %s", needle)
		}
	}
}

func TestConfigSchemaShape(t *testing.T) {
	b, err := json.Marshal(Config())
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{
		`"enum":["dark","light","system"]`,
		`"enum":["card","cylinder","hexagon","circle"]`,
		`"pattern":"^#[0-9a-fA-F]{6}$"`,
		`"^(service|function|database|gateway|queue|bucket|cache)$"`,
		`"^[a-z][a-z0-9]*(-[a-z0-9]+)*$"`,
		`"additionalProperties":false`,
	} {
		if !strings.Contains(string(b), needle) {
			t.Errorf("schema lacks %s", needle)
		}
	}
	if _, ok := Config()["required"]; ok {
		t.Error("nothing in the configuration is required")
	}
}

func nodeVariants(t *testing.T) []any {
	t.Helper()
	doc := Project()
	props, ok := doc["properties"].(map[string]any)
	if !ok {
		t.Fatal("no properties")
	}
	nodes, ok := props["nodes"].(map[string]any)
	if !ok {
		t.Fatal("no nodes")
	}
	items, ok := nodes["items"].(map[string]any)
	if !ok {
		t.Fatal("no node items")
	}
	variants, ok := items["oneOf"].([]any)
	if !ok {
		t.Fatal("no node variants")
	}
	return variants
}

func expectDescriptions(t *testing.T, where string, fields map[string]any) {
	t.Helper()
	for name, field := range fields {
		sub, ok := field.(map[string]any)
		if !ok {
			t.Errorf("%s.%s is not an object", where, name)
			continue
		}
		if text, ok := sub["description"].(string); !ok || text == "" {
			t.Errorf("%s.%s has no description", where, name)
		}
	}
}

func TestEveryNodeVariantPropertyIsDescribed(t *testing.T) {
	for _, variant := range nodeVariants(t) {
		fields, ok := variant.(map[string]any)["properties"].(map[string]any)
		if !ok {
			t.Fatal("a node variant has no properties")
		}
		name, _ := fields["type"].(map[string]any)["const"].(string)
		expectDescriptions(t, name, fields)
		props, ok := fields["properties"].(map[string]any)["properties"].(map[string]any)
		if !ok {
			continue
		}
		expectDescriptions(t, name+".properties", props)
	}
}

func TestEnginesShape(t *testing.T) {
	doc := Engines()
	if _, ok := doc["gcp"]; ok {
		t.Error("gcp has no engine table yet")
	}
	b, err := json.Marshal(doc["aws"])
	if err != nil {
		t.Fatal(err)
	}
	want := `{"mysql":{"port":3306,"versions":["8.4","8.0"]},"postgres":{"port":5432,"versions":["17","16","15"]}}`
	if got := string(b); got != want {
		t.Errorf("aws engines = %s, want %s", got, want)
	}
}

func TestRelationsShape(t *testing.T) {
	doc := Relations()
	rel, ok := doc["relations"].(map[string]any)
	if !ok {
		t.Fatal("no relations")
	}
	routes, ok := rel["routes"].(map[string]any)
	if !ok {
		t.Fatal("no routes")
	}
	b, err := json.Marshal(routes)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(b), `{"from":["gateway"],"to":["service","function"]}`; got != want {
		t.Fatalf("routes = %s, want %s", got, want)
	}
}
