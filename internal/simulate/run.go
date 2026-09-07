package simulate

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/mooncitizen/togen/internal/cost"
	"github.com/mooncitizen/togen/internal/ir"
)

const SecondsPerMonth = cost.HoursPerMonth * 3600

type Result struct {
	Nodes map[string]float64 `json:"nodes"`
	Edges map[string]float64 `json:"edges"`
}

// A consumer's work follows from the queue's messages, so that edge is walked backwards.
type arc struct {
	edge, from, to string
	per            float64
}

func arcs(project *ir.Project, sim Simulation) []arc {
	out := make([]arc, 0, len(project.Edges))
	for _, e := range project.Edges {
		per := 1.0
		if v, ok := sim.Edges[e.ID]; ok {
			per = v
		}
		a := arc{edge: e.ID, from: e.From, to: e.To, per: per}
		if e.Relation == ir.RelConsumes {
			a.from, a.to = e.To, e.From
		}
		out = append(out, a)
	}
	return out
}

const (
	settleTolerance = 1e-9
	settleSweeps    = 500
)

// Reversing 'consumes' can make a loop out of edges the IR is right to allow, because a queue
// decouples the two halves of an async cycle. So the rates are found by sweeping to a fixed
// point rather than by sorting: every arc multiplies by a non-negative fan-out, so a loop whose
// gain is below 1 converges on its geometric sum and one at or above 1 grows without limit.
func Run(project *ir.Project, sim Simulation, injection map[string]float64) (Result, error) {
	as := arcs(project, sim)
	incoming := map[string][]arc{}
	outgoing := map[string][]arc{}
	for _, a := range as {
		incoming[a.to] = append(incoming[a.to], a)
		outgoing[a.from] = append(outgoing[a.from], a)
	}
	order := ordered(project, incoming, outgoing)

	entry := map[string]float64{}
	for _, s := range sim.Sources {
		entry[s.Target] += injection[s.ID]
	}

	nodes := make(map[string]float64, len(project.Nodes))
	for _, n := range project.Nodes {
		nodes[n.ID] = 0
	}

	var moving []string
	for sweep := 0; sweep < settleSweeps; sweep++ {
		moving = moving[:0]
		for _, id := range order {
			rate := entry[id]
			for _, a := range incoming[id] {
				rate += nodes[a.from] * a.per
			}
			if !settledAt(nodes[id], rate) {
				moving = append(moving, id)
			}
			nodes[id] = rate
		}
		if len(moving) == 0 {
			edges := make(map[string]float64, len(as))
			for _, a := range as {
				edges[a.edge] = nodes[a.from] * a.per
			}
			return Result{Nodes: nodes, Edges: edges}, nil
		}
	}
	return Result{}, fmt.Errorf("the traffic through %s keeps growing every time it is worked out, because those nodes feed each other round a loop that amplifies: each pass sends back at least as much as it received. Give one edge on the loop a fan-out below 1 so the loop dies away", list(moving))
}

func settledAt(was, now float64) bool {
	change := math.Abs(now - was)
	if scale := math.Abs(now); scale > 1e-12 {
		change /= scale
	}
	return change < settleTolerance
}

func list(ids []string) string {
	out := slices.Clone(ids)
	slices.Sort(out)
	out = slices.Compact(out)
	for i, id := range out {
		out[i] = "'" + id + "'"
	}
	if len(out) < 2 {
		return strings.Join(out, "")
	}
	return strings.Join(out[:len(out)-1], ", ") + " and " + out[len(out)-1]
}

// Sweeping in topological order settles a graph with no loop in a single pass. Nodes left over
// are the ones on a loop, and they follow in the order they were declared.
func ordered(project *ir.Project, incoming, outgoing map[string][]arc) []string {
	left := make(map[string]int, len(project.Nodes))
	var queue []string
	for _, n := range project.Nodes {
		left[n.ID] = len(incoming[n.ID])
		if left[n.ID] == 0 {
			queue = append(queue, n.ID)
		}
	}
	out := make([]string, 0, len(project.Nodes))
	placed := make(map[string]bool, len(project.Nodes))
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		out = append(out, id)
		placed[id] = true
		for _, a := range outgoing[id] {
			left[a.to]--
			if left[a.to] == 0 {
				queue = append(queue, a.to)
			}
		}
	}
	for _, n := range project.Nodes {
		if !placed[n.ID] {
			out = append(out, n.ID)
		}
	}
	return out
}

func Monthly(sim Simulation) (map[string]float64, error) {
	out := make(map[string]float64, len(sim.Sources))
	for _, s := range sim.Sources {
		base, err := s.Rate.PerMonth()
		if err != nil {
			return nil, err
		}
		total := base
		for _, b := range sim.Bursts {
			if b.Source == s.ID {
				total += (b.Multiplier - 1) * (base / SecondsPerMonth) * b.Minutes * 60 * b.TimesPerMonth
			}
		}
		out[s.ID] = total
	}
	return out, nil
}

// Rates per second at one scenario: an empty id or "baseline" for the flat rate,
// otherwise the burst that multiplies its own source.
func Instant(sim Simulation, scenario string) (map[string]float64, error) {
	out := make(map[string]float64, len(sim.Sources))
	for _, s := range sim.Sources {
		base, err := s.Rate.PerMonth()
		if err != nil {
			return nil, err
		}
		rate := base / SecondsPerMonth
		for _, b := range sim.Bursts {
			if b.ID == scenario && b.Source == s.ID {
				rate *= b.Multiplier
			}
		}
		out[s.ID] = rate
	}
	return out, nil
}
