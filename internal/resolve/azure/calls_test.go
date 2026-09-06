package azure

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/mooncitizen/togen/internal/emit/hcl"
	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

var (
	mailerURL = ir.Str("https://shop-dev-mailer.azurewebsites.net")
	adminID   = ir.ID{Type: "azurerm_container_app", Name: "admin_ui"}
)

// The hostnames a caller is given name the environment, never the target app.
func callURL(app string, public bool) ir.Value {
	host := "https://shop-dev-" + app + "."
	if !public {
		host += "internal."
	}
	return ir.C(ir.Str(host), ir.R(environmentID, ir.Field("default_domain")))
}

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
	if diff := cmp.Diff(callURL("web", false), appSetting(t, ctx, "WEB_URL")); diff != "" {
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

	if diff := cmp.Diff(callURL("web", true), appSetting(t, ctx, "WEB_URL")); diff != "" {
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

func TestMutualCallsNameTheEnvironmentAndNotEachOther(t *testing.T) {
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

	if diff := cmp.Diff(ir.Attrs{ir.A("ADMIN_UI_URL", callURL("admin-ui", false))}, containerEnvOf(t, ctx, appID)); diff != "" {
		t.Errorf("web env (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(ir.Attrs{ir.A("WEB_URL", callURL("web", false))}, containerEnvOf(t, ctx, adminID)); diff != "" {
		t.Errorf("admin-ui env (-want +got):\n%s", diff)
	}
	if got := countOfType(ctx, "azurerm_container_app_environment"); got != 1 {
		t.Errorf("environments = %d", got)
	}
}

func TestMutualCallsEmitWithoutACycle(t *testing.T) {
	p := newProject(t, []ir.Node{
		serviceNodeWith(t, "n4", "web", defaultService),
		serviceNodeWith(t, "n5", "admin-ui", defaultService),
	})
	p.Edges = []ir.Edge{callEdge("e1", "n4", "n5"), callEdge("e2", "n5", "n4")}
	g, err := resolve.Run(p, New())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if errs := ir.ValidateGraph(g); len(errs) > 0 {
		t.Errorf("graph is not valid:\n%s", errs.Error())
	}
	files, err := hcl.Emit(g)
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	main := unaligned(files["main.tf"])
	for _, want := range []string{
		`env { name = "ADMIN_UI_URL" value = "https://shop-dev-admin-ui.internal.${azurerm_container_app_environment.main.default_domain}" }`,
		`env { name = "WEB_URL" value = "https://shop-dev-web.internal.${azurerm_container_app_environment.main.default_domain}" }`,
	} {
		if !strings.Contains(main, want) {
			t.Errorf("main.tf lacks %s:\n%s", want, files["main.tf"])
		}
	}
	for _, absent := range []string{"azurerm_container_app.web.", "azurerm_container_app.admin_ui."} {
		if strings.Contains(main, absent) {
			t.Errorf("main.tf refers to %s:\n%s", absent, files["main.tf"])
		}
	}
}

func TestARouteAfterACallGivesTheCallerTheExternalHostname(t *testing.T) {
	ctx := newContext(t, []ir.Node{
		gatewayNode,
		functionNode(t, "n2", "handler", defaultFunction),
		serviceNodeWith(t, "n4", "web", defaultService),
	})
	gateway := resolveGateway(ctx.Project.Nodes[0])
	fn := resolveFunction(ctx, ctx.Project.Nodes[1])
	svc := resolveService(ctx, ctx.Project.Nodes[2])
	resolveCalls(ctx, callEdge("e1", "n2", "n4"), fn, svc)
	resolveRoutes(ctx, routeEdge("e2", "n4", "/web", ir.MethodGet), gateway, svc)
	fn.Finalise()
	svc.Finalise()

	if diff := cmp.Diff(callURL("web", true), appSetting(t, ctx, "WEB_URL")); diff != "" {
		t.Errorf("WEB_URL (-want +got):\n%s", diff)
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
