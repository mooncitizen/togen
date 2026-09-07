package workspace

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mooncitizen/togen/internal/simulate"
)

func TestLoadSimulationMissingIsEmpty(t *testing.T) {
	dir := t.TempDir()
	sim, err := LoadSimulation(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !sim.IsEmpty() {
		t.Fatalf("want an empty simulation, got %+v", sim)
	}
}

func TestWriteThenLoadSimulation(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(TogenDir(dir), 0o755); err != nil {
		t.Fatal(err)
	}
	want := simulate.Simulation{
		Version: simulate.Version,
		Sources: []simulate.Source{{ID: "mobile", Name: "Mobile app", Target: "gateway-1", Rate: "800/min"}},
		Edges:   map[string]float64{"edge-2": 3},
	}
	if err := WriteSimulation(dir, want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadSimulation(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Sources) != 1 || got.Sources[0].Rate != "800/min" || got.Edges["edge-2"] != 3 {
		t.Fatalf("round trip lost something: %+v", got)
	}
}

func TestParseSimulationRefusesJunk(t *testing.T) {
	if _, err := ParseSimulation([]byte(`{"version":1,"sources":[{"id":"m"}]}`)); err == nil {
		t.Fatal("want a schema error for a source with no target")
	}
	if _, err := ParseSimulation([]byte("not json")); err == nil {
		t.Fatal("want an error for junk")
	}
}

func TestParseSimulationRefusesFutureVersion(t *testing.T) {
	if _, err := ParseSimulation([]byte(`{"version":99,"sources":[]}`)); err == nil {
		t.Fatal("want a version error")
	}
}

func TestSimulationPath(t *testing.T) {
	if got, want := SimulationPath("/tmp/x"), filepath.Join("/tmp/x", "togen", "simulation.json"); got != want {
		t.Fatalf("got %s want %s", got, want)
	}
}
