package aws

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

var (
	vpcLinkID   = ir.ID{Type: "aws_apigatewayv2_vpc_link", Name: "main"}
	vpcLinkSGID = ir.ID{Type: "aws_security_group", Name: "shop_dev_vpc_link"}
)

func setupServiceRoutes(t *testing.T, p ir.ServiceProps, edges []ir.Edge) (*resolve.Context, *resolve.Handle) {
	t.Helper()
	ctx, project := newContext(t, []ir.Node{
		{ID: "n1", Type: ir.NodeGateway, Name: "api"},
		serviceNode(t, "n4", "web", p),
	}, edges)
	gateway := resolveGateway(ctx, project.Nodes[0])
	service := resolveService(ctx, project.Nodes[1])
	ctx.SetHandle("n1", gateway)
	ctx.SetHandle("n4", service)
	for _, e := range project.Edges {
		resolveRoutes(ctx, e, gateway, service)
	}
	service.Finalise()
	return ctx, service
}

func serviceRouteEdge(id, path string, methods []ir.Method) ir.Edge {
	return ir.Edge{
		ID:         id,
		From:       "n1",
		To:         "n4",
		Relation:   ir.RelRoutes,
		Properties: ir.EdgeProperties{Path: path, Methods: methods},
	}
}

func TestRoutesToAServiceWiresAVPCLinkAndAnHTTPProxyIntegration(t *testing.T) {
	p := defaultService
	p.Port = 80
	p.Public = true
	ctx, _ := setupServiceRoutes(t, p, []ir.Edge{
		serviceRouteEdge("e1", "/web", []ir.Method{ir.MethodGet, ir.MethodPost}),
	})
	if len(ctx.Errors) != 0 {
		t.Fatalf("errors = %v", ctx.Errors)
	}

	wantLinkSG := ir.Attrs{
		ir.A("name", ir.Str("shop-dev-vpc-link")),
		ir.A("description", ir.Str("Gateway access to load balancers in shop-dev")),
		ir.A("vpc_id", ir.R(ir.ID{Type: "aws_vpc", Name: "main"}, ir.Field("id"))),
	}
	if diff := cmp.Diff(wantLinkSG, named(t, ctx, vpcLinkSGID).Args); diff != "" {
		t.Errorf("vpc link security group args (-want +got):\n%s", diff)
	}
	wantLinkEgress := ir.Attrs{
		ir.A("security_group_id", ir.R(vpcLinkSGID, ir.Field("id"))),
		ir.A("ip_protocol", ir.Str("-1")),
		ir.A("cidr_ipv4", ir.Str("0.0.0.0/0")),
	}
	egress := named(t, ctx, ir.ID{Type: "aws_vpc_security_group_egress_rule", Name: "shop_dev_vpc_link_all"})
	if diff := cmp.Diff(wantLinkEgress, egress.Args); diff != "" {
		t.Errorf("vpc link egress rule args (-want +got):\n%s", diff)
	}
	wantLink := ir.Attrs{
		ir.A("name", ir.Str("shop-dev")),
		ir.A("subnet_ids", privateSub),
		ir.A("security_group_ids", ir.L(ir.R(vpcLinkSGID, ir.Field("id")))),
	}
	if diff := cmp.Diff(wantLink, named(t, ctx, vpcLinkID).Args); diff != "" {
		t.Errorf("vpc link args (-want +got):\n%s", diff)
	}

	wantIngress := ir.Attrs{
		ir.A("security_group_id", ir.R(albSGID, ir.Field("id"))),
		ir.A("referenced_security_group_id", ir.R(vpcLinkSGID, ir.Field("id"))),
		ir.A("from_port", ir.Num(80)),
		ir.A("to_port", ir.Num(80)),
		ir.A("ip_protocol", ir.Str("tcp")),
		ir.A("description", ir.Str("Gateway to web")),
	}
	ingress := named(t, ctx, ir.ID{Type: "aws_vpc_security_group_ingress_rule", Name: "web_alb_from_vpc_link"})
	if diff := cmp.Diff(wantIngress, ingress.Args); diff != "" {
		t.Errorf("alb ingress rule args (-want +got):\n%s", diff)
	}

	integration := firstOfType(t, ctx, "aws_apigatewayv2_integration")
	if integration.Name != "api_web" {
		t.Errorf("integration name = %q", integration.Name)
	}
	wantIntegration := ir.Attrs{
		ir.A("api_id", ir.R(apiID, ir.Field("id"))),
		ir.A("integration_type", ir.Str("HTTP_PROXY")),
		ir.A("integration_uri", ir.R(albListener, ir.Field("arn"))),
		ir.A("integration_method", ir.Str("ANY")),
		ir.A("connection_type", ir.Str("VPC_LINK")),
		ir.A("connection_id", ir.R(vpcLinkID, ir.Field("id"))),
		ir.A("payload_format_version", ir.Str("1.0")),
	}
	if diff := cmp.Diff(wantIntegration, integration.Args); diff != "" {
		t.Errorf("integration args (-want +got):\n%s", diff)
	}

	routes := byType(ctx, "aws_apigatewayv2_route")
	var names []string
	for _, r := range routes {
		names = append(names, r.Name)
	}
	if diff := cmp.Diff([]string{"api_web_get_web", "api_web_post_web"}, names); diff != "" {
		t.Errorf("route names (-want +got):\n%s", diff)
	}
	wantRoute := ir.Attrs{
		ir.A("api_id", ir.R(apiID, ir.Field("id"))),
		ir.A("route_key", ir.Str("GET /web")),
		ir.A("target", ir.C(
			ir.Str("integrations/"),
			ir.R(ir.ID{Type: "aws_apigatewayv2_integration", Name: "api_web"}, ir.Field("id")),
		)),
	}
	if diff := cmp.Diff(wantRoute, routes[0].Args); diff != "" {
		t.Errorf("route args (-want +got):\n%s", diff)
	}

	if countOfType(ctx, "aws_lambda_permission") != 0 {
		t.Error("a service route asked for a lambda permission")
	}
	wantOutputs := []ir.Output{
		{
			Name:        "api_url",
			Description: "Public URL of the api gateway",
			Value:       ir.R(apiID, ir.Field("api_endpoint")),
		},
		{
			Name:        "web_url",
			Description: "Public URL of the web service",
			Value:       ir.C(ir.Str("http://"), ir.R(albID, ir.Field("dns_name"))),
		},
	}
	if diff := cmp.Diff(wantOutputs, ctx.Outputs); diff != "" {
		t.Errorf("outputs (-want +got):\n%s", diff)
	}
}

func TestRoutesToAPrivateServiceGiveItAnInternalLoadBalancer(t *testing.T) {
	ctx, handle := setupServiceRoutes(t, defaultService, []ir.Edge{
		serviceRouteEdge("e1", "/web", []ir.Method{ir.MethodAny}),
	})
	if len(ctx.Errors) != 0 {
		t.Fatalf("errors = %v", ctx.Errors)
	}

	wantALBSG := ir.Attrs{
		ir.A("name", ir.Str("shop-dev-web-alb")),
		ir.A("description", ir.Str("Internal access to web")),
		ir.A("vpc_id", ir.R(ir.ID{Type: "aws_vpc", Name: "main"}, ir.Field("id"))),
	}
	if diff := cmp.Diff(wantALBSG, named(t, ctx, albSGID).Args); diff != "" {
		t.Errorf("alb security group args (-want +got):\n%s", diff)
	}
	for _, r := range byType(ctx, "aws_vpc_security_group_ingress_rule") {
		if v, ok := r.Args.Get("cidr_ipv4"); ok {
			t.Errorf("%s opens %v on an internal load balancer", r.Name, v)
		}
	}

	wantLB := ir.Attrs{
		ir.A("name", ir.Str("shop-dev-web")),
		ir.A("load_balancer_type", ir.Str("application")),
		ir.A("internal", ir.Bool(true)),
		ir.A("subnets", privateSub),
		ir.A("security_groups", ir.L(ir.R(albSGID, ir.Field("id")))),
	}
	if diff := cmp.Diff(wantLB, named(t, ctx, albID).Args); diff != "" {
		t.Errorf("load balancer args (-want +got):\n%s", diff)
	}
	wantTargets := ir.Attrs{
		ir.A("name", ir.Str("shop-dev-web")),
		ir.A("port", ir.Num(8080)),
		ir.A("protocol", ir.Str("HTTP")),
		ir.A("vpc_id", ir.R(ir.ID{Type: "aws_vpc", Name: "main"}, ir.Field("id"))),
		ir.A("target_type", ir.Str("ip")),
		ir.A("health_check", ir.B(ir.Attrs{ir.A("path", ir.Str("/"))})),
	}
	if diff := cmp.Diff(wantTargets, named(t, ctx, albTargets).Args); diff != "" {
		t.Errorf("target group args (-want +got):\n%s", diff)
	}
	wantListener := ir.Attrs{
		ir.A("load_balancer_arn", ir.R(albID, ir.Field("arn"))),
		ir.A("port", ir.Num(80)),
		ir.A("protocol", ir.Str("HTTP")),
		ir.A("default_action", ir.B(ir.Attrs{
			ir.A("type", ir.Str("forward")),
			ir.A("target_group_arn", ir.R(albTargets, ir.Field("arn"))),
		})),
	}
	if diff := cmp.Diff(wantListener, named(t, ctx, albListener).Args); diff != "" {
		t.Errorf("listener args (-want +got):\n%s", diff)
	}

	svc := named(t, ctx, svcID)
	got, _ := svc.Args.Get("load_balancer")
	wantBlock := ir.B(ir.Attrs{
		ir.A("target_group_arn", ir.R(albTargets, ir.Field("arn"))),
		ir.A("container_name", ir.Str("web")),
		ir.A("container_port", ir.Num(8080)),
	})
	if diff := cmp.Diff(ir.Value(wantBlock), got); diff != "" {
		t.Errorf("load_balancer block (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]ir.ID{albListener}, svc.DependsOn); diff != "" {
		t.Errorf("depends_on (-want +got):\n%s", diff)
	}

	// A private service still has no URL of its own. The gateway is the way in.
	wantExports := resolve.ServiceExports{
		Port:        ir.Num(8080),
		ListenerARN: ir.R(albListener, ir.Field("arn")),
	}
	if diff := cmp.Diff(wantExports, handle.Exports); diff != "" {
		t.Errorf("exports (-want +got):\n%s", diff)
	}
	wantOutputs := []ir.Output{{
		Name:        "api_url",
		Description: "Public URL of the api gateway",
		Value:       ir.R(apiID, ir.Field("api_endpoint")),
	}}
	if diff := cmp.Diff(wantOutputs, ctx.Outputs); diff != "" {
		t.Errorf("outputs (-want +got):\n%s", diff)
	}
}

func TestRoutesToTheSameServiceTwiceWireItOnce(t *testing.T) {
	p := defaultService
	p.Public = true
	ctx, _ := setupServiceRoutes(t, p, []ir.Edge{
		serviceRouteEdge("e1", "/web", []ir.Method{ir.MethodGet}),
		serviceRouteEdge("e2", "/web/{proxy+}", []ir.Method{ir.MethodGet}),
	})
	if len(ctx.Errors) != 0 {
		t.Fatalf("errors = %v", ctx.Errors)
	}
	for _, typ := range []string{
		"aws_apigatewayv2_integration",
		"aws_apigatewayv2_vpc_link",
		"aws_lb",
		"aws_lb_listener",
	} {
		if got := countOfType(ctx, typ); got != 1 {
			t.Errorf("%s = %d, want 1", typ, got)
		}
	}
	if got := countOfType(ctx, "aws_apigatewayv2_route"); got != 2 {
		t.Errorf("routes = %d, want 2", got)
	}
	ingress := byType(ctx, "aws_vpc_security_group_ingress_rule")
	var names []string
	for _, r := range ingress {
		names = append(names, r.Name)
	}
	wantIngress := []string{"web_alb_http", "web_from_alb", "web_alb_from_vpc_link"}
	if diff := cmp.Diff(wantIngress, names); diff != "" {
		t.Errorf("ingress rules (-want +got):\n%s", diff)
	}
}

func TestTwoRoutedServicesShareOneVPCLink(t *testing.T) {
	ctx, project := newContext(t, []ir.Node{
		{ID: "n1", Type: ir.NodeGateway, Name: "api"},
		serviceNode(t, "n4", "web", defaultService),
		serviceNode(t, "n5", "admin", defaultService),
	}, []ir.Edge{
		serviceRouteEdge("e1", "/web", []ir.Method{ir.MethodGet}),
		{
			ID:         "e2",
			From:       "n1",
			To:         "n5",
			Relation:   ir.RelRoutes,
			Properties: ir.EdgeProperties{Path: "/admin", Methods: []ir.Method{ir.MethodGet}},
		},
	})
	gateway := resolveGateway(ctx, project.Nodes[0])
	web := resolveService(ctx, project.Nodes[1])
	admin := resolveService(ctx, project.Nodes[2])
	resolveRoutes(ctx, project.Edges[0], gateway, web)
	resolveRoutes(ctx, project.Edges[1], gateway, admin)

	if got := countOfType(ctx, "aws_apigatewayv2_vpc_link"); got != 1 {
		t.Errorf("vpc links = %d, want 1", got)
	}
	if got := countOfType(ctx, "aws_lb"); got != 2 {
		t.Errorf("load balancers = %d, want 2", got)
	}
	for _, name := range []string{"web_alb_from_vpc_link", "admin_alb_from_vpc_link"} {
		named(t, ctx, ir.ID{Type: "aws_vpc_security_group_ingress_rule", Name: name})
	}
	for _, name := range []string{"api_web", "api_admin"} {
		named(t, ctx, ir.ID{Type: "aws_apigatewayv2_integration", Name: name})
	}
}

func TestRoutesToAFunctionAndAServiceShareOneGatewayURL(t *testing.T) {
	p := &ir.Project{
		Version:     1,
		Name:        "shop",
		Provider:    ir.ProviderAWS,
		Region:      "eu-west-2",
		Environment: "dev",
		Nodes: []ir.Node{
			{ID: "n1", Type: ir.NodeGateway, Name: "api"},
			{ID: "n2", Type: ir.NodeFunction, Name: "handler"},
			serviceNode(t, "n4", "web", defaultService),
		},
		Edges: []ir.Edge{
			{
				ID: "e1", From: "n1", To: "n2", Relation: ir.RelRoutes,
				Properties: ir.EdgeProperties{Path: "/orders", Methods: []ir.Method{ir.MethodGet}},
			},
			serviceRouteEdge("e2", "/web", []ir.Method{ir.MethodGet}),
		},
	}
	g, err := resolve.Run(p, New())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	var integrations []string
	for _, r := range g.Resources {
		if r.Type == "aws_apigatewayv2_integration" {
			integrations = append(integrations, r.Name)
		}
	}
	if diff := cmp.Diff([]string{"api_handler", "api_web"}, integrations); diff != "" {
		t.Errorf("integrations (-want +got):\n%s", diff)
	}
	var outputs []string
	for _, o := range g.Outputs {
		outputs = append(outputs, o.Name)
	}
	if diff := cmp.Diff([]string{"api_url"}, outputs); diff != "" {
		t.Errorf("outputs (-want +got):\n%s", diff)
	}
}

func TestRoutesRejectsAFunctionAndAServiceClaimingTheSameRouteKey(t *testing.T) {
	p := &ir.Project{
		Version:     1,
		Name:        "shop",
		Provider:    ir.ProviderAWS,
		Region:      "eu-west-2",
		Environment: "dev",
		Nodes: []ir.Node{
			{ID: "n1", Type: ir.NodeGateway, Name: "api"},
			{ID: "n2", Type: ir.NodeFunction, Name: "handler"},
			serviceNode(t, "n4", "web", defaultService),
		},
		Edges: []ir.Edge{
			{
				ID: "e1", From: "n1", To: "n2", Relation: ir.RelRoutes,
				Properties: ir.EdgeProperties{Path: "/web", Methods: []ir.Method{ir.MethodGet}},
			},
			serviceRouteEdge("e2", "/web", []ir.Method{ir.MethodGet}),
		},
	}
	want := ir.Errors{{
		EdgeID:  "e2",
		Message: "route 'GET /web' on gateway 'api' is already used by edge 'e1'",
	}}
	if diff := cmp.Diff(want, runErrors(t, p)); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
}
