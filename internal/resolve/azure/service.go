package azure

import (
	"fmt"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

func resolveService(ctx *resolve.Context, node ir.Node) *resolve.Handle {
	p := resolve.Props[ir.ServiceProps](ctx, node)
	size, ok := appSizes[p.Size]
	if !ok {
		ctx.Fail(fmt.Sprintf("service '%s' has an unknown size '%s'", node.Name, p.Size))
	}
	local := ctx.Local(node.Name)
	group := ensureGroup(ctx)
	environment := ensureEnvironment(ctx)

	// No location: a container app lives wherever its environment does.
	app := ctx.Add(ir.Resource{
		Type:        "azurerm_container_app",
		Name:        local,
		SourceNode:  node.ID,
		SourceLabel: node.Name,
		Args: group.Grouped(ctx.Named(node.Name),
			ir.A("container_app_environment_id", ir.R(environment, ir.Field("id"))),
			ir.A("revision_mode", ir.Str("Single")),
			ir.A("workload_profile_name", ir.Str(consumptionProfile)),
			ir.A("identity", ir.B(ir.Attrs{ir.A("type", ir.Str("SystemAssigned"))})),
		),
	})
	appID := ir.ID{Type: app.Type, Name: app.Name}

	exports := resolve.ServiceExports{
		Port:        ir.Num(float64(p.Port)),
		Public:      p.Public,
		URL:         ir.C(ir.Str("https://"), ir.R(appID, ir.Field("ingress"), ir.Index(0), ir.Field("fqdn"))),
		PrincipalID: ir.R(appID, ir.Field("identity"), ir.Index(0), ir.Field("principal_id")),
	}
	if p.Public {
		ctx.AddOutput(ir.Output{
			Name:        local + "_url",
			Description: fmt.Sprintf("Public URL of the %s service", node.Name),
			Value:       exports.URL,
		})
	}

	h := &resolve.Handle{
		Node:    node,
		Primary: appID,
		Env:     resolve.SortedEnv(p.Env),
		Exports: exports,
	}
	// Edges add env and a route can make the ingress external, so both blocks wait for them.
	h.Finalise = func() {
		container := ir.Attrs{
			ir.A("name", ir.Str(node.Name)),
			ir.A("image", ir.Str(p.Image)),
			ir.A("cpu", ir.Num(size.cpu)),
			ir.A("memory", ir.Str(size.memory)),
		}
		if len(h.Env) > 0 {
			container = append(container, ir.A("env", containerEnv(h.Env)))
		}
		app.Args.Set("template", ir.B(ir.Attrs{
			ir.A("min_replicas", ir.Num(float64(p.MinReplicas))),
			ir.A("max_replicas", ir.Num(float64(p.MaxReplicas))),
			ir.A("container", ir.B(container)),
		}))
		app.Args.Set("ingress", ir.B(ir.Attrs{
			ir.A("external_enabled", ir.Bool(h.Exports.(resolve.ServiceExports).Public)),
			ir.A("target_port", ir.Num(float64(p.Port))),
			ir.A("traffic_weight", ir.B(ir.Attrs{
				ir.A("percentage", ir.Num(100)),
				ir.A("latest_revision", ir.Bool(true)),
			})),
		}))
	}
	return h
}

func containerEnv(env ir.Attrs) ir.Block {
	entries := make([]ir.Attrs, 0, len(env))
	for _, a := range env {
		entries = append(entries, ir.Attrs{ir.A("name", ir.Str(a.Key)), ir.A("value", a.Value)})
	}
	return ir.B(entries...)
}
