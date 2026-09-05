package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/mooncitizen/togen/internal/workspace"
)

func readJSONDoc(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return out
}

func TestInitWritesTheThreeProjectFiles(t *testing.T) {
	cwd := t.TempDir()
	result := Init(cwd, "gcp", "shop", false)
	if result.Code != 0 {
		t.Fatalf("code = %d, lines = %v", result.Code, result.Lines)
	}

	project := readJSONDoc(t, filepath.Join(cwd, "togen", "project.json"))
	want := map[string]any{
		"version":     float64(1),
		"name":        "shop",
		"provider":    "gcp",
		"region":      "europe-west2",
		"environment": "dev",
		"nodes":       []any{},
		"edges":       []any{},
	}
	if diff := cmp.Diff(want, project); diff != "" {
		t.Errorf("project.json (-want +got):\n%s", diff)
	}

	layout := readJSONDoc(t, filepath.Join(cwd, "togen", "layout.json"))
	wantLayout := map[string]any{
		"version": float64(2),
		"views": map[string]any{
			"overview": map[string]any{
				"nodes":    map[string]any{},
				"viewport": map[string]any{"x": float64(0), "y": float64(0), "zoom": float64(1)},
			},
		},
	}
	if diff := cmp.Diff(wantLayout, layout); diff != "" {
		t.Errorf("layout.json (-want +got):\n%s", diff)
	}

	if want := []string{"created togen/project.json, togen/layout.json and togen.yml"}; !cmp.Equal(want, result.Lines) {
		t.Errorf("lines = %v, want %v", result.Lines, want)
	}
	expectConfig(t, cwd, "infra")
}

func expectConfig(t *testing.T, cwd, outDir string) {
	t.Helper()
	text := readFileText(t, workspace.ConfigPath(cwd))
	for _, want := range []string{
		"version: 1\n",
		"targets: [hcl]\n",
		"outDir: " + outDir + "\n",
		"# style:\n",
		"#   theme: dark",
		"#   kinds:",
		"#     database:",
		"#   nodes:",
		"#     orders-db:",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("togen.yml has no %q:\n%s", want, text)
		}
	}
	config, note, err := workspace.LoadConfig(cwd)
	if err != nil {
		t.Fatalf("the written togen.yml does not load: %v", err)
	}
	if note != "" {
		t.Errorf("note = %q, want none", note)
	}
	if config.OutDir != outDir || config.Style.Theme != workspace.DefaultTheme {
		t.Errorf("config = %+v", config)
	}
}

func TestInitMigratesTheLegacyConfig(t *testing.T) {
	cwd := t.TempDir()
	if result := Init(cwd, "", "shop", false); result.Code != 0 {
		t.Fatalf("init: %v", result.Lines)
	}
	if err := os.Remove(workspace.ConfigPath(cwd)); err != nil {
		t.Fatal(err)
	}
	writeFileText(t, workspace.LegacyConfigPath(cwd), `{"version":1,"targets":["hcl"],"outDir":"build"}`)

	result := Init(cwd, "", "", true)
	if result.Code != 0 {
		t.Fatalf("code = %d, lines = %v", result.Code, result.Lines)
	}
	if want := []string{"moved togen/togen.json to togen.yml"}; !cmp.Equal(want, result.Lines) {
		t.Errorf("lines = %v, want %v", result.Lines, want)
	}
	if workspace.Exists(workspace.LegacyConfigPath(cwd)) {
		t.Error("togen/togen.json survived the migration")
	}
	expectConfig(t, cwd, "build")
}

func TestInitMigrateRefusesWhenThereIsNothingToDo(t *testing.T) {
	cwd := t.TempDir()
	result := Init(cwd, "", "", true)
	if result.Code != 1 {
		t.Fatalf("code = %d, want 1", result.Code)
	}
	if want := "there is no togen/togen.json to migrate"; result.Lines[0] != want {
		t.Errorf("line = %q, want %q", result.Lines[0], want)
	}
}

func TestInitMigrateRefusesWhenTheYAMLIsAlreadyThere(t *testing.T) {
	cwd := t.TempDir()
	if result := Init(cwd, "", "shop", false); result.Code != 0 {
		t.Fatalf("init: %v", result.Lines)
	}
	writeFileText(t, workspace.LegacyConfigPath(cwd), `{"version":1}`)

	result := Init(cwd, "", "", true)
	if result.Code != 1 {
		t.Fatalf("code = %d, want 1", result.Code)
	}
	if want := "togen.yml already exists, so there is nothing to migrate"; result.Lines[0] != want {
		t.Errorf("line = %q, want %q", result.Lines[0], want)
	}
	if !workspace.Exists(workspace.LegacyConfigPath(cwd)) {
		t.Error("togen/togen.json was removed by a refused migration")
	}
}

func TestInitDefaultsNameAndProvider(t *testing.T) {
	cwd := filepath.Join(t.TempDir(), "My Shop 2026")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	result := Init(cwd, "", "", false)
	if result.Code != 0 {
		t.Fatalf("code = %d, lines = %v", result.Code, result.Lines)
	}
	project := readJSONDoc(t, filepath.Join(cwd, "togen", "project.json"))
	if project["provider"] != "aws" {
		t.Errorf("provider = %v, want aws", project["provider"])
	}
	if project["region"] != "eu-west-2" {
		t.Errorf("region = %v, want eu-west-2", project["region"])
	}
	if project["name"] != "my-shop-2026" {
		t.Errorf("name = %v, want my-shop-2026", project["name"])
	}
}

func TestInitFallsBackToProjectWhenTheDirectoryNameGivesNothing(t *testing.T) {
	cwd := filepath.Join(t.TempDir(), "2026")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	if result := Init(cwd, "", "", false); result.Code != 0 {
		t.Fatalf("code = %d, lines = %v", result.Code, result.Lines)
	}
	project := readJSONDoc(t, filepath.Join(cwd, "togen", "project.json"))
	if project["name"] != "project" {
		t.Errorf("name = %v, want project", project["name"])
	}
}

func TestInitRefusesAnExistingTogenDirectory(t *testing.T) {
	cwd := t.TempDir()
	if first := Init(cwd, "", "", false); first.Code != 0 {
		t.Fatalf("first init: %v", first.Lines)
	}
	second := Init(cwd, "", "", false)
	if second.Code != 1 {
		t.Fatalf("code = %d, want 1", second.Code)
	}
	if want := "togen/ already exists in " + cwd; second.Lines[0] != want {
		t.Errorf("line = %q, want %q", second.Lines[0], want)
	}
}

func TestInitRejectsAnUnknownProvider(t *testing.T) {
	result := Init(t.TempDir(), "oracle", "", false)
	if result.Code != 1 {
		t.Fatalf("code = %d, want 1", result.Code)
	}
	want := "unknown provider 'oracle'. Use one of aws, gcp, azure."
	if result.Lines[0] != want {
		t.Errorf("line = %q, want %q", result.Lines[0], want)
	}
}

func TestInitRejectsANameTheSchemaWillNotAccept(t *testing.T) {
	cwd := t.TempDir()
	result := Init(cwd, "", "Bad Name", false)
	if result.Code != 1 {
		t.Fatalf("code = %d, want 1", result.Code)
	}
	if !strings.Contains(result.Lines[0], "pattern") {
		t.Errorf("line = %q, want it to mention the pattern", result.Lines[0])
	}
	if !strings.HasPrefix(result.Lines[0], "name: ") {
		t.Errorf("line = %q, want it to name the field", result.Lines[0])
	}
	if _, err := os.Stat(filepath.Join(cwd, "togen")); !os.IsNotExist(err) {
		t.Error("togen/ was written despite the failure")
	}
}

func TestInitDerivesAValidNameFromAwkwardDirectories(t *testing.T) {
	for _, dirName := range []string{"Infrastructure Platform Services 2026", "2026-shop"} {
		t.Run(dirName, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), dirName)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if result := Init(dir, "", "", false); result.Code != 0 {
				t.Fatalf("init: %v", result.Lines)
			}
			if result := Validate(dir); result.Code != 0 {
				t.Fatalf("validate: %v", result.Lines)
			}
		})
	}
}
