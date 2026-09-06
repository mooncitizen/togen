package gcp

import (
	"fmt"

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
	}
}

func (provider) ResolveNode(ctx *resolve.Context, n ir.Node) (*resolve.Handle, bool) {
	switch n.Type {
	case ir.NodeGateway:
		return resolveGateway(n), true
	case ir.NodeFunction:
		return resolveFunction(ctx, n), true
	}
	ctx.Report(ir.ValidationError{
		NodeID:  n.ID,
		Message: fmt.Sprintf("node type '%s' is not supported by the gcp resolver yet", n.Type),
	})
	return nil, false
}

func (provider) ResolveEdge(ctx *resolve.Context, e ir.Edge, from, to *resolve.Handle) {
	switch e.Relation {
	case ir.RelRoutes:
		resolveRoutes(ctx, e, from, to)
	default:
		ctx.Report(ir.ValidationError{
			EdgeID:  e.ID,
			Message: fmt.Sprintf("'%s' edges are not supported by the gcp resolver yet", e.Relation),
		})
	}
}

func nodeProps[T any](ctx *resolve.Context, n ir.Node) T {
	p, err := ir.NodeProps[T](n)
	if err != nil {
		ctx.Fail(fmt.Sprintf("node '%s' has properties the gcp resolver cannot read: %v", n.ID, err))
	}
	return p
}
