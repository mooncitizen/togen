package gcp

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

func resolveDataAccess(ctx *resolve.Context, edge ir.Edge, from, to *resolve.Handle) {
	caller, ok := from.Exports.(resolve.CloudRunExports)
	if !ok {
		ctx.Report(ir.ValidationError{
			EdgeID:  edge.ID,
			Message: fmt.Sprintf("%s from a %s is not supported by the gcp resolver yet", edge.Relation, from.Node.Type),
		})
		return
	}
	switch target := to.Exports.(type) {
	case resolve.DatabaseExports:
		connectDatabase(ctx, from, to, target)
	case resolve.BucketExports:
		grantBucket(ctx, edge, from, to, caller, target)
	default:
		ctx.Report(ir.ValidationError{
			EdgeID:  edge.ID,
			Message: fmt.Sprintf("%s to a %s is not supported by the gcp resolver yet", edge.Relation, to.Node.Type),
		})
	}
}

// The instance answers on its private address alone, so the caller goes onto the connector. There
// is no port to open: the one user owns the database, and reads and writes wire the same way.
func connectDatabase(ctx *resolve.Context, from, to *resolve.Handle, target resolve.DatabaseExports) {
	port, ok := target.Port.(ir.Number)
	if !ok {
		ctx.Fail(fmt.Sprintf("database '%s' exports a port that is not a number", to.Node.Name))
	}
	prefix := strings.ToUpper(ctx.Local(to.Node.Name))
	from.SetEnv(prefix+"_HOST", target.Host)
	from.SetEnv(prefix+"_PORT", ir.Str(strconv.FormatFloat(float64(port), 'f', -1, 64)))
	from.SetEnv(prefix+"_NAME", target.Name)
	from.SetEnv(prefix+"_USER", target.User)
	from.SetEnv(prefix+"_PASSWORD", target.Password)
	from.NeedsNetwork = true
}

// Storage answers on its public endpoint, so a bucket edge grants a role and joins nothing.
func grantBucket(ctx *resolve.Context, edge ir.Edge, from, to *resolve.Handle, caller resolve.CloudRunExports, target resolve.BucketExports) {
	role := objectViewerRole
	if edge.Relation == ir.RelWrites {
		role = objectUserRole
	}
	bindingID := ir.ID{
		Type: "google_storage_bucket_iam_member",
		Name: ctx.Local(to.Node.Name) + "_" + string(edge.Relation) + "_from_" + ctx.Local(from.Node.Name),
	}
	if !ctx.HasResource(bindingID) {
		ctx.Add(ir.Resource{
			Type:        bindingID.Type,
			Name:        bindingID.Name,
			SourceNode:  to.Node.ID,
			SourceLabel: to.Node.Name,
			Args: ir.Attrs{
				ir.A("bucket", target.ID),
				ir.A("role", ir.Str(role)),
				ir.A("member", member(caller)),
			},
		})
	}
	from.SetEnv(strings.ToUpper(ctx.Local(to.Node.Name))+"_BUCKET", target.Name)
}
