package server

import (
	"net/http"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/mooncitizen/togen/internal/workspace"
)

func exampleSimulation() map[string]any {
	return map[string]any{
		"version": 1,
		"sources": []any{
			map[string]any{"id": "mobile", "name": "Mobile app", "target": "n1", "rate": "800/min"},
		},
	}
}

func TestGetSimulationAnswersWithAnEmptySimulationWhenTheFileIsMissing(t *testing.T) {
	_, front, _ := harness(t)
	want := map[string]any{"version": float64(1), "sources": nil}
	if diff := cmp.Diff(want, getJSON(t, front, "/api/simulation", http.StatusOK)); diff != "" {
		t.Errorf("simulation (-want +got):\n%s", diff)
	}
}

func TestSimulationRoundTrips(t *testing.T) {
	dir, front, _ := harness(t)
	if code, raw := send(t, front, http.MethodPut, "/api/simulation", marshal(t, exampleSimulation())); code != http.StatusNoContent {
		t.Fatalf("put: code = %d, body = %s", code, raw)
	}
	if !workspace.Exists(workspace.SimulationPath(dir)) {
		t.Fatal("simulation.json was not written")
	}
	want := map[string]any{
		"version": float64(1),
		"sources": []any{
			map[string]any{"id": "mobile", "name": "Mobile app", "target": "n1", "rate": "800/min"},
		},
	}
	if diff := cmp.Diff(want, getJSON(t, front, "/api/simulation", http.StatusOK)); diff != "" {
		t.Errorf("simulation (-want +got):\n%s", diff)
	}
}

func TestPutSimulationRefusesAMissingTarget(t *testing.T) {
	dir, front, _ := harness(t)
	sim := exampleSimulation()
	sim["sources"].([]any)[0].(map[string]any)["target"] = "no-such-node"
	code, raw := send(t, front, http.MethodPut, "/api/simulation", marshal(t, sim))
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("code = %d, body = %s", code, raw)
	}
	want := []string{"sources.0.target: there is no node 'no-such-node'"}
	if diff := cmp.Diff(want, errorLines(t, raw)); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
	if workspace.Exists(workspace.SimulationPath(dir)) {
		t.Error("simulation.json was written despite the refusal")
	}
}

func TestPutSimulationNeedsAProjectThatLoads(t *testing.T) {
	dir, front, _ := harness(t)
	if err := workspace.WriteRaw(workspace.ProjectPath(dir), []byte(`{"version":1,"name":"shop"}`)); err != nil {
		t.Fatal(err)
	}
	code, raw := send(t, front, http.MethodPut, "/api/simulation", marshal(t, exampleSimulation()))
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("code = %d, body = %s", code, raw)
	}
	want := []string{"project: the simulation cannot be checked without a valid project"}
	if diff := cmp.Diff(want, errorLines(t, raw)); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
}

func TestGetSimulationReportsABrokenFile(t *testing.T) {
	dir, front, _ := harness(t)
	if err := workspace.WriteRaw(workspace.SimulationPath(dir), []byte(`{"version":1,"sources":[{"id":"Bad Id","name":"x","target":"n1","rate":"800/min"}]}`)); err != nil {
		t.Fatal(err)
	}
	code, _ := send(t, front, http.MethodGet, "/api/simulation", nil)
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("code = %d", code)
	}
}
