package aws

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"togen/internal/ir"
	"togen/internal/resolve"
)

var apiID = ir.ID{Type: "aws_apigatewayv2_api", Name: "api"}

func setupRoutes(t *testing.T, edges []ir.Edge) *resolve.Context {
	t.Helper()
	ctx, project := newContext(t, []ir.Node{
		{ID: "n1", Type: ir.NodeGateway, Name: "api"},
		{ID: "n2", Type: ir.NodeFunction, Name: "handler", Properties: props(t, defaultFunction)},
	}, edges)
	gateway := resolveGateway(ctx, project.Nodes[0])
	fn := resolveFunction(ctx, project.Nodes[1])
	ctx.SetHandle("n1", gateway)
	ctx.SetHandle("n2", fn)
	for _, e := range project.Edges {
		resolveRoutes(ctx, e, gateway, fn)
	}
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

func TestGatewayEmitsAnHTTPAPIWithADefaultStageAndURLOutput(t *testing.T) {
	ctx, project := newContext(t, []ir.Node{{ID: "n1", Type: ir.NodeGateway, Name: "api"}}, nil)
	handle := resolveGateway(ctx, project.Nodes[0])

	wantAPI := ir.Attrs{
		ir.A("name", ir.Str("shop-dev-api")),
		ir.A("protocol_type", ir.Str("HTTP")),
	}
	if diff := cmp.Diff(wantAPI, firstOfType(t, ctx, "aws_apigatewayv2_api").Args); diff != "" {
		t.Errorf("api args (-want +got):\n%s", diff)
	}
	wantStage := ir.Attrs{
		ir.A("api_id", ir.R(apiID, ir.Field("id"))),
		ir.A("name", ir.Str("$default")),
		ir.A("auto_deploy", ir.Bool(true)),
	}
	if diff := cmp.Diff(wantStage, firstOfType(t, ctx, "aws_apigatewayv2_stage").Args); diff != "" {
		t.Errorf("stage args (-want +got):\n%s", diff)
	}
	wantOutputs := []ir.Output{{
		Name:        "api_url",
		Description: "Public URL of the api gateway",
		Value:       ir.R(apiID, ir.Field("api_endpoint")),
	}}
	if diff := cmp.Diff(wantOutputs, ctx.Outputs); diff != "" {
		t.Errorf("outputs (-want +got):\n%s", diff)
	}
	wantExports := resolve.GatewayExports{
		APIID:        ir.R(apiID, ir.Field("id")),
		ExecutionARN: ir.R(apiID, ir.Field("execution_arn")),
		URL:          ir.R(apiID, ir.Field("api_endpoint")),
	}
	if diff := cmp.Diff(wantExports, handle.Exports); diff != "" {
		t.Errorf("exports (-want +got):\n%s", diff)
	}
}

func TestRoutesWiresIntegrationRoutesAndPermission(t *testing.T) {
	ctx := setupRoutes(t, []ir.Edge{
		routeEdge("e1", "/users/{id}", []ir.Method{ir.MethodGet, ir.MethodDelete}),
	})

	integration := firstOfType(t, ctx, "aws_apigatewayv2_integration")
	if integration.Name != "api_handler" {
		t.Errorf("integration name = %q", integration.Name)
	}
	wantIntegration := ir.Attrs{
		ir.A("api_id", ir.R(apiID, ir.Field("id"))),
		ir.A("integration_type", ir.Str("AWS_PROXY")),
		ir.A("integration_uri", ir.R(fnID, ir.Field("invoke_arn"))),
		ir.A("integration_method", ir.Str("POST")),
		ir.A("payload_format_version", ir.Str("2.0")),
	}
	if diff := cmp.Diff(wantIntegration, integration.Args); diff != "" {
		t.Errorf("integration args (-want +got):\n%s", diff)
	}

	routes := byType(ctx, "aws_apigatewayv2_route")
	var names []string
	for _, r := range routes {
		names = append(names, r.Name)
	}
	wantNames := []string{"api_handler_get_users_id", "api_handler_delete_users_id"}
	if diff := cmp.Diff(wantNames, names); diff != "" {
		t.Errorf("route names (-want +got):\n%s", diff)
	}
	wantRoute := ir.Attrs{
		ir.A("api_id", ir.R(apiID, ir.Field("id"))),
		ir.A("route_key", ir.Str("GET /users/{id}")),
		ir.A("target", ir.C(
			ir.Str("integrations/"),
			ir.R(ir.ID{Type: "aws_apigatewayv2_integration", Name: "api_handler"}, ir.Field("id")),
		)),
	}
	if diff := cmp.Diff(wantRoute, routes[0].Args); diff != "" {
		t.Errorf("route args (-want +got):\n%s", diff)
	}

	permission := firstOfType(t, ctx, "aws_lambda_permission")
	wantPermission := ir.Attrs{
		ir.A("statement_id", ir.Str("AllowInvokeFromApiGateway")),
		ir.A("action", ir.Str("lambda:InvokeFunction")),
		ir.A("function_name", ir.R(fnID, ir.Field("function_name"))),
		ir.A("principal", ir.Str("apigateway.amazonaws.com")),
		ir.A("source_arn", ir.C(ir.R(apiID, ir.Field("execution_arn")), ir.Str("/*/*"))),
	}
	if diff := cmp.Diff(wantPermission, permission.Args); diff != "" {
		t.Errorf("permission args (-want +got):\n%s", diff)
	}
}

func TestRoutesNamesTheRootRouteSensibly(t *testing.T) {
	ctx := setupRoutes(t, []ir.Edge{routeEdge("e1", "/", []ir.Method{ir.MethodAny})})
	route := firstOfType(t, ctx, "aws_apigatewayv2_route")
	if route.Name != "api_handler_any_root" {
		t.Errorf("route name = %q", route.Name)
	}
	got, _ := route.Args.Get("route_key")
	if diff := cmp.Diff(ir.Str("ANY /"), got); diff != "" {
		t.Errorf("route_key (-want +got):\n%s", diff)
	}
}

func TestRoutesDeduplicatesRouteNamesThatSlugifyTheSame(t *testing.T) {
	ctx := setupRoutes(t, []ir.Edge{
		routeEdge("e1", "/users-x", []ir.Method{ir.MethodGet}),
		routeEdge("e2", "/users_x", []ir.Method{ir.MethodGet}),
	})
	var names []string
	for _, r := range byType(ctx, "aws_apigatewayv2_route") {
		names = append(names, r.Name)
	}
	wantNames := []string{"api_handler_get_users_x", "api_handler_get_users_x_2"}
	if diff := cmp.Diff(wantNames, names); diff != "" {
		t.Errorf("route names (-want +got):\n%s", diff)
	}
	if countOfType(ctx, "aws_apigatewayv2_integration") != 1 {
		t.Error("the integration was created twice")
	}
	if countOfType(ctx, "aws_lambda_permission") != 1 {
		t.Error("the permission was created twice")
	}
}

func TestRoutesRejectsTwoEdgesClaimingTheSameRouteKey(t *testing.T) {
	p := &ir.Project{
		Version:     1,
		Name:        "shop",
		Provider:    ir.ProviderAWS,
		Region:      "eu-west-2",
		Environment: "dev",
		Nodes: []ir.Node{
			{ID: "n1", Type: ir.NodeGateway, Name: "api"},
			{ID: "n2", Type: ir.NodeFunction, Name: "handler"},
			{ID: "n3", Type: ir.NodeFunction, Name: "worker"},
		},
		Edges: []ir.Edge{
			{ID: "e1", From: "n1", To: "n2", Relation: ir.RelRoutes},
			{ID: "e2", From: "n1", To: "n3", Relation: ir.RelRoutes},
		},
	}
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
	if countOfType(ctx, "aws_apigatewayv2_route") != 2 {
		t.Error("want two routes")
	}
}

func TestRoutesToANonFunctionAreReported(t *testing.T) {
	ctx, project := newContext(t, []ir.Node{
		{ID: "n1", Type: ir.NodeGateway, Name: "api"},
		{ID: "n3", Type: ir.NodeDatabase, Name: "main-db"},
	}, []ir.Edge{{ID: "e1", From: "n1", To: "n3", Relation: ir.RelRoutes}})
	gateway := resolveGateway(ctx, project.Nodes[0])
	db := resolveDatabase(ctx, project.Nodes[1])
	resolveRoutes(ctx, project.Edges[0], gateway, db)

	want := ir.Errors{{
		EdgeID:  "e1",
		Message: "routes to a database are not supported by the aws resolver yet",
	}}
	if diff := cmp.Diff(want, ctx.Errors); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
}
