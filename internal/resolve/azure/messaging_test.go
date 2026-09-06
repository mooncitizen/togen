package azure

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

func setupMessaging(t *testing.T, relations ...ir.Relation) (*resolve.Context, *resolve.Handle) {
	t.Helper()
	ctx := newContext(t, []ir.Node{
		functionNode(t, "n6", "worker", defaultFunction),
		queueNode(t, "n5", "jobs", defaultQueue),
	})
	from := resolveFunction(ctx, ctx.Project.Nodes[0])
	queue := resolveQueue(ctx, ctx.Project.Nodes[1])
	for i, r := range relations {
		edge := ir.Edge{ID: "e" + string(rune('1'+i)), From: "n6", To: "n5", Relation: r}
		resolveMessaging(ctx, edge, from, queue)
	}
	from.Finalise()
	return ctx, from
}

var (
	workerID        = ir.ID{Type: "azurerm_linux_function_app", Name: "worker"}
	workerPrincipal = ir.R(workerID, ir.Field("identity"), ir.Index(0), ir.Field("principal_id"))
	webPrincipal    = ir.R(appID, ir.Field("identity"), ir.Index(0), ir.Field("principal_id"))
	timeoutAttr     = ir.A("AzureFunctionsJobHost__functionTimeout", ir.Str("00:00:30"))
)

func roleAssignment(role string, principal ir.Value) ir.Attrs {
	return ir.Attrs{
		ir.A("scope", ir.R(queueID, ir.Field("id"))),
		ir.A("role_definition_name", ir.Str(role)),
		ir.A("principal_id", principal),
		ir.A("principal_type", ir.Str("ServicePrincipal")),
	}
}

func TestPublishesFromAFunctionGrantsSendAndSetsTheQueueAndNamespace(t *testing.T) {
	ctx, from := setupMessaging(t, ir.RelPublishes)
	if len(ctx.Errors) != 0 {
		t.Fatalf("errors = %v", ctx.Errors)
	}

	assignment := firstOfType(t, ctx, "azurerm_role_assignment")
	if assignment.Name != "worker_jobs_send" || assignment.SourceNode != "n6" || assignment.SourceLabel != "worker" {
		t.Errorf("assignment = %q %q %q", assignment.Name, assignment.SourceNode, assignment.SourceLabel)
	}
	if diff := cmp.Diff(roleAssignment("Azure Service Bus Data Sender", workerPrincipal), assignment.Args); diff != "" {
		t.Errorf("assignment args (-want +got):\n%s", diff)
	}
	settings, _ := firstOfType(t, ctx, "azurerm_linux_function_app").Args.Get("app_settings")
	want := ir.M(
		timeoutAttr,
		ir.A("JOBS_QUEUE", ir.R(queueID, ir.Field("name"))),
		ir.A("JOBS_NAMESPACE", busFQDN),
	)
	if diff := cmp.Diff(ir.Value(want), settings); diff != "" {
		t.Errorf("app_settings (-want +got):\n%s", diff)
	}
	if from.NeedsNetwork {
		t.Error("a publisher asked for the network")
	}
	if got := countOfType(ctx, "azurerm_role_assignment"); got != 1 {
		t.Errorf("role assignments = %d", got)
	}
}

func TestConsumesFromAFunctionGrantsReceiveAndSetsTheTriggerConnection(t *testing.T) {
	ctx, _ := setupMessaging(t, ir.RelConsumes)
	if len(ctx.Errors) != 0 {
		t.Fatalf("errors = %v", ctx.Errors)
	}

	assignment := firstOfType(t, ctx, "azurerm_role_assignment")
	if assignment.Name != "worker_jobs_receive" {
		t.Errorf("assignment name = %q", assignment.Name)
	}
	if diff := cmp.Diff(roleAssignment("Azure Service Bus Data Receiver", workerPrincipal), assignment.Args); diff != "" {
		t.Errorf("assignment args (-want +got):\n%s", diff)
	}
	settings, _ := firstOfType(t, ctx, "azurerm_linux_function_app").Args.Get("app_settings")
	want := ir.M(
		timeoutAttr,
		ir.A("JOBS__fullyQualifiedNamespace", busFQDN),
		ir.A("JOBS_QUEUE", ir.R(queueID, ir.Field("name"))),
	)
	if diff := cmp.Diff(ir.Value(want), settings); diff != "" {
		t.Errorf("app_settings (-want +got):\n%s", diff)
	}
}

func TestPublishesAndConsumesBetweenTheSamePairWireEverythingOnce(t *testing.T) {
	ctx, from := setupMessaging(t, ir.RelConsumes, ir.RelConsumes, ir.RelPublishes, ir.RelPublishes)

	var names []string
	for _, r := range byType(ctx, "azurerm_role_assignment") {
		names = append(names, r.Name)
	}
	if diff := cmp.Diff([]string{"worker_jobs_receive", "worker_jobs_send"}, names); diff != "" {
		t.Errorf("role assignments (-want +got):\n%s", diff)
	}
	// The timeout, the trigger connection, the queue and the namespace.
	if len(from.Env) != 4 {
		t.Errorf("env = %v", from.Env)
	}
}

func TestMessagingFromAServiceGrantsTheRoleAndSetsContainerEnv(t *testing.T) {
	for _, tc := range []struct {
		relation ir.Relation
		name     string
		role     string
	}{
		{ir.RelPublishes, "web_jobs_send", "Azure Service Bus Data Sender"},
		{ir.RelConsumes, "web_jobs_receive", "Azure Service Bus Data Receiver"},
	} {
		ctx := newContext(t, []ir.Node{
			serviceNodeWith(t, "n4", "web", defaultService),
			queueNode(t, "n5", "jobs", defaultQueue),
		})
		svc := resolveService(ctx, ctx.Project.Nodes[0])
		queue := resolveQueue(ctx, ctx.Project.Nodes[1])
		resolveMessaging(ctx, ir.Edge{ID: "e1", From: "n4", To: "n5", Relation: tc.relation}, svc, queue)
		svc.Finalise()

		if len(ctx.Errors) != 0 {
			t.Fatalf("%s: errors = %v", tc.relation, ctx.Errors)
		}
		assignment := firstOfType(t, ctx, "azurerm_role_assignment")
		if assignment.Name != tc.name {
			t.Errorf("%s assignment name = %q", tc.relation, assignment.Name)
		}
		if diff := cmp.Diff(roleAssignment(tc.role, webPrincipal), assignment.Args); diff != "" {
			t.Errorf("%s assignment args (-want +got):\n%s", tc.relation, diff)
		}
		wantEnv := ir.Attrs{
			ir.A("JOBS_QUEUE", ir.R(queueID, ir.Field("name"))),
			ir.A("JOBS_NAMESPACE", busFQDN),
		}
		if diff := cmp.Diff(wantEnv, containerEnvOf(t, ctx, appID)); diff != "" {
			t.Errorf("%s container env (-want +got):\n%s", tc.relation, diff)
		}
		if svc.NeedsNetwork {
			t.Errorf("%s asked for the network", tc.relation)
		}
	}
}

func TestMessagingReportsAnUnsupportedTarget(t *testing.T) {
	ctx := newContext(t, []ir.Node{
		functionNode(t, "n2", "handler", defaultFunction),
		databaseNode(t, "n3", "main-db", smallPostgres),
	})
	fn := resolveFunction(ctx, ctx.Project.Nodes[0])
	db := resolveDatabase(ctx, ctx.Project.Nodes[1])
	before := len(ctx.Resources())
	resolveMessaging(ctx, ir.Edge{ID: "e1", From: "n2", To: "n3", Relation: ir.RelPublishes}, fn, db)

	want := ir.Errors{{
		EdgeID:  "e1",
		Message: "publishes to a database is not supported by the azure resolver yet",
	}}
	if diff := cmp.Diff(want, ctx.Errors); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
	if len(fn.Env) != 1 || len(ctx.Resources()) != before {
		t.Errorf("the function was wired anyway: env = %v", fn.Env)
	}
}

func TestMessagingReportsAnUnsupportedSource(t *testing.T) {
	ctx := newContext(t, []ir.Node{gatewayNode, queueNode(t, "n5", "jobs", defaultQueue)})
	gateway := resolveGateway(ctx.Project.Nodes[0])
	queue := resolveQueue(ctx, ctx.Project.Nodes[1])
	resolveMessaging(ctx, ir.Edge{ID: "e1", From: "n1", To: "n5", Relation: ir.RelConsumes}, gateway, queue)

	want := ir.Errors{{
		EdgeID:  "e1",
		Message: "consumes from a gateway is not supported by the azure resolver yet",
	}}
	if diff := cmp.Diff(want, ctx.Errors); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
	if countOfType(ctx, "azurerm_role_assignment") != 0 {
		t.Error("a gateway was given a role assignment")
	}
}
