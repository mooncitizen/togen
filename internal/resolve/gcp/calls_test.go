package gcp

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/mooncitizen/togen/internal/emit/hcl"
	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

func callEdges(from, to string, count int) []ir.Edge {
	edges := make([]ir.Edge, count)
	for i := range edges {
		edges[i] = ir.Edge{ID: "e" + string(rune('1'+i)), From: from, To: to, Relation: ir.RelCalls}
	}
	return edges
}

func runURL(name string) ir.Value {
	return ir.C(ir.Str("https://shop-dev-"+name+"-"), ir.D(projectID, ir.Field("number")), ir.Str(".europe-west2.run.app"))
}

func invokerBinding(name, location ir.Value, account ir.ID) ir.Attrs {
	return ir.Attrs{
		ir.A("name", name),
		ir.A("location", location),
		ir.A("role", ir.Str("roles/run.invoker")),
		ir.A("member", ir.C(ir.Str("serviceAccount:"), ir.R(account, ir.Field("email")))),
	}
}

func TestCallsToAServiceGrantsTheFunctionsAccountInvokerAndInjectsTheURL(t *testing.T) {
	ctx, project := newContext(t,
		[]ir.Node{functionNode(t, "n2", "handler", defaultFunction), serviceNode(t, "n4", "web", defaultService)},
		callEdges("n2", "n4", 1))
	caller := resolveFunction(ctx, project.Nodes[0])
	web := resolveService(ctx, project.Nodes[1])
	resolveCalls(ctx, project.Edges[0], caller, web)
	caller.Finalise()
	web.Finalise()

	binding := named(t, ctx, ir.ID{Type: "google_cloud_run_v2_service_iam_member", Name: "web_from_handler"})
	if binding.SourceNode != "n4" || binding.SourceLabel != "web" {
		t.Errorf("binding source = %q/%q", binding.SourceNode, binding.SourceLabel)
	}
	want := invokerBinding(ir.R(svcID, ir.Field("name")), ir.R(svcID, ir.Field("location")), fnAccountID)
	if diff := cmp.Diff(want, binding.Args); diff != "" {
		t.Errorf("binding args (-want +got):\n%s", diff)
	}
	wantEnv := ir.Attrs{ir.A("WEB_URL", runURL("web"))}
	if diff := cmp.Diff(wantEnv, caller.Env); diff != "" {
		t.Errorf("env (-want +got):\n%s", diff)
	}
	env, _ := serviceConfig(t, ctx).Get("environment_variables")
	if diff := cmp.Diff(ir.Value(ir.Map(wantEnv)), env); diff != "" {
		t.Errorf("environment_variables (-want +got):\n%s", diff)
	}
	if caller.NeedsNetwork || countOfType(ctx, "google_compute_network") != 0 {
		t.Error("calling a service pulled the caller onto the connector")
	}
	wantData := []ir.DataSource{{Type: "google_project", Name: "current", SourceLabel: "project"}}
	if diff := cmp.Diff(wantData, ctx.DataSources()); diff != "" {
		t.Errorf("data sources (-want +got):\n%s", diff)
	}
	if len(ctx.Errors) != 0 {
		t.Errorf("errors = %v", ctx.Errors)
	}
}

func TestCallsToAFunctionGrantsInvokerOnItsCloudRunService(t *testing.T) {
	ctx, project := newContext(t,
		[]ir.Node{serviceNode(t, "n4", "web", defaultService), functionNode(t, "n2", "handler", defaultFunction)},
		callEdges("n4", "n2", 1))
	web := resolveService(ctx, project.Nodes[0])
	fn := resolveFunction(ctx, project.Nodes[1])
	resolveCalls(ctx, project.Edges[0], web, fn)
	web.Finalise()
	fn.Finalise()

	binding := named(t, ctx, ir.ID{Type: "google_cloud_run_v2_service_iam_member", Name: "handler_from_web"})
	if binding.SourceNode != "n2" || binding.SourceLabel != "handler" {
		t.Errorf("binding source = %q/%q", binding.SourceNode, binding.SourceLabel)
	}
	want := invokerBinding(ir.R(fnID, ir.Field("name")), ir.R(fnID, ir.Field("location")), svcAccountID)
	if diff := cmp.Diff(want, binding.Args); diff != "" {
		t.Errorf("binding args (-want +got):\n%s", diff)
	}
	wantEnv := ir.Attrs{ir.A("HANDLER_URL", runURL("handler"))}
	if diff := cmp.Diff(wantEnv, web.Env); diff != "" {
		t.Errorf("env (-want +got):\n%s", diff)
	}
	env, _ := container(t, ctx).Get("env")
	wantBlock := ir.B(ir.Attrs{ir.A("name", ir.Str("HANDLER_URL")), ir.A("value", wantEnv[0].Value)})
	if diff := cmp.Diff(ir.Value(wantBlock), env); diff != "" {
		t.Errorf("container env (-want +got):\n%s", diff)
	}
}

func TestCallsBetweenTwoServicesAndBetweenTwoFunctionsWireTheSameWay(t *testing.T) {
	ctx, project := newContext(t, []ir.Node{
		serviceNode(t, "n4", "web", defaultService),
		serviceNode(t, "n5", "admin", defaultService),
		functionNode(t, "n2", "handler", defaultFunction),
		functionNode(t, "n10", "mailer", defaultFunction),
	}, []ir.Edge{
		{ID: "e1", From: "n5", To: "n4", Relation: ir.RelCalls},
		{ID: "e2", From: "n2", To: "n10", Relation: ir.RelCalls},
	})
	web := resolveService(ctx, project.Nodes[0])
	admin := resolveService(ctx, project.Nodes[1])
	handler := resolveFunction(ctx, project.Nodes[2])
	mailer := resolveFunction(ctx, project.Nodes[3])
	resolveCalls(ctx, project.Edges[0], admin, web)
	resolveCalls(ctx, project.Edges[1], handler, mailer)

	adminAccount := ir.ID{Type: "google_service_account", Name: "admin"}
	binding := named(t, ctx, ir.ID{Type: "google_cloud_run_v2_service_iam_member", Name: "web_from_admin"})
	want := invokerBinding(ir.R(svcID, ir.Field("name")), ir.R(svcID, ir.Field("location")), adminAccount)
	if diff := cmp.Diff(want, binding.Args); diff != "" {
		t.Errorf("service to service binding (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(ir.Attrs{ir.A("WEB_URL", runURL("web"))}, admin.Env); diff != "" {
		t.Errorf("admin env (-want +got):\n%s", diff)
	}

	mailerID := ir.ID{Type: "google_cloudfunctions2_function", Name: "mailer"}
	binding = named(t, ctx, ir.ID{Type: "google_cloud_run_v2_service_iam_member", Name: "mailer_from_handler"})
	want = invokerBinding(ir.R(mailerID, ir.Field("name")), ir.R(mailerID, ir.Field("location")), fnAccountID)
	if diff := cmp.Diff(want, binding.Args); diff != "" {
		t.Errorf("function to function binding (-want +got):\n%s", diff)
	}
	wantEnv := ir.Attrs{ir.A("MAILER_URL", runURL("mailer"))}
	if diff := cmp.Diff(wantEnv, handler.Env); diff != "" {
		t.Errorf("handler env (-want +got):\n%s", diff)
	}
}

func TestRepeatedCallsEdgesBindOnce(t *testing.T) {
	ctx, project := newContext(t,
		[]ir.Node{functionNode(t, "n2", "handler", defaultFunction), serviceNode(t, "n4", "web", defaultService)},
		callEdges("n2", "n4", 2))
	caller := resolveFunction(ctx, project.Nodes[0])
	web := resolveService(ctx, project.Nodes[1])
	for _, e := range project.Edges {
		resolveCalls(ctx, e, caller, web)
	}
	if got := countOfType(ctx, "google_cloud_run_v2_service_iam_member"); got != 1 {
		t.Errorf("bindings = %d, want 1", got)
	}
	if len(caller.Env) != 1 {
		t.Errorf("env = %v", caller.Env)
	}
}

func TestCallsReportsUnsupportedEnds(t *testing.T) {
	ctx, project := newContext(t, []ir.Node{
		functionNode(t, "n2", "handler", defaultFunction),
		databaseNode(t, "n3", "main-db", defaultDatabase),
	}, []ir.Edge{
		{ID: "e1", From: "n2", To: "n3", Relation: ir.RelCalls},
		{ID: "e2", From: "n3", To: "n2", Relation: ir.RelCalls},
	})
	fn := resolveFunction(ctx, project.Nodes[0])
	db := resolveDatabase(ctx, project.Nodes[1])
	resolveCalls(ctx, project.Edges[0], fn, db)
	resolveCalls(ctx, project.Edges[1], db, fn)

	want := ir.Errors{
		{EdgeID: "e1", Message: "calls to a database is not supported by the gcp resolver yet"},
		{EdgeID: "e2", Message: "calls from a database is not supported by the gcp resolver yet"},
	}
	if diff := cmp.Diff(want, ctx.Errors); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
	if len(fn.Env) != 0 || countOfType(ctx, "google_cloud_run_v2_service_iam_member") != 0 {
		t.Errorf("a refused edge wired the function: env %v", fn.Env)
	}
}

func TestResolveRunsACallFromAServiceToAFunction(t *testing.T) {
	g, err := resolve.Run(newProject(t, []ir.Node{
		serviceNode(t, "n4", "web", defaultService),
		functionNode(t, "n2", "handler", defaultFunction),
	}, callEdges("n4", "n2", 1)), New())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if errs := ir.ValidateGraph(g); len(errs) > 0 {
		t.Fatalf("graph is not valid:\n%s", errs.Error())
	}
	var names []string
	for _, r := range g.Resources {
		if r.Type == "google_cloud_run_v2_service_iam_member" {
			names = append(names, r.Name)
		}
	}
	if diff := cmp.Diff([]string{"handler_from_web"}, names); diff != "" {
		t.Errorf("bindings (-want +got):\n%s", diff)
	}
}

func TestResolveRunsServicesThatCallEachOther(t *testing.T) {
	g, err := resolve.Run(newProject(t, []ir.Node{
		serviceNode(t, "n4", "web", defaultService),
		serviceNode(t, "n5", "admin", defaultService),
	}, []ir.Edge{
		{ID: "e1", From: "n4", To: "n5", Relation: ir.RelCalls},
		{ID: "e2", From: "n5", To: "n4", Relation: ir.RelCalls},
	}), New())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if errs := ir.ValidateGraph(g); len(errs) > 0 {
		t.Fatalf("graph is not valid:\n%s", errs.Error())
	}
	if diff := cmp.Diff([]string{"google_project"}, dataTypes(g.Data)); diff != "" {
		t.Errorf("data sources (-want +got):\n%s", diff)
	}
	files, err := hcl.Emit(g)
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	main := string(files["main.tf"])
	for _, want := range []string{
		`data "google_project" "current"`,
		`value = "https://shop-dev-admin-${data.google_project.current.number}.europe-west2.run.app"`,
		`value = "https://shop-dev-web-${data.google_project.current.number}.europe-west2.run.app"`,
	} {
		if !strings.Contains(main, want) {
			t.Errorf("main.tf lacks %q", want)
		}
	}
	if strings.Contains(main, ".uri") {
		t.Error("main.tf still reads a URL off a Cloud Run resource")
	}
	terraformFmt(t, files)
}
