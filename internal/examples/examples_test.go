package examples

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/mooncitizen/togen/internal/workspace"
)

func exampleDirs(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join("..", "..", "examples"))
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, entry := range entries {
		if entry.IsDir() {
			ids = append(ids, entry.Name())
		}
	}
	return ids
}

func TestBundledExamplesAreCurrent(t *testing.T) {
	var listed []string
	for _, e := range List() {
		listed = append(listed, e.ID)
		if e.Description == "" {
			t.Errorf("%s has no description", e.ID)
		}
	}
	if diff := cmp.Diff(exampleDirs(t), listed); diff != "" {
		t.Fatalf("examples/ and List() differ (-disk +list):\n%s", diff)
	}
	for _, id := range listed {
		got, ok := Files(id)
		if !ok {
			t.Fatalf("%s is not bundled, run just generate", id)
		}
		want := map[string][]byte{}
		for _, name := range workspace.ProjectFiles {
			raw, err := os.ReadFile(filepath.Join("..", "..", "examples", id, filepath.FromSlash(name)))
			if err != nil {
				t.Fatal(err)
			}
			want[name] = raw
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("%s is stale, run just generate (-examples +bundled):\n%s", id, diff)
		}
	}
}

func TestEveryBundledExampleValidates(t *testing.T) {
	for _, e := range List() {
		files, _ := Files(e.ID)
		_, errs, err := workspace.ValidateRaw(files["togen/project.json"])
		if err != nil {
			t.Fatalf("%s: %v", e.ID, err)
		}
		if len(errs) > 0 {
			t.Errorf("%s: %v", e.ID, errs)
		}
	}
}

func TestFilesRefusesAnUnknownExample(t *testing.T) {
	for _, id := range []string{"nope", "", "aws-basic/togen", "../examples/aws-basic"} {
		if _, ok := Files(id); ok {
			t.Errorf("Files(%q) found something", id)
		}
	}
}
