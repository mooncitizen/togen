package simulate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mooncitizen/togen/internal/ir"
)

type fixture struct {
	Name       string             `json:"name"`
	Project    ir.Project         `json:"project"`
	Simulation Simulation         `json:"simulation"`
	Injection  map[string]float64 `json:"injection"`
	Expect     struct {
		Nodes map[string]float64 `json:"nodes"`
		Edges map[string]float64 `json:"edges"`
	} `json:"expect"`
}

func TestSharedCases(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("testdata", "cases", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("no cases found")
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var f fixture
			if err := json.Unmarshal(raw, &f); err != nil {
				t.Fatal(err)
			}
			if errs := Validate(f.Simulation, &f.Project); len(errs) > 0 {
				t.Fatalf("the case itself is invalid: %v", errs)
			}
			got, err := Run(&f.Project, f.Simulation, f.Injection)
			if err != nil {
				t.Fatal(err)
			}
			for id, want := range f.Expect.Nodes {
				near(t, got.Nodes[id], want, "node "+id)
			}
			for id, want := range f.Expect.Edges {
				near(t, got.Edges[id], want, "edge "+id)
			}
		})
	}
}
