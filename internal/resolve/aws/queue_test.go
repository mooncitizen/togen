package aws

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

func queueNode(t *testing.T, id, name string, p ir.QueueProps) ir.Node {
	t.Helper()
	return ir.Node{ID: id, Type: ir.NodeQueue, Name: name, Properties: props(t, p)}
}

func setupQueue(t *testing.T, p ir.QueueProps) (*resolve.Context, *resolve.Handle) {
	t.Helper()
	ctx, project := newContext(t, []ir.Node{queueNode(t, "n5", "jobs", p)}, nil)
	return ctx, resolveQueue(ctx, project.Nodes[0])
}

var defaultQueue = ir.QueueProps{DeadLetter: true, RetentionDays: 4}

var (
	queueID = ir.ID{Type: "aws_sqs_queue", Name: "jobs"}
	dlqID   = ir.ID{Type: "aws_sqs_queue", Name: "jobs_dlq"}
)

func TestQueueEmitsAQueueAndADeadLetterQueue(t *testing.T) {
	ctx, handle := setupQueue(t, defaultQueue)

	queue := named(t, ctx, queueID)
	if queue.SourceNode != "n5" || queue.SourceLabel != "jobs" {
		t.Errorf("queue source = %q/%q", queue.SourceNode, queue.SourceLabel)
	}
	want := ir.Attrs{
		ir.A("name", ir.Str("shop-dev-jobs")),
		ir.A("message_retention_seconds", ir.Num(345600)),
		ir.A("visibility_timeout_seconds", ir.Num(30)),
		ir.A("sqs_managed_sse_enabled", ir.Bool(true)),
		ir.A("redrive_policy", ir.J(ir.M(
			ir.A("deadLetterTargetArn", ir.R(dlqID, ir.Field("arn"))),
			ir.A("maxReceiveCount", ir.Num(5)),
		))),
	}
	if diff := cmp.Diff(want, queue.Args); diff != "" {
		t.Errorf("queue args (-want +got):\n%s", diff)
	}

	wantDLQ := ir.Attrs{
		ir.A("name", ir.Str("shop-dev-jobs-dlq")),
		ir.A("message_retention_seconds", ir.Num(1209600)),
		ir.A("sqs_managed_sse_enabled", ir.Bool(true)),
	}
	if diff := cmp.Diff(wantDLQ, named(t, ctx, dlqID).Args); diff != "" {
		t.Errorf("dead letter queue args (-want +got):\n%s", diff)
	}

	wantExports := resolve.QueueExports{
		ARN:  ir.R(queueID, ir.Field("arn")),
		URL:  ir.R(queueID, ir.Field("url")),
		Name: ir.R(queueID, ir.Field("name")),
	}
	if diff := cmp.Diff(resolve.Exports(wantExports), handle.Exports); diff != "" {
		t.Errorf("exports (-want +got):\n%s", diff)
	}
	if handle.Primary != queueID {
		t.Errorf("primary = %s", handle.Primary)
	}

	wantOutputs := []ir.Output{{
		Name:        "jobs_url",
		Description: "URL of the jobs queue",
		Value:       ir.R(queueID, ir.Field("url")),
	}}
	if diff := cmp.Diff(wantOutputs, ctx.Outputs); diff != "" {
		t.Errorf("outputs (-want +got):\n%s", diff)
	}
	if countOfType(ctx, "aws_vpc") != 0 {
		t.Error("a queue pulled in the network")
	}
}

func TestQueueWithoutADeadLetterQueueEmitsOneQueueAndNoRedrivePolicy(t *testing.T) {
	// deadLetter defaults to true, so it has to be turned off in the raw properties rather
	// than through QueueProps, where omitempty would drop the false.
	ctx, project := newContext(t, []ir.Node{{
		ID:         "n5",
		Type:       ir.NodeQueue,
		Name:       "jobs",
		Properties: json.RawMessage(`{"deadLetter":false}`),
	}}, nil)
	resolveQueue(ctx, project.Nodes[0])

	if got := countOfType(ctx, "aws_sqs_queue"); got != 1 {
		t.Errorf("queues = %d", got)
	}
	if _, ok := named(t, ctx, queueID).Args.Get("redrive_policy"); ok {
		t.Error("a redrive policy was emitted with no dead letter queue")
	}
}

func TestQueueFIFONamesBothQueuesAndSetsTheFlags(t *testing.T) {
	p := defaultQueue
	p.FIFO = true
	ctx, handle := setupQueue(t, p)

	want := ir.Attrs{
		ir.A("name", ir.Str("shop-dev-jobs.fifo")),
		ir.A("fifo_queue", ir.Bool(true)),
		ir.A("content_based_deduplication", ir.Bool(true)),
		ir.A("message_retention_seconds", ir.Num(345600)),
		ir.A("visibility_timeout_seconds", ir.Num(30)),
		ir.A("sqs_managed_sse_enabled", ir.Bool(true)),
		ir.A("redrive_policy", ir.J(ir.M(
			ir.A("deadLetterTargetArn", ir.R(dlqID, ir.Field("arn"))),
			ir.A("maxReceiveCount", ir.Num(5)),
		))),
	}
	if diff := cmp.Diff(want, named(t, ctx, queueID).Args); diff != "" {
		t.Errorf("queue args (-want +got):\n%s", diff)
	}

	wantDLQ := ir.Attrs{
		ir.A("name", ir.Str("shop-dev-jobs-dlq.fifo")),
		ir.A("fifo_queue", ir.Bool(true)),
		ir.A("message_retention_seconds", ir.Num(1209600)),
		ir.A("sqs_managed_sse_enabled", ir.Bool(true)),
	}
	if diff := cmp.Diff(wantDLQ, named(t, ctx, dlqID).Args); diff != "" {
		t.Errorf("dead letter queue args (-want +got):\n%s", diff)
	}
	if !handle.Exports.(resolve.QueueExports).FIFO {
		t.Error("the exports do not say the queue is fifo")
	}
}

func TestQueueRetentionIsGivenInDays(t *testing.T) {
	p := defaultQueue
	p.RetentionDays = 14
	ctx, _ := setupQueue(t, p)

	retention, _ := named(t, ctx, queueID).Args.Get("message_retention_seconds")
	if diff := cmp.Diff(ir.Value(ir.Num(1209600)), retention); diff != "" {
		t.Errorf("retention (-want +got):\n%s", diff)
	}
}
