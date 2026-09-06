package azure

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/mooncitizen/togen/internal/ir"
)

var (
	mailerID  = ir.ID{Type: "azurerm_linux_function_app", Name: "mailer"}
	mailerURL = ir.C(ir.Str("https://"), ir.R(mailerID, ir.Field("default_hostname")))
	adminID   = ir.ID{Type: "azurerm_container_app", Name: "admin_ui"}
	adminURL  = ir.C(ir.Str("https://"), ir.R(adminID, ir.Field("ingress"), ir.Index(0), ir.Field("fqdn")))
)

func callEdge(id, from, to string) ir.Edge {
	return ir.Edge{ID: id, From: from, To: to, Relation: ir.RelCalls}
}

func TestCallsToAPrivateServiceGiveTheCallerItsURLAndAskForTheNetwork(t *testing.T) {
	ctx := newContext(t, []ir.Node{
		functionNode(t, "n2", "handler", defaultFunction),
		serviceNodeWith(t, "n4", "web", defaultService),
	})
	fn := resolveFunction(ctx, ctx.Project.Nodes[0])
	svc := resolveService(ctx, ctx.Project.Nodes[1])
	before := len(ctx.Resources())
	resolveCalls(ctx, callEdge("e1", "n2", "n4"), fn, svc)
	fn.Finalise()
	svc.Finalise()

	if len(ctx.Errors) != 0 {
		t.Fatalf("errors = %v", ctx.Errors)
	}
	if diff := cmp.Diff(ir.Value(webURL), appSetting(t, ctx, "WEB_URL")); diff != "" {
		t.Errorf("WEB_URL (-want +got):\n%s", diff)
	}
	if !fn.NeedsNetwork {
		t.Error("calling a private service did not ask for the network")
	}
	if got := len(ctx.Resources()); got != before {
		t.Errorf("the edge added resources: %d, was %d", got, before)
	}
	if external, _ := appIngress(t, ctx, appID).Get("external_enabled"); external != ir.Bool(false) {
		t.Errorf("a call made the ingress external: %v", external)
	}
}

func TestCallsToAPublicServiceNeedNoNetwork(t *testing.T) {
	p := defaultService
	p.Public = true
	ctx := newContext(t, []ir.Node{
		functionNode(t, "n2", "handler", defaultFunction),
		serviceNodeWith(t, "n4", "web", p),
	})
	fn := resolveFunction(ctx, ctx.Project.Nodes[0])
	svc := resolveService(ctx, ctx.Project.Nodes[1])
	resolveCalls(ctx, callEdge("e1", "n2", "n4"), fn, svc)
	fn.Finalise()

	if diff := cmp.Diff(ir.Value(webURL), appSetting(t, ctx, "WEB_URL")); diff != "" {
		t.Errorf("WEB_URL (-want +got):\n%s", diff)
	}
	if fn.NeedsNetwork {
		t.Error("calling a public service asked for the network")
	}
}

func TestCallsToAFunctionSetItsHostnameOnAServiceOrAFunction(t *testing.T) {
	ctx := newContext(t, []ir.Node{
		serviceNodeWith(t, "n4", "web", defaultService),
		functionNode(t, "n2", "handler", defaultFunction),
		functionNode(t, "n10", "mailer", defaultFunction),
	})
	svc := resolveService(ctx, ctx.Project.Nodes[0])
	handler := resolveFunction(ctx, ctx.Project.Nodes[1])
	mailer := resolveFunction(ctx, ctx.Project.Nodes[2])
	resolveCalls(ctx, callEdge("e1", "n4", "n10"), svc, mailer)
	resolveCalls(ctx, callEdge("e2", "n2", "n10"), handler, mailer)
	svc.Finalise()
	handler.Finalise()

	if len(ctx.Errors) != 0 {
		t.Fatalf("errors = %v", ctx.Errors)
	}
	wantEnv := ir.Attrs{ir.A("MAILER_URL", mailerURL)}
	if diff := cmp.Diff(wantEnv, containerEnvOf(t, ctx, appID)); diff != "" {
		t.Errorf("container env (-want +got):\n%s", diff)
	}
	settings, _ := firstOfType(t, ctx, "azurerm_linux_function_app").Args.Get("app_settings")
	wantSettings := ir.M(
		ir.A("AzureFunctionsJobHost__functionTimeout", ir.Str("00:00:30")),
		ir.A("MAILER_URL", mailerURL),
	)
	if diff := cmp.Diff(ir.Value(wantSettings), settings); diff != "" {
		t.Errorf("app_settings (-want +got):\n%s", diff)
	}
	if svc.NeedsNetwork || handler.NeedsNetwork {
		t.Error("calling a function asked for the network")
	}
}

func TestCallsBetweenServicesUseTheIngressFQDN(t *testing.T) {
	ctx := newContext(t, []ir.Node{
		serviceNodeWith(t, "n4", "web", defaultService),
		serviceNodeWith(t, "n5", "admin-ui", defaultService),
	})
	web := resolveService(ctx, ctx.Project.Nodes[0])
	admin := resolveService(ctx, ctx.Project.Nodes[1])
	resolveCalls(ctx, callEdge("e1", "n4", "n5"), web, admin)
	resolveCalls(ctx, callEdge("e2", "n5", "n4"), admin, web)
	web.Finalise()
	admin.Finalise()

	if diff := cmp.Diff(ir.Attrs{ir.A("ADMIN_UI_URL", adminURL)}, containerEnvOf(t, ctx, appID)); diff != "" {
		t.Errorf("web env (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(ir.Attrs{ir.A("WEB_URL", webURL)}, containerEnvOf(t, ctx, adminID)); diff != "" {
		t.Errorf("admin-ui env (-want +got):\n%s", diff)
	}
	if got := countOfType(ctx, "azurerm_container_app_environment"); got != 1 {
		t.Errorf("environments = %d", got)
	}
}

func TestRepeatedCallsEdgesSetTheURLOnce(t *testing.T) {
	ctx := newContext(t, []ir.Node{
		functionNode(t, "n2", "handler", defaultFunction),
		serviceNodeWith(t, "n4", "web", defaultService),
	})
	fn := resolveFunction(ctx, ctx.Project.Nodes[0])
	svc := resolveService(ctx, ctx.Project.Nodes[1])
	resolveCalls(ctx, callEdge("e1", "n2", "n4"), fn, svc)
	resolveCalls(ctx, callEdge("e2", "n2", "n4"), fn, svc)

	if len(fn.Env) != 2 {
		t.Errorf("env = %v", fn.Env)
	}
}

func TestCallsReportsUnsupportedEnds(t *testing.T) {
	ctx := newContext(t, []ir.Node{
		gatewayNode,
		functionNode(t, "n2", "handler", defaultFunction),
		databaseNode(t, "n3", "main-db", smallPostgres),
	})
	gateway := resolveGateway(ctx.Project.Nodes[0])
	fn := resolveFunction(ctx, ctx.Project.Nodes[1])
	db := resolveDatabase(ctx, ctx.Project.Nodes[2])
	resolveCalls(ctx, callEdge("e1", "n1", "n2"), gateway, fn)
	resolveCalls(ctx, callEdge("e2", "n2", "n3"), fn, db)

	want := ir.Errors{
		{EdgeID: "e1", Message: "calls from a gateway is not supported by the azure resolver yet"},
		{EdgeID: "e2", Message: "calls to a database is not supported by the azure resolver yet"},
	}
	if diff := cmp.Diff(want, ctx.Errors); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
	if len(fn.Env) != 1 || fn.NeedsNetwork {
		t.Errorf("the function was wired anyway: env = %v, needs network = %v", fn.Env, fn.NeedsNetwork)
	}
}
