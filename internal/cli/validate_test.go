package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
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
		"nodes":       []any{map[string]any{"id": "n1", "type": "gateway", "name": "api"}},
		"edges":       []any{},
	})
	result := Validate(cwd)
	if result.Code != 0 {
		t.Fatalf("code = %d, lines = %v", result.Code, result.Lines)
	}
	want := []string{"togen/project.json is valid (1 node, 0 edges)"}
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

func TestValidateReportsUnsupportedNodesFromTheResolver(t *testing.T) {
	cwd := t.TempDir()
	writeProject(t, cwd, map[string]any{
		"version":     1,
		"name":        "shop",
		"provider":    "aws",
		"region":      "eu-west-2",
		"environment": "dev",
		"nodes":       []any{map[string]any{"id": "q1", "type": "queue", "name": "jobs"}},
		"edges":       []any{},
	})
	result := Validate(cwd)
	if result.Code != 1 {
		t.Fatalf("code = %d, lines = %v", result.Code, result.Lines)
	}
	if len(result.Lines) != 1 || !strings.Contains(result.Lines[0], "queue") || !strings.Contains(result.Lines[0], "node q1") {
		t.Fatalf("lines = %v", result.Lines)
	}
}

func TestValidateSkipsResolverChecksWhenThereIsNoResolver(t *testing.T) {
	cwd := t.TempDir()
	writeProject(t, cwd, map[string]any{
		"version":     1,
		"name":        "shop",
		"provider":    "gcp",
		"region":      "europe-west2",
		"environment": "dev",
		"nodes":       []any{map[string]any{"id": "q1", "type": "queue", "name": "jobs"}},
		"edges":       []any{},
	})
	result := Validate(cwd)
	if result.Code != 0 {
		t.Fatalf("code = %d, lines = %v", result.Code, result.Lines)
	}
}
