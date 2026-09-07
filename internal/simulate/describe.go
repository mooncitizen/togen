package simulate

import (
	"fmt"
	"strings"

	"github.com/mooncitizen/togen/internal/ir"
)

type Document struct {
	Nodes []NodeRate `json:"nodes"`
	Edges []EdgeRate `json:"edges"`
}

type NodeRate struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Type         string  `json:"type"`
	RatePerMonth float64 `json:"ratePerMonth"`
	UsageField   string  `json:"usageField,omitempty"`
}

type EdgeRate struct {
	ID           string  `json:"id"`
	From         string  `json:"from"`
	To           string  `json:"to"`
	Relation     string  `json:"relation"`
	Per          float64 `json:"per"`
	RatePerMonth float64 `json:"ratePerMonth"`
}

func usageField(t ir.NodeType) string {
	switch t {
	case ir.NodeGateway, ir.NodeService, ir.NodeBucket:
		return "requests"
	case ir.NodeFunction:
		return "invocations"
	case ir.NodeQueue:
		return "messages"
	}
	return ""
}

func Describe(project *ir.Project, sim Simulation, monthly map[string]float64, result Result) Document {
	doc := Document{
		Nodes: make([]NodeRate, 0, len(project.Nodes)),
		Edges: make([]EdgeRate, 0, len(project.Edges)),
	}
	for _, n := range project.Nodes {
		doc.Nodes = append(doc.Nodes, NodeRate{
			ID:           n.ID,
			Name:         n.Name,
			Type:         string(n.Type),
			RatePerMonth: result.Nodes[n.ID],
			UsageField:   usageField(n.Type),
		})
	}
	names := make(map[string]string, len(project.Nodes))
	for _, n := range project.Nodes {
		names[n.ID] = n.Name
	}
	for _, e := range project.Edges {
		per := 1.0
		if v, ok := sim.Edges[e.ID]; ok {
			per = v
		}
		doc.Edges = append(doc.Edges, EdgeRate{
			ID:           e.ID,
			From:         names[e.From],
			To:           names[e.To],
			Relation:     string(e.Relation),
			Per:          per,
			RatePerMonth: result.Edges[e.ID],
		})
	}
	return doc
}

func (d Document) Table() []string {
	var nameW, kindW int
	for _, n := range d.Nodes {
		nameW = max(nameW, len(n.Name))
		kindW = max(kindW, len(n.Type))
	}
	out := []string{"nodes"}
	for _, n := range d.Nodes {
		out = append(out, strings.TrimRight(fmt.Sprintf("  %-*s  %-*s  %14s /month  %s",
			nameW, n.Name, kindW, n.Type, count(n.RatePerMonth), n.UsageField), " "))
	}
	out = append(out, "", "edges")
	for _, e := range d.Edges {
		out = append(out, fmt.Sprintf("  %s %s %s  x%s  %14s /month",
			e.From, e.Relation, e.To, trim(e.Per), count(e.RatePerMonth)))
	}
	return out
}

func count(v float64) string { return fmt.Sprintf("%.0f", v) }

func trim(v float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", v), "0"), ".")
}
