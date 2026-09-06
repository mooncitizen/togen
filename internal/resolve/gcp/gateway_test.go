package gcp

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

var bindingID = ir.ID{Type: "google_cloud_run_v2_service_iam_member", Name: "api_handler"}

func setupRoutes(t *testing.T, edges []ir.Edge) *resolve.Context {
	t.Helper()
	ctx, project := newContext(t, []ir.Node{
		{ID: "n1", Type: ir.NodeGateway, Name: "api"},
		functionNode(t, "n2", "handler", defaultFunction),
	}, edges)
	gateway := resolveGateway(project.Nodes[0])
	fn := resolveFunction(ctx, project.Nodes[1])
	ctx.SetHandle("n1", gateway)
	ctx.SetHandle("n2", fn)
	for _, e := range project.Edges {
		resolveRoutes(ctx, e, gateway, fn)
	}
	fn.Finalise()
	return ctx
}

func routeEdge(id, path string, methods []ir.Method) ir.Edge {
	return ir.Edge{
		ID:         id,
		From:       "n1",
		To:         "n2",
		Relation:   ir.RelRoutes,
		Properties: ir.EdgeProperties{Path: path, Methods: methods},
	}
}

func twoFunctionProject(edges []ir.Edge) *ir.Project {
	return &ir.Project{
		Version:     1,
		Name:        "shop",
		Provider:    ir.ProviderGCP,
		Region:      "europe-west2",
		Environment: "dev",
		Nodes: []ir.Node{
			{ID: "n1", Type: ir.NodeGateway, Name: "api"},
			{ID: "n2", Type: ir.NodeFunction, Name: "handler"},
			{ID: "n3", Type: ir.NodeFunction, Name: "worker"},
		},
		Edges: edges,
	}
}

func outputNames(outputs []ir.Output) []string {
	var names []string
	for _, o := range outputs {
		names = append(names, o.Name)
	}
	return names
}

func TestGatewayCreatesNothing(t *testing.T) {
	ctx, project := newContext(t, []ir.Node{{ID: "n1", Type: ir.NodeGateway, Name: "api"}}, nil)
	handle := resolveGateway(project.Nodes[0])
	if len(ctx.Resources()) != 0 || len(ctx.Outputs) != 0 || len(ctx.Variables) != 0 {
		t.Errorf("gateway produced resources %d, outputs %d, variables %d", len(ctx.Resources()), len(ctx.Outputs), len(ctx.Variables))
	}
	if diff := cmp.Diff(resolve.Exports(resolve.GatewayExports{}), handle.Exports); diff != "" {
		t.Errorf("exports (-want +got):\n%s", diff)
	}
	if handle.Node.ID != "n1" {
		t.Errorf("handle node = %q", handle.Node.ID)
	}
}

func TestRoutesOpenTheFunctionToAllUsersAndOutputItsURL(t *testing.T) {
	ctx := setupRoutes(t, []ir.Edge{
		routeEdge("e1", "/users/{id}", []ir.Method{ir.MethodGet, ir.MethodDelete}),
	})
	if len(ctx.Errors) != 0 {
		t.Fatalf("errors = %v", ctx.Errors)
	}

	binding := firstOfType(t, ctx, bindingID.Type)
	if binding.Name != bindingID.Name || binding.SourceNode != "n1" || binding.SourceLabel != "api" {
		t.Errorf("binding = %s, source %q/%q", binding.Name, binding.SourceNode, binding.SourceLabel)
	}
	want := ir.Attrs{
		ir.A("name", ir.R(fnID, ir.Field("name"))),
		ir.A("location", ir.R(fnID, ir.Field("location"))),
		ir.A("role", ir.Str("roles/run.invoker")),
		ir.A("member", ir.Str("allUsers")),
	}
	if diff := cmp.Diff(want, binding.Args); diff != "" {
		t.Errorf("binding args (-want +got):\n%s", diff)
	}
	if got := len(ctx.Resources()); got != 5 {
		t.Errorf("resources = %d, want the function's four and the binding", got)
	}

	wantOutputs := []ir.Output{{
		Name:        "api_handler_url",
		Description: "Public URL of the handler function the api gateway routes to",
		Value:       ir.R(fnID, ir.Field("service_config"), ir.Index(0), ir.Field("uri")),
	}}
	if diff := cmp.Diff(wantOutputs, ctx.Outputs); diff != "" {
		t.Errorf("outputs (-want +got):\n%s", diff)
	}
}

func TestRoutesToTheSameFunctionTwiceBindAndOutputOnce(t *testing.T) {
	ctx := setupRoutes(t, []ir.Edge{
		routeEdge("e1", "/users", []ir.Method{ir.MethodGet}),
		routeEdge("e2", "/users/{id}", []ir.Method{ir.MethodGet}),
	})
	if len(ctx.Errors) != 0 {
		t.Fatalf("errors = %v", ctx.Errors)
	}
	if got := countOfType(ctx, bindingID.Type); got != 1 {
		t.Errorf("bindings = %d, want 1", got)
	}
	if diff := cmp.Diff([]string{"api_handler_url"}, outputNames(ctx.Outputs)); diff != "" {
		t.Errorf("outputs (-want +got):\n%s", diff)
	}
}

func TestRoutesToTwoFunctionsGiveEachItsOwnURL(t *testing.T) {
	g, err := resolve.Run(twoFunctionProject([]ir.Edge{
		{
			ID: "e1", From: "n1", To: "n2", Relation: ir.RelRoutes,
			Properties: ir.EdgeProperties{Path: "/orders", Methods: []ir.Method{ir.MethodGet}},
		},
		{
			ID: "e2", From: "n1", To: "n3", Relation: ir.RelRoutes,
			Properties: ir.EdgeProperties{Path: "/jobs", Methods: []ir.Method{ir.MethodGet}},
		},
	}), New())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	var bindings []string
	for _, r := range g.Resources {
		if r.Type == bindingID.Type {
			bindings = append(bindings, r.Name)
		}
	}
	if diff := cmp.Diff([]string{"api_handler", "api_worker"}, bindings); diff != "" {
		t.Errorf("bindings (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"api_handler_url", "api_worker_url"}, outputNames(g.Outputs)); diff != "" {
		t.Errorf("outputs (-want +got):\n%s", diff)
	}
}

func setupServiceRoutes(t *testing.T, p ir.ServiceProps, edges []ir.Edge) *resolve.Context {
	t.Helper()
	ctx, project := newContext(t, []ir.Node{
		{ID: "n1", Type: ir.NodeGateway, Name: "api"},
		serviceNode(t, "n4", "web", p),
	}, edges)
	gateway := resolveGateway(project.Nodes[0])
	svc := resolveService(ctx, project.Nodes[1])
	ctx.SetHandle("n1", gateway)
	ctx.SetHandle("n4", svc)
	for _, e := range project.Edges {
		resolveRoutes(ctx, e, gateway, svc)
	}
	svc.Finalise()
	return ctx
}

func TestRoutesToAServiceOpenItToAllUsersAndOutputItsURL(t *testing.T) {
	ctx := setupServiceRoutes(t, defaultService, []ir.Edge{{
		ID: "e1", From: "n1", To: "n4", Relation: ir.RelRoutes,
		Properties: ir.EdgeProperties{Path: "/", Methods: []ir.Method{ir.MethodAny}},
	}})
	if len(ctx.Errors) != 0 {
		t.Fatalf("errors = %v", ctx.Errors)
	}

	binding := named(t, ctx, ir.ID{Type: bindingID.Type, Name: "api_web"})
	if binding.SourceNode != "n1" || binding.SourceLabel != "api" {
		t.Errorf("binding source = %q/%q", binding.SourceNode, binding.SourceLabel)
	}
	want := ir.Attrs{
		ir.A("name", ir.R(svcID, ir.Field("name"))),
		ir.A("location", ir.R(svcID, ir.Field("location"))),
		ir.A("role", ir.Str("roles/run.invoker")),
		ir.A("member", ir.Str("allUsers")),
	}
	if diff := cmp.Diff(want, binding.Args); diff != "" {
		t.Errorf("binding args (-want +got):\n%s", diff)
	}
	wantOutputs := []ir.Output{{
		Name:        "api_web_url",
		Description: "Public URL of the web service the api gateway routes to",
		Value:       ir.R(svcID, ir.Field("uri")),
	}}
	if diff := cmp.Diff(wantOutputs, ctx.Outputs); diff != "" {
		t.Errorf("outputs (-want +got):\n%s", diff)
	}
	ingress, _ := named(t, ctx, svcID).Args.Get("ingress")
	if diff := cmp.Diff(ir.Value(ir.Str("INGRESS_TRAFFIC_INTERNAL_ONLY")), ingress); diff != "" {
		t.Errorf("ingress (-want +got):\n%s", diff)
	}
}

// The service's own binding already opens it, and a second allUsers member on the same service
// would fight the first on destroy.
func TestRoutesToAPublicServiceAddTheOutputAlone(t *testing.T) {
	p := defaultService
	p.Public = true
	ctx := setupServiceRoutes(t, p, []ir.Edge{{
		ID: "e1", From: "n1", To: "n4", Relation: ir.RelRoutes,
		Properties: ir.EdgeProperties{Path: "/", Methods: []ir.Method{ir.MethodAny}},
	}})
	if len(ctx.Errors) != 0 {
		t.Fatalf("errors = %v", ctx.Errors)
	}
	var bindings []string
	for _, r := range byType(ctx, bindingID.Type) {
		bindings = append(bindings, r.Name)
	}
	if diff := cmp.Diff([]string{"web"}, bindings); diff != "" {
		t.Errorf("bindings (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"web_url", "api_web_url"}, outputNames(ctx.Outputs)); diff != "" {
		t.Errorf("outputs (-want +got):\n%s", diff)
	}
}

func TestRoutesToAFunctionAndAServiceGiveEachItsOwnURL(t *testing.T) {
	g, err := resolve.Run(newProject(t, []ir.Node{
		{ID: "n1", Type: ir.NodeGateway, Name: "api"},
		functionNode(t, "n2", "handler", defaultFunction),
		serviceNode(t, "n4", "web", defaultService),
	}, []ir.Edge{
		{
			ID: "e1", From: "n1", To: "n2", Relation: ir.RelRoutes,
			Properties: ir.EdgeProperties{Path: "/orders", Methods: []ir.Method{ir.MethodGet}},
		},
		{
			ID: "e2", From: "n1", To: "n4", Relation: ir.RelRoutes,
			Properties: ir.EdgeProperties{Path: "/", Methods: []ir.Method{ir.MethodGet}},
		},
	}), New())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	var bindings []string
	for _, r := range g.Resources {
		if r.Type == bindingID.Type {
			bindings = append(bindings, r.Name)
		}
	}
	if diff := cmp.Diff([]string{"api_handler", "api_web"}, bindings); diff != "" {
		t.Errorf("bindings (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"api_handler_url", "api_web_url"}, outputNames(g.Outputs)); diff != "" {
		t.Errorf("outputs (-want +got):\n%s", diff)
	}
}

func TestRoutesRejectsAFunctionAndAServiceClaimingTheSameRouteKey(t *testing.T) {
	p := newProject(t, []ir.Node{
		{ID: "n1", Type: ir.NodeGateway, Name: "api"},
		functionNode(t, "n2", "handler", defaultFunction),
		serviceNode(t, "n4", "web", defaultService),
	}, []ir.Edge{
		{ID: "e1", From: "n1", To: "n2", Relation: ir.RelRoutes},
		{ID: "e2", From: "n1", To: "n4", Relation: ir.RelRoutes},
	})
	want := ir.Errors{{
		EdgeID:  "e2",
		Message: "route 'ANY /' on gateway 'api' is already used by edge 'e1'",
	}}
	if diff := cmp.Diff(want, runErrors(t, p)); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
}

func TestRoutesRejectsTwoEdgesClaimingTheSameRouteKey(t *testing.T) {
	p := twoFunctionProject([]ir.Edge{
		{ID: "e1", From: "n1", To: "n2", Relation: ir.RelRoutes},
		{ID: "e2", From: "n1", To: "n3", Relation: ir.RelRoutes},
	})
	want := ir.Errors{{
		EdgeID:  "e2",
		Message: "route 'ANY /' on gateway 'api' is already used by edge 'e1'",
	}}
	if diff := cmp.Diff(want, runErrors(t, p)); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
}

func TestRoutesAllowsTheSamePathOnDifferentMethods(t *testing.T) {
	ctx := setupRoutes(t, []ir.Edge{
		routeEdge("e1", "/users", []ir.Method{ir.MethodGet}),
		routeEdge("e2", "/users", []ir.Method{ir.MethodPost}),
	})
	if len(ctx.Errors) != 0 {
		t.Fatalf("errors = %v", ctx.Errors)
	}
}

func TestRoutesToAnUnsupportedTargetAreReported(t *testing.T) {
	ctx, project := newContext(t, []ir.Node{
		{ID: "n1", Type: ir.NodeGateway, Name: "api"},
		{ID: "n3", Type: ir.NodeDatabase, Name: "main-db"},
	}, []ir.Edge{{ID: "e1", From: "n1", To: "n3", Relation: ir.RelRoutes}})
	gateway := resolveGateway(project.Nodes[0])
	db := &resolve.Handle{Node: project.Nodes[1]}
	resolveRoutes(ctx, project.Edges[0], gateway, db)

	want := ir.Errors{{
		EdgeID:  "e1",
		Message: "routes to a database are not supported by the gcp resolver yet",
	}}
	if diff := cmp.Diff(want, ctx.Errors); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
	if len(ctx.Resources()) != 0 || len(ctx.Outputs) != 0 {
		t.Errorf("an unsupported route produced resources %d, outputs %d", len(ctx.Resources()), len(ctx.Outputs))
	}
}
