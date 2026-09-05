package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteRawLeavesNoTempFileBehind(t *testing.T) {
	cwd := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cwd, "togen"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := ProjectPath(cwd)
	if err := WriteRaw(path, []byte(`{"a":1}`)); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"a":1}` {
		t.Errorf("content = %s", raw)
	}

	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".project-") {
			t.Errorf("temp file left behind: %s", entry.Name())
		}
	}
}
