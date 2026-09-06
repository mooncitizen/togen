package gcp

import (
	"fmt"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

const (
	projectLabel  = "project"
	publisherRole = "roles/pubsub.publisher"
	// Someone looks at a dead letter days later, so its subscription keeps the thirty one days
	// Pub/Sub allows rather than the retention the node asks for.
	deadLetterRetentionSeconds = 2678400
	secondsPerDay              = 86400
)

func resolveQueue(ctx *resolve.Context, node ir.Node) *resolve.Handle {
	p := resolve.Props[ir.QueueProps](ctx, node)
	local := ctx.Local(node.Name)
	name := ctx.Named(node.Name)

	topic := ctx.Add(ir.Resource{
		Type:        "google_pubsub_topic",
		Name:        local,
		SourceNode:  node.ID,
		SourceLabel: node.Name,
		Args:        ir.Attrs{ir.A("name", ir.Str(name))},
	})
	topicID := ir.ID{Type: topic.Type, Name: topic.Name}

	exports := resolve.QueueExports{
		ID:   ir.R(topicID, ir.Field("id")),
		Name: ir.R(topicID, ir.Field("name")),
		FIFO: p.FIFO,
	}
	if p.DeadLetter {
		exports.DeadLetter = ir.R(deadLetterTopic(ctx, node, local, name), ir.Field("id"))
	}

	ctx.AddOutput(ir.Output{
		Name:        local + "_topic",
		Description: fmt.Sprintf("Name of the %s topic", node.Name),
		Value:       exports.Name,
	})

	return &resolve.Handle{Node: node, Primary: topicID, Exports: exports}
}

// Pub/Sub forwards dead letters as the service agent, which needs to publish on the dead letter
// topic, and a topic with no subscription drops what it is given, so one holds the messages.
func deadLetterTopic(ctx *resolve.Context, node ir.Node, local, name string) ir.ID {
	dead := ctx.Add(ir.Resource{
		Type:        "google_pubsub_topic",
		Name:        local + "_dead_letter",
		SourceNode:  node.ID,
		SourceLabel: node.Name,
		Args:        ir.Attrs{ir.A("name", ir.Str(name+"-dead-letter"))},
	})
	deadID := ir.ID{Type: dead.Type, Name: dead.Name}
	ctx.Add(ir.Resource{
		Type:        "google_pubsub_topic_iam_member",
		Name:        dead.Name,
		SourceNode:  node.ID,
		SourceLabel: node.Name,
		Args: ir.Attrs{
			ir.A("topic", ir.R(deadID, ir.Field("id"))),
			ir.A("role", ir.Str(publisherRole)),
			ir.A("member", pubsubServiceAgent(ctx)),
		},
	})
	ctx.Add(ir.Resource{
		Type:        "google_pubsub_subscription",
		Name:        dead.Name,
		SourceNode:  node.ID,
		SourceLabel: node.Name,
		Args: ir.Attrs{
			ir.A("name", ir.Str(name+"-dead-letter")),
			ir.A("topic", ir.R(deadID, ir.Field("id"))),
			ir.A("message_retention_duration", ir.Str(seconds(deadLetterRetentionSeconds))),
		},
	})
	return deadID
}

func pubsubServiceAgent(ctx *resolve.Context) ir.Value {
	return ir.C(
		ir.Str("serviceAccount:service-"),
		ir.D(ensureProject(ctx), ir.Field("number")),
		ir.Str("@gcp-sa-pubsub.iam.gserviceaccount.com"),
	)
}

func ensureProject(ctx *resolve.Context) ir.ID {
	if id, ok := ctx.Scratch[projectLabel].(ir.ID); ok {
		return id
	}
	project := ctx.AddData(ir.DataSource{Type: "google_project", Name: "current", SourceLabel: projectLabel})
	id := ir.ID{Type: project.Type, Name: project.Name}
	ctx.Scratch[projectLabel] = id
	return id
}

func seconds(n int) string { return fmt.Sprintf("%ds", n) }
