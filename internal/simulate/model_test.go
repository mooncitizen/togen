package simulate

import (
	"testing"

	"github.com/mooncitizen/togen/internal/ir"
)

func project() *ir.Project {
	return &ir.Project{
		Version:     ir.Version,
		Name:        "shop",
		Provider:    ir.ProviderAWS,
		Region:      "eu-west-2",
		Environment: "dev",
		Nodes: []ir.Node{
			{ID: "gateway-1", Type: ir.NodeGateway, Name: "edge"},
			{ID: "service-1", Type: ir.NodeService, Name: "orders"},
			{ID: "database-1", Type: ir.NodeDatabase, Name: "orders-db"},
		},
		Edges: []ir.Edge{
			{ID: "edge-1", From: "gateway-1", To: "service-1", Relation: ir.RelRoutes},
			{ID: "edge-2", From: "service-1", To: "database-1", Relation: ir.RelWrites},
		},
	}
}

func sound() Simulation {
	return Simulation{
		Version: Version,
		Sources: []Source{{ID: "mobile", Name: "Mobile app", Target: "gateway-1", Rate: "800/min"}},
		Bursts:  []Burst{{ID: "launch", Name: "Launch", Source: "mobile", Multiplier: 6, Minutes: 30, TimesPerMonth: 2}},
		Edges:   map[string]float64{"edge-2": 3},
	}
}

func TestValidateAccepts(t *testing.T) {
	if errs := Validate(sound(), project()); len(errs) > 0 {
		t.Fatalf("want no errors, got %v", errs)
	}
}

func TestValidateRejects(t *testing.T) {
	cases := map[string]func(*Simulation){
		"sources.0.target": func(s *Simulation) { s.Sources[0].Target = "node-9" },
		"sources.0.rate":   func(s *Simulation) { s.Sources[0].Rate = "800 a minute" },
		"sources.1.id":     func(s *Simulation) { s.Sources = append(s.Sources, s.Sources[0]) },
		"bursts.0.source":  func(s *Simulation) { s.Bursts[0].Source = "web" },
		"edges.edge-9":     func(s *Simulation) { s.Edges = map[string]float64{"edge-9": 2} },
	}
	for path, spoil := range cases {
		t.Run(path, func(t *testing.T) {
			sim := sound()
			spoil(&sim)
			errs := Validate(sim, project())
			if len(errs) == 0 {
				t.Fatal("want an error, got none")
			}
			if errs[0].Path != path {
				t.Fatalf("want path %q, got %q", path, errs[0].Path)
			}
		})
	}
}

func TestValidateRejectsTargetType(t *testing.T) {
	sim := sound()
	sim.Sources[0].Target = "database-1"
	errs := Validate(sim, project())
	if len(errs) != 1 || errs[0].Path != "sources.0.target" {
		t.Fatalf("want one target error, got %v", errs)
	}
}
