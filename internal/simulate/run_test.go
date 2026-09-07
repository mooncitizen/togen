package simulate

import (
	"math"
	"testing"

	"github.com/mooncitizen/togen/internal/ir"
)

func near(t *testing.T, got, want float64, what string) {
	t.Helper()
	if math.Abs(got-want) > 1e-6*math.Max(1, math.Abs(want)) {
		t.Fatalf("%s: got %v want %v", what, got, want)
	}
}

func TestRunMultipliesAlongEdges(t *testing.T) {
	sim := sound()
	got, err := Run(project(), sim, map[string]float64{"mobile": 1000})
	if err != nil {
		t.Fatal(err)
	}
	near(t, got.Nodes["gateway-1"], 1000, "gateway")
	near(t, got.Nodes["service-1"], 1000, "service")
	near(t, got.Nodes["database-1"], 3000, "database")
	near(t, got.Edges["edge-1"], 1000, "edge-1")
	near(t, got.Edges["edge-2"], 3000, "edge-2")
}

func TestRunSumsTwoSources(t *testing.T) {
	sim := sound()
	sim.Sources = append(sim.Sources, Source{ID: "partner", Name: "Partner", Target: "service-1", Rate: "100/min"})
	got, err := Run(project(), sim, map[string]float64{"mobile": 1000, "partner": 500})
	if err != nil {
		t.Fatal(err)
	}
	near(t, got.Nodes["service-1"], 1500, "service")
	near(t, got.Nodes["database-1"], 4500, "database")
}

func TestRunLeavesUnreachedNodesAtZero(t *testing.T) {
	p := project()
	p.Nodes = append(p.Nodes, ir.Node{ID: "bucket-1", Type: ir.NodeBucket, Name: "assets"})
	got, err := Run(p, sound(), map[string]float64{"mobile": 1000})
	if err != nil {
		t.Fatal(err)
	}
	near(t, got.Nodes["bucket-1"], 0, "bucket")
}

func TestRunConsumesPullsFromTheQueue(t *testing.T) {
	p := project()
	p.Nodes = append(p.Nodes,
		ir.Node{ID: "queue-1", Type: ir.NodeQueue, Name: "orders-q"},
		ir.Node{ID: "function-1", Type: ir.NodeFunction, Name: "worker"},
	)
	p.Edges = append(p.Edges,
		ir.Edge{ID: "edge-3", From: "service-1", To: "queue-1", Relation: ir.RelPublishes},
		ir.Edge{ID: "edge-4", From: "function-1", To: "queue-1", Relation: ir.RelConsumes},
	)
	sim := sound()
	sim.Edges["edge-3"] = 0.5
	got, err := Run(p, sim, map[string]float64{"mobile": 1000})
	if err != nil {
		t.Fatal(err)
	}
	near(t, got.Nodes["queue-1"], 500, "queue")
	near(t, got.Nodes["function-1"], 500, "consumer")
	near(t, got.Edges["edge-4"], 500, "consumes edge")
}

func TestRunRefusesACycle(t *testing.T) {
	p := project()
	p.Edges = append(p.Edges, ir.Edge{ID: "edge-3", From: "service-1", To: "gateway-1", Relation: ir.RelCalls})
	if _, err := Run(p, sound(), map[string]float64{"mobile": 1000}); err == nil {
		t.Fatal("want a cycle error")
	}
}

func TestMonthlyAddsTheBurst(t *testing.T) {
	got, err := Monthly(sound())
	if err != nil {
		t.Fatal(err)
	}
	base := 800.0 * 60 * 730
	want := base + 5*(base/SecondsPerMonth)*30*60*2
	near(t, got["mobile"], want, "monthly")
}

func TestInstantAppliesTheChosenBurst(t *testing.T) {
	base := 800.0 * 60 * 730 / SecondsPerMonth
	flat, err := Instant(sound(), "")
	if err != nil {
		t.Fatal(err)
	}
	near(t, flat["mobile"], base, "baseline")
	peak, err := Instant(sound(), "launch")
	if err != nil {
		t.Fatal(err)
	}
	near(t, peak["mobile"], base*6, "burst")
}
