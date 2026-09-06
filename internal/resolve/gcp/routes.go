package gcp

import (
	"fmt"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

func resolveRoutes(ctx *resolve.Context, edge ir.Edge, from, to *resolve.Handle) {
	if _, ok := from.Exports.(resolve.GatewayExports); !ok {
		ctx.Fail(fmt.Sprintf("edge '%s' routes from a %s, which has no gateway exports", edge.ID, from.Node.Type))
	}
	target, ok := to.Exports.(resolve.CloudRunExports)
	if !ok {
		ctx.Report(ir.ValidationError{
			EdgeID:  edge.ID,
			Message: fmt.Sprintf("routes to a %s are not supported by the gcp resolver yet", to.Node.Type),
		})
		return
	}
	claimRoutes(ctx, edge, from)
	exposeTarget(ctx, from, to, target)
}

// Cloud Run does not route by path, so the routes only exist to say what answers where. Two
// edges claiming one still contradict each other, and are refused as they are on AWS.
func claimRoutes(ctx *resolve.Context, edge ir.Edge, from *resolve.Handle) {
	for _, method := range edge.Properties.Methods {
		if owner, ok := ctx.ClaimRoute(from.Node.ID, method, edge.Properties.Path, edge.ID); !ok {
			ctx.Report(ir.ValidationError{
				EdgeID: edge.ID,
				Message: fmt.Sprintf("route '%s %s' on gateway '%s' is already used by edge '%s'",
					method, edge.Properties.Path, from.Node.Name, owner),
			})
		}
	}
}

// A 2nd gen function answers HTTP through its Cloud Run service, so the binding goes on that
// service. roles/cloudfunctions.invoker on the function itself would not open it.
func exposeTarget(ctx *resolve.Context, from, to *resolve.Handle, target resolve.CloudRunExports) {
	base := ctx.Local(from.Node.Name) + "_" + ctx.Local(to.Node.Name)
	bindingID := ir.ID{Type: "google_cloud_run_v2_service_iam_member", Name: base}
	if ctx.HasResource(bindingID) {
		return
	}
	ctx.Add(ir.Resource{
		Type:        bindingID.Type,
		Name:        bindingID.Name,
		SourceNode:  from.Node.ID,
		SourceLabel: from.Node.Name,
		Args: ir.Attrs{
			ir.A("name", target.Service),
			ir.A("location", target.Location),
			ir.A("role", ir.Str("roles/run.invoker")),
			ir.A("member", ir.Str("allUsers")),
		},
	})
	ctx.AddOutput(ir.Output{
		Name:        base + "_url",
		Description: fmt.Sprintf("Public URL of the %s %s the %s gateway routes to", to.Node.Name, to.Node.Type, from.Node.Name),
		Value:       target.URL,
	})
}
