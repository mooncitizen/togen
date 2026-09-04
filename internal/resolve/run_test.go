package resolve

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"togen/internal/ir"
)

type fakeProvider struct {
	edges     []string
	failOnAdd bool
}

func (f *fakeProvider) Name() ir.CloudProvider { return ir.ProviderAWS }

func (f *fakeProvider) ProviderBlock(p *ir.Project) ir.Provider {
	return ir.Provider{
		Name:    "fake",
		Source:  "togen/fake",
		Version: "~> 1.0",
		Config:  ir.Attrs{ir.A("region", ir.Str(p.Region))},
	}
}

func (f *fakeProvider) NameLimits() []NameLimit {
	return []NameLimit{{Type: "fake_api", Arg: "name", Max: 12}}
}

func (f *fakeProvider) ResolveNode(ctx *Context, n ir.Node) (*Handle, bool) {
	if f.failOnAdd {
		ctx.Fail("the fake provider gave up")
		return nil, false
	}
	if n.Type != ir.NodeGateway {
		ctx.Report(ir.ValidationError{NodeID: n.ID, Message: "unsupported"})
		return nil, false
	}
	local := ctx.Local(n.Name)
	r := ctx.Add(ir.Resource{
		Type:        "fake_api",
		Name:        local,
		Args:        ir.Attrs{ir.A("name", ir.Str(ctx.Named(n.Name)))},
		SourceNode:  n.ID,
		SourceLabel: n.Name,
	})
	id := ir.ID{Type: r.Type, Name: r.Name}
	h := &Handle{
		Node:    n,
		Primary: id,
		Exports: GatewayExports{APIID: ir.R(id, ir.Field("id"))},
	}
	h.Finalise = func() {
		ctx.AddOutput(ir.Output{Name: local + "_url", Value: ir.R(id, ir.Field("url"))})
	}
	return h, true
}

func (f *fakeProvider) ResolveEdge(ctx *Context, e ir.Edge, from, to *Handle) {
	f.edges = append(f.edges, e.ID)
}

func project(nodes []ir.Node, edges []ir.Edge) *ir.Project {
	return &ir.Project{
		Version:     1,
		Name:        "shop",
		Provider:    ir.ProviderAWS,
		Region:      "eu-west-2",
		Environment: "dev",
		Nodes:       nodes,
		Edges:       edges,
	}
}

func resolveErrors(t *testing.T, err error) ir.Errors {
	t.Helper()
	if err == nil {
		t.Fatal("want a ResolveError, got nil")
	}
	var re *ResolveError
	if !errors.As(err, &re) {
		t.Fatalf("want a *ResolveError, got %T: %v", err, err)
	}
	return re.Errors
}

func TestRunBuildsAGraph(t *testing.T) {
	p := project([]ir.Node{{ID: "n1", Type: ir.NodeGateway, Name: "api"}}, nil)
	g, err := Run(p, &fakeProvider{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if g.TerraformVersion != ">= 1.5" {
		t.Errorf("terraform version = %q", g.TerraformVersion)
	}
	want := ir.Provider{
		Name:    "fake",
		Source:  "togen/fake",
		Version: "~> 1.0",
		Config:  ir.Attrs{ir.A("region", ir.Str("eu-west-2"))},
	}
	if diff := cmp.Diff(want, g.Provider); diff != "" {
		t.Errorf("provider block (-want +got):\n%s", diff)
	}
	if len(g.Resources) != 1 || g.Resources[0].Name != "api" {
		t.Fatalf("resources = %+v", g.Resources)
	}
	if len(g.Outputs) != 1 || g.Outputs[0].Name != "api_url" {
		t.Errorf("finalise did not run: %+v", g.Outputs)
	}
}

func TestRunReportsUnsupportedNodeAndSkipsItsEdges(t *testing.T) {
	p := project(
		[]ir.Node{
			{ID: "n1", Type: ir.NodeGateway, Name: "api"},
			{ID: "n2", Type: ir.NodeFunction, Name: "handler"},
		},
		[]ir.Edge{{ID: "e1", From: "n1", To: "n2", Relation: ir.RelRoutes}},
	)
	prov := &fakeProvider{}
	_, err := Run(p, prov)
	errs := resolveErrors(t, err)
	if diff := cmp.Diff(ir.Errors{{NodeID: "n2", Message: "unsupported"}}, errs); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
	if len(prov.edges) != 0 {
		t.Errorf("edge resolver ran for %v", prov.edges)
	}
}

func TestRunResolvesEdgesWithBothHandles(t *testing.T) {
	p := project(
		[]ir.Node{
			{ID: "n1", Type: ir.NodeGateway, Name: "api"},
			{ID: "n2", Type: ir.NodeGateway, Name: "adm"},
		},
		[]ir.Edge{{ID: "e1", From: "n1", To: "n2", Relation: ir.RelRoutes}},
	)
	prov := &fakeProvider{}
	if _, err := Run(p, prov); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if diff := cmp.Diff([]string{"e1"}, prov.edges); diff != "" {
		t.Errorf("edges (-want +got):\n%s", diff)
	}
}

func TestRunRejectsAnotherProvider(t *testing.T) {
	p := project([]ir.Node{{ID: "n1", Type: ir.NodeGateway, Name: "api"}}, nil)
	p.Provider = ir.ProviderGCP
	_, err := Run(p, &fakeProvider{})
	errs := resolveErrors(t, err)
	want := ir.Errors{{Path: "provider", Message: "the aws resolver cannot resolve a 'gcp' project"}}
	if diff := cmp.Diff(want, errs); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
}

func TestRunChecksNameLimits(t *testing.T) {
	p := project([]ir.Node{{ID: "n1", Type: ir.NodeGateway, Name: "api-gateway"}}, nil)
	_, err := Run(p, &fakeProvider{})
	errs := resolveErrors(t, err)
	want := ir.Errors{{
		NodeID:  "n1",
		Message: "fake_api name 'shop-dev-api-gateway' is 20 characters, the limit is 12. Shorten the project, environment or node name.",
	}}
	if diff := cmp.Diff(want, errs); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
}

func TestRunTurnsFailIntoAResolveError(t *testing.T) {
	p := project([]ir.Node{{ID: "n1", Type: ir.NodeGateway, Name: "api"}}, nil)
	_, err := Run(p, &fakeProvider{failOnAdd: true})
	errs := resolveErrors(t, err)
	if len(errs) != 1 || !strings.Contains(errs[0].Message, "gave up") {
		t.Errorf("errors = %v", errs)
	}
}

func TestContextAddRefusesDuplicates(t *testing.T) {
	ctx := NewContext(project(nil, nil))
	ctx.Add(ir.Resource{Type: "fake_api", Name: "main"})
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("want a panic on a duplicate resource")
		}
	}()
	ctx.Add(ir.Resource{Type: "fake_api", Name: "main"})
}

func TestContextNaming(t *testing.T) {
	ctx := NewContext(project(nil, nil))
	if ctx.Prefix() != "shop-dev" {
		t.Errorf("Prefix() = %q", ctx.Prefix())
	}
	if ctx.Named("main-db") != "shop-dev-main-db" {
		t.Errorf("Named() = %q", ctx.Named("main-db"))
	}
	if ctx.Local("main-db") != "main_db" {
		t.Errorf("Local() = %q", ctx.Local("main-db"))
	}
}

func TestHandleSetEnvReplacesInPlace(t *testing.T) {
	h := &Handle{}
	h.SetEnv("A", ir.Str("1"))
	h.SetEnv("B", ir.Str("2"))
	h.SetEnv("A", ir.Str("3"))
	want := ir.Attrs{ir.A("A", ir.Str("3")), ir.A("B", ir.Str("2"))}
	if diff := cmp.Diff(want, h.Env); diff != "" {
		t.Errorf("env (-want +got):\n%s", diff)
	}
}

func TestHandleAddStatementDeduplicates(t *testing.T) {
	h := &Handle{}
	s := ir.M(ir.A("Effect", ir.Str("Allow")), ir.A("Action", ir.L(ir.Str("s3:GetObject"))))
	h.AddStatement(s)
	h.AddStatement(ir.M(ir.A("Effect", ir.Str("Allow")), ir.A("Action", ir.L(ir.Str("s3:GetObject")))))
	h.AddStatement(ir.M(ir.A("Effect", ir.Str("Deny"))))
	if len(h.Statements) != 2 {
		t.Errorf("statements = %v", h.Statements)
	}
}
