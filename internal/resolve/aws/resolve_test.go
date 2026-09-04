package aws

import (
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

func example() *ir.Project {
	return &ir.Project{
		Version:     1,
		Name:        "shop",
		Provider:    ir.ProviderAWS,
		Region:      "eu-west-2",
		Environment: "dev",
		Nodes: []ir.Node{
			{ID: "n1", Type: ir.NodeGateway, Name: "api"},
			{ID: "n2", Type: ir.NodeFunction, Name: "handler"},
			{ID: "n3", Type: ir.NodeDatabase, Name: "main-db"},
		},
		Edges: []ir.Edge{
			{ID: "e1", From: "n1", To: "n2", Relation: ir.RelRoutes},
			{ID: "e2", From: "n2", To: "n3", Relation: ir.RelReads},
		},
	}
}

func runErrors(t *testing.T, p *ir.Project) ir.Errors {
	t.Helper()
	_, err := resolve.Run(p, New())
	if err == nil {
		t.Fatal("want a ResolveError, got nil")
	}
	re, ok := err.(*resolve.ResolveError)
	if !ok {
		t.Fatalf("want a *resolve.ResolveError, got %T: %v", err, err)
	}
	return re.Errors
}

func TestResolveProducesAValidGraphForTheExampleProject(t *testing.T) {
	g, err := resolve.Run(example(), New())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if errs := ir.ValidateGraph(g); len(errs) > 0 {
		t.Errorf("graph is not valid:\n%s", errs.Error())
	}
	want := ir.Provider{
		Name:    "aws",
		Source:  "hashicorp/aws",
		Version: "~> 6.0",
		Config:  ir.Attrs{ir.A("region", ir.Str("eu-west-2"))},
	}
	if diff := cmp.Diff(want, g.Provider); diff != "" {
		t.Errorf("provider (-want +got):\n%s", diff)
	}

	var types []string
	for _, r := range g.Resources {
		types = append(types, r.Type)
	}
	for _, want := range []string{
		"aws_vpc",
		"aws_apigatewayv2_api",
		"aws_lambda_function",
		"aws_db_instance",
		"aws_vpc_security_group_ingress_rule",
		"aws_iam_role_policy",
	} {
		if !slices.Contains(types, want) {
			t.Errorf("missing %s", want)
		}
	}

	i := slices.IndexFunc(g.Resources, func(r ir.Resource) bool { return r.Type == "aws_lambda_function" })
	fn := g.Resources[i]
	if _, ok := fn.Args.Get("vpc_config"); !ok {
		t.Error("the lambda has no vpc_config")
	}
	if _, ok := fn.Args.Get("environment"); !ok {
		t.Error("the lambda has no environment")
	}

	var outputs []string
	for _, o := range g.Outputs {
		outputs = append(outputs, o.Name)
	}
	slices.Sort(outputs)
	if diff := cmp.Diff([]string{"api_url", "main_db_endpoint"}, outputs); diff != "" {
		t.Errorf("outputs (-want +got):\n%s", diff)
	}
	var variables []string
	for _, v := range g.Variables {
		variables = append(variables, v.Name)
	}
	if diff := cmp.Diff([]string{"handler_package"}, variables); diff != "" {
		t.Errorf("variables (-want +got):\n%s", diff)
	}
	if len(g.Data) != 1 || g.Data[0].Type != "aws_availability_zones" {
		t.Errorf("data sources = %+v", g.Data)
	}
}

func TestResolveDoesNotCreateANetworkWhenNothingNeedsOne(t *testing.T) {
	p := example()
	p.Nodes = p.Nodes[:2]
	p.Edges = p.Edges[:1]
	g, err := resolve.Run(p, New())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, r := range g.Resources {
		if r.Type == "aws_vpc" {
			t.Fatal("a vpc was created")
		}
	}
	if len(g.Data) != 0 {
		t.Errorf("data sources = %+v", g.Data)
	}
}

func TestResolveRejectsOtherProviders(t *testing.T) {
	p := example()
	p.Provider = ir.ProviderGCP
	errs := runErrors(t, p)
	want := ir.Errors{{Path: "provider", Message: "the aws resolver cannot resolve a 'gcp' project"}}
	if diff := cmp.Diff(want, errs); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
}

func TestResolveRejectsNodeTypesThisMilestoneDoesNotSupport(t *testing.T) {
	p := example()
	p.Nodes = append(p.Nodes, ir.Node{ID: "n9", Type: ir.NodeQueue, Name: "jobs"})
	errs := runErrors(t, p)
	if len(errs) != 1 || !strings.Contains(errs[0].Message, "queue") {
		t.Errorf("errors = %v", errs)
	}
}

func TestResolveReportsEveryUnsupportedNodeAndEdgeInFileOrder(t *testing.T) {
	p := example()
	p.Nodes = append(p.Nodes,
		ir.Node{ID: "n9", Type: ir.NodeQueue, Name: "jobs"},
		ir.Node{ID: "n10", Type: ir.NodeFunction, Name: "worker"},
		ir.Node{ID: "n11", Type: ir.NodeBucket, Name: "uploads"},
	)
	p.Edges = append(p.Edges, ir.Edge{ID: "e3", From: "n2", To: "n10", Relation: ir.RelCalls})
	errs := runErrors(t, p)
	if len(errs) != 3 {
		t.Fatalf("errors = %v", errs)
	}
	want := [][2]string{{"n9", ""}, {"n11", ""}, {"", "e3"}}
	for i, w := range want {
		if errs[i].NodeID != w[0] || errs[i].EdgeID != w[1] {
			t.Errorf("errors[%d] = %+v, want node %q edge %q", i, errs[i], w[0], w[1])
		}
	}
	for i, fragment := range []string{"queue", "bucket", "'calls' edges"} {
		if !strings.Contains(errs[i].Message, fragment) {
			t.Errorf("errors[%d].Message = %q, want %q in it", i, errs[i].Message, fragment)
		}
	}
}

func TestResolveReportsTheUnsupportedNodeOnceWhenAnEdgePointsAtIt(t *testing.T) {
	p := example()
	p.Nodes = append(p.Nodes, ir.Node{ID: "n9", Type: ir.NodeQueue, Name: "jobs"})
	p.Edges = append(p.Edges, ir.Edge{ID: "e3", From: "n2", To: "n9", Relation: ir.RelPublishes})
	errs := runErrors(t, p)
	if len(errs) != 1 || errs[0].NodeID != "n9" {
		t.Errorf("errors = %v", errs)
	}
}

func TestResolveRejectsNamesThatExceedAWSLimits(t *testing.T) {
	p := example()
	p.Name = "a-project-name-that-is-long-too"
	p.Environment = "production-like"
	p.Nodes = []ir.Node{{ID: "n2", Type: ir.NodeFunction, Name: "a-very-long-function-name-that-g"}}
	p.Edges = nil
	errs := runErrors(t, p)
	if len(errs) != 1 {
		t.Fatalf("errors = %v", errs)
	}
	if errs[0].NodeID != "n2" || !strings.Contains(errs[0].Message, "64") {
		t.Errorf("error = %+v", errs[0])
	}
}
