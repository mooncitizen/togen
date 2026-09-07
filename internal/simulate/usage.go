package simulate

import (
	"fmt"
	"math"

	"github.com/mooncitizen/togen/internal/cost"
	"github.com/mooncitizen/togen/internal/ir"
)

func perMonth(count float64) cost.Rate {
	return cost.Rate(fmt.Sprintf("%d/month", int64(math.Round(count))))
}

// A database or a cache has no traffic meter in the price snapshot, so its rate is for
// the canvas only and it gets no usage entry.
func Usage(project *ir.Project, sim Simulation, monthly map[string]float64, result Result) cost.Usage {
	usage := cost.Usage{}
	for _, n := range project.Nodes {
		rate := result.Nodes[n.ID]
		if rate == 0 {
			continue
		}
		entry := usage[n.Name]
		switch n.Type {
		case ir.NodeGateway, ir.NodeService, ir.NodeBucket:
			entry.Requests = perMonth(rate)
		case ir.NodeFunction:
			entry.Invocations = perMonth(rate)
		case ir.NodeQueue:
			entry.Messages = perMonth(rate)
		default:
			continue
		}
		usage[n.Name] = entry
	}

	names := make(map[string]string, len(project.Nodes))
	for _, n := range project.Nodes {
		names[n.ID] = n.Name
	}
	network := usage[cost.NetworkUsage]
	var nat float64
	for _, s := range sim.Sources {
		if s.BytesPerRequest == 0 {
			continue
		}
		gb := monthly[s.ID] * s.BytesPerRequest / 1e9
		name, ok := names[s.Target]
		if !ok {
			continue
		}
		entry := usage[name]
		entry.EgressGb += gb
		usage[name] = entry
		nat += gb
	}
	if nat > 0 {
		network.NatGb += nat
		usage[cost.NetworkUsage] = network
	}
	return usage
}
