package aws

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

var workerID = ir.ID{Type: "aws_lambda_function", Name: "worker"}

func setupMessaging(
	t *testing.T,
	fn ir.FunctionProps,
	relations ...ir.Relation,
) (*resolve.Context, *resolve.Handle) {
	t.Helper()
	edges := make([]ir.Edge, len(relations))
	for i, r := range relations {
		edges[i] = ir.Edge{ID: "e" + string(rune('1'+i)), From: "n6", To: "n5", Relation: r}
	}
	ctx, project := newContext(t, []ir.Node{
		{ID: "n6", Type: ir.NodeFunction, Name: "worker", Properties: props(t, fn)},
		queueNode(t, "n5", "jobs", defaultQueue),
	}, edges)
	from := resolveFunction(ctx, project.Nodes[0])
	queue := resolveQueue(ctx, project.Nodes[1])
	for _, e := range project.Edges {
		resolveMessaging(ctx, e, from, queue)
	}
	return ctx, from
}

func TestPublishesGrantsSendAndInjectsTheQueueURL(t *testing.T) {
	ctx, from := setupMessaging(t, defaultFunction, ir.RelPublishes)

	wantStatements := []ir.Value{ir.M(
		ir.A("Effect", ir.Str("Allow")),
		ir.A("Action", ir.L(
			ir.Str("sqs:SendMessage"),
			ir.Str("sqs:GetQueueUrl"),
			ir.Str("sqs:GetQueueAttributes"),
		)),
		ir.A("Resource", ir.R(queueID, ir.Field("arn"))),
	)}
	if diff := cmp.Diff(wantStatements, from.Statements); diff != "" {
		t.Errorf("statements (-want +got):\n%s", diff)
	}
	wantEnv := ir.Attrs{ir.A("JOBS_URL", ir.R(queueID, ir.Field("url")))}
	if diff := cmp.Diff(wantEnv, from.Env); diff != "" {
		t.Errorf("env (-want +got):\n%s", diff)
	}
	if countOfType(ctx, "aws_lambda_event_source_mapping") != 0 {
		t.Error("a publisher was given an event source mapping")
	}
	if from.NeedsNetwork {
		t.Error("a publisher was put in the vpc")
	}
}

func TestConsumesFromAFunctionMapsTheQueueAndRaisesTheVisibilityTimeout(t *testing.T) {
	ctx, from := setupMessaging(t, defaultFunction, ir.RelConsumes)

	mapping := firstOfType(t, ctx, "aws_lambda_event_source_mapping")
	if mapping.Name != "worker_jobs" || mapping.SourceNode != "n6" || mapping.SourceLabel != "worker" {
		t.Errorf("mapping = %q %q %q", mapping.Name, mapping.SourceNode, mapping.SourceLabel)
	}
	want := ir.Attrs{
		ir.A("event_source_arn", ir.R(queueID, ir.Field("arn"))),
		ir.A("function_name", ir.R(workerID, ir.Field("function_name"))),
		ir.A("batch_size", ir.Num(10)),
	}
	if diff := cmp.Diff(want, mapping.Args); diff != "" {
		t.Errorf("mapping args (-want +got):\n%s", diff)
	}

	wantStatements := []ir.Value{ir.M(
		ir.A("Effect", ir.Str("Allow")),
		ir.A("Action", ir.L(
			ir.Str("sqs:ReceiveMessage"),
			ir.Str("sqs:DeleteMessage"),
			ir.Str("sqs:GetQueueAttributes"),
			ir.Str("sqs:ChangeMessageVisibility"),
		)),
		ir.A("Resource", ir.R(queueID, ir.Field("arn"))),
	)}
	if diff := cmp.Diff(wantStatements, from.Statements); diff != "" {
		t.Errorf("statements (-want +got):\n%s", diff)
	}
	if len(from.Env) != 0 {
		t.Errorf("env = %v", from.Env)
	}

	visibility, _ := named(t, ctx, queueID).Args.Get("visibility_timeout_seconds")
	if diff := cmp.Diff(ir.Value(ir.Num(180)), visibility); diff != "" {
		t.Errorf("visibility timeout (-want +got):\n%s", diff)
	}
}

func TestConsumesRaisesTheVisibilityTimeoutToSixTimesALongFunctionTimeout(t *testing.T) {
	p := defaultFunction
	p.TimeoutSeconds = 120
	ctx, _ := setupMessaging(t, p, ir.RelConsumes)

	visibility, _ := named(t, ctx, queueID).Args.Get("visibility_timeout_seconds")
	if diff := cmp.Diff(ir.Value(ir.Num(720)), visibility); diff != "" {
		t.Errorf("visibility timeout (-want +got):\n%s", diff)
	}
}

func TestPublishesAndConsumesBetweenTheSamePairWireEverythingOnce(t *testing.T) {
	ctx, from := setupMessaging(t, defaultFunction, ir.RelConsumes, ir.RelConsumes, ir.RelPublishes, ir.RelPublishes)

	if got := countOfType(ctx, "aws_lambda_event_source_mapping"); got != 1 {
		t.Errorf("mappings = %d", got)
	}
	if len(from.Statements) != 2 {
		t.Errorf("statements = %v", from.Statements)
	}
	if len(from.Env) != 1 {
		t.Errorf("env = %v", from.Env)
	}
}

func TestConsumesFromAServiceGrantsReceiveAndLeavesTheQueueAlone(t *testing.T) {
	ctx, project := newContext(t, []ir.Node{
		serviceNode(t, "n4", "web", defaultService),
		queueNode(t, "n5", "jobs", defaultQueue),
	}, []ir.Edge{{ID: "e1", From: "n4", To: "n5", Relation: ir.RelConsumes}})
	svc := resolveService(ctx, project.Nodes[0])
	queue := resolveQueue(ctx, project.Nodes[1])
	resolveMessaging(ctx, project.Edges[0], svc, queue)
	svc.Finalise()

	wantEnv := ir.Attrs{ir.A("JOBS_URL", ir.R(queueID, ir.Field("url")))}
	if diff := cmp.Diff(wantEnv, svc.Env); diff != "" {
		t.Errorf("env (-want +got):\n%s", diff)
	}
	if countOfType(ctx, "aws_lambda_event_source_mapping") != 0 {
		t.Error("a service consumer was given an event source mapping")
	}

	visibility, _ := named(t, ctx, queueID).Args.Get("visibility_timeout_seconds")
	if diff := cmp.Diff(ir.Value(ir.Num(30)), visibility); diff != "" {
		t.Errorf("visibility timeout (-want +got):\n%s", diff)
	}

	policy := named(t, ctx, ir.ID{Type: "aws_iam_role_policy", Name: "web"})
	statements, _ := policy.Args.Get("policy")
	wantStatements := ir.J(ir.M(
		ir.A("Version", ir.Str("2012-10-17")),
		ir.A("Statement", ir.L(ir.M(
			ir.A("Effect", ir.Str("Allow")),
			ir.A("Action", ir.L(
				ir.Str("sqs:ReceiveMessage"),
				ir.Str("sqs:DeleteMessage"),
				ir.Str("sqs:GetQueueAttributes"),
				ir.Str("sqs:ChangeMessageVisibility"),
			)),
			ir.A("Resource", ir.R(queueID, ir.Field("arn"))),
		))),
	))
	if diff := cmp.Diff(ir.Value(wantStatements), statements); diff != "" {
		t.Errorf("policy statements (-want +got):\n%s", diff)
	}
}

func TestMessagingReportsAnUnsupportedTarget(t *testing.T) {
	ctx, project := newContext(t, []ir.Node{
		{ID: "n2", Type: ir.NodeFunction, Name: "handler", Properties: props(t, defaultFunction)},
		{ID: "n3", Type: ir.NodeDatabase, Name: "main-db"},
	}, []ir.Edge{{ID: "e1", From: "n2", To: "n3", Relation: ir.RelPublishes}})
	fn := resolveFunction(ctx, project.Nodes[0])
	db := resolveDatabase(ctx, project.Nodes[1])
	resolveMessaging(ctx, project.Edges[0], fn, db)

	want := ir.Errors{{
		EdgeID:  "e1",
		Message: "publishes to a database is not supported by the aws resolver yet",
	}}
	if diff := cmp.Diff(want, ctx.Errors); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
}

func TestMessagingReportsAnUnsupportedSource(t *testing.T) {
	ctx, project := newContext(t, []ir.Node{
		{ID: "n3", Type: ir.NodeDatabase, Name: "main-db"},
		queueNode(t, "n5", "jobs", defaultQueue),
	}, []ir.Edge{{ID: "e1", From: "n3", To: "n5", Relation: ir.RelPublishes}})
	db := resolveDatabase(ctx, project.Nodes[0])
	queue := resolveQueue(ctx, project.Nodes[1])
	resolveMessaging(ctx, project.Edges[0], db, queue)

	want := ir.Errors{{
		EdgeID:  "e1",
		Message: "publishes from a database is not supported by the aws resolver yet",
	}}
	if diff := cmp.Diff(want, ctx.Errors); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
}
