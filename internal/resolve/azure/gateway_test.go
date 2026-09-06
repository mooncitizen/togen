package azure

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/mooncitizen/togen/internal/emit/hcl"
	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

var gatewayNode = ir.Node{ID: "n1", Type: ir.NodeGateway, Name: "api"}

func routeEdge(id, to, path string, methods ...ir.Method) ir.Edge {
	return ir.Edge{
		ID:         id,
		From:       "n1",
		To:         to,
		Relation:   ir.RelRoutes,
		Properties: ir.EdgeProperties{Path: path, Methods: methods},
	}
}

func setupRoutes(t *testing.T, edges ...ir.Edge) (*resolve.Context, *resolve.Handle) {
	t.Helper()
	ctx := newContext(t, []ir.Node{gatewayNode, functionNode(t, "n2", "handler", defaultFunction)})
	gateway := resolveGateway(ctx.Project.Nodes[0])
	fn := resolveFunction(ctx, ctx.Project.Nodes[1])
	for _, e := range edges {
		resolveRoutes(ctx, e, gateway, fn)
	}
	fn.Finalise()
	return ctx, fn
}

func appSetting(t *testing.T, ctx *resolve.Context, key string) ir.Value {
	t.Helper()
	settings, ok := firstOfType(t, ctx, "azurerm_linux_function_app").Args.Get("app_settings")
	if !ok {
		t.Fatal("no app_settings")
	}
	v, _ := ir.Attrs(settings.(ir.Map)).Get(key)
	return v
}

func TestGatewayCreatesNothing(t *testing.T) {
	ctx := newContext(t, []ir.Node{gatewayNode})
	handle := resolveGateway(ctx.Project.Nodes[0])
	if diff := cmp.Diff(resolve.Exports(resolve.GatewayExports{}), handle.Exports); diff != "" {
		t.Errorf("exports (-want +got):\n%s", diff)
	}
	if got := len(ctx.Resources()); got != 0 {
		t.Errorf("resources = %d", got)
	}
	if len(ctx.Outputs) != 0 {
		t.Errorf("outputs = %+v", ctx.Outputs)
	}
}

func TestRoutesOutputTheFunctionAppURLAndRecordTheRoutes(t *testing.T) {
	ctx, _ := setupRoutes(t, routeEdge("e1", "n2", "/orders", ir.MethodGet, ir.MethodPost))
	if len(ctx.Errors) != 0 {
		t.Fatalf("errors = %v", ctx.Errors)
	}
	wantOutputs := []ir.Output{{
		Name:        "api_handler_url",
		Description: "Public URL of the handler function behind the api gateway",
		Value:       ir.C(ir.Str("https://"), ir.R(fnID, ir.Field("default_hostname"))),
	}}
	if diff := cmp.Diff(wantOutputs, ctx.Outputs); diff != "" {
		t.Errorf("outputs (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(ir.Value(ir.Str("GET /orders,POST /orders")), appSetting(t, ctx, "API_ROUTES")); diff != "" {
		t.Errorf("API_ROUTES (-want +got):\n%s", diff)
	}
	if got := len(ctx.Resources()); got != 4 {
		t.Errorf("a route added resources: %d in total, want the group, plan, storage account and app", got)
	}
}

func TestRoutesToTheSameFunctionTwiceGiveOneOutput(t *testing.T) {
	ctx, _ := setupRoutes(t,
		routeEdge("e1", "n2", "/orders", ir.MethodGet),
		routeEdge("e2", "n2", "/orders/{id}", ir.MethodGet, ir.MethodDelete),
	)
	if len(ctx.Errors) != 0 {
		t.Fatalf("errors = %v", ctx.Errors)
	}
	if len(ctx.Outputs) != 1 || ctx.Outputs[0].Name != "api_handler_url" {
		t.Errorf("outputs = %+v", ctx.Outputs)
	}
	want := ir.Str("GET /orders,GET /orders/{id},DELETE /orders/{id}")
	if diff := cmp.Diff(ir.Value(want), appSetting(t, ctx, "API_ROUTES")); diff != "" {
		t.Errorf("API_ROUTES (-want +got):\n%s", diff)
	}
}

func TestRoutesAllowTheSamePathOnDifferentMethods(t *testing.T) {
	ctx, _ := setupRoutes(t,
		routeEdge("e1", "n2", "/users", ir.MethodGet),
		routeEdge("e2", "n2", "/users", ir.MethodPost),
	)
	if len(ctx.Errors) != 0 {
		t.Fatalf("errors = %v", ctx.Errors)
	}
	if diff := cmp.Diff(ir.Value(ir.Str("GET /users,POST /users")), appSetting(t, ctx, "API_ROUTES")); diff != "" {
		t.Errorf("API_ROUTES (-want +got):\n%s", diff)
	}
}

func TestRoutesToTwoFunctionsGiveOneOutputEach(t *testing.T) {
	p := newProject(t, []ir.Node{
		gatewayNode,
		functionNode(t, "n2", "orders", defaultFunction),
		functionNode(t, "n3", "users", defaultFunction),
	})
	p.Edges = []ir.Edge{
		routeEdge("e1", "n2", "/orders", ir.MethodGet),
		routeEdge("e2", "n3", "/users", ir.MethodGet),
	}
	g, err := resolve.Run(p, New())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	var names []string
	for _, o := range g.Outputs {
		names = append(names, o.Name)
	}
	if diff := cmp.Diff([]string{"api_orders_url", "api_users_url"}, names); diff != "" {
		t.Errorf("outputs (-want +got):\n%s", diff)
	}
}

func TestRoutesRejectTwoEdgesClaimingTheSameRouteKey(t *testing.T) {
	p := newProject(t, []ir.Node{
		gatewayNode,
		functionNode(t, "n2", "orders", defaultFunction),
		functionNode(t, "n3", "users", defaultFunction),
	})
	p.Edges = []ir.Edge{
		{ID: "e1", From: "n1", To: "n2", Relation: ir.RelRoutes},
		{ID: "e2", From: "n1", To: "n3", Relation: ir.RelRoutes},
	}
	want := ir.Errors{{
		EdgeID:  "e2",
		Message: "route 'ANY /' on gateway 'api' is already used by edge 'e1'",
	}}
	if diff := cmp.Diff(want, runErrors(t, p)); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
}

func TestRoutesToAPrivateServiceOpenItsIngressAndOutputTheFQDN(t *testing.T) {
	ctx := newContext(t, []ir.Node{gatewayNode, serviceNodeWith(t, "n4", "web", defaultService)})
	gateway := resolveGateway(ctx.Project.Nodes[0])
	svc := resolveService(ctx, ctx.Project.Nodes[1])
	before := len(ctx.Resources())
	resolveRoutes(ctx, routeEdge("e1", "n4", "/web", ir.MethodGet, ir.MethodPost), gateway, svc)
	svc.Finalise()

	if len(ctx.Errors) != 0 {
		t.Fatalf("errors = %v", ctx.Errors)
	}
	wantOutputs := []ir.Output{{
		Name:        "api_web_url",
		Description: "Public URL of the web service behind the api gateway",
		Value:       webURL,
	}}
	if diff := cmp.Diff(wantOutputs, ctx.Outputs); diff != "" {
		t.Errorf("outputs (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(ingressBlock(true, 8080)[0], appIngress(t, ctx, appID)); diff != "" {
		t.Errorf("ingress (-want +got):\n%s", diff)
	}
	wantEnv := ir.Attrs{ir.A("API_ROUTES", ir.Str("GET /web,POST /web"))}
	if diff := cmp.Diff(wantEnv, containerEnvOf(t, ctx, appID)); diff != "" {
		t.Errorf("container env (-want +got):\n%s", diff)
	}
	wantExports := resolve.ServiceExports{Port: ir.Num(8080), Public: true, URL: webURL}
	if diff := cmp.Diff(wantExports, svc.Exports); diff != "" {
		t.Errorf("exports (-want +got):\n%s", diff)
	}
	if got := len(ctx.Resources()); got != before {
		t.Errorf("a route added resources: %d, was %d", got, before)
	}
}

func TestRoutesToAFunctionAndAServiceGiveOneOutputEach(t *testing.T) {
	p := newProject(t, []ir.Node{
		gatewayNode,
		functionNode(t, "n2", "orders", defaultFunction),
		serviceNodeWith(t, "n4", "web", defaultService),
	})
	p.Edges = []ir.Edge{
		routeEdge("e1", "n2", "/orders", ir.MethodGet),
		routeEdge("e2", "n4", "/web", ir.MethodGet),
		routeEdge("e3", "n4", "/web/{proxy+}", ir.MethodAny),
	}
	g, err := resolve.Run(p, New())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if errs := ir.ValidateGraph(g); len(errs) > 0 {
		t.Errorf("graph is not valid:\n%s", errs.Error())
	}
	var names []string
	for _, o := range g.Outputs {
		names = append(names, o.Name)
	}
	if diff := cmp.Diff([]string{"api_orders_url", "api_web_url"}, names); diff != "" {
		t.Errorf("outputs (-want +got):\n%s", diff)
	}
	files, err := hcl.Emit(g)
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	for _, want := range []string{
		`"https://${azurerm_linux_function_app.orders.default_hostname}"`,
		`"https://${azurerm_container_app.web.ingress[0].fqdn}"`,
	} {
		if !strings.Contains(string(files["outputs.tf"]), want) {
			t.Errorf("outputs.tf lacks %s:\n%s", want, files["outputs.tf"])
		}
	}
}

func TestRoutesRejectAFunctionAndAServiceClaimingTheSameRouteKey(t *testing.T) {
	p := newProject(t, []ir.Node{
		gatewayNode,
		functionNode(t, "n2", "orders", defaultFunction),
		serviceNodeWith(t, "n4", "web", defaultService),
	})
	p.Edges = []ir.Edge{
		routeEdge("e1", "n2", "/web", ir.MethodGet),
		routeEdge("e2", "n4", "/web", ir.MethodGet),
	}
	want := ir.Errors{{
		EdgeID:  "e2",
		Message: "route 'GET /web' on gateway 'api' is already used by edge 'e1'",
	}}
	if diff := cmp.Diff(want, runErrors(t, p)); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
}

func TestRoutesToAnUnsupportedTargetAreReported(t *testing.T) {
	ctx := newContext(t, []ir.Node{gatewayNode, databaseNode(t, "n3", "main-db", smallPostgres)})
	gateway := resolveGateway(ctx.Project.Nodes[0])
	db := resolveDatabase(ctx, ctx.Project.Nodes[1])
	resolveRoutes(ctx, routeEdge("e1", "n3", "/db", ir.MethodGet), gateway, db)

	want := ir.Errors{{
		EdgeID:  "e1",
		Message: "routes to a database are not supported by the azure resolver yet",
	}}
	if diff := cmp.Diff(want, ctx.Errors); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
	if len(ctx.Outputs) != 1 || ctx.Outputs[0].Name != "main_db_fqdn" {
		t.Errorf("outputs = %+v", ctx.Outputs)
	}
}
