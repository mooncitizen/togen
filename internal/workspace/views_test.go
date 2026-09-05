package workspace

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/mooncitizen/togen/internal/ir"
)

func writeViews(t *testing.T, cwd, text string) {
	t.Helper()
	if err := os.MkdirAll(TogenDir(cwd), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ViewsPath(cwd), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func errorStrings(t *testing.T, err error) []string {
	t.Helper()
	if err == nil {
		t.Fatal("want an error")
	}
	var errs ir.Errors
	if !errors.As(err, &errs) {
		return []string{err.Error()}
	}
	lines := make([]string, len(errs))
	for i, e := range errs {
		lines[i] = e.String()
	}
	return lines
}

func twoViews() Views {
	return Views{Version: 1, Views: []View{
		{ID: "overview", Name: "Overview", Nodes: ViewNodes{All: true}},
		{ID: "orders", Name: "Orders path", Nodes: ViewNodes{IDs: []string{"n1", "n2"}}},
	}}
}

func TestLoadViewsReadsTheFile(t *testing.T) {
	cwd := t.TempDir()
	writeViews(t, cwd, `{
  "version": 1,
  "views": [
    { "id": "overview", "name": "Overview", "nodes": "*" },
    { "id": "orders", "name": "Orders path", "nodes": ["n1", "n2"] }
  ]
}`)
	views, err := LoadViews(cwd)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(twoViews(), views); diff != "" {
		t.Errorf("views (-want +got):\n%s", diff)
	}
}

func TestLoadViewsIsTheOverviewWhenTheFileIsMissing(t *testing.T) {
	views, err := LoadViews(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(DefaultViews(), views); diff != "" {
		t.Errorf("views (-want +got):\n%s", diff)
	}
}

func TestViewsRoundTripThroughTheFile(t *testing.T) {
	cwd := t.TempDir()
	if err := os.MkdirAll(TogenDir(cwd), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := WriteViews(cwd, twoViews()); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(ViewsPath(cwd))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"nodes": "*"`) || !strings.Contains(string(raw), "\"nodes\": [\n") {
		t.Errorf("file = %s", raw)
	}
	views, err := LoadViews(cwd)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(twoViews(), views); diff != "" {
		t.Errorf("views (-want +got):\n%s", diff)
	}
}

func TestAnEmptyNodeListIsWrittenAsAList(t *testing.T) {
	raw, err := Marshal(View{ID: "empty", Name: "Empty", Nodes: ViewNodes{}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"nodes": []`) {
		t.Errorf("view = %s", raw)
	}
}

func TestLoadViewsReportsABrokenFileWithPaths(t *testing.T) {
	for _, c := range []struct{ name, text, want string }{
		{"not json", "{ not json", "togen/views.json is not valid JSON"},
		{"no views", `{"version":1}`, "togen/views.json: missing property 'views'"},
		{"unknown key", `{"version":1,"views":[],"theme":"dark"}`, "togen/views.json: additional properties 'theme' not allowed"},
		{"future version", `{"version":2,"views":[]}`, "togen/views.json is version 2 but this Togen only understands up to 1"},
		{"bad id", `{"version":1,"views":[{"id":"Overview","name":"x","nodes":"*"}]}`, "views.0.id: "},
		{"empty name", `{"version":1,"views":[{"id":"overview","name":"","nodes":"*"}]}`, "views.0.name: "},
		{"nodes word", `{"version":1,"views":[{"id":"overview","name":"x","nodes":"all"}]}`, "views.0.nodes: value must be '*'"},
		{"nodes number", `{"version":1,"views":[{"id":"overview","name":"x","nodes":7}]}`, "views.0.nodes: got number, want string or array"},
		{"nodes item", `{"version":1,"views":[{"id":"overview","name":"x","nodes":["n1",3]}]}`, "views.0.nodes.1: got number, want string"},
		{"no overview", `{"version":1,"views":[{"id":"orders","name":"Orders","nodes":[]}]}`, "views: every project has an 'overview' view"},
		{"duplicate id", `{"version":1,"views":[{"id":"overview","name":"a","nodes":"*"},{"id":"overview","name":"b","nodes":"*"}]}`, "views.1.id: duplicate view id 'overview'"},
	} {
		t.Run(c.name, func(t *testing.T) {
			cwd := t.TempDir()
			writeViews(t, cwd, c.text)
			_, err := LoadViews(cwd)
			lines := errorStrings(t, err)
			if len(lines) != 1 || !strings.HasPrefix(lines[0], c.want) {
				t.Errorf("errors = %v, want %q", lines, c.want)
			}
		})
	}
}

func TestValidateViewsChecksTheNodesAgainstTheProject(t *testing.T) {
	project := &ir.Project{Nodes: []ir.Node{{ID: "n1"}, {ID: "n2"}}}
	views := twoViews()
	views.Views = append(views.Views,
		View{ID: "stores", Name: "Stores", Nodes: ViewNodes{IDs: []string{"n2", "zz", "yy"}}},
		View{ID: "orders", Name: "Again", Nodes: ViewNodes{IDs: []string{"n1"}}},
	)
	got := make([]string, 0)
	for _, e := range ValidateViews(views, project) {
		got = append(got, e.String())
	}
	want := []string{
		"views.3.id: duplicate view id 'orders'",
		"views.2.nodes.1: view 'stores' refers to missing node 'zz'",
		"views.2.nodes.2: view 'stores' refers to missing node 'yy'",
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
	if errs := ValidateViews(twoViews(), project); len(errs) > 0 {
		t.Errorf("errors = %v, want none", errs)
	}
}
