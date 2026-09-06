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

func writeProject(t *testing.T, cwd string, value any) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	writeProjectRaw(t, cwd, string(raw))
}

func writeProjectRaw(t *testing.T, cwd, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(cwd, "togen"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cwd, "togen", "project.json"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestValidatePassesAValidProject(t *testing.T) {
	cwd := t.TempDir()
	writeProject(t, cwd, map[string]any{
		"version":     1,
		"name":        "shop",
		"provider":    "aws",
		"region":      "eu-west-2",
		"environment": "dev",
		"nodes": []any{
			map[string]any{"id": "n1", "type": "gateway", "name": "api"},
			map[string]any{
				"id":         "n2",
				"type":       "service",
				"name":       "web",
				"properties": map[string]any{"image": "nginx:1.27", "port": 80, "public": true},
			},
		},
		"edges": []any{},
	})
	result := Validate(cwd)
	if result.Code != 0 {
		t.Fatalf("code = %d, lines = %v", result.Code, result.Lines)
	}
	want := []string{"togen/project.json is valid (2 nodes, 0 edges)"}
	if diff := cmp.Diff(want, result.Lines); diff != "" {
		t.Errorf("lines (-want +got):\n%s", diff)
	}
}

func TestValidateListsEveryError(t *testing.T) {
	cwd := t.TempDir()
	writeProject(t, cwd, map[string]any{
		"version":     1,
		"name":        "shop",
		"provider":    "aws",
		"region":      "eu-west-2",
		"environment": "dev",
		"nodes":       []any{map[string]any{"id": "n1", "type": "gateway", "name": "api"}},
		"edges": []any{
			map[string]any{"id": "e1", "from": "n1", "to": "nope", "relation": "routes"},
			map[string]any{"id": "e2", "from": "n1", "to": "n1", "relation": "calls"},
		},
	})
	result := Validate(cwd)
	if result.Code != 1 {
		t.Fatalf("code = %d, want 1", result.Code)
	}
	want := []string{
		"edges.0 (edge e1): edge refers to missing node 'nope'",
		"edges.1 (edge e2): an edge cannot connect a node to itself",
	}
	if diff := cmp.Diff(want, result.Lines); diff != "" {
		t.Errorf("lines (-want +got):\n%s", diff)
	}
}

func TestValidateReportsAMissingProjectFile(t *testing.T) {
	cwd := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cwd, "togen"), 0o755); err != nil {
		t.Fatal(err)
	}
	result := Validate(cwd)
	if result.Code != 1 {
		t.Fatalf("code = %d, want 1", result.Code)
	}
	want := "togen/project.json not found. Run 'togen init' first."
	if result.Lines[0] != want {
		t.Errorf("line = %q, want %q", result.Lines[0], want)
	}
}

func TestValidateReportsMalformedJSON(t *testing.T) {
	cwd := t.TempDir()
	writeProjectRaw(t, cwd, "{ not json")
	result := Validate(cwd)
	if result.Code != 1 {
		t.Fatalf("code = %d, want 1", result.Code)
	}
	want := "togen/project.json is not valid JSON"
	if result.Lines[0] != want {
		t.Errorf("line = %q, want %q", result.Lines[0], want)
	}
}

func TestValidateReportsAFutureVersion(t *testing.T) {
	cwd := t.TempDir()
	writeProject(t, cwd, map[string]any{"version": 9})
	result := Validate(cwd)
	if result.Code != 1 {
		t.Fatalf("code = %d, want 1", result.Code)
	}
	if !strings.Contains(result.Lines[0], "version 9") {
		t.Errorf("line = %q, want it to mention version 9", result.Lines[0])
	}
}

func TestValidateReportsResolverErrors(t *testing.T) {
	cwd := t.TempDir()
	writeProject(t, cwd, map[string]any{
		"version":     1,
		"name":        "shop",
		"provider":    "aws",
		"region":      "eu-west-2",
		"environment": "dev",
		"nodes": []any{
			map[string]any{"id": "n1", "type": "gateway", "name": "api"},
			map[string]any{"id": "n2", "type": "function", "name": "orders"},
			map[string]any{"id": "n3", "type": "function", "name": "users"},
		},
		"edges": []any{
			map[string]any{"id": "e1", "from": "n1", "to": "n2", "relation": "routes"},
			map[string]any{"id": "e2", "from": "n1", "to": "n3", "relation": "routes"},
		},
	})
	result := Validate(cwd)
	if result.Code != 1 {
		t.Fatalf("code = %d, lines = %v", result.Code, result.Lines)
	}
	want := []string{"project (edge e2): route 'ANY /' on gateway 'api' is already used by edge 'e1'"}
	if diff := cmp.Diff(want, result.Lines); diff != "" {
		t.Errorf("lines (-want +got):\n%s", diff)
	}
}

func TestValidateReportsResolverErrorsAgainstTheNode(t *testing.T) {
	cwd := t.TempDir()
	writeProject(t, cwd, map[string]any{
		"version":     1,
		"name":        "shop",
		"provider":    "aws",
		"region":      "eu-west-2",
		"environment": "dev",
		"nodes": []any{
			map[string]any{"id": "c1", "type": "bucket", "name": strings.Repeat("u", 30)},
		},
		"edges": []any{},
	})
	result := Validate(cwd)
	if result.Code != 1 {
		t.Fatalf("code = %d, lines = %v", result.Code, result.Lines)
	}
	if len(result.Lines) != 1 || !strings.Contains(result.Lines[0], "the limit is 37") || !strings.Contains(result.Lines[0], "node c1") {
		t.Fatalf("lines = %v", result.Lines)
	}
}

func TestValidateReportsANodeTheGCPResolverDoesNotSupportYet(t *testing.T) {
	cwd := t.TempDir()
	writeProject(t, cwd, map[string]any{
		"version":     1,
		"name":        "shop",
		"provider":    "gcp",
		"region":      "europe-west2",
		"environment": "dev",
		"nodes":       []any{map[string]any{"id": "c1", "type": "cache", "name": "sessions"}},
		"edges":       []any{},
	})
	result := Validate(cwd)
	if result.Code != 1 {
		t.Fatalf("code = %d, lines = %v", result.Code, result.Lines)
	}
	want := []string{"project (node c1): node type 'cache' is not supported by the gcp resolver yet"}
	if diff := cmp.Diff(want, result.Lines); diff != "" {
		t.Errorf("lines (-want +got):\n%s", diff)
	}
}

func TestValidateReportsAzureNodesTheResolverDoesNotSupportYet(t *testing.T) {
	cwd := t.TempDir()
	writeProject(t, cwd, map[string]any{
		"version":     1,
		"name":        "shop",
		"provider":    "azure",
		"region":      "uksouth",
		"environment": "dev",
		"nodes":       []any{map[string]any{"id": "b1", "type": "bucket", "name": "uploads"}},
		"edges":       []any{},
	})
	result := Validate(cwd)
	if result.Code != 1 {
		t.Fatalf("code = %d, lines = %v", result.Code, result.Lines)
	}
	if len(result.Lines) != 1 || !strings.Contains(result.Lines[0], "node b1") || !strings.Contains(result.Lines[0], "not supported by the azure resolver yet") {
		t.Fatalf("lines = %v", result.Lines)
	}
}

func legacyConfig(t *testing.T, cwd string) {
	t.Helper()
	if err := os.Remove(workspace.ConfigPath(cwd)); err != nil {
		t.Fatal(err)
	}
	writeFileText(t, workspace.LegacyConfigPath(cwd), `{"version":1,"targets":["hcl"],"outDir":"infra"}`)
}

func TestValidateNotesTheLegacyConfig(t *testing.T) {
	cwd := t.TempDir()
	if result := Init(cwd, "", "shop", false); result.Code != 0 {
		t.Fatalf("init: %v", result.Lines)
	}
	if result := Validate(cwd); result.Note != "" {
		t.Errorf("note = %q, want none", result.Note)
	}

	legacyConfig(t, cwd)
	result := Validate(cwd)
	if result.Code != 0 {
		t.Fatalf("code = %d, lines = %v", result.Code, result.Lines)
	}
	if result.Note != workspace.LegacyNote {
		t.Errorf("note = %q, want %q", result.Note, workspace.LegacyNote)
	}
}

func TestValidateReportsAStyleForANodeThatDoesNotExist(t *testing.T) {
	cwd := t.TempDir()
	writeProject(t, cwd, map[string]any{
		"version":     1,
		"name":        "shop",
		"provider":    "aws",
		"region":      "eu-west-2",
		"environment": "dev",
		"nodes":       []any{map[string]any{"id": "n1", "type": "gateway", "name": "api"}},
		"edges":       []any{},
	})
	writeFileText(t, workspace.ConfigPath(cwd), "style:\n  nodes:\n    api:\n      color: \"#DD344C\"\n    orders-db:\n      shape: cylinder\n")
	result := Validate(cwd)
	if result.Code != 1 {
		t.Fatalf("code = %d, want 1", result.Code)
	}
	want := []string{"style.nodes.orders-db: there is no node named 'orders-db'"}
	if diff := cmp.Diff(want, result.Lines); diff != "" {
		t.Errorf("lines (-want +got):\n%s", diff)
	}

	writeFileText(t, workspace.ConfigPath(cwd), "style:\n  nodes:\n    api:\n      color: \"#DD344C\"\n")
	if result := Validate(cwd); result.Code != 0 {
		t.Errorf("code = %d, lines = %v", result.Code, result.Lines)
	}
}

func TestValidateIgnoresABrokenViewsFile(t *testing.T) {
	cwd := t.TempDir()
	if result := Init(cwd, "", "shop", false); result.Code != 0 {
		t.Fatalf("init: %v", result.Lines)
	}
	writeFileText(t, workspace.ViewsPath(cwd), "{ not json")
	result := Validate(cwd)
	if result.Code != 0 {
		t.Fatalf("code = %d, lines = %v", result.Code, result.Lines)
	}
}

func TestValidateReportsAnInvalidConfig(t *testing.T) {
	cwd := t.TempDir()
	if result := Init(cwd, "", "shop", false); result.Code != 0 {
		t.Fatalf("init: %v", result.Lines)
	}
	writeFileText(t, workspace.ConfigPath(cwd), "style:\n  theme: neon\n")
	result := Validate(cwd)
	if result.Code != 1 {
		t.Fatalf("code = %d, want 1", result.Code)
	}
	want := []string{"style.theme: value must be one of 'dark', 'light', 'system'"}
	if diff := cmp.Diff(want, result.Lines); diff != "" {
		t.Errorf("lines (-want +got):\n%s", diff)
	}
}
