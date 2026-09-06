package azure

import (
	"fmt"
	"strings"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

// Everything answers HTTPS on its own hostname, so a call is the target's URL and nothing else.
// A private service only answers inside the virtual network, and the caller is marked as on
// the database edge.
func resolveCalls(ctx *resolve.Context, edge ir.Edge, from, to *resolve.Handle) {
	switch from.Exports.(type) {
	case resolve.FunctionExports, resolve.ServiceExports:
	default:
		ctx.Report(ir.ValidationError{
			EdgeID:  edge.ID,
			Message: fmt.Sprintf("%s from a %s is not supported by the azure resolver yet", edge.Relation, from.Node.Type),
		})
		return
	}
	var url ir.Value
	switch target := to.Exports.(type) {
	case resolve.FunctionExports:
		url = target.URL
	case resolve.ServiceExports:
		url = target.URL
		if !target.Public {
			from.NeedsNetwork = true
		}
	default:
		ctx.Report(ir.ValidationError{
			EdgeID:  edge.ID,
			Message: fmt.Sprintf("%s to a %s is not supported by the azure resolver yet", edge.Relation, to.Node.Type),
		})
		return
	}
	from.SetEnv(strings.ToUpper(ctx.Local(to.Node.Name))+"_URL", url)
}
