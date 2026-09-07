package simulate

import (
	"math"
	"slices"
	"strconv"
	"strings"
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

// The error names nodes the way the table and the canvas do, by name, and names the loop and
// nothing else, so a database hanging off it stays unnamed.
func namesExactly(t *testing.T, err error, loop ...string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected a refusal")
	}
	quoted := func(name string) bool { return strings.Contains(err.Error(), "'"+name+"'") }
	named := map[string]bool{}
	for _, name := range loop {
		named[name] = true
		if !quoted(name) {
			t.Errorf("the error should name '%s': %v", name, err)
		}
	}
	for _, name := range []string{"edge", "orders", "orders-db", "jobs", "worker", "web", "api", "assets"} {
		if !named[name] && quoted(name) {
			t.Errorf("'%s' is not on the loop, so the error should not name it: %v", name, err)
		}
	}
	for _, id := range []string{"gateway-1", "service-1", "database-1", "queue-1", "function-1", "function-2"} {
		if strings.Contains(err.Error(), id) {
			t.Errorf("the error should use names, not the id '%s': %v", id, err)
		}
	}
}

func TestRunRefusesALoopThatDoesNotDieAway(t *testing.T) {
	p := project()
	p.Edges = append(p.Edges, ir.Edge{ID: "edge-3", From: "service-1", To: "gateway-1", Relation: ir.RelCalls})
	_, err := Run(p, sound(), map[string]float64{"mobile": 1000})
	namesExactly(t, err, "edge", "orders")
}

// The aws-basic shape: a queue decouples an async loop the IR allows.
func feedback() *ir.Project {
	return &ir.Project{
		Version: ir.Version, Name: "shop", Provider: ir.ProviderAWS, Region: "eu-west-2", Environment: "dev",
		Nodes: []ir.Node{
			{ID: "gateway-1", Type: ir.NodeGateway, Name: "api"},
			{ID: "function-1", Type: ir.NodeFunction, Name: "orders"},
			{ID: "queue-1", Type: ir.NodeQueue, Name: "jobs"},
			{ID: "function-2", Type: ir.NodeFunction, Name: "worker"},
			{ID: "service-1", Type: ir.NodeService, Name: "web"},
		},
		Edges: []ir.Edge{
			{ID: "edge-1", From: "gateway-1", To: "function-1", Relation: ir.RelRoutes},
			{ID: "edge-2", From: "function-1", To: "queue-1", Relation: ir.RelPublishes},
			{ID: "edge-3", From: "function-2", To: "queue-1", Relation: ir.RelConsumes},
			{ID: "edge-4", From: "function-2", To: "service-1", Relation: ir.RelCalls},
			{ID: "edge-5", From: "service-1", To: "function-1", Relation: ir.RelCalls},
		},
	}
}

func feedbackSim(per float64) Simulation {
	return Simulation{
		Version: Version,
		Sources: []Source{{ID: "mobile", Name: "Mobile app", Target: "gateway-1", Rate: "800/min"}},
		Edges:   map[string]float64{"edge-2": per},
	}
}

func TestRunSettlesAQueueFeedbackLoop(t *testing.T) {
	got, err := Run(feedback(), feedbackSim(0.2), map[string]float64{"mobile": 1000})
	if err != nil {
		t.Fatal(err)
	}
	// 1000 * (1 + 0.2 + 0.04 + ...) = 1000 / (1 - 0.2) = 1250.
	near(t, got.Nodes["function-1"], 1250, "function-1")
	near(t, got.Nodes["queue-1"], 250, "queue-1")
	near(t, got.Nodes["function-2"], 250, "function-2")
	near(t, got.Nodes["service-1"], 250, "service-1")
	near(t, got.Edges["edge-2"], 250, "publishes")
	near(t, got.Edges["edge-3"], 250, "consumes")
	near(t, got.Edges["edge-5"], 250, "calls back")
}

func TestRunRefusesAQueueLoopThatAmplifies(t *testing.T) {
	for _, per := range []float64{1, 1.0000001, 1.5, 40} {
		_, err := Run(feedback(), feedbackSim(per), map[string]float64{"mobile": 1000})
		namesExactly(t, err, "orders", "jobs", "worker", "web")
		if err != nil && !strings.Contains(err.Error(), "fan-out") {
			t.Errorf("the error should say how to damp the loop: %v", err)
		}
	}
}

// The loop is function-1 -> queue-1 -> function-2 -> service-1 -> function-1, and only the
// publishes edge carries a fan-out, so the gain round the loop is that fan-out and
// function-1 = 1000 / (1 - gain), everything else on the loop being gain times that.
func TestRunSettlesLoopsCloseToGainOne(t *testing.T) {
	for _, c := range []struct{ gain, head float64 }{
		{0.9, 10000},         // 1000 / 0.10
		{0.97, 100000.0 / 3}, // 1000 / 0.03 = 33333.333...
		{0.99, 100000},       // 1000 / 0.01
	} {
		got, err := Run(feedback(), feedbackSim(c.gain), map[string]float64{"mobile": 1000})
		if err != nil {
			t.Fatalf("gain %v converges on %v, so it should not be refused: %v", c.gain, c.head, err)
		}
		near(t, got.Nodes["function-1"], c.head, "function-1")
		for _, id := range []string{"queue-1", "function-2", "service-1"} {
			near(t, got.Nodes[id], c.gain*c.head, id)
		}
	}
}

// A gain this close to 1 does converge, but only after millions of passes, so the sweep budget
// runs out. That refusal has to name the loop too, and say something true about why.
func TestRunGivesUpOnALoopThatSettlesTooSlowly(t *testing.T) {
	_, err := Run(feedback(), feedbackSim(0.9999), map[string]float64{"mobile": 1000})
	namesExactly(t, err, "orders", "jobs", "worker", "web")
	if err != nil && !strings.Contains(err.Error(), "only just less than") {
		t.Errorf("the error should say the loop is only just damped: %v", err)
	}
}

// The whole point of sweeping from the previous pass's rates: project.json's node order is
// cosmetic, and the studio reorders it freely, so it must move neither the rates nor the verdict.
func TestRunDoesNotDependOnNodeOrder(t *testing.T) {
	reversed := func(p *ir.Project) *ir.Project {
		slices.Reverse(p.Nodes)
		return p
	}
	for _, gain := range []float64{0.2, 0.9, 0.96, 0.97, 0.99, 1, 2} {
		forward, ferr := Run(feedback(), feedbackSim(gain), map[string]float64{"mobile": 1000})
		back, berr := Run(reversed(feedback()), feedbackSim(gain), map[string]float64{"mobile": 1000})
		if (ferr == nil) != (berr == nil) {
			t.Fatalf("gain %v: reversing the nodes changed the verdict: %v then %v", gain, ferr, berr)
		}
		if ferr != nil {
			if ferr.Error() != berr.Error() {
				t.Errorf("gain %v: reversing the nodes changed the refusal:\n%v\n%v", gain, ferr, berr)
			}
			continue
		}
		for id, want := range forward.Nodes {
			near(t, back.Nodes[id], want, "gain "+strconv.FormatFloat(gain, 'g', -1, 64)+" node "+id)
		}
		for id, want := range forward.Edges {
			near(t, back.Edges[id], want, "gain "+strconv.FormatFloat(gain, 'g', -1, 64)+" edge "+id)
		}
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

// Two loops through one node, each of gain 0.6. Every loop on its own multiplies out to less
// than 1, and the traffic still runs away, because 'hub' adds up what both send back. This is
// the case the old wording called settled and told the user to fix by lowering a fan-out that
// was already low enough.
func TestRunRefusesTwoDampedLoopsThroughOneNode(t *testing.T) {
	p := &ir.Project{
		Version: ir.Version, Name: "shop", Provider: ir.ProviderAWS, Region: "eu-west-2", Environment: "dev",
		Nodes: []ir.Node{
			{ID: "gateway-1", Type: ir.NodeGateway, Name: "api"},
			{ID: "service-1", Type: ir.NodeService, Name: "hub"},
			{ID: "service-2", Type: ir.NodeService, Name: "left"},
			{ID: "service-3", Type: ir.NodeService, Name: "right"},
		},
		Edges: []ir.Edge{
			{ID: "edge-1", From: "gateway-1", To: "service-1", Relation: ir.RelRoutes},
			{ID: "edge-2", From: "service-1", To: "service-2", Relation: ir.RelCalls},
			{ID: "edge-3", From: "service-2", To: "service-1", Relation: ir.RelCalls},
			{ID: "edge-4", From: "service-1", To: "service-3", Relation: ir.RelCalls},
			{ID: "edge-5", From: "service-3", To: "service-1", Relation: ir.RelCalls},
		},
	}
	sim := Simulation{
		Version: Version,
		Sources: []Source{{ID: "mobile", Name: "Mobile app", Target: "gateway-1", Rate: "800/min"}},
		Edges:   map[string]float64{"edge-2": 0.6, "edge-4": 0.6},
	}
	_, err := Run(p, sim, map[string]float64{"mobile": 1000})
	if err == nil {
		t.Fatal("0.6 + 0.6 comes back for every 1 that goes in, so this should be refused")
	}
	for _, name := range []string{"hub", "left", "right"} {
		if !strings.Contains(err.Error(), "'"+name+"'") {
			t.Errorf("the error should name '%s': %v", name, err)
		}
	}
}
