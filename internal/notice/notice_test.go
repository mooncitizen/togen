package notice

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const root = "../.."

func TestTheRepositoryIsLicensed(t *testing.T) {
	text := read(t, filepath.Join(root, "LICENSE"))
	for _, want := range []string{"Apache License", "Version 2.0, January 2004"} {
		if !strings.Contains(text, want) {
			t.Errorf("LICENSE does not contain %q", want)
		}
	}
}

// Every bundled icon set carries a NOTICE.md beside it, and Apache-2.0 section
// 4(d) only carries those obligations onward if the root NOTICE names them.
func TestTheRootNoticeNamesEveryBundledIconSet(t *testing.T) {
	notice := read(t, filepath.Join(root, "NOTICE"))

	sets, err := filepath.Glob(filepath.Join(root, "ui", "icons", "*", "NOTICE.md"))
	if err != nil {
		t.Fatal(err)
	}
	if len(sets) == 0 {
		t.Fatal("no icon set notices found under ui/icons")
	}

	for _, path := range sets {
		title := strings.TrimSpace(strings.TrimPrefix(firstLine(read(t, path)), "#"))
		if !strings.Contains(notice, title) {
			t.Errorf("NOTICE does not name the icon set %q from %s", title, path)
		}
	}
}

func TestTheNoticeNamesTheCopyrightHolder(t *testing.T) {
	notice := read(t, filepath.Join(root, "NOTICE"))
	if !strings.Contains(notice, "Copyright 2026 Paul <paul@gitglue.com>") {
		t.Error("NOTICE does not carry the copyright line")
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(b)
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
