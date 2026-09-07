package simulate

import (
	"fmt"

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

func Run(project *ir.Project, sim Simulation, injection map[string]float64) (Result, error) {
	as := arcs(project, sim)
	incoming := map[string][]arc{}
	outgoing := map[string][]arc{}
	for _, a := range as {
		incoming[a.to] = append(incoming[a.to], a)
		outgoing[a.from] = append(outgoing[a.from], a)
	}
	order, err := ordered(project, incoming, outgoing)
	if err != nil {
		return Result{}, err
	}

	entry := map[string]float64{}
	for _, s := range sim.Sources {
		entry[s.Target] += injection[s.ID]
	}

	nodes := make(map[string]float64, len(project.Nodes))
	edges := make(map[string]float64, len(as))
	for _, id := range order {
		rate := entry[id]
		for _, a := range incoming[id] {
			rate += nodes[a.from] * a.per
		}
		nodes[id] = rate
		for _, a := range outgoing[id] {
			edges[a.edge] = rate * a.per
		}
	}
	return Result{Nodes: nodes, Edges: edges}, nil
}

func ordered(project *ir.Project, incoming, outgoing map[string][]arc) ([]string, error) {
	left := make(map[string]int, len(project.Nodes))
	var queue []string
	for _, n := range project.Nodes {
		left[n.ID] = len(incoming[n.ID])
		if left[n.ID] == 0 {
			queue = append(queue, n.ID)
		}
	}
	out := make([]string, 0, len(project.Nodes))
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		out = append(out, id)
		for _, a := range outgoing[id] {
			left[a.to]--
			if left[a.to] == 0 {
				queue = append(queue, a.to)
			}
		}
	}
	if len(out) < len(project.Nodes) {
		for _, n := range project.Nodes {
			if left[n.ID] > 0 {
				return nil, fmt.Errorf("the edges into '%s' go round in a circle, so there is no traffic to work out", n.ID)
			}
		}
	}
	return out, nil
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
