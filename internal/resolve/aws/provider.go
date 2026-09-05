package aws

import (
	"fmt"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

type provider struct{}

func New() resolve.Provider { return provider{} }

func (provider) Name() ir.CloudProvider { return ir.ProviderAWS }

func (provider) ProviderBlock(p *ir.Project) ir.Provider {
	return ir.Provider{
		Name:    "aws",
		Source:  "hashicorp/aws",
		Version: "~> 6.0",
		Config:  ir.Attrs{ir.A("region", ir.Str(p.Region))},
	}
}

func (provider) NameLimits() []resolve.NameLimit {
	return []resolve.NameLimit{
		{Type: "aws_lambda_function", Arg: "function_name", Max: 64},
		{Type: "aws_db_instance", Arg: "identifier", Max: 63},
		{Type: "aws_lb", Arg: "name", Max: 32},
		{Type: "aws_lb_target_group", Arg: "name", Max: 32},
		{Type: "aws_sqs_queue", Arg: "name", Max: 80},
		{Type: "aws_elasticache_replication_group", Arg: "replication_group_id", Max: 40},
		// 63 minus the 26 character suffix Terraform appends to a bucket prefix.
		{Type: "aws_s3_bucket", Arg: "bucket_prefix", Max: 37},
	}
}

func (provider) ResolveNode(ctx *resolve.Context, n ir.Node) (*resolve.Handle, bool) {
	switch n.Type {
	case ir.NodeService:
		return resolveService(ctx, n), true
	case ir.NodeGateway:
		return resolveGateway(ctx, n), true
	case ir.NodeFunction:
		return resolveFunction(ctx, n), true
	case ir.NodeDatabase:
		return resolveDatabase(ctx, n), true
	case ir.NodeQueue:
		return resolveQueue(ctx, n), true
	case ir.NodeBucket:
		return resolveBucket(ctx, n), true
	case ir.NodeCache:
		return resolveCache(ctx, n), true
	}
	ctx.Report(ir.ValidationError{
		NodeID:  n.ID,
		Message: fmt.Sprintf("node type '%s' is not supported by the aws resolver yet", n.Type),
	})
	return nil, false
}

func (provider) ResolveEdge(ctx *resolve.Context, e ir.Edge, from, to *resolve.Handle) {
	switch e.Relation {
	case ir.RelRoutes:
		resolveRoutes(ctx, e, from, to)
	case ir.RelCalls:
		resolveCalls(ctx, e, from, to)
	case ir.RelReads, ir.RelWrites:
		resolveDataAccess(ctx, e, from, to)
	case ir.RelPublishes, ir.RelConsumes:
		resolveMessaging(ctx, e, from, to)
	default:
		ctx.Report(ir.ValidationError{
			EdgeID:  e.ID,
			Message: fmt.Sprintf("'%s' edges are not supported by the aws resolver yet", e.Relation),
		})
	}
}

func nodeProps[T any](ctx *resolve.Context, n ir.Node) T {
	p, err := ir.NodeProps[T](n)
	if err != nil {
		ctx.Fail(fmt.Sprintf("node '%s' has properties the aws resolver cannot read: %v", n.ID, err))
	}
	return p
}
