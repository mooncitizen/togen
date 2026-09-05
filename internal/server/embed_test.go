package server

import (
	"io/fs"
	"strings"
	"testing"
)

func TestUIServesThePlaceholderUntilTheCanvasIsBuilt(t *testing.T) {
	dist, err := fs.Sub(built, "dist")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fs.Stat(dist, "index.html"); err == nil {
		t.Skip("a built canvas is embedded, so there is nothing to fall back to")
	}

	raw, err := fs.ReadFile(UI(), "index.html")
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	if !strings.Contains(string(raw), "The canvas is not built yet") {
		t.Errorf("index.html = %s, want the placeholder", raw)
	}
}
