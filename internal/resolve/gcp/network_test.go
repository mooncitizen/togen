package gcp

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

func newProject(t *testing.T, nodes []ir.Node, edges []ir.Edge) *ir.Project {
	t.Helper()
	p, err := ir.ApplyDefaults(&ir.Project{
		Version:     1,
		Name:        "shop",
		Provider:    ir.ProviderGCP,
		Region:      "europe-west2",
		Environment: "dev",
		Nodes:       nodes,
		Edges:       edges,
	})
	if err != nil {
		t.Fatalf("apply defaults: %v", err)
	}
	return p
}

func newContext(t *testing.T, nodes []ir.Node, edges []ir.Edge) (*resolve.Context, *ir.Project) {
	t.Helper()
	p := newProject(t, nodes, edges)
	return resolve.NewContext(p), p
}

func resourceTypes(ctx *resolve.Context) []string {
	rs := ctx.Resources()
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.Type
	}
	return out
}

func byType(ctx *resolve.Context, typ string) []ir.Resource {
	var out []ir.Resource
	for _, r := range ctx.Resources() {
		if r.Type == typ {
			out = append(out, r)
		}
	}
	return out
}

func firstOfType(t *testing.T, ctx *resolve.Context, typ string) ir.Resource {
	t.Helper()
	rs := byType(ctx, typ)
	if len(rs) == 0 {
		t.Fatalf("no %s resource", typ)
	}
	return rs[0]
}

func countOfType(ctx *resolve.Context, typ string) int { return len(byType(ctx, typ)) }

func TestNetworkCreatesTheVPCSubnetConnectorAndPeering(t *testing.T) {
	ctx, _ := newContext(t, nil, nil)
	net := ensureNetwork(ctx)

	wantTypes := []string{
		"google_compute_network",
		"google_compute_subnetwork",
		"google_vpc_access_connector",
		"google_compute_global_address",
		"google_service_networking_connection",
	}
	if diff := cmp.Diff(wantTypes, resourceTypes(ctx)); diff != "" {
		t.Errorf("resources (-want +got):\n%s", diff)
	}
	if len(ctx.DataSources()) != 0 {
		t.Errorf("data sources = %+v", ctx.DataSources())
	}
	want := &Network{
		VPC:        ir.ID{Type: "google_compute_network", Name: "main"},
		Subnet:     ir.ID{Type: "google_compute_subnetwork", Name: "main"},
		Connector:  ir.ID{Type: "google_vpc_access_connector", Name: "main"},
		Connection: ir.ID{Type: "google_service_networking_connection", Name: "main"},
	}
	if diff := cmp.Diff(want, net); diff != "" {
		t.Errorf("network (-want +got):\n%s", diff)
	}
	for _, r := range ctx.Resources() {
		if r.SourceLabel != "network" || r.SourceNode != "" {
			t.Errorf("%s source = %q/%q", r.Type, r.SourceNode, r.SourceLabel)
		}
	}
}

func TestNetworkHasNoAutomaticSubnetsAndOneInTheProjectRegion(t *testing.T) {
	ctx, _ := newContext(t, nil, nil)
	ensureNetwork(ctx)

	vpc := firstOfType(t, ctx, "google_compute_network")
	wantVPC := ir.Attrs{
		ir.A("name", ir.Str("shop-dev-network")),
		ir.A("auto_create_subnetworks", ir.Bool(false)),
	}
	if diff := cmp.Diff(wantVPC, vpc.Args); diff != "" {
		t.Errorf("network args (-want +got):\n%s", diff)
	}

	subnet := firstOfType(t, ctx, "google_compute_subnetwork")
	wantSubnet := ir.Attrs{
		ir.A("name", ir.Str("shop-dev-subnet")),
		ir.A("network", ir.R(ir.ID{Type: "google_compute_network", Name: "main"}, ir.Field("id"))),
		ir.A("region", ir.Str("europe-west2")),
		ir.A("ip_cidr_range", ir.Str("10.0.0.0/24")),
	}
	if diff := cmp.Diff(wantSubnet, subnet.Args); diff != "" {
		t.Errorf("subnet args (-want +got):\n%s", diff)
	}
}

func TestNetworkConnectorHasItsOwnRangeAndScalesBetweenTwoAndThree(t *testing.T) {
	ctx, _ := newContext(t, nil, nil)
	ensureNetwork(ctx)

	connector := firstOfType(t, ctx, "google_vpc_access_connector")
	want := ir.Attrs{
		ir.A("name", ir.Str("shop-dev-connector")),
		ir.A("network", ir.R(ir.ID{Type: "google_compute_network", Name: "main"}, ir.Field("name"))),
		ir.A("region", ir.Str("europe-west2")),
		ir.A("ip_cidr_range", ir.Str("10.8.0.0/28")),
		ir.A("min_instances", ir.Num(2)),
		ir.A("max_instances", ir.Num(3)),
	}
	if diff := cmp.Diff(want, connector.Args); diff != "" {
		t.Errorf("connector args (-want +got):\n%s", diff)
	}
}

func TestNetworkConnectorNameStaysWithinTheLimitForALongProject(t *testing.T) {
	p := newProject(t, nil, nil)
	p.Name = "a-project-name-that-is-long-too"
	p.Environment = "production-like"
	ctx := resolve.NewContext(p)
	ensureNetwork(ctx)

	connector := firstOfType(t, ctx, "google_vpc_access_connector")
	name, _ := connector.Args.Get("name")
	if got := string(name.(ir.String)); got != "a-project-name-connector" || len(got) > 25 {
		t.Errorf("connector name = %q (%d characters)", got, len(got))
	}
}

func TestNetworkReservesAPeeringRangeForServiceNetworking(t *testing.T) {
	ctx, _ := newContext(t, nil, nil)
	ensureNetwork(ctx)

	address := firstOfType(t, ctx, "google_compute_global_address")
	wantAddress := ir.Attrs{
		ir.A("name", ir.Str("shop-dev-peering")),
		ir.A("purpose", ir.Str("VPC_PEERING")),
		ir.A("address_type", ir.Str("INTERNAL")),
		ir.A("prefix_length", ir.Num(16)),
		ir.A("network", ir.R(ir.ID{Type: "google_compute_network", Name: "main"}, ir.Field("id"))),
	}
	if diff := cmp.Diff(wantAddress, address.Args); diff != "" {
		t.Errorf("address args (-want +got):\n%s", diff)
	}

	connection := firstOfType(t, ctx, "google_service_networking_connection")
	wantConnection := ir.Attrs{
		ir.A("network", ir.R(ir.ID{Type: "google_compute_network", Name: "main"}, ir.Field("id"))),
		ir.A("service", ir.Str("servicenetworking.googleapis.com")),
		ir.A("reserved_peering_ranges", ir.L(ir.R(ir.ID{Type: "google_compute_global_address", Name: "peering"}, ir.Field("name")))),
	}
	if diff := cmp.Diff(wantConnection, connection.Args); diff != "" {
		t.Errorf("connection args (-want +got):\n%s", diff)
	}
}

func TestNetworkIsCreatedOncePerContext(t *testing.T) {
	ctx, _ := newContext(t, nil, nil)
	a := ensureNetwork(ctx)
	b := ensureNetwork(ctx)
	if a != b {
		t.Error("ensureNetwork built a second network")
	}
	if got := countOfType(ctx, "google_compute_network"); got != 1 {
		t.Errorf("networks = %d", got)
	}
	if got := countOfType(ctx, "google_service_networking_connection"); got != 1 {
		t.Errorf("connections = %d", got)
	}
}

func TestProviderBlock(t *testing.T) {
	p := newProject(t, nil, nil)
	want := ir.Provider{
		Name:    "google",
		Source:  "hashicorp/google",
		Version: "~> 8.0",
		Config: ir.Attrs{
			ir.A("project", ir.V("project")),
			ir.A("region", ir.Str("europe-west2")),
		},
	}
	if diff := cmp.Diff(want, New().ProviderBlock(p)); diff != "" {
		t.Errorf("provider block (-want +got):\n%s", diff)
	}
	wantVariables := []ir.Variable{{
		Name:        "project",
		Description: "The id of the Google Cloud project to deploy into",
		Type:        "string",
	}}
	if diff := cmp.Diff(wantVariables, New().(resolve.VariableProvider).Variables(p)); diff != "" {
		t.Errorf("variables (-want +got):\n%s", diff)
	}
	wantLimits := []resolve.NameLimit{
		{Type: "google_cloud_run_v2_service", Arg: "name", Max: 63},
		{Type: "google_cloudfunctions2_function", Arg: "name", Max: 63},
		{Type: "google_service_account", Arg: "account_id", Max: 30},
		{Type: "google_vpc_access_connector", Arg: "name", Max: 25},
	}
	if diff := cmp.Diff(wantLimits, New().NameLimits()); diff != "" {
		t.Errorf("name limits (-want +got):\n%s", diff)
	}
}
