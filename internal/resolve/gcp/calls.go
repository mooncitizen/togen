package gcp

import (
	"fmt"
	"strings"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

// A function answers through its Cloud Run service, so a call to either grants the same role on
// the same kind of resource. The caller still has to send an identity token with the request.
func resolveCalls(ctx *resolve.Context, edge ir.Edge, from, to *resolve.Handle) {
	caller, ok := from.Exports.(resolve.CloudRunExports)
	if !ok {
		ctx.Report(ir.ValidationError{
			EdgeID:  edge.ID,
			Message: fmt.Sprintf("%s from a %s is not supported by the gcp resolver yet", edge.Relation, from.Node.Type),
		})
		return
	}
	target, ok := to.Exports.(resolve.CloudRunExports)
	if !ok {
		ctx.Report(ir.ValidationError{
			EdgeID:  edge.ID,
			Message: fmt.Sprintf("%s to a %s is not supported by the gcp resolver yet", edge.Relation, to.Node.Type),
		})
		return
	}

	bindingID := ir.ID{
		Type: "google_cloud_run_v2_service_iam_member",
		Name: ctx.Local(to.Node.Name) + "_from_" + ctx.Local(from.Node.Name),
	}
	if !ctx.HasResource(bindingID) {
		ctx.Add(ir.Resource{
			Type:        bindingID.Type,
			Name:        bindingID.Name,
			SourceNode:  to.Node.ID,
			SourceLabel: to.Node.Name,
			Args: ir.Attrs{
				ir.A("name", target.Service),
				ir.A("location", target.Location),
				ir.A("role", ir.Str(invokerRole)),
				ir.A("member", ir.C(ir.Str("serviceAccount:"), caller.ServiceAccount)),
			},
		})
	}
	from.SetEnv(strings.ToUpper(ctx.Local(to.Node.Name))+"_URL", target.URL)
}
