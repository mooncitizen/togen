package aws

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/mooncitizen/togen/internal/ir"
)

var (
	mailerID     = ir.ID{Type: "aws_lambda_function", Name: "mailer"}
	namespaceID  = ir.ID{Type: "aws_service_discovery_private_dns_namespace", Name: "main"}
	webRegistry  = ir.ID{Type: "aws_service_discovery_service", Name: "web"}
	invokeMailer = ir.M(
		ir.A("Effect", ir.Str("Allow")),
		ir.A("Action", ir.L(ir.Str("lambda:InvokeFunction"))),
		ir.A("Resource", ir.R(mailerID, ir.Field("arn"))),
	)
	mailerEnv = ir.Attrs{ir.A("MAILER_FUNCTION_NAME", ir.R(mailerID, ir.Field("function_name")))}
	webURL    = ir.Attrs{ir.A("WEB_URL", ir.Str("http://web.shop-dev.local:8080"))}
)

func mailerNode(t *testing.T) ir.Node {
	t.Helper()
	return ir.Node{ID: "n10", Type: ir.NodeFunction, Name: "mailer", Properties: props(t, defaultFunction)}
}

func handlerNode(t *testing.T) ir.Node {
	t.Helper()
	return ir.Node{ID: "n2", Type: ir.NodeFunction, Name: "handler", Properties: props(t, defaultFunction)}
}

func callEdges(from, to string, count int) []ir.Edge {
	edges := make([]ir.Edge, count)
	for i := range edges {
		edges[i] = ir.Edge{ID: "e" + string(rune('1'+i)), From: from, To: to, Relation: ir.RelCalls}
	}
	return edges
}

func TestCallsToAFunctionGrantsInvokeAndInjectsTheFunctionName(t *testing.T) {
	ctx, project := newContext(t,
		[]ir.Node{handlerNode(t), mailerNode(t)},
		callEdges("n2", "n10", 1))
	caller := resolveFunction(ctx, project.Nodes[0])
	callee := resolveFunction(ctx, project.Nodes[1])
	resolveCalls(ctx, project.Edges[0], caller, callee)
	caller.Finalise()

	if diff := cmp.Diff([]ir.Value{invokeMailer}, caller.Statements); diff != "" {
		t.Errorf("statements (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(mailerEnv, caller.Env); diff != "" {
		t.Errorf("env (-want +got):\n%s", diff)
	}

	policy := named(t, ctx, ir.ID{Type: "aws_iam_role_policy", Name: "handler"})
	role, _ := policy.Args.Get("role")
	if diff := cmp.Diff(ir.Value(ir.R(fnRoleID, ir.Field("id"))), role); diff != "" {
		t.Errorf("policy role (-want +got):\n%s", diff)
	}

	if caller.NeedsNetwork || caller.SecurityGroup != nil {
		t.Error("calling a function pulled the caller into the vpc")
	}
	for _, typ := range []string{"aws_vpc", "aws_security_group", "aws_service_discovery_service"} {
		if got := countOfType(ctx, typ); got != 0 {
			t.Errorf("%s = %d", typ, got)
		}
	}
}

func TestCallsToAFunctionFromAServiceLandOnTheTaskRole(t *testing.T) {
	ctx, project := newContext(t,
		[]ir.Node{serviceNode(t, "n4", "web", defaultService), mailerNode(t)},
		callEdges("n4", "n10", 1))
	svc := resolveService(ctx, project.Nodes[0])
	callee := resolveFunction(ctx, project.Nodes[1])
	resolveCalls(ctx, project.Edges[0], svc, callee)
	svc.Finalise()

	policy := named(t, ctx, ir.ID{Type: "aws_iam_role_policy", Name: "web"})
	role, _ := policy.Args.Get("role")
	if diff := cmp.Diff(ir.Value(ir.R(svcRoleID, ir.Field("id"))), role); diff != "" {
		t.Errorf("policy role (-want +got):\n%s", diff)
	}
	statements, _ := policy.Args.Get("policy")
	want := ir.J(ir.M(
		ir.A("Version", ir.Str("2012-10-17")),
		ir.A("Statement", ir.L(invokeMailer)),
	))
	if diff := cmp.Diff(ir.Value(want), statements); diff != "" {
		t.Errorf("policy statements (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(mailerEnv, svc.Env); diff != "" {
		t.Errorf("env (-want +got):\n%s", diff)
	}
	if countOfType(ctx, "aws_service_discovery_private_dns_namespace") != 0 {
		t.Error("calling a function created a dns namespace")
	}
}

func TestCallsToAServiceRegisterItInCloudMapAndOpenThePort(t *testing.T) {
	ctx, project := newContext(t,
		[]ir.Node{serviceNode(t, "n4", "web", defaultService), handlerNode(t)},
		callEdges("n2", "n4", 1))
	svc := resolveService(ctx, project.Nodes[0])
	caller := resolveFunction(ctx, project.Nodes[1])
	resolveCalls(ctx, project.Edges[0], caller, svc)
	caller.Finalise()

	namespace := named(t, ctx, namespaceID)
	if namespace.SourceNode != "" || namespace.SourceLabel != "network" {
		t.Errorf("namespace source = %q/%q", namespace.SourceNode, namespace.SourceLabel)
	}
	wantNamespace := ir.Attrs{
		ir.A("name", ir.Str("shop-dev.local")),
		ir.A("description", ir.Str("Private DNS for shop-dev")),
		ir.A("vpc", ir.R(ir.ID{Type: "aws_vpc", Name: "main"}, ir.Field("id"))),
	}
	if diff := cmp.Diff(wantNamespace, namespace.Args); diff != "" {
		t.Errorf("namespace args (-want +got):\n%s", diff)
	}

	registry := named(t, ctx, webRegistry)
	if registry.SourceNode != "n4" || registry.SourceLabel != "web" {
		t.Errorf("registry source = %q/%q", registry.SourceNode, registry.SourceLabel)
	}
	wantRegistry := ir.Attrs{
		ir.A("name", ir.Str("web")),
		ir.A("dns_config", ir.B(ir.Attrs{
			ir.A("namespace_id", ir.R(namespaceID, ir.Field("id"))),
			ir.A("routing_policy", ir.Str("MULTIVALUE")),
			ir.A("dns_records", ir.B(ir.Attrs{
				ir.A("ttl", ir.Num(10)),
				ir.A("type", ir.Str("A")),
			})),
		})),
		ir.A("health_check_custom_config", ir.B(ir.Attrs{})),
	}
	if diff := cmp.Diff(wantRegistry, registry.Args); diff != "" {
		t.Errorf("registry args (-want +got):\n%s", diff)
	}

	registries, _ := named(t, ctx, svcID).Args.Get("service_registries")
	wantRegistries := ir.B(ir.Attrs{ir.A("registry_arn", ir.R(webRegistry, ir.Field("arn")))})
	if diff := cmp.Diff(ir.Value(wantRegistries), registries); diff != "" {
		t.Errorf("service_registries (-want +got):\n%s", diff)
	}

	rule := named(t, ctx, ir.ID{Type: "aws_vpc_security_group_ingress_rule", Name: "web_from_handler"})
	if rule.SourceNode != "n4" || rule.SourceLabel != "web" {
		t.Errorf("rule source = %q/%q", rule.SourceNode, rule.SourceLabel)
	}
	wantRule := ir.Attrs{
		ir.A("security_group_id", ir.R(svcSGID, ir.Field("id"))),
		ir.A("referenced_security_group_id", ir.R(fnSGID, ir.Field("id"))),
		ir.A("from_port", ir.Num(8080)),
		ir.A("to_port", ir.Num(8080)),
		ir.A("ip_protocol", ir.Str("tcp")),
		ir.A("description", ir.Str("handler to web")),
	}
	if diff := cmp.Diff(wantRule, rule.Args); diff != "" {
		t.Errorf("ingress rule args (-want +got):\n%s", diff)
	}

	if !caller.NeedsNetwork {
		t.Error("the caller did not join the vpc")
	}
	if _, ok := named(t, ctx, fnID).Args.Get("vpc_config"); !ok {
		t.Error("the caller has no vpc_config")
	}
	if diff := cmp.Diff(webURL, caller.Env); diff != "" {
		t.Errorf("env (-want +got):\n%s", diff)
	}
	if len(caller.Statements) != 0 {
		t.Errorf("statements = %v", caller.Statements)
	}
}

func TestCallsToAServiceFromAServiceUseTheCallersOwnSecurityGroup(t *testing.T) {
	ctx, project := newContext(t, []ir.Node{
		serviceNode(t, "n4", "web", defaultService),
		serviceNode(t, "n5", "admin", defaultService),
	}, callEdges("n5", "n4", 1))
	web := resolveService(ctx, project.Nodes[0])
	admin := resolveService(ctx, project.Nodes[1])
	resolveCalls(ctx, project.Edges[0], admin, web)
	admin.Finalise()

	rule := named(t, ctx, ir.ID{Type: "aws_vpc_security_group_ingress_rule", Name: "web_from_admin"})
	referenced, _ := rule.Args.Get("referenced_security_group_id")
	adminSG := ir.ID{Type: "aws_security_group", Name: "admin"}
	if diff := cmp.Diff(ir.Value(ir.R(adminSG, ir.Field("id"))), referenced); diff != "" {
		t.Errorf("referenced security group (-want +got):\n%s", diff)
	}
	if got := countOfType(ctx, "aws_service_discovery_private_dns_namespace"); got != 1 {
		t.Errorf("namespaces = %d", got)
	}
	if _, ok := named(t, ctx, ir.ID{Type: "aws_ecs_service", Name: "admin"}).Args.Get("service_registries"); ok {
		t.Error("the caller was registered in cloud map")
	}

	definitions, _ := named(t, ctx, ir.ID{Type: "aws_ecs_task_definition", Name: "admin"}).Args.Get("container_definitions")
	container := ir.Attrs(definitions.(ir.JSON).Value.(ir.List)[0].(ir.Map))
	env, _ := container.Get("environment")
	wantEnv := ir.L(ir.M(
		ir.A("name", ir.Str("WEB_URL")),
		ir.A("value", ir.Str("http://web.shop-dev.local:8080")),
	))
	if diff := cmp.Diff(ir.Value(wantEnv), env); diff != "" {
		t.Errorf("container environment (-want +got):\n%s", diff)
	}
}

func TestTwoCallersOfOneServiceShareTheNamespaceAndTheRegistration(t *testing.T) {
	ctx, project := newContext(t, []ir.Node{
		serviceNode(t, "n4", "web", defaultService),
		handlerNode(t),
		{ID: "n6", Type: ir.NodeFunction, Name: "worker", Properties: props(t, defaultFunction)},
	}, []ir.Edge{
		{ID: "e1", From: "n2", To: "n4", Relation: ir.RelCalls},
		{ID: "e2", From: "n6", To: "n4", Relation: ir.RelCalls},
	})
	web := resolveService(ctx, project.Nodes[0])
	handler := resolveFunction(ctx, project.Nodes[1])
	worker := resolveFunction(ctx, project.Nodes[2])
	resolveCalls(ctx, project.Edges[0], handler, web)
	resolveCalls(ctx, project.Edges[1], worker, web)

	for typ, want := range map[string]int{
		"aws_service_discovery_private_dns_namespace": 1,
		"aws_service_discovery_service":               1,
		"aws_vpc_security_group_ingress_rule":         2,
	} {
		if got := countOfType(ctx, typ); got != want {
			t.Errorf("%s = %d, want %d", typ, got, want)
		}
	}
	named(t, ctx, ir.ID{Type: "aws_vpc_security_group_ingress_rule", Name: "web_from_worker"})
	if diff := cmp.Diff(webURL, worker.Env); diff != "" {
		t.Errorf("worker env (-want +got):\n%s", diff)
	}
}

func TestRepeatedCallsEdgesWireEverythingOnce(t *testing.T) {
	ctx, project := newContext(t,
		[]ir.Node{serviceNode(t, "n4", "web", defaultService), handlerNode(t), mailerNode(t)},
		append(callEdges("n2", "n4", 2), callEdges("n2", "n10", 2)...))
	web := resolveService(ctx, project.Nodes[0])
	caller := resolveFunction(ctx, project.Nodes[1])
	mailer := resolveFunction(ctx, project.Nodes[2])
	for _, e := range project.Edges[:2] {
		resolveCalls(ctx, e, caller, web)
	}
	for _, e := range project.Edges[2:] {
		resolveCalls(ctx, e, caller, mailer)
	}

	for typ, want := range map[string]int{
		"aws_service_discovery_private_dns_namespace": 1,
		"aws_service_discovery_service":               1,
		"aws_vpc_security_group_ingress_rule":         1,
		"aws_security_group":                          2,
	} {
		if got := countOfType(ctx, typ); got != want {
			t.Errorf("%s = %d, want %d", typ, got, want)
		}
	}
	if len(caller.Statements) != 1 {
		t.Errorf("statements = %v", caller.Statements)
	}
	if len(caller.Env) != 2 {
		t.Errorf("env = %v", caller.Env)
	}
}

func TestCallsReportsAnUnsupportedTarget(t *testing.T) {
	ctx, project := newContext(t,
		[]ir.Node{handlerNode(t), queueNode(t, "n5", "jobs", defaultQueue)},
		callEdges("n2", "n5", 1))
	fn := resolveFunction(ctx, project.Nodes[0])
	queue := resolveQueue(ctx, project.Nodes[1])
	resolveCalls(ctx, project.Edges[0], fn, queue)

	want := ir.Errors{{
		EdgeID:  "e1",
		Message: "calls to a queue is not supported by the aws resolver yet",
	}}
	if diff := cmp.Diff(want, ctx.Errors); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
}

func TestCallsReportsAnUnsupportedSource(t *testing.T) {
	ctx, project := newContext(t,
		[]ir.Node{{ID: "n3", Type: ir.NodeDatabase, Name: "main-db"}, mailerNode(t)},
		callEdges("n3", "n10", 1))
	db := resolveDatabase(ctx, project.Nodes[0])
	fn := resolveFunction(ctx, project.Nodes[1])
	resolveCalls(ctx, project.Edges[0], db, fn)

	want := ir.Errors{{
		EdgeID:  "e1",
		Message: "calls from a database is not supported by the aws resolver yet",
	}}
	if diff := cmp.Diff(want, ctx.Errors); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
}
