package gcp

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

func setupFunctionMessaging(t *testing.T, queue ir.QueueProps, relations ...ir.Relation) (*resolve.Context, *resolve.Handle) {
	t.Helper()
	edges := make([]ir.Edge, len(relations))
	for i, r := range relations {
		edges[i] = ir.Edge{ID: "e" + string(rune('1'+i)), From: "n2", To: "n5", Relation: r}
	}
	ctx, project := newContext(t, []ir.Node{
		functionNode(t, "n2", "handler", defaultFunction),
		queueNode(t, "n5", "jobs", queue),
	}, edges)
	fn := resolveFunction(ctx, project.Nodes[0])
	q := resolveQueue(ctx, project.Nodes[1])
	for _, e := range project.Edges {
		resolveMessaging(ctx, e, fn, q)
	}
	fn.Finalise()
	return ctx, fn
}

func setupServiceMessaging(t *testing.T, queue ir.QueueProps, relations ...ir.Relation) (*resolve.Context, *resolve.Handle) {
	t.Helper()
	edges := make([]ir.Edge, len(relations))
	for i, r := range relations {
		edges[i] = ir.Edge{ID: "e" + string(rune('1'+i)), From: "n4", To: "n5", Relation: r}
	}
	ctx, project := newContext(t, []ir.Node{
		serviceNode(t, "n4", "web", defaultService),
		queueNode(t, "n5", "jobs", queue),
	}, edges)
	svc := resolveService(ctx, project.Nodes[0])
	q := resolveQueue(ctx, project.Nodes[1])
	for _, e := range project.Edges {
		resolveMessaging(ctx, e, svc, q)
	}
	svc.Finalise()
	return ctx, svc
}

func publisherBinding(account ir.ID) ir.Attrs {
	return ir.Attrs{
		ir.A("topic", ir.R(topicID, ir.Field("id"))),
		ir.A("role", ir.Str("roles/pubsub.publisher")),
		ir.A("member", ir.C(ir.Str("serviceAccount:"), ir.R(account, ir.Field("email")))),
	}
}

func TestPublishesFromAFunctionGrantsPublisherAndInjectsTheTopic(t *testing.T) {
	ctx, fn := setupFunctionMessaging(t, defaultQueue, ir.RelPublishes)

	binding := named(t, ctx, ir.ID{Type: "google_pubsub_topic_iam_member", Name: "jobs_from_handler"})
	if binding.SourceNode != "n5" || binding.SourceLabel != "jobs" {
		t.Errorf("binding source = %q/%q", binding.SourceNode, binding.SourceLabel)
	}
	if diff := cmp.Diff(publisherBinding(fnAccountID), binding.Args); diff != "" {
		t.Errorf("binding args (-want +got):\n%s", diff)
	}
	wantEnv := ir.Attrs{ir.A("JOBS_TOPIC", ir.R(topicID, ir.Field("name")))}
	if diff := cmp.Diff(wantEnv, fn.Env); diff != "" {
		t.Errorf("env (-want +got):\n%s", diff)
	}
	env, _ := serviceConfig(t, ctx).Get("environment_variables")
	if diff := cmp.Diff(ir.Value(ir.Map(wantEnv)), env); diff != "" {
		t.Errorf("environment_variables (-want +got):\n%s", diff)
	}
	if countOfType(ctx, "google_eventarc_trigger") != 0 || countOfType(ctx, "google_cloud_run_v2_service_iam_member") != 0 {
		t.Error("a publisher was wired as a consumer")
	}
	if got := countOfType(ctx, "google_pubsub_subscription"); got != 1 {
		t.Errorf("subscriptions = %d, want the dead letter one alone", got)
	}
	if fn.NeedsNetwork {
		t.Error("a publisher was put on the connector")
	}
	if len(ctx.Errors) != 0 {
		t.Errorf("errors = %v", ctx.Errors)
	}
}

func TestPublishesFromAServiceGrantsPublisherAndInjectsTheTopic(t *testing.T) {
	ctx, _ := setupServiceMessaging(t, defaultQueue, ir.RelPublishes)

	binding := named(t, ctx, ir.ID{Type: "google_pubsub_topic_iam_member", Name: "jobs_from_web"})
	if diff := cmp.Diff(publisherBinding(svcAccountID), binding.Args); diff != "" {
		t.Errorf("binding args (-want +got):\n%s", diff)
	}
	env, _ := container(t, ctx).Get("env")
	want := ir.B(ir.Attrs{ir.A("name", ir.Str("JOBS_TOPIC")), ir.A("value", ir.R(topicID, ir.Field("name")))})
	if diff := cmp.Diff(ir.Value(want), env); diff != "" {
		t.Errorf("container env (-want +got):\n%s", diff)
	}
	if countOfType(ctx, "google_cloud_run_v2_service_iam_member") != 0 {
		t.Error("a publisher was granted invoker on itself")
	}
}

func TestConsumesFromAFunctionTriggersItThroughEventarc(t *testing.T) {
	ctx, fn := setupFunctionMessaging(t, defaultQueue, ir.RelConsumes)

	trigger := named(t, ctx, ir.ID{Type: "google_eventarc_trigger", Name: "handler_jobs"})
	if trigger.SourceNode != "n2" || trigger.SourceLabel != "handler" {
		t.Errorf("trigger source = %q/%q", trigger.SourceNode, trigger.SourceLabel)
	}
	want := ir.Attrs{
		ir.A("name", ir.Str("shop-dev-handler-jobs")),
		ir.A("location", ir.Str("europe-west2")),
		ir.A("service_account", ir.R(fnAccountID, ir.Field("email"))),
		ir.A("matching_criteria", ir.B(ir.Attrs{
			ir.A("attribute", ir.Str("type")),
			ir.A("value", ir.Str("google.cloud.pubsub.topic.v1.messagePublished")),
		})),
		ir.A("destination", ir.B(ir.Attrs{
			ir.A("cloud_run_service", ir.B(ir.Attrs{
				ir.A("service", ir.R(fnID, ir.Field("name"))),
				ir.A("region", ir.R(fnID, ir.Field("location"))),
			})),
		})),
		ir.A("transport", ir.B(ir.Attrs{
			ir.A("pubsub", ir.B(ir.Attrs{ir.A("topic", ir.R(topicID, ir.Field("id")))})),
		})),
	}
	if diff := cmp.Diff(want, trigger.Args); diff != "" {
		t.Errorf("trigger args (-want +got):\n%s", diff)
	}

	invoker := named(t, ctx, ir.ID{Type: "google_cloud_run_v2_service_iam_member", Name: "handler_jobs"})
	if diff := cmp.Diff(invokerBinding(ir.R(fnID, ir.Field("name")), ir.R(fnID, ir.Field("location")), fnAccountID), invoker.Args); diff != "" {
		t.Errorf("invoker binding (-want +got):\n%s", diff)
	}
	receiver := named(t, ctx, ir.ID{Type: "google_project_iam_member", Name: "handler_event_receiver"})
	wantReceiver := ir.Attrs{
		ir.A("project", ir.V("project")),
		ir.A("role", ir.Str("roles/eventarc.eventReceiver")),
		ir.A("member", ir.C(ir.Str("serviceAccount:"), ir.R(fnAccountID, ir.Field("email")))),
	}
	if diff := cmp.Diff(wantReceiver, receiver.Args); diff != "" {
		t.Errorf("receiver binding (-want +got):\n%s", diff)
	}
	if len(fn.Env) != 0 {
		t.Errorf("env = %v", fn.Env)
	}
	// Eventarc owns the trigger's subscription, so the queue's settings have nowhere to go.
	if got := countOfType(ctx, "google_pubsub_subscription"); got != 1 {
		t.Errorf("subscriptions = %d, want the dead letter one alone", got)
	}
	if countOfType(ctx, "google_pubsub_subscription_iam_member") != 0 {
		t.Error("a subscriber grant was made on a subscription this graph does not own")
	}
}

func TestConsumesFromAFunctionTwiceWiresTheTriggerOnce(t *testing.T) {
	ctx, fn := setupFunctionMessaging(t, defaultQueue, ir.RelConsumes, ir.RelConsumes, ir.RelPublishes, ir.RelPublishes)
	if got := countOfType(ctx, "google_eventarc_trigger"); got != 1 {
		t.Errorf("triggers = %d", got)
	}
	if got := countOfType(ctx, "google_pubsub_topic_iam_member"); got != 2 {
		t.Errorf("topic grants = %d, want the publisher and the dead letter one", got)
	}
	if len(fn.Env) != 1 {
		t.Errorf("env = %v", fn.Env)
	}
}

func TestConsumesFromAServicePushesToItWithAnOIDCToken(t *testing.T) {
	ctx, svc := setupServiceMessaging(t, defaultQueue, ir.RelConsumes)

	subscription := named(t, ctx, ir.ID{Type: "google_pubsub_subscription", Name: "web_jobs"})
	if subscription.SourceNode != "n4" || subscription.SourceLabel != "web" {
		t.Errorf("subscription source = %q/%q", subscription.SourceNode, subscription.SourceLabel)
	}
	want := ir.Attrs{
		ir.A("name", ir.Str("shop-dev-web-jobs")),
		ir.A("topic", ir.R(topicID, ir.Field("id"))),
		ir.A("message_retention_duration", ir.Str("345600s")),
		ir.A("push_config", ir.B(ir.Attrs{
			ir.A("push_endpoint", ir.R(svcID, ir.Field("uri"))),
			ir.A("oidc_token", ir.B(ir.Attrs{ir.A("service_account_email", ir.R(svcAccountID, ir.Field("email")))})),
		})),
		ir.A("dead_letter_policy", ir.B(ir.Attrs{
			ir.A("dead_letter_topic", ir.R(deadLetterID, ir.Field("id"))),
			ir.A("max_delivery_attempts", ir.Num(5)),
		})),
	}
	if diff := cmp.Diff(want, subscription.Args); diff != "" {
		t.Errorf("subscription args (-want +got):\n%s", diff)
	}

	invoker := named(t, ctx, ir.ID{Type: "google_cloud_run_v2_service_iam_member", Name: "web_jobs"})
	if diff := cmp.Diff(invokerBinding(ir.R(svcID, ir.Field("name")), ir.R(svcID, ir.Field("location")), svcAccountID), invoker.Args); diff != "" {
		t.Errorf("invoker binding (-want +got):\n%s", diff)
	}
	token := named(t, ctx, ir.ID{Type: "google_service_account_iam_member", Name: "web_token_creator"})
	wantToken := ir.Attrs{
		ir.A("service_account_id", ir.C(ir.Str("projects/"), ir.V("project"), ir.Str("/serviceAccounts/"), ir.R(svcAccountID, ir.Field("email")))),
		ir.A("role", ir.Str("roles/iam.serviceAccountTokenCreator")),
		ir.A("member", pubsubAgent),
	}
	if diff := cmp.Diff(wantToken, token.Args); diff != "" {
		t.Errorf("token creator binding (-want +got):\n%s", diff)
	}
	subscriber := named(t, ctx, ir.ID{Type: "google_pubsub_subscription_iam_member", Name: "web_jobs"})
	wantSubscriber := ir.Attrs{
		ir.A("subscription", ir.R(ir.ID{Type: "google_pubsub_subscription", Name: "web_jobs"}, ir.Field("id"))),
		ir.A("role", ir.Str("roles/pubsub.subscriber")),
		ir.A("member", pubsubAgent),
	}
	if diff := cmp.Diff(wantSubscriber, subscriber.Args); diff != "" {
		t.Errorf("subscriber binding (-want +got):\n%s", diff)
	}
	if len(svc.Env) != 0 {
		t.Errorf("env = %v", svc.Env)
	}
	if countOfType(ctx, "google_eventarc_trigger") != 0 {
		t.Error("a service consumer was given an Eventarc trigger")
	}
	if svc.NeedsNetwork {
		t.Error("a consumer was put on the connector")
	}
}

func TestServiceSubscriptionTakesFIFOAndRetentionFromTheQueue(t *testing.T) {
	p := defaultQueue
	p.FIFO = true
	p.RetentionDays = 14
	ctx, _ := setupServiceMessaging(t, p, ir.RelConsumes)

	args := named(t, ctx, ir.ID{Type: "google_pubsub_subscription", Name: "web_jobs"}).Args
	ordering, _ := args.Get("enable_message_ordering")
	if diff := cmp.Diff(ir.Value(ir.Bool(true)), ordering); diff != "" {
		t.Errorf("enable_message_ordering (-want +got):\n%s", diff)
	}
	retention, _ := args.Get("message_retention_duration")
	if diff := cmp.Diff(ir.Value(ir.Str("1209600s")), retention); diff != "" {
		t.Errorf("message_retention_duration (-want +got):\n%s", diff)
	}
}

func TestServiceSubscriptionWithoutADeadLetterTopicHasNoPolicyAndNoSubscriberGrant(t *testing.T) {
	ctx, project := newContext(t, []ir.Node{
		serviceNode(t, "n4", "web", defaultService),
		{ID: "n5", Type: ir.NodeQueue, Name: "jobs", Properties: []byte(`{"deadLetter":false}`)},
	}, []ir.Edge{{ID: "e1", From: "n4", To: "n5", Relation: ir.RelConsumes}})
	svc := resolveService(ctx, project.Nodes[0])
	q := resolveQueue(ctx, project.Nodes[1])
	resolveMessaging(ctx, project.Edges[0], svc, q)

	args := named(t, ctx, ir.ID{Type: "google_pubsub_subscription", Name: "web_jobs"}).Args
	if _, ok := args.Get("dead_letter_policy"); ok {
		t.Error("a dead letter policy was emitted with no dead letter topic")
	}
	if _, ok := args.Get("enable_message_ordering"); ok {
		t.Error("ordering was enabled on a queue that is not fifo")
	}
	if countOfType(ctx, "google_pubsub_subscription_iam_member") != 0 {
		t.Error("a subscriber grant was made with no dead letter topic to forward to")
	}
}

func TestOneServiceConsumingTwoQueuesGetsOneTokenCreatorGrant(t *testing.T) {
	ctx, project := newContext(t, []ir.Node{
		serviceNode(t, "n4", "web", defaultService),
		queueNode(t, "n5", "jobs", defaultQueue),
		queueNode(t, "n6", "events", defaultQueue),
	}, []ir.Edge{
		{ID: "e1", From: "n4", To: "n5", Relation: ir.RelConsumes},
		{ID: "e2", From: "n4", To: "n6", Relation: ir.RelConsumes},
	})
	svc := resolveService(ctx, project.Nodes[0])
	jobs := resolveQueue(ctx, project.Nodes[1])
	events := resolveQueue(ctx, project.Nodes[2])
	resolveMessaging(ctx, project.Edges[0], svc, jobs)
	resolveMessaging(ctx, project.Edges[1], svc, events)

	if got := countOfType(ctx, "google_service_account_iam_member"); got != 1 {
		t.Errorf("token creator grants = %d, want 1", got)
	}
	if got := countOfType(ctx, "google_pubsub_subscription"); got != 4 {
		t.Errorf("subscriptions = %d, want two push and two dead letter", got)
	}
}

func TestMessagingReportsAnUnsupportedTarget(t *testing.T) {
	ctx, project := newContext(t, []ir.Node{
		functionNode(t, "n2", "handler", defaultFunction),
		databaseNode(t, "n3", "main-db", defaultDatabase),
	}, []ir.Edge{{ID: "e1", From: "n2", To: "n3", Relation: ir.RelPublishes}})
	fn := resolveFunction(ctx, project.Nodes[0])
	db := resolveDatabase(ctx, project.Nodes[1])
	resolveMessaging(ctx, project.Edges[0], fn, db)

	want := ir.Errors{{EdgeID: "e1", Message: "publishes to a database is not supported by the gcp resolver yet"}}
	if diff := cmp.Diff(want, ctx.Errors); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
}

func TestMessagingReportsAnUnsupportedSource(t *testing.T) {
	ctx, project := newContext(t, []ir.Node{
		databaseNode(t, "n3", "main-db", defaultDatabase),
		queueNode(t, "n5", "jobs", defaultQueue),
	}, []ir.Edge{{ID: "e1", From: "n3", To: "n5", Relation: ir.RelConsumes}})
	db := resolveDatabase(ctx, project.Nodes[0])
	q := resolveQueue(ctx, project.Nodes[1])
	resolveMessaging(ctx, project.Edges[0], db, q)

	want := ir.Errors{{EdgeID: "e1", Message: "consumes from a database is not supported by the gcp resolver yet"}}
	if diff := cmp.Diff(want, ctx.Errors); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
}
