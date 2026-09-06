package ir

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func baseDoc() map[string]any {
	return map[string]any{
		"version":     1,
		"name":        "shop",
		"provider":    "aws",
		"region":      "eu-west-2",
		"environment": "dev",
		"nodes": []any{
			map[string]any{"id": "n1", "type": "gateway", "name": "api"},
			map[string]any{"id": "n2", "type": "function", "name": "handler"},
			map[string]any{"id": "n3", "type": "database", "name": "main-db"},
		},
		"edges": []any{
			map[string]any{"id": "e1", "from": "n1", "to": "n2", "relation": "routes"},
			map[string]any{"id": "e2", "from": "n2", "to": "n3", "relation": "reads"},
		},
	}
}

func withDoc(t *testing.T, key string, value any) []byte {
	t.Helper()
	doc := baseDoc()
	doc[key] = value
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func errorsOf(t *testing.T, raw []byte) Errors {
	t.Helper()
	_, errs := ValidateProject(raw)
	return errs
}

func messages(errs Errors) []string {
	out := make([]string, len(errs))
	for i, e := range errs {
		out[i] = e.Message
	}
	return out
}

func TestValidateProjectAcceptsAValidProject(t *testing.T) {
	raw, err := json.Marshal(baseDoc())
	if err != nil {
		t.Fatal(err)
	}
	p, errs := ValidateProject(raw)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", messages(errs))
	}
	if p.Edges[0].Properties.Path != "/" {
		t.Fatalf("edge path = %q, want /", p.Edges[0].Properties.Path)
	}
	if d := cmp.Diff([]Method{MethodAny}, p.Edges[0].Properties.Methods); d != "" {
		t.Fatal(d)
	}
}

func TestValidateProjectReportsSchemaErrorsWithAPath(t *testing.T) {
	errs := errorsOf(t, withDoc(t, "name", "Bad Name"))
	if len(errs) != 1 {
		t.Fatalf("errors = %v, want one", messages(errs))
	}
	if errs[0].Path != "name" {
		t.Fatalf("path = %q, want name", errs[0].Path)
	}
	if !strings.Contains(errs[0].Message, "pattern") {
		t.Fatalf("message = %q, want it to mention the pattern", errs[0].Message)
	}
}

func TestValidateProjectReportsOneSchemaErrorPerMistake(t *testing.T) {
	cases := []struct {
		name    string
		nodes   []any
		path    string
		nodeID  string
		message string
	}{
		{
			name:    "an unknown node type",
			nodes:   []any{map[string]any{"id": "x", "type": "mainframe", "name": "big"}},
			path:    "nodes.0.type",
			nodeID:  "x",
			message: "unknown node type 'mainframe', use one of service, function, database, gateway, queue, bucket, cache",
		},
		{
			name:    "an unknown property",
			nodes:   []any{map[string]any{"id": "x", "type": "bucket", "name": "b", "properties": map[string]any{"colour": "red"}}},
			path:    "nodes.0.properties",
			nodeID:  "x",
			message: "colour",
		},
		{
			name:    "an env key that is not upper snake case",
			nodes:   []any{map[string]any{"id": "x", "type": "function", "name": "f", "properties": map[string]any{"env": map[string]any{"bad-key": "1"}}}},
			path:    "nodes.0.properties.env.bad-key",
			nodeID:  "x",
			message: "bad-key",
		},
		{
			name:    "a service with no image",
			nodes:   []any{map[string]any{"id": "x", "type": "service", "name": "s"}},
			path:    "nodes.0",
			nodeID:  "x",
			message: "missing property 'properties'",
		},
		{
			name:    "a size outside the enum",
			nodes:   []any{map[string]any{"id": "x", "type": "cache", "name": "c", "properties": map[string]any{"size": "enormous"}}},
			path:    "nodes.0.properties.size",
			nodeID:  "x",
			message: "'small'",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			errs := errorsOf(t, withDoc(t, "nodes", c.nodes))
			if len(errs) != 1 {
				t.Fatalf("errors = %v, want one", messages(errs))
			}
			if errs[0].Path != c.path {
				t.Errorf("path = %q, want %q", errs[0].Path, c.path)
			}
			if errs[0].NodeID != c.nodeID {
				t.Errorf("node id = %q, want %q", errs[0].NodeID, c.nodeID)
			}
			if !strings.Contains(errs[0].Message, c.message) {
				t.Errorf("message = %q, want it to mention %q", errs[0].Message, c.message)
			}
		})
	}
}

func TestValidateProjectReportsSchemaErrorsOnEdges(t *testing.T) {
	errs := errorsOf(t, withDoc(t, "edges", []any{
		map[string]any{"id": "e1", "from": "n1", "to": "n2", "relation": "nope"},
	}))
	if len(errs) != 1 {
		t.Fatalf("errors = %v, want one", messages(errs))
	}
	if errs[0].Path != "edges.0.relation" || errs[0].EdgeID != "e1" {
		t.Fatalf("error = %+v", errs[0])
	}
}

func TestValidateProjectReportsEachBadEnvKeyAtItsOwnPath(t *testing.T) {
	errs := errorsOf(t, withDoc(t, "nodes", []any{map[string]any{
		"id": "x", "type": "function", "name": "f",
		"properties": map[string]any{"env": map[string]any{"bad-key": "1", "OK": "2", "also bad": "3"}},
	}}))
	paths := make([]string, len(errs))
	for i, e := range errs {
		paths[i] = e.Path
	}
	slices.Sort(paths)
	want := []string{"nodes.0.properties.env.also bad", "nodes.0.properties.env.bad-key"}
	if d := cmp.Diff(want, paths); d != "" {
		t.Fatal(d)
	}
}

func TestValidateProjectReportsEveryErrorRatherThanTheFirst(t *testing.T) {
	errs := errorsOf(t, withDoc(t, "edges", []any{
		map[string]any{"id": "e1", "from": "n1", "to": "missing", "relation": "routes"},
		map[string]any{"id": "e2", "from": "n3", "to": "n2", "relation": "reads"},
	}))
	ids := make([]string, len(errs))
	for i, e := range errs {
		ids[i] = e.EdgeID
	}
	if d := cmp.Diff([]string{"e1", "e2"}, ids); d != "" {
		t.Fatal(d)
	}
	if !strings.Contains(errs[0].Message, "missing") {
		t.Fatalf("message = %q", errs[0].Message)
	}
	if !strings.Contains(errs[1].Message, "reads") {
		t.Fatalf("message = %q", errs[1].Message)
	}
}

func TestValidateProjectRejectsDuplicateNodeIDsAndNames(t *testing.T) {
	nodes := append(baseDoc()["nodes"].([]any),
		map[string]any{"id": "n3", "type": "bucket", "name": "handler"})
	errs := errorsOf(t, withDoc(t, "nodes", nodes))
	want := []string{"duplicate node id 'n3'", "duplicate node name 'handler'"}
	if d := cmp.Diff(want, messages(errs)); d != "" {
		t.Fatal(d)
	}
	if errs[0].Path != "nodes.3" {
		t.Fatalf("path = %q, want nodes.3", errs[0].Path)
	}
}

func TestValidateProjectRejectsMoreThanOneGatewayOncePerExtra(t *testing.T) {
	nodes := append(baseDoc()["nodes"].([]any),
		map[string]any{"id": "n4", "type": "gateway", "name": "api-two"},
		map[string]any{"id": "n5", "type": "gateway", "name": "api-three"})
	errs := errorsOf(t, withDoc(t, "nodes", nodes))
	ids := make([]string, len(errs))
	for i, e := range errs {
		ids[i] = e.NodeID
	}
	if d := cmp.Diff([]string{"n4", "n5"}, ids); d != "" {
		t.Fatal(d)
	}
	if errs[0].Message != "a project can have at most one gateway" {
		t.Fatalf("message = %q", errs[0].Message)
	}
}

func TestValidateProjectRejectsDuplicateEdgeIDs(t *testing.T) {
	edges := append(baseDoc()["edges"].([]any),
		map[string]any{"id": "e1", "from": "n1", "to": "n3", "relation": "reads"})
	errs := errorsOf(t, withDoc(t, "edges", edges))
	if !slices.Contains(messages(errs), "duplicate edge id 'e1'") {
		t.Fatalf("messages = %v", messages(errs))
	}
}

func TestValidateProjectRejectsAnEdgeFromANodeToItself(t *testing.T) {
	errs := errorsOf(t, withDoc(t, "edges", []any{
		map[string]any{"id": "e1", "from": "n2", "to": "n2", "relation": "calls"},
	}))
	if errs[0].Message != "an edge cannot connect a node to itself" {
		t.Fatalf("message = %q", errs[0].Message)
	}
}

func TestValidateProjectRejectsIllegalRelations(t *testing.T) {
	doc := baseDoc()
	doc["nodes"] = append(doc["nodes"].([]any),
		map[string]any{"id": "n4", "type": "function", "name": "worker"})
	doc["edges"] = []any{
		map[string]any{"id": "e1", "from": "n2", "to": "n4", "relation": "reads"},
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	errs := errorsOf(t, raw)
	if errs[0].Message != "a function cannot have a 'reads' edge to a function" {
		t.Fatalf("message = %q", errs[0].Message)
	}
}

func TestValidateProjectRejectsDuplicateEdges(t *testing.T) {
	edges := append(baseDoc()["edges"].([]any),
		map[string]any{"id": "e3", "from": "n2", "to": "n3", "relation": "reads"})
	errs := errorsOf(t, withDoc(t, "edges", edges))
	if errs[0].EdgeID != "e3" {
		t.Fatalf("edge id = %q, want e3", errs[0].EdgeID)
	}
	if errs[0].Message != "duplicate 'reads' edge from 'handler' to 'main-db'" {
		t.Fatalf("message = %q", errs[0].Message)
	}
}

func databaseWithVersion(version string) []any {
	return []any{
		map[string]any{"id": "n1", "type": "gateway", "name": "api"},
		map[string]any{"id": "n2", "type": "function", "name": "handler"},
		map[string]any{
			"id": "n3", "type": "database", "name": "main-db",
			"properties": map[string]any{"engine": "postgres", "version": version},
		},
	}
}

func TestValidateProjectRejectsAnEngineVersionTheProviderDoesNotHave(t *testing.T) {
	errs := errorsOf(t, withDoc(t, "nodes", databaseWithVersion("9.6")))
	if len(errs) != 1 {
		t.Fatalf("errors = %v, want one", messages(errs))
	}
	want := "database 'main-db': postgres version '9.6' is not supported on aws (use one of 17, 16, 15)"
	if errs[0].Message != want {
		t.Errorf("message = %q, want %q", errs[0].Message, want)
	}
	if errs[0].NodeID != "n3" {
		t.Errorf("node id = %q, want n3", errs[0].NodeID)
	}
}

func TestValidateProjectAcceptsASupportedEngineVersion(t *testing.T) {
	if errs := errorsOf(t, withDoc(t, "nodes", databaseWithVersion("16"))); len(errs) > 0 {
		t.Fatalf("errors = %v", messages(errs))
	}
}

func TestValidateProjectSkipsTheEngineCheckForAProviderWithNoTable(t *testing.T) {
	doc := baseDoc()
	doc["provider"] = "gcp"
	doc["region"] = "europe-west2"
	doc["nodes"] = databaseWithVersion("9.6")
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if errs := errorsOf(t, raw); len(errs) > 0 {
		t.Fatalf("errors = %v", messages(errs))
	}
}

func TestValidationErrorString(t *testing.T) {
	cases := []struct {
		err  ValidationError
		want string
	}{
		{ValidationError{Path: "nodes.0", NodeID: "n1", Message: "boom"}, "nodes.0 (node n1): boom"},
		{ValidationError{Path: "edges.1", EdgeID: "e2", Message: "boom"}, "edges.1 (edge e2): boom"},
		{ValidationError{Path: "name", Message: "boom"}, "name: boom"},
		{ValidationError{Message: "boom"}, "project: boom"},
	}
	for _, c := range cases {
		if got := c.err.String(); got != c.want {
			t.Errorf("String() = %q, want %q", got, c.want)
		}
	}
}
