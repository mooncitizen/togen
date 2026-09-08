package gcp

import (
	"fmt"
	"slices"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

type provider struct{}

func New() resolve.Provider { return provider{} }

func (provider) Name() ir.CloudProvider { return ir.ProviderGCP }

const projectVar = "project"

func (provider) ProviderBlock(p *ir.Project) ir.Provider {
	return ir.Provider{
		Name:    "google",
		Source:  "hashicorp/google",
		Version: "~> 8.0",
		Config: ir.Attrs{
			ir.A("project", ir.V(projectVar)),
			ir.A("region", ir.Str(p.Region)),
		},
	}
}

func (provider) Variables(*ir.Project) []ir.Variable {
	return []ir.Variable{{
		Name:        projectVar,
		Description: "The id of the Google Cloud project to deploy into",
		Type:        "string",
	}}
}

func (provider) NameLimits() []resolve.NameLimit {
	return []resolve.NameLimit{
		{Type: "google_cloud_run_v2_service", Arg: "name", Max: 63},
		{Type: "google_cloudfunctions2_function", Arg: "name", Max: functionNameMax},
		{Type: "google_service_account", Arg: "account_id", Max: accountIDMax},
		{Type: "google_vpc_access_connector", Arg: "name", Max: connectorNameMax},
		{Type: "google_redis_instance", Arg: "name", Max: cacheNameMax},
	}
}

// Every type the gcp catalogue says it generates has a function here, and a test holds
// the two together.
var resolvers = map[ir.NodeType]func(*resolve.Context, ir.Node) *resolve.Handle{
	ir.NodeGateway:  func(_ *resolve.Context, n ir.Node) *resolve.Handle { return resolveGateway(n) },
	ir.NodeFunction: resolveFunction,
	ir.NodeService:  resolveService,
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
			Message: fmt.Sprintf("the gcp resolver has no function for '%s'", n.Type),
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
			Message: fmt.Sprintf("'%s' edges are not supported by the gcp resolver yet", e.Relation),
		})
	}
}
