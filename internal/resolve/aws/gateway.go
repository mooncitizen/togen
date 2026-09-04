package aws

import (
	"fmt"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

func resolveGateway(ctx *resolve.Context, node ir.Node) *resolve.Handle {
	local := ctx.Local(node.Name)
	api := ctx.Add(ir.Resource{
		Type:        "aws_apigatewayv2_api",
		Name:        local,
		SourceNode:  node.ID,
		SourceLabel: node.Name,
		Args: ir.Attrs{
			ir.A("name", ir.Str(ctx.Named(node.Name))),
			ir.A("protocol_type", ir.Str("HTTP")),
		},
	})
	apiID := ir.ID{Type: api.Type, Name: api.Name}

	ctx.Add(ir.Resource{
		Type:        "aws_apigatewayv2_stage",
		Name:        local,
		SourceNode:  node.ID,
		SourceLabel: node.Name,
		Args: ir.Attrs{
			ir.A("api_id", ir.R(apiID, ir.Field("id"))),
			ir.A("name", ir.Str("$default")),
			ir.A("auto_deploy", ir.Bool(true)),
		},
	})
	ctx.AddOutput(ir.Output{
		Name:        local + "_url",
		Description: fmt.Sprintf("Public URL of the %s gateway", node.Name),
		Value:       ir.R(apiID, ir.Field("api_endpoint")),
	})

	return &resolve.Handle{
		Node:    node,
		Primary: apiID,
		Exports: resolve.GatewayExports{
			APIID:        ir.R(apiID, ir.Field("id")),
			ExecutionARN: ir.R(apiID, ir.Field("execution_arn")),
			URL:          ir.R(apiID, ir.Field("api_endpoint")),
		},
	}
}
