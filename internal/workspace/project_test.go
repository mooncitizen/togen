package workspace

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func projectRaw(t *testing.T, value map[string]any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func exampleProject() map[string]any {
	return map[string]any{
		"version":     1,
		"name":        "shop",
		"provider":    "aws",
		"region":      "eu-west-2",
		"environment": "dev",
		"nodes": []any{
			map[string]any{"id": "n1", "type": "gateway", "name": "api"},
			map[string]any{"id": "n2", "type": "function", "name": "handler"},
		},
		"edges": []any{
			map[string]any{"id": "e1", "from": "n1", "to": "n2", "relation": "routes"},
		},
	}
}

func TestValidateRawAcceptsAValidProject(t *testing.T) {
	project, errs, err := ValidateRaw(projectRaw(t, exampleProject()))
	if err != nil {
		t.Fatal(err)
	}
	if len(errs) > 0 {
		t.Fatalf("errors = %v", errs)
	}
	if project.Name != "shop" {
		t.Errorf("name = %q", project.Name)
	}
}

func TestValidateRawReportsSemanticErrors(t *testing.T) {
	document := exampleProject()
	document["edges"] = []any{map[string]any{"id": "e1", "from": "n1", "to": "zz", "relation": "routes"}}
	_, errs, err := ValidateRaw(projectRaw(t, document))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"edges.0 (edge e1): edge refers to missing node 'zz'"}
	got := make([]string, len(errs))
	for i, e := range errs {
		got[i] = e.String()
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
}

func TestValidateRawRunsTheResolver(t *testing.T) {
	document := exampleProject()
	document["nodes"] = append(document["nodes"].([]any),
		map[string]any{"id": "n3", "type": "function", "name": "second"})
	document["edges"] = append(document["edges"].([]any),
		map[string]any{"id": "e2", "from": "n1", "to": "n3", "relation": "routes"})

	_, errs, err := ValidateRaw(projectRaw(t, document))
	if err != nil {
		t.Fatal(err)
	}
	if len(errs) != 1 || !strings.Contains(errs[0].Message, "already used by edge 'e1'") {
		t.Fatalf("errors = %v", errs)
	}
}

func TestValidateRawReportsAFutureVersion(t *testing.T) {
	_, _, err := ValidateRaw([]byte(`{"version":9}`))
	if err == nil {
		t.Fatal("want an error")
	}
	if !strings.Contains(err.Error(), "version 9") {
		t.Errorf("error = %q, want it to mention version 9", err)
	}
}
