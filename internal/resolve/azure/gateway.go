package azure

import (
	"fmt"
	"strings"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

// There is no gateway resource on Azure (ADR 0010). A route hands out the target's own URL.
func resolveGateway(node ir.Node) *resolve.Handle {
	return &resolve.Handle{Node: node, Exports: resolve.GatewayExports{}}
}

func resolveRoutes(ctx *resolve.Context, edge ir.Edge, from, to *resolve.Handle) {
	if _, ok := from.Exports.(resolve.GatewayExports); !ok {
		ctx.Fail(fmt.Sprintf("edge '%s' routes from a %s, which has no gateway exports", edge.ID, from.Node.Type))
	}
	target, ok := to.Exports.(resolve.FunctionExports)
	if !ok {
		ctx.Report(ir.ValidationError{
			EdgeID:  edge.ID,
			Message: fmt.Sprintf("routes to a %s are not supported by the azure resolver yet", to.Node.Type),
		})
		return
	}

	var keys []string
	for _, method := range edge.Properties.Methods {
		if owner, ok := ctx.ClaimRoute(from.Node.ID, method, edge.Properties.Path, edge.ID); !ok {
			ctx.Report(ir.ValidationError{
				EdgeID: edge.ID,
				Message: fmt.Sprintf("route '%s %s' on gateway '%s' is already used by edge '%s'",
					method, edge.Properties.Path, from.Node.Name, owner),
			})
			continue
		}
		keys = append(keys, fmt.Sprintf("%s %s", method, edge.Properties.Path))
	}
	recordRoutes(ctx, from, to, keys)
	addURLOutput(ctx, from, to, target.URL)
}

// The function's own HTTP triggers do the routing, so the paths and methods go in as an app
// setting the code can read, accumulated across the edges from one gateway.
func recordRoutes(ctx *resolve.Context, from, to *resolve.Handle, keys []string) {
	if len(keys) == 0 {
		return
	}
	name := strings.ToUpper(ctx.Local(from.Node.Name)) + "_ROUTES"
	if existing, ok := to.Env.Get(name); ok {
		if s, ok := existing.(ir.String); ok {
			keys = append([]string{string(s)}, keys...)
		}
	}
	to.SetEnv(name, ir.Str(strings.Join(keys, ",")))
}

func addURLOutput(ctx *resolve.Context, from, to *resolve.Handle, url ir.Value) {
	name := ctx.Local(from.Node.Name) + "_" + ctx.Local(to.Node.Name) + "_url"
	for _, o := range ctx.Outputs {
		if o.Name == name {
			return
		}
	}
	ctx.AddOutput(ir.Output{
		Name:        name,
		Description: fmt.Sprintf("Public URL of the %s %s behind the %s gateway", to.Node.Name, to.Node.Type, from.Node.Name),
		Value:       url,
	})
}
