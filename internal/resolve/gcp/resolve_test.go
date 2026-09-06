package gcp

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/mooncitizen/togen/internal/emit/hcl"
	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

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

func terraformFmt(t *testing.T, files map[string][]byte) {
	t.Helper()
	bin, err := exec.LookPath("terraform")
	if err != nil {
		t.Skip("terraform is not on PATH")
	}
	dir := t.TempDir()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), body, 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	out, err := exec.Command(bin, "fmt", "-check", "-diff", dir).CombinedOutput()
	if err != nil {
		t.Fatalf("terraform fmt reported changes: %v\n%s", err, out)
	}
}

func TestResolveRefusesTheNodeTypesItDoesNotSupportYetAndSkipsTheirEdges(t *testing.T) {
	p := newProject(t, nil, nil)
	for i, typ := range ir.NodeTypes {
		n := ir.Node{ID: fmt.Sprintf("n%d", i+1), Type: typ, Name: string(typ)}
		if typ == ir.NodeService {
			n.Properties = json.RawMessage(`{"image":"nginx:1.27","port":80}`)
		}
		p.Nodes = append(p.Nodes, n)
	}
	p.Edges = []ir.Edge{
		{ID: "e1", From: "n4", To: "n2", Relation: ir.RelRoutes},
		{ID: "e2", From: "n4", To: "n1", Relation: ir.RelRoutes},
		{ID: "e3", From: "n2", To: "n3", Relation: ir.RelReads},
	}

	errs := runErrors(t, p)
	var want ir.Errors
	for _, n := range p.Nodes {
		if n.Type == ir.NodeFunction || n.Type == ir.NodeGateway {
			continue
		}
		want = append(want, ir.ValidationError{
			NodeID:  n.ID,
			Message: "node type '" + string(n.Type) + "' is not supported by the gcp resolver yet",
		})
	}
	if diff := cmp.Diff(want, errs); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
}

func TestResolveProducesAValidGraphForAGatewayAndAFunction(t *testing.T) {
	p := newProject(t, []ir.Node{
		{ID: "n1", Type: ir.NodeGateway, Name: "api"},
		functionNode(t, "n2", "orders", ir.FunctionProps{Env: map[string]string{"LOG_LEVEL": "info"}}),
	}, []ir.Edge{{
		ID: "e1", From: "n1", To: "n2", Relation: ir.RelRoutes,
		Properties: ir.EdgeProperties{Path: "/orders", Methods: []ir.Method{ir.MethodGet, ir.MethodPost}},
	}})
	g, err := resolve.Run(p, New())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if errs := ir.ValidateGraph(g); len(errs) > 0 {
		t.Fatalf("graph is not valid:\n%s", errs.Error())
	}

	var types []string
	for _, r := range g.Resources {
		types = append(types, r.Type)
	}
	wantTypes := []string{
		"google_storage_bucket",
		"google_service_account",
		"google_storage_bucket_object",
		"google_cloudfunctions2_function",
		"google_cloud_run_v2_service_iam_member",
	}
	if diff := cmp.Diff(wantTypes, types); diff != "" {
		t.Errorf("resources (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"project", "orders_package"}, variableNames(g.Variables)); diff != "" {
		t.Errorf("variables (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"api_orders_url"}, outputNames(g.Outputs)); diff != "" {
		t.Errorf("outputs (-want +got):\n%s", diff)
	}

	files, err := hcl.Emit(g)
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	names := slices.Sorted(maps.Keys(files))
	if diff := cmp.Diff([]string{"main.tf", "outputs.tf", "providers.tf", "variables.tf"}, names); diff != "" {
		t.Errorf("files (-want +got):\n%s", diff)
	}
	terraformFmt(t, files)
}

func variableNames(vars []ir.Variable) []string {
	var names []string
	for _, v := range vars {
		names = append(names, v.Name)
	}
	return names
}

func TestResolveReportsAnEdgeItDoesNotSupport(t *testing.T) {
	ctx, _ := newContext(t, nil, nil)
	New().ResolveEdge(ctx, ir.Edge{ID: "e1", Relation: ir.RelCalls}, nil, nil)
	want := ir.Errors{{EdgeID: "e1", Message: "'calls' edges are not supported by the gcp resolver yet"}}
	if diff := cmp.Diff(want, ctx.Errors); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
}

func TestResolveRejectsOtherProviders(t *testing.T) {
	p := newProject(t, nil, nil)
	p.Provider = ir.ProviderAWS
	p.Region = "eu-west-2"
	errs := runErrors(t, p)
	want := ir.Errors{{Path: "provider", Message: "the gcp resolver cannot resolve a 'aws' project"}}
	if diff := cmp.Diff(want, errs); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
}

func TestResolveEmitsOnlyTheProviderAndItsVariableWhenThereAreNoNodes(t *testing.T) {
	p := newProject(t, nil, nil)
	g, err := resolve.Run(p, New())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(g.Resources) != 0 || len(g.Data) != 0 || len(g.Outputs) != 0 {
		t.Errorf("graph has resources %d, data %d, outputs %d", len(g.Resources), len(g.Data), len(g.Outputs))
	}
	if diff := cmp.Diff([]ir.Provider{New().ProviderBlock(p)}, g.Providers); diff != "" {
		t.Errorf("provider (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(New().(resolve.VariableProvider).Variables(p), g.Variables); diff != "" {
		t.Errorf("variables (-want +got):\n%s", diff)
	}

	files, err := hcl.Emit(g)
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	names := slices.Sorted(maps.Keys(files))
	if diff := cmp.Diff([]string{"main.tf", "providers.tf", "variables.tf"}, names); diff != "" {
		t.Errorf("files (-want +got):\n%s", diff)
	}
	terraformFmt(t, files)
}

func TestNetworkAloneEmitsAValidFormattedGraph(t *testing.T) {
	ctx, p := newContext(t, nil, nil)
	ensureNetwork(ctx)
	g := &ir.Graph{
		TerraformVersion: ">= 1.5",
		Providers:        []ir.Provider{New().ProviderBlock(p)},
		Resources:        ctx.Resources(),
		Variables:        New().(resolve.VariableProvider).Variables(p),
	}
	if errs := ir.ValidateGraph(g); len(errs) > 0 {
		t.Fatalf("graph is not valid:\n%s", errs.Error())
	}
	files, err := hcl.Emit(g)
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	terraformFmt(t, files)
}
