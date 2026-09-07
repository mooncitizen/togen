package simulate

import "testing"

func TestDescribeNamesTheUsageField(t *testing.T) {
	result := Result{
		Nodes: map[string]float64{"gateway-1": 1000, "service-1": 1000, "database-1": 3000},
		Edges: map[string]float64{"edge-1": 1000, "edge-2": 3000},
	}
	doc := Describe(project(), sound(), map[string]float64{"mobile": 1000}, result)
	if len(doc.Nodes) != 3 || doc.Nodes[0].ID != "gateway-1" {
		t.Fatalf("nodes in project order: %+v", doc.Nodes)
	}
	if doc.Nodes[0].UsageField != "requests" {
		t.Fatalf("gateway field: %+v", doc.Nodes[0])
	}
	if doc.Nodes[2].UsageField != "" {
		t.Fatalf("a database has no usage field: %+v", doc.Nodes[2])
	}
	if doc.Edges[1].Per != 3 || doc.Edges[1].RatePerMonth != 3000 {
		t.Fatalf("edge-2: %+v", doc.Edges[1])
	}
	if len(doc.Table()) == 0 {
		t.Fatal("the table should have lines")
	}
}
