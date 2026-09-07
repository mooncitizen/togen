package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mooncitizen/togen/internal/workspace"
)

func writeSimulation(t *testing.T, cwd string) {
	t.Helper()
	writeFileText(t, workspace.SimulationPath(cwd), `{
  "version": 1,
  "sources": [
    {"id": "web", "name": "Web traffic", "target": "n1", "rate": "500/min"}
  ]
}
`)
}

func writeBadSimulation(t *testing.T, cwd string) {
	t.Helper()
	writeFileText(t, workspace.SimulationPath(cwd), `{
  "version": 1,
  "sources": [
    {"id": "web", "name": "Web traffic", "target": "node-404", "rate": "500/min"}
  ]
}
`)
}

func TestSimulatePrintsATable(t *testing.T) {
	cwd := generateCwd(t)
	writeSimulation(t, cwd)
	got := Simulate(cwd, false)
	if got.Code != 0 {
		t.Fatalf("code %d: %v", got.Code, got.Lines)
	}
	if !strings.Contains(strings.Join(got.Lines, "\n"), "nodes") {
		t.Fatalf("no table: %v", got.Lines)
	}
}

func TestSimulateJSON(t *testing.T) {
	cwd := generateCwd(t)
	writeSimulation(t, cwd)
	got := Simulate(cwd, true)
	var doc struct {
		Nodes []struct {
			RatePerMonth float64 `json:"ratePerMonth"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal([]byte(strings.Join(got.Lines, "\n")), &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Nodes) == 0 || doc.Nodes[0].RatePerMonth == 0 {
		t.Fatalf("the entry node should carry the load: %+v", doc.Nodes)
	}
}

func TestSimulateRefusesABrokenFile(t *testing.T) {
	cwd := generateCwd(t)
	writeBadSimulation(t, cwd)
	if got := Simulate(cwd, false); got.Code == 0 {
		t.Fatalf("want a non-zero code, got %v", got.Lines)
	}
}

func TestSimulateSaysThereIsNoSimulation(t *testing.T) {
	cwd := generateCwd(t)
	got := Simulate(cwd, false)
	if got.Code != 0 {
		t.Fatalf("code %d: %v", got.Code, got.Lines)
	}
	line := strings.Join(got.Lines, "\n")
	if !strings.Contains(line, "no togen/simulation.json") {
		t.Fatalf("it should say there is nothing to simulate: %v", got.Lines)
	}
	if strings.Contains(line, "nodes") {
		t.Fatalf("no table of zeros: %v", got.Lines)
	}
}
