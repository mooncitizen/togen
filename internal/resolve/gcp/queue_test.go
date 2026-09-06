package gcp

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
	topicID      = ir.ID{Type: "google_pubsub_topic", Name: "jobs"}
	deadLetterID = ir.ID{Type: "google_pubsub_topic", Name: "jobs_dead_letter"}
	projectID    = ir.ID{Type: "google_project", Name: "current"}
	pubsubAgent  = ir.C(
		ir.Str("serviceAccount:service-"),
		ir.D(projectID, ir.Field("number")),
		ir.Str("@gcp-sa-pubsub.iam.gserviceaccount.com"),
	)
)

func TestQueueEmitsATopicAndADeadLetterTopicTheServiceAgentCanPublishTo(t *testing.T) {
	ctx, handle := setupQueue(t, defaultQueue)

	wantTypes := []string{
		"google_pubsub_topic",
		"google_pubsub_topic",
		"google_pubsub_topic_iam_member",
		"google_pubsub_subscription",
	}
	if diff := cmp.Diff(wantTypes, resourceTypes(ctx)); diff != "" {
		t.Errorf("resources (-want +got):\n%s", diff)
	}
	topic := named(t, ctx, topicID)
	if diff := cmp.Diff(ir.Attrs{ir.A("name", ir.Str("shop-dev-jobs"))}, topic.Args); diff != "" {
		t.Errorf("topic args (-want +got):\n%s", diff)
	}
	dead := named(t, ctx, deadLetterID)
	if diff := cmp.Diff(ir.Attrs{ir.A("name", ir.Str("shop-dev-jobs-dead-letter"))}, dead.Args); diff != "" {
		t.Errorf("dead letter topic args (-want +got):\n%s", diff)
	}
	for _, r := range ctx.Resources() {
		if r.SourceNode != "n5" || r.SourceLabel != "jobs" {
			t.Errorf("%s source = %q/%q", ir.ID{Type: r.Type, Name: r.Name}, r.SourceNode, r.SourceLabel)
		}
	}

	grant := named(t, ctx, ir.ID{Type: "google_pubsub_topic_iam_member", Name: "jobs_dead_letter"})
	wantGrant := ir.Attrs{
		ir.A("topic", ir.R(deadLetterID, ir.Field("id"))),
		ir.A("role", ir.Str("roles/pubsub.publisher")),
		ir.A("member", pubsubAgent),
	}
	if diff := cmp.Diff(wantGrant, grant.Args); diff != "" {
		t.Errorf("dead letter grant (-want +got):\n%s", diff)
	}
	holder := named(t, ctx, ir.ID{Type: "google_pubsub_subscription", Name: "jobs_dead_letter"})
	wantHolder := ir.Attrs{
		ir.A("name", ir.Str("shop-dev-jobs-dead-letter")),
		ir.A("topic", ir.R(deadLetterID, ir.Field("id"))),
		ir.A("message_retention_duration", ir.Str("2678400s")),
	}
	if diff := cmp.Diff(wantHolder, holder.Args); diff != "" {
		t.Errorf("dead letter subscription (-want +got):\n%s", diff)
	}

	wantData := []ir.DataSource{{Type: "google_project", Name: "current", SourceLabel: "project"}}
	if diff := cmp.Diff(wantData, ctx.DataSources()); diff != "" {
		t.Errorf("data sources (-want +got):\n%s", diff)
	}

	wantExports := resolve.QueueExports{
		ID:         ir.R(topicID, ir.Field("id")),
		Name:       ir.R(topicID, ir.Field("name")),
		DeadLetter: ir.R(deadLetterID, ir.Field("id")),
	}
	if diff := cmp.Diff(resolve.Exports(wantExports), handle.Exports); diff != "" {
		t.Errorf("exports (-want +got):\n%s", diff)
	}
	if handle.Primary != topicID {
		t.Errorf("primary = %s", handle.Primary)
	}
	wantOutputs := []ir.Output{{
		Name:        "jobs_topic",
		Description: "Name of the jobs topic",
		Value:       ir.R(topicID, ir.Field("name")),
	}}
	if diff := cmp.Diff(wantOutputs, ctx.Outputs); diff != "" {
		t.Errorf("outputs (-want +got):\n%s", diff)
	}
	if countOfType(ctx, "google_compute_network") != 0 {
		t.Error("a queue pulled in the network")
	}
}

func TestQueueWithoutADeadLetterTopicEmitsTheTopicAlone(t *testing.T) {
	// deadLetter defaults to true, so it has to be turned off in the raw properties rather
	// than through QueueProps, where omitempty would drop the false.
	ctx, project := newContext(t, []ir.Node{{
		ID:         "n5",
		Type:       ir.NodeQueue,
		Name:       "jobs",
		Properties: json.RawMessage(`{"deadLetter":false}`),
	}}, nil)
	handle := resolveQueue(ctx, project.Nodes[0])

	if diff := cmp.Diff([]string{"google_pubsub_topic"}, resourceTypes(ctx)); diff != "" {
		t.Errorf("resources (-want +got):\n%s", diff)
	}
	if len(ctx.DataSources()) != 0 {
		t.Error("the project was looked up with no service agent to grant to")
	}
	if handle.Exports.(resolve.QueueExports).DeadLetter != nil {
		t.Error("the exports name a dead letter topic")
	}
}

func TestQueueExportsFIFOForItsSubscriptions(t *testing.T) {
	p := defaultQueue
	p.FIFO = true
	_, handle := setupQueue(t, p)
	if !handle.Exports.(resolve.QueueExports).FIFO {
		t.Error("the exports do not say the queue is fifo")
	}
}

func TestTwoQueuesShareOneProjectLookup(t *testing.T) {
	ctx, project := newContext(t, []ir.Node{
		queueNode(t, "n5", "jobs", defaultQueue),
		queueNode(t, "n6", "events", defaultQueue),
	}, nil)
	resolveQueue(ctx, project.Nodes[0])
	resolveQueue(ctx, project.Nodes[1])
	if got := len(ctx.DataSources()); got != 1 {
		t.Errorf("data sources = %d, want 1", got)
	}
	if got := countOfType(ctx, "google_pubsub_topic"); got != 4 {
		t.Errorf("topics = %d, want 4", got)
	}
}
