package gcp

import (
	"fmt"
	"strings"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

const (
	subscriberRole      = "roles/pubsub.subscriber"
	eventReceiverRole   = "roles/eventarc.eventReceiver"
	tokenCreatorRole    = "roles/iam.serviceAccountTokenCreator"
	pubsubPublishedType = "google.cloud.pubsub.topic.v1.messagePublished"
	maxDeliveryAttempts = 5
)

func resolveMessaging(ctx *resolve.Context, edge ir.Edge, from, to *resolve.Handle) {
	caller, ok := from.Exports.(resolve.CloudRunExports)
	if !ok {
		ctx.Report(ir.ValidationError{
			EdgeID:  edge.ID,
			Message: fmt.Sprintf("%s from a %s is not supported by the gcp resolver yet", edge.Relation, from.Node.Type),
		})
		return
	}
	queue, ok := to.Exports.(resolve.QueueExports)
	if !ok {
		ctx.Report(ir.ValidationError{
			EdgeID:  edge.ID,
			Message: fmt.Sprintf("%s to a %s is not supported by the gcp resolver yet", edge.Relation, to.Node.Type),
		})
		return
	}

	if edge.Relation == ir.RelPublishes {
		publish(ctx, from, to, caller, queue)
		return
	}
	base := ctx.Local(from.Node.Name) + "_" + ctx.Local(to.Node.Name)
	if ctx.HasResource(ir.ID{Type: "google_cloud_run_v2_service_iam_member", Name: base}) {
		return
	}
	grantInvoker(ctx, base, from, caller)
	if from.Node.Type == ir.NodeFunction {
		triggerFunction(ctx, base, from, to, caller, queue)
	} else {
		pushToService(ctx, base, from, to, caller, queue)
	}
}

func publish(ctx *resolve.Context, from, to *resolve.Handle, caller resolve.CloudRunExports, queue resolve.QueueExports) {
	bindingID := ir.ID{
		Type: "google_pubsub_topic_iam_member",
		Name: ctx.Local(to.Node.Name) + "_from_" + ctx.Local(from.Node.Name),
	}
	if !ctx.HasResource(bindingID) {
		ctx.Add(ir.Resource{
			Type:        bindingID.Type,
			Name:        bindingID.Name,
			SourceNode:  to.Node.ID,
			SourceLabel: to.Node.Name,
			Args: ir.Attrs{
				ir.A("topic", queue.ID),
				ir.A("role", ir.Str(publisherRole)),
				ir.A("member", member(caller)),
			},
		})
	}
	from.SetEnv(strings.ToUpper(ctx.Local(to.Node.Name))+"_TOPIC", queue.Name)
}

// The consumer's own account delivers to it, so the trigger and the push subscription need no
// account of their own and the invoker grant is the consumer invoking itself.
func grantInvoker(ctx *resolve.Context, base string, from *resolve.Handle, caller resolve.CloudRunExports) {
	ctx.Add(ir.Resource{
		Type:        "google_cloud_run_v2_service_iam_member",
		Name:        base,
		SourceNode:  from.Node.ID,
		SourceLabel: from.Node.Name,
		Args: ir.Attrs{
			ir.A("name", caller.Service),
			ir.A("location", caller.Location),
			ir.A("role", ir.Str(invokerRole)),
			ir.A("member", member(caller)),
		},
	})
}

// Eventarc owns the trigger's subscription, so ordering, retention and dead lettering cannot be
// set on it from here. A function gets Eventarc's retry policy and the queue's settings do not apply.
func triggerFunction(ctx *resolve.Context, base string, from, to *resolve.Handle, caller resolve.CloudRunExports, queue resolve.QueueExports) {
	receiverID := ir.ID{Type: "google_project_iam_member", Name: ctx.Local(from.Node.Name) + "_event_receiver"}
	if !ctx.HasResource(receiverID) {
		ctx.Add(ir.Resource{
			Type:        receiverID.Type,
			Name:        receiverID.Name,
			SourceNode:  from.Node.ID,
			SourceLabel: from.Node.Name,
			Args: ir.Attrs{
				ir.A("project", ir.V(projectVar)),
				ir.A("role", ir.Str(eventReceiverRole)),
				ir.A("member", member(caller)),
			},
		})
	}
	ctx.Add(ir.Resource{
		Type:        "google_eventarc_trigger",
		Name:        base,
		SourceNode:  from.Node.ID,
		SourceLabel: from.Node.Name,
		Args: ir.Attrs{
			ir.A("name", ir.Str(ctx.Named(from.Node.Name+"-"+to.Node.Name))),
			ir.A("location", ir.Str(ctx.Project.Region)),
			ir.A("service_account", caller.ServiceAccount),
			ir.A("matching_criteria", ir.B(ir.Attrs{
				ir.A("attribute", ir.Str("type")),
				ir.A("value", ir.Str(pubsubPublishedType)),
			})),
			ir.A("destination", ir.B(ir.Attrs{
				ir.A("cloud_run_service", ir.B(ir.Attrs{
					ir.A("service", caller.Service),
					ir.A("region", caller.Location),
				})),
			})),
			ir.A("transport", ir.B(ir.Attrs{
				ir.A("pubsub", ir.B(ir.Attrs{ir.A("topic", queue.ID)})),
			})),
		},
	})
}

func pushToService(ctx *resolve.Context, base string, from, to *resolve.Handle, caller resolve.CloudRunExports, queue resolve.QueueExports) {
	p := resolve.Props[ir.QueueProps](ctx, to.Node)
	tokenID := ir.ID{Type: "google_service_account_iam_member", Name: ctx.Local(from.Node.Name) + "_token_creator"}
	if !ctx.HasResource(tokenID) {
		// Pub/Sub mints the push token as the service agent, which has to be allowed to on the account.
		ctx.Add(ir.Resource{
			Type:        tokenID.Type,
			Name:        tokenID.Name,
			SourceNode:  from.Node.ID,
			SourceLabel: from.Node.Name,
			Args: ir.Attrs{
				ir.A("service_account_id", ir.C(ir.Str("projects/"), ir.V(projectVar), ir.Str("/serviceAccounts/"), caller.ServiceAccount)),
				ir.A("role", ir.Str(tokenCreatorRole)),
				ir.A("member", pubsubServiceAgent(ctx)),
			},
		})
	}

	args := ir.Attrs{
		ir.A("name", ir.Str(ctx.Named(from.Node.Name+"-"+to.Node.Name))),
		ir.A("topic", queue.ID),
		ir.A("message_retention_duration", ir.Str(seconds(p.RetentionDays*secondsPerDay))),
	}
	if queue.FIFO {
		args = append(args, ir.A("enable_message_ordering", ir.Bool(true)))
	}
	args = append(args, ir.A("push_config", ir.B(ir.Attrs{
		ir.A("push_endpoint", caller.URL),
		ir.A("oidc_token", ir.B(ir.Attrs{ir.A("service_account_email", caller.ServiceAccount)})),
	})))
	if queue.DeadLetter != nil {
		args = append(args, ir.A("dead_letter_policy", ir.B(ir.Attrs{
			ir.A("dead_letter_topic", queue.DeadLetter),
			ir.A("max_delivery_attempts", ir.Num(maxDeliveryAttempts)),
		})))
	}
	subscription := ctx.Add(ir.Resource{
		Type:        "google_pubsub_subscription",
		Name:        base,
		SourceNode:  from.Node.ID,
		SourceLabel: from.Node.Name,
		Args:        args,
	})
	if queue.DeadLetter != nil {
		// Forwarding a dead letter acks it on the source, which the service agent does as a subscriber.
		ctx.Add(ir.Resource{
			Type:        "google_pubsub_subscription_iam_member",
			Name:        base,
			SourceNode:  from.Node.ID,
			SourceLabel: from.Node.Name,
			Args: ir.Attrs{
				ir.A("subscription", ir.R(ir.ID{Type: subscription.Type, Name: subscription.Name}, ir.Field("id"))),
				ir.A("role", ir.Str(subscriberRole)),
				ir.A("member", pubsubServiceAgent(ctx)),
			},
		})
	}
}

func member(caller resolve.CloudRunExports) ir.Value {
	return ir.C(ir.Str("serviceAccount:"), caller.ServiceAccount)
}
