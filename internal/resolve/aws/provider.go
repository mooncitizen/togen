package aws

import (
	"fmt"
	"slices"

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

// Every type the aws catalogue says it generates has a function here, and a test holds
// the two together.
var resolvers = map[ir.NodeType]func(*resolve.Context, ir.Node) *resolve.Handle{
	ir.NodeService:  resolveService,
	ir.NodeGateway:  resolveGateway,
	ir.NodeFunction: resolveFunction,
	ir.NodeDatabase: resolveDatabase,
	ir.NodeQueue:    resolveQueue,
	ir.NodeBucket:   resolveBucket,
	ir.NodeCache:    resolveCache,
}

func Resolvers() []ir.NodeType {
	out := make([]ir.NodeType, 0, len(resolvers))
	for t := range resolvers {
		out = append(out, t)
	}
	slices.Sort(out)
	return out
}

func (provider) ResolveNode(ctx *resolve.Context, n ir.Node) (*resolve.Handle, bool) {
	resolveNode, ok := resolvers[n.Type]
	if !ok {
		ctx.Report(ir.ValidationError{
			NodeID:  n.ID,
			Message: fmt.Sprintf("the aws resolver has no function for '%s'", n.Type),
		})
		return nil, false
	}
	return resolveNode(ctx, n), true
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
