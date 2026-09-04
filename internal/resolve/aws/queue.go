package aws

import (
	"fmt"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

const (
	queuesLabel = "queues"
	// Someone looks in a dead letter queue days later, so it keeps the fourteen days SQS
	// allows rather than the retention the node asks for.
	deadLetterRetentionSeconds = 1209600
	defaultVisibilitySeconds   = 30
	maxReceiveCount            = 5
	secondsPerDay              = 86400
)

// queueResources tracks the main queue of every queue node so a consumes edge can raise its
// visibility timeout after the node itself has been resolved.
func queueResources(ctx *resolve.Context) map[string]*ir.Resource {
	queues, ok := ctx.Scratch[queuesLabel].(map[string]*ir.Resource)
	if !ok {
		queues = map[string]*ir.Resource{}
		ctx.Scratch[queuesLabel] = queues
	}
	return queues
}

func resolveQueue(ctx *resolve.Context, node ir.Node) *resolve.Handle {
	p := nodeProps[ir.QueueProps](ctx, node)
	local := ctx.Local(node.Name)
	name := ctx.Named(node.Name)
	suffix := ""
	if p.FIFO {
		suffix = ".fifo"
	}

	var deadLetter *ir.ID
	if p.DeadLetter {
		args := ir.Attrs{ir.A("name", ir.Str(name+"-dlq"+suffix))}
		// SQS only lets a FIFO queue redrive to another FIFO queue.
		if p.FIFO {
			args = append(args, ir.A("fifo_queue", ir.Bool(true)))
		}
		args = append(args,
			ir.A("message_retention_seconds", ir.Num(deadLetterRetentionSeconds)),
			ir.A("sqs_managed_sse_enabled", ir.Bool(true)),
		)
		dlq := ctx.Add(ir.Resource{
			Type:        "aws_sqs_queue",
			Name:        local + "_dlq",
			SourceNode:  node.ID,
			SourceLabel: node.Name,
			Args:        args,
		})
		deadLetter = &ir.ID{Type: dlq.Type, Name: dlq.Name}
	}

	args := ir.Attrs{ir.A("name", ir.Str(name+suffix))}
	if p.FIFO {
		args = append(args,
			ir.A("fifo_queue", ir.Bool(true)),
			ir.A("content_based_deduplication", ir.Bool(true)),
		)
	}
	args = append(args,
		ir.A("message_retention_seconds", ir.Num(float64(p.RetentionDays*secondsPerDay))),
		ir.A("visibility_timeout_seconds", ir.Num(defaultVisibilitySeconds)),
		ir.A("sqs_managed_sse_enabled", ir.Bool(true)),
	)
	if deadLetter != nil {
		args = append(args, ir.A("redrive_policy", ir.J(ir.M(
			ir.A("deadLetterTargetArn", ir.R(*deadLetter, ir.Field("arn"))),
			ir.A("maxReceiveCount", ir.Num(maxReceiveCount)),
		))))
	}

	queue := ctx.Add(ir.Resource{
		Type:        "aws_sqs_queue",
		Name:        local,
		SourceNode:  node.ID,
		SourceLabel: node.Name,
		Args:        args,
	})
	queueID := ir.ID{Type: queue.Type, Name: queue.Name}
	queueResources(ctx)[node.ID] = queue

	ctx.AddOutput(ir.Output{
		Name:        local + "_url",
		Description: fmt.Sprintf("URL of the %s queue", node.Name),
		Value:       ir.R(queueID, ir.Field("url")),
	})

	return &resolve.Handle{
		Node:    node,
		Primary: queueID,
		Exports: resolve.QueueExports{
			ARN:  ir.R(queueID, ir.Field("arn")),
			URL:  ir.R(queueID, ir.Field("url")),
			Name: ir.R(queueID, ir.Field("name")),
			FIFO: p.FIFO,
		},
	}
}

func raiseVisibilityTimeout(ctx *resolve.Context, node ir.Node, seconds float64) {
	queue, ok := queueResources(ctx)[node.ID]
	if !ok {
		ctx.Fail(fmt.Sprintf("queue '%s' has no resource to raise the visibility timeout on", node.Name))
	}
	if current, ok := queue.Args.Get("visibility_timeout_seconds"); ok {
		if n, isNumber := current.(ir.Number); isNumber && float64(n) >= seconds {
			return
		}
	}
	queue.Args.Set("visibility_timeout_seconds", ir.Num(seconds))
}
