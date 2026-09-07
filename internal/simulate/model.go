package simulate

import (
	"fmt"
	"maps"
	"math"
	"regexp"
	"slices"

	"github.com/mooncitizen/togen/internal/cost"
	"github.com/mooncitizen/togen/internal/ir"
)

const Version = 1

var kebab = regexp.MustCompile(ir.KebabPattern)

type Simulation struct {
	Version int                `json:"version"`
	Sources []Source           `json:"sources"`
	Bursts  []Burst            `json:"bursts,omitempty"`
	Edges   map[string]float64 `json:"edges,omitempty"`
}

type Source struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Target          string    `json:"target"`
	Rate            cost.Rate `json:"rate"`
	BytesPerRequest float64   `json:"bytesPerRequest,omitempty"`
}

type Burst struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Source        string  `json:"source"`
	Multiplier    float64 `json:"multiplier"`
	Minutes       float64 `json:"minutes"`
	TimesPerMonth float64 `json:"timesPerMonth"`
}

func Empty() Simulation { return Simulation{Version: Version} }

func (s Simulation) IsEmpty() bool { return len(s.Sources) == 0 }

// Where a source may enter the system: the node types that receive work.
var entryTypes = []ir.NodeType{ir.NodeGateway, ir.NodeService, ir.NodeFunction}

func Validate(sim Simulation, project *ir.Project) ir.Errors {
	nodes := make(map[string]ir.NodeType, len(project.Nodes))
	for _, n := range project.Nodes {
		nodes[n.ID] = n.Type
	}
	edges := make(map[string]bool, len(project.Edges))
	for _, e := range project.Edges {
		edges[e.ID] = true
	}

	var errs ir.Errors
	seen := map[string]bool{}
	for i, s := range sim.Sources {
		at := fmt.Sprintf("sources.%d", i)
		if !kebab.MatchString(s.ID) || seen[s.ID] {
			errs = append(errs, ir.ValidationError{Path: at + ".id", Message: fmt.Sprintf("source id '%s' must be kebab-case and used once", s.ID)})
		}
		seen[s.ID] = true
		kind, known := nodes[s.Target]
		switch {
		case !known:
			errs = append(errs, ir.ValidationError{Path: at + ".target", Message: fmt.Sprintf("there is no node '%s'", s.Target)})
		case !contains(entryTypes, kind):
			errs = append(errs, ir.ValidationError{Path: at + ".target", Message: fmt.Sprintf("a source enters at a gateway, service or function, not a %s", kind)})
		}
		if _, err := s.Rate.PerMonth(); err != nil {
			errs = append(errs, ir.ValidationError{Path: at + ".rate", Message: err.Error()})
		}
		if s.BytesPerRequest < 0 {
			errs = append(errs, ir.ValidationError{Path: at + ".bytesPerRequest", Message: "bytes per request cannot be negative"})
		}
	}

	burstIDs := map[string]bool{}
	for i, b := range sim.Bursts {
		at := fmt.Sprintf("bursts.%d", i)
		if !kebab.MatchString(b.ID) || burstIDs[b.ID] {
			errs = append(errs, ir.ValidationError{Path: at + ".id", Message: fmt.Sprintf("burst id '%s' must be kebab-case and used once", b.ID)})
		}
		burstIDs[b.ID] = true
		if !seen[b.Source] {
			errs = append(errs, ir.ValidationError{Path: at + ".source", Message: fmt.Sprintf("there is no source '%s'", b.Source)})
		}
		if b.Multiplier < 1 {
			errs = append(errs, ir.ValidationError{Path: at + ".multiplier", Message: "a burst multiplies by at least 1"})
		}
		if b.Minutes <= 0 {
			errs = append(errs, ir.ValidationError{Path: at + ".minutes", Message: "a burst lasts more than no time"})
		}
		if b.TimesPerMonth <= 0 {
			errs = append(errs, ir.ValidationError{Path: at + ".timesPerMonth", Message: "a burst happens at least once a month"})
		}
	}

	for _, id := range sortedKeys(sim.Edges) {
		if !edges[id] {
			errs = append(errs, ir.ValidationError{Path: "edges." + id, Message: fmt.Sprintf("there is no edge '%s'", id)})
			continue
		}
		if per := sim.Edges[id]; per < 0 || math.IsInf(per, 0) || math.IsNaN(per) {
			errs = append(errs, ir.ValidationError{Path: "edges." + id, Message: "a fan-out is a number of calls per request, zero or more"})
		}
	}
	return errs
}

func contains(types []ir.NodeType, want ir.NodeType) bool { return slices.Contains(types, want) }

func sortedKeys(m map[string]float64) []string { return slices.Sorted(maps.Keys(m)) }
