package azure

import (
	"fmt"
	"strings"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

// Everything answers HTTPS on its own hostname, so a call is the target's URL and nothing else.
// The hostname is built from the name the resolver chose rather than read off the target's
// resource, so two apps calling each other are not a cycle. A private service only answers
// inside the virtual network, and the caller is marked as on the database edge.
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
	key := strings.ToUpper(ctx.Local(to.Node.Name)) + "_URL"
	switch target := to.Exports.(type) {
	case resolve.FunctionExports:
		from.SetEnv(key, functionURL(ctx, to))
	case resolve.ServiceExports:
		if !target.Public {
			from.NeedsNetwork = true
		}
		from.SetEnv(key, serviceURL(ctx, to))
		// A routes edge after this one can still make the target external, which changes
		// its hostname, so the value is settled again once every edge has run.
		finalise := from.Finalise
		from.Finalise = func() {
			from.SetEnv(key, serviceURL(ctx, to))
			finalise()
		}
	default:
		ctx.Report(ir.ValidationError{
			EdgeID:  edge.ID,
			Message: fmt.Sprintf("%s to a %s is not supported by the azure resolver yet", edge.Relation, to.Node.Type),
		})
	}
}

func functionURL(ctx *resolve.Context, fn *resolve.Handle) ir.Value {
	return ir.Str("https://" + ctx.Named(fn.Node.Name) + ".azurewebsites.net")
}

// The ingress FQDN is the app's name under the environment's default domain, with `internal`
// in between when the ingress is not external.
func serviceURL(ctx *resolve.Context, svc *resolve.Handle) ir.Value {
	host := "https://" + ctx.Named(svc.Node.Name) + "."
	if !svc.Exports.(resolve.ServiceExports).Public {
		host += "internal."
	}
	return ir.C(ir.Str(host), ir.R(ensureEnvironment(ctx), ir.Field("default_domain")))
}
