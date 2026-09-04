package aws

import (
	"fmt"
	"strings"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

// Lambda refuses an event source mapping whose queue visibility timeout is below the function
// timeout, and recommends six times it so a retried batch is not delivered twice.
const visibilityTimeoutFactor = 6

func resolveMessaging(ctx *resolve.Context, edge ir.Edge, from, to *resolve.Handle) {
	switch from.Exports.(type) {
	case resolve.FunctionExports, resolve.ServiceExports:
	default:
		ctx.Report(ir.ValidationError{
			EdgeID:  edge.ID,
			Message: fmt.Sprintf("%s from a %s is not supported by the aws resolver yet", edge.Relation, from.Node.Type),
		})
		return
	}
	queue, ok := to.Exports.(resolve.QueueExports)
	if !ok {
		ctx.Report(ir.ValidationError{
			EdgeID:  edge.ID,
			Message: fmt.Sprintf("%s to a %s is not supported by the aws resolver yet", edge.Relation, to.Node.Type),
		})
		return
	}
	prefix := strings.ToUpper(ctx.Local(to.Node.Name))

	if edge.Relation == ir.RelPublishes {
		from.AddStatement(ir.M(
			ir.A("Effect", ir.Str("Allow")),
			ir.A("Action", ir.L(
				ir.Str("sqs:SendMessage"),
				ir.Str("sqs:GetQueueUrl"),
				ir.Str("sqs:GetQueueAttributes"),
			)),
			ir.A("Resource", queue.ARN),
		))
		from.SetEnv(prefix+"_URL", queue.URL)
		return
	}

	from.AddStatement(ir.M(
		ir.A("Effect", ir.Str("Allow")),
		ir.A("Action", ir.L(
			ir.Str("sqs:ReceiveMessage"),
			ir.Str("sqs:DeleteMessage"),
			ir.Str("sqs:GetQueueAttributes"),
			ir.Str("sqs:ChangeMessageVisibility"),
		)),
		ir.A("Resource", queue.ARN),
	))

	target, isFunction := from.Exports.(resolve.FunctionExports)
	if !isFunction {
		// A service polls the queue itself, so it needs the URL rather than a mapping.
		from.SetEnv(prefix+"_URL", queue.URL)
		return
	}

	mapping := ir.ID{
		Type: "aws_lambda_event_source_mapping",
		Name: ctx.Local(from.Node.Name) + "_" + ctx.Local(to.Node.Name),
	}
	if !ctx.HasResource(mapping) {
		ctx.Add(ir.Resource{
			Type:        mapping.Type,
			Name:        mapping.Name,
			SourceNode:  from.Node.ID,
			SourceLabel: from.Node.Name,
			Args: ir.Attrs{
				ir.A("event_source_arn", queue.ARN),
				ir.A("function_name", target.FunctionName),
				ir.A("batch_size", ir.Num(10)),
			},
		})
	}

	p := nodeProps[ir.FunctionProps](ctx, from.Node)
	raiseVisibilityTimeout(ctx, to.Node, float64(visibilityTimeoutFactor*p.TimeoutSeconds))
}
