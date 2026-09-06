package workspace

import (
	"os"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func writeLayout(t *testing.T, cwd, text string) {
	t.Helper()
	if err := os.MkdirAll(TogenDir(cwd), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(LayoutPath(cwd), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadLayoutMigratesVersionOneUnderTheOverview(t *testing.T) {
	cwd := t.TempDir()
	text := `{ "version": 1, "nodes": { "n1": { "x": 40, "y": 80 } }, "viewport": { "x": -10, "y": 5, "zoom": 1.5 } }`
	writeLayout(t, cwd, text)
	layout, err := LoadLayout(cwd)
	if err != nil {
		t.Fatal(err)
	}
	want := Layout{Version: 2, Views: map[string]ViewLayout{
		"overview": {Nodes: map[string]Position{"n1": {X: 40, Y: 80}}, Viewport: Viewport{X: -10, Y: 5, Zoom: 1.5}},
	}}
	if diff := cmp.Diff(want, layout); diff != "" {
		t.Errorf("layout (-want +got):\n%s", diff)
	}
	raw, err := os.ReadFile(LayoutPath(cwd))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != text {
		t.Error("reading the layout rewrote the file")
	}
}

func TestLoadLayoutReadsVersionTwo(t *testing.T) {
	cwd := t.TempDir()
	writeLayout(t, cwd, `{"version":2,"views":{
		"overview":{"nodes":{"n1":{"x":1,"y":2}},"viewport":{"x":0,"y":0,"zoom":1}},
		"orders":{"nodes":{},"viewport":{"x":3,"y":4,"zoom":2}}}}`)
	layout, err := LoadLayout(cwd)
	if err != nil {
		t.Fatal(err)
	}
	want := Layout{Version: 2, Views: map[string]ViewLayout{
		"overview": {Nodes: map[string]Position{"n1": {X: 1, Y: 2}}, Viewport: Viewport{Zoom: 1}},
		"orders":   {Nodes: map[string]Position{}, Viewport: Viewport{X: 3, Y: 4, Zoom: 2}},
	}}
	if diff := cmp.Diff(want, layout); diff != "" {
		t.Errorf("layout (-want +got):\n%s", diff)
	}
}

func TestLoadLayoutFillsInWhatAViewLeavesOut(t *testing.T) {
	for _, c := range []struct{ name, text string }{
		{"version one", `{"version":1}`},
		{"version two", `{"version":2,"views":{"overview":{}}}`},
	} {
		t.Run(c.name, func(t *testing.T) {
			cwd := t.TempDir()
			writeLayout(t, cwd, c.text)
			layout, err := LoadLayout(cwd)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(DefaultLayout(), layout); diff != "" {
				t.Errorf("layout (-want +got):\n%s", diff)
			}
		})
	}
}

func TestDefaultLayoutRoundTripsThroughTheFile(t *testing.T) {
	cwd := t.TempDir()
	if err := os.MkdirAll(TogenDir(cwd), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := WriteLayout(cwd, DefaultLayout()); err != nil {
		t.Fatal(err)
	}
	layout, err := LoadLayout(cwd)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(DefaultLayout(), layout); diff != "" {
		t.Errorf("layout (-want +got):\n%s", diff)
	}
}

func TestLoadLayoutReportsWhatItCannotRead(t *testing.T) {
	for _, c := range []struct{ name, text, want string }{
		{"not json", "{ not json", "togen/layout.json is not valid JSON"},
		{"not an object", `[]`, "togen/layout.json: got array, want object"},
		{"another version", `{"version":3,"views":{}}`, "togen/layout.json is version 3 but this Togen only understands up to 2"},
		{"no version", `{"views":{}}`, "togen/layout.json: missing property 'version'"},
		{"unknown key", `{"version":2,"views":{},"theme":"dark"}`, "togen/layout.json: additional properties 'theme' not allowed"},
		{"nodes shape", `{"version":1,"nodes":[]}`, "nodes: got array, want object"},
		{"position shape", `{"version":2,"views":{"overview":{"nodes":{"n1":{"x":"far","y":0}}}}}`, "views.overview.nodes.n1.x: got string, want number"},
		{"zoom", `{"version":2,"views":{"overview":{"viewport":{"x":0,"y":0,"zoom":0}}}}`, "views.overview.viewport.zoom: "},
	} {
		t.Run(c.name, func(t *testing.T) {
			cwd := t.TempDir()
			writeLayout(t, cwd, c.text)
			_, err := LoadLayout(cwd)
			lines := errorStrings(t, err)
			if len(lines) != 1 || !strings.HasPrefix(lines[0], c.want) {
				t.Errorf("errors = %v, want %q", lines, c.want)
			}
		})
	}
}
