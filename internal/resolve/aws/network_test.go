package aws

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

func props(t *testing.T, v any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal properties: %v", err)
	}
	return raw
}

func newProject(t *testing.T, nodes []ir.Node, edges []ir.Edge) *ir.Project {
	t.Helper()
	p, err := ir.ApplyDefaults(&ir.Project{
		Version:     1,
		Name:        "shop",
		Provider:    ir.ProviderAWS,
		Region:      "eu-west-2",
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

func TestNetworkCreatesTwoPublicAndTwoPrivateSubnets(t *testing.T) {
	ctx, _ := newContext(t, nil, nil)
	net := ensureNetwork(ctx)

	if got := countOfType(ctx, "aws_subnet"); got != 4 {
		t.Errorf("subnets = %d", got)
	}
	types := resourceTypes(ctx)
	for _, want := range []string{"aws_nat_gateway", "aws_internet_gateway"} {
		found := false
		for _, got := range types {
			if got == want {
				found = true
			}
		}
		if !found {
			t.Errorf("missing %s in %v", want, types)
		}
	}
	wantPublic := []ir.ID{{Type: "aws_subnet", Name: "public_a"}, {Type: "aws_subnet", Name: "public_b"}}
	if diff := cmp.Diff(wantPublic, net.PublicSubnets); diff != "" {
		t.Errorf("public subnets (-want +got):\n%s", diff)
	}
	wantPrivate := []ir.ID{{Type: "aws_subnet", Name: "private_a"}, {Type: "aws_subnet", Name: "private_b"}}
	if diff := cmp.Diff(wantPrivate, net.PrivateSubnets); diff != "" {
		t.Errorf("private subnets (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(ir.ID{Type: "aws_vpc", Name: "main"}, net.VPC); diff != "" {
		t.Errorf("vpc (-want +got):\n%s", diff)
	}
}

func TestNetworkTakesAvailabilityZonesFromADataSource(t *testing.T) {
	ctx, _ := newContext(t, nil, nil)
	ensureNetwork(ctx)

	wantData := []ir.DataSource{{
		Type:        "aws_availability_zones",
		Name:        "available",
		Args:        ir.Attrs{ir.A("state", ir.Str("available"))},
		SourceLabel: "network",
	}}
	if diff := cmp.Diff(wantData, ctx.DataSources()); diff != "" {
		t.Errorf("data sources (-want +got):\n%s", diff)
	}

	privateA, ok := ctx.Resource(ir.ID{Type: "aws_subnet", Name: "private_a"})
	if !ok {
		t.Fatal("no private_a subnet")
	}
	want := ir.Attrs{
		ir.A("vpc_id", ir.R(ir.ID{Type: "aws_vpc", Name: "main"}, ir.Field("id"))),
		ir.A("cidr_block", ir.Str("10.0.10.0/24")),
		ir.A("availability_zone", ir.D(ir.ID{Type: "aws_availability_zones", Name: "available"}, ir.Field("names"), ir.Index(0))),
		ir.A("tags", ir.M(ir.A("Name", ir.Str("shop-dev-private-a")))),
	}
	if diff := cmp.Diff(want, privateA.Args); diff != "" {
		t.Errorf("private_a args (-want +got):\n%s", diff)
	}
	if privateA.SourceLabel != "network" || privateA.SourceNode != "" {
		t.Errorf("source = %q/%q", privateA.SourceNode, privateA.SourceLabel)
	}
}

func TestNetworkNatDependsOnTheInternetGateway(t *testing.T) {
	ctx, _ := newContext(t, nil, nil)
	ensureNetwork(ctx)
	nat := firstOfType(t, ctx, "aws_nat_gateway")
	want := []ir.ID{{Type: "aws_internet_gateway", Name: "main"}}
	if diff := cmp.Diff(want, nat.DependsOn); diff != "" {
		t.Errorf("depends_on (-want +got):\n%s", diff)
	}
}

func TestNetworkIsCreatedOncePerContext(t *testing.T) {
	ctx, _ := newContext(t, nil, nil)
	a := ensureNetwork(ctx)
	b := ensureNetwork(ctx)
	if a != b {
		t.Error("ensureNetwork built a second network")
	}
	if got := countOfType(ctx, "aws_vpc"); got != 1 {
		t.Errorf("vpcs = %d", got)
	}
}

func TestProviderBlock(t *testing.T) {
	p := newProject(t, nil, nil)
	want := ir.Provider{
		Name:    "aws",
		Source:  "hashicorp/aws",
		Version: "~> 6.0",
		Config:  ir.Attrs{ir.A("region", ir.Str("eu-west-2"))},
	}
	if diff := cmp.Diff(want, New().ProviderBlock(p)); diff != "" {
		t.Errorf("provider block (-want +got):\n%s", diff)
	}
	wantLimits := []resolve.NameLimit{
		{Type: "aws_lambda_function", Arg: "function_name", Max: 64},
		{Type: "aws_db_instance", Arg: "identifier", Max: 63},
		{Type: "aws_lb", Arg: "name", Max: 32},
		{Type: "aws_lb_target_group", Arg: "name", Max: 32},
		{Type: "aws_sqs_queue", Arg: "name", Max: 80},
		{Type: "aws_elasticache_replication_group", Arg: "replication_group_id", Max: 40},
		{Type: "aws_s3_bucket", Arg: "bucket_prefix", Max: 37},
	}
	if diff := cmp.Diff(wantLimits, New().NameLimits()); diff != "" {
		t.Errorf("name limits (-want +got):\n%s", diff)
	}
}
