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
	settleSweeps    = 200000
	settleWindow    = 1000
	runawayFactor   = 1e12
)

// Reversing 'consumes' can make a loop out of edges the IR is right to allow, because a queue
// decouples the two halves of an async cycle. So the rates are found by sweeping to a fixed point
// rather than by sorting. Each sweep reads only the previous sweep's rates, so the answer and the
// number of sweeps do not depend on the order the nodes were declared in.
//
// Every arc multiplies by a non-negative fan-out, so the sweep contracts, and the rates settle on
// their geometric sum, exactly when the fan-outs round every loop multiply out to less than 1. A
// loop at 1 or more is caught two ways: it runs away past a huge multiple of the injected traffic
// (or to Inf, or to NaN once Inf meets Inf), or, at exactly 1, the change per sweep stops
// shrinking from one window of sweeps to the next.
func Run(project *ir.Project, sim Simulation, injection map[string]float64) (Result, error) {
	as := arcs(project, sim)
	incoming := map[string][]arc{}
	outgoing := map[string][]arc{}
	for _, a := range as {
		incoming[a.to] = append(incoming[a.to], a)
		outgoing[a.from] = append(outgoing[a.from], a)
	}

	entry := map[string]float64{}
	injected := 0.0
	for _, s := range sim.Sources {
		entry[s.Target] += injection[s.ID]
		injected += math.Abs(injection[s.ID])
	}
	runaway := injected * runawayFactor

	ids := make([]string, 0, len(project.Nodes))
	nodes := make(map[string]float64, len(project.Nodes))
	for _, n := range project.Nodes {
		if _, seen := nodes[n.ID]; !seen {
			ids = append(ids, n.ID)
		}
		nodes[n.ID] = 0
	}
	next := make(map[string]float64, len(nodes))

	moved, movedBefore := map[string]bool{}, map[string]bool{}
	var thisWindow, lastWindow float64
	for sweep := 1; sweep <= settleSweeps; sweep++ {
		settled := true
		biggest := 0.0
		for _, id := range ids {
			rate := entry[id]
			for _, a := range incoming[id] {
				rate += nodes[a.from] * a.per
			}
			if math.IsNaN(rate) || math.Abs(rate) > runaway {
				return Result{}, amplifying(ids, outgoing, moving(moved, movedBefore))
			}
			if !settledAt(nodes[id], rate) {
				settled = false
				moved[id] = true
			}
			if change := math.Abs(rate - nodes[id]); change > biggest {
				biggest = change
			}
			next[id] = rate
		}
		nodes, next = next, nodes
		if settled {
			edges := make(map[string]float64, len(as))
			for _, a := range as {
				edges[a.edge] = nodes[a.from] * a.per
			}
			return Result{Nodes: nodes, Edges: edges}, nil
		}
		if biggest > thisWindow {
			thisWindow = biggest
		}
		if sweep%settleWindow == 0 {
			if lastWindow > 0 && thisWindow >= lastWindow {
				return Result{}, amplifying(ids, outgoing, moving(moved, movedBefore))
			}
			lastWindow, thisWindow = thisWindow, 0
			moved, movedBefore = map[string]bool{}, moved
		}
	}
	return Result{}, fmt.Errorf("the traffic through %s has still not settled after %d passes: the fan-outs round that loop multiply out to just under 1, so the traffic takes an impractical number of passes to die away. Lower the fan-out on one of the loop's edges", list(loopNodes(ids, outgoing, moving(moved, movedBefore))), settleSweeps)
}

func moving(moved, before map[string]bool) map[string]bool {
	if len(moved) > 0 {
		return moved
	}
	return before
}

func amplifying(ids []string, outgoing map[string][]arc, moved map[string]bool) error {
	return fmt.Errorf("the traffic through %s keeps growing every time it is worked out, because those nodes feed each other round a loop whose fan-outs multiply out to 1 or more, so each pass round the loop sends back at least as much as it received. Lower the fan-out on one of the loop's edges until the fan-outs round the loop multiply out to less than 1", list(loopNodes(ids, outgoing, moved)))
}

// The nodes still moving include everything downstream of the loop, so name the loop itself: the
// strongly connected components, of more than one node or with a self-arc, that the movers sit in.
func loopNodes(ids []string, outgoing map[string][]arc, moved map[string]bool) []string {
	var out []string
	for _, comp := range components(ids, outgoing) {
		if !cyclic(comp, outgoing) {
			continue
		}
		for _, id := range comp {
			if moved[id] {
				out = append(out, comp...)
				break
			}
		}
	}
	if len(out) == 0 {
		for id := range moved {
			out = append(out, id)
		}
	}
	return out
}

func cyclic(comp []string, outgoing map[string][]arc) bool {
	if len(comp) > 1 {
		return true
	}
	for _, a := range outgoing[comp[0]] {
		if a.to == comp[0] {
			return true
		}
	}
	return false
}

func components(ids []string, outgoing map[string][]arc) [][]string {
	index := map[string]int{}
	low := map[string]int{}
	stacked := map[string]bool{}
	var stack []string
	var out [][]string
	seen := 0
	var visit func(string)
	visit = func(id string) {
		index[id], low[id] = seen, seen
		seen++
		stack = append(stack, id)
		stacked[id] = true
		for _, a := range outgoing[id] {
			if _, been := index[a.to]; !been {
				visit(a.to)
				low[id] = min(low[id], low[a.to])
			} else if stacked[a.to] {
				low[id] = min(low[id], index[a.to])
			}
		}
		if low[id] != index[id] {
			return
		}
		var comp []string
		for {
			top := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			stacked[top] = false
			comp = append(comp, top)
			if top == id {
				break
			}
		}
		out = append(out, comp)
	}
	for _, id := range ids {
		if _, been := index[id]; !been {
			visit(id)
		}
	}
	return out
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

// Rates per second at one scenario: an id that matches no burst gives the flat rate,
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
