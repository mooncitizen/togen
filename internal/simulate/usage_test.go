package simulate

import "testing"

func TestUsageLandsInTheRightFields(t *testing.T) {
	p := project()
	result := Result{Nodes: map[string]float64{"gateway-1": 1000, "service-1": 1000, "database-1": 3000}}
	got := Usage(p, sound(), map[string]float64{"mobile": 1000}, result)
	if got["edge"].Requests != "1000/month" {
		t.Fatalf("gateway requests: %+v", got["edge"])
	}
	if got["orders"].Requests != "1000/month" {
		t.Fatalf("service requests: %+v", got["orders"])
	}
	if _, priced := got["orders-db"]; priced {
		t.Fatal("a database has no traffic meter, so it should not appear")
	}
}

func TestUsageDerivesEgressFromBytes(t *testing.T) {
	sim := sound()
	sim.Sources[0].BytesPerRequest = 1e6
	result := Result{Nodes: map[string]float64{"gateway-1": 1000}}
	got := Usage(project(), sim, map[string]float64{"mobile": 1000}, result)
	near(t, got["edge"].EgressGb, 1e-3*1000, "egress")
	near(t, got["network"].NatGb, 1e-3*1000, "nat")
}
