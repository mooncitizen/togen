package azure

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

func resolveDataAccess(ctx *resolve.Context, edge ir.Edge, from, to *resolve.Handle) {
	switch from.Exports.(type) {
	case resolve.FunctionExports, resolve.ServiceExports:
	default:
		ctx.Report(ir.ValidationError{
			EdgeID:  edge.ID,
			Message: fmt.Sprintf("%s from a %s is not supported by the azure resolver yet", edge.Relation, from.Node.Type),
		})
		return
	}
	target, ok := to.Exports.(resolve.DatabaseExports)
	if !ok {
		ctx.Report(ir.ValidationError{
			EdgeID:  edge.ID,
			Message: fmt.Sprintf("%s to a %s is not supported by the azure resolver yet", edge.Relation, to.Node.Type),
		})
		return
	}
	connectDatabase(ctx, from, to, target)
}

// The server answers only from inside the virtual network, so the caller is marked as needing
// it and its own resolver decides what that means: a Container App joins, a function app on
// the consumption plan cannot and gets the settings all the same.
func connectDatabase(ctx *resolve.Context, from, to *resolve.Handle, target resolve.DatabaseExports) {
	port, ok := target.Port.(ir.Number)
	if !ok {
		ctx.Fail(fmt.Sprintf("database '%s' exports a port that is not a number", to.Node.Name))
	}
	from.NeedsNetwork = true
	prefix := strings.ToUpper(ctx.Local(to.Node.Name))
	from.SetEnv(prefix+"_HOST", target.Host)
	from.SetEnv(prefix+"_PORT", ir.Str(strconv.FormatFloat(float64(port), 'f', -1, 64)))
	from.SetEnv(prefix+"_NAME", target.Name)
	from.SetEnv(prefix+"_USER", target.User)
	from.SetEnv(prefix+"_PASSWORD", target.Password)
}
