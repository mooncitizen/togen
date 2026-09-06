package gcp

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

func resolveDataAccess(ctx *resolve.Context, edge ir.Edge, from, to *resolve.Handle) {
	if _, ok := from.Exports.(resolve.CloudRunExports); !ok {
		ctx.Report(ir.ValidationError{
			EdgeID:  edge.ID,
			Message: fmt.Sprintf("%s from a %s is not supported by the gcp resolver yet", edge.Relation, from.Node.Type),
		})
		return
	}
	switch target := to.Exports.(type) {
	case resolve.DatabaseExports:
		connectDatabase(ctx, from, to, target)
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
