package gcp

import (
	"fmt"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

const invokerRole = "roles/run.invoker"

func resolveService(ctx *resolve.Context, node ir.Node) *resolve.Handle {
	p := resolve.Props[ir.ServiceProps](ctx, node)
	size, ok := serviceSizes[p.Size]
	if !ok {
		ctx.Fail(fmt.Sprintf("service '%s' has an unknown size '%s'", node.Name, p.Size))
	}
	local := ctx.Local(node.Name)

	account := ctx.Add(ir.Resource{
		Type:        "google_service_account",
		Name:        local,
		SourceNode:  node.ID,
		SourceLabel: node.Name,
		Args: ir.Attrs{
			ir.A("account_id", ir.Str(ctx.Named(node.Name))),
			ir.A("display_name", ir.Str(ctx.Named(node.Name))),
		},
	})
	accountID := ir.ID{Type: account.Type, Name: account.Name}

	ingress := "INGRESS_TRAFFIC_INTERNAL_ONLY"
	if p.Public {
		ingress = "INGRESS_TRAFFIC_ALL"
	}
	container := ir.Attrs{
		ir.A("image", ir.Str(p.Image)),
		ir.A("ports", ir.B(ir.Attrs{ir.A("container_port", ir.Num(float64(p.Port)))})),
		ir.A("resources", ir.B(ir.Attrs{
			ir.A("limits", ir.M(ir.A("cpu", ir.Str(size.CPU)), ir.A("memory", ir.Str(size.Memory)))),
		})),
	}
	template := ir.Attrs{
		ir.A("service_account", ir.R(accountID, ir.Field("email"))),
		ir.A("scaling", ir.B(ir.Attrs{
			ir.A("min_instance_count", ir.Num(float64(p.MinReplicas))),
			ir.A("max_instance_count", ir.Num(float64(p.MaxReplicas))),
		})),
	}
	svc := ctx.Add(ir.Resource{
		Type:        "google_cloud_run_v2_service",
		Name:        local,
		SourceNode:  node.ID,
		SourceLabel: node.Name,
		Args: ir.Attrs{
			ir.A("name", ir.Str(ctx.Named(node.Name))),
			ir.A("location", ir.Str(ctx.Project.Region)),
			ir.A("deletion_protection", ir.Bool(false)),
			ir.A("ingress", ir.Str(ingress)),
		},
	})
	svcID := ir.ID{Type: svc.Type, Name: svc.Name}
	url := ir.R(svcID, ir.Field("uri"))

	if p.Public {
		binding := publicBindingID(ctx, node)
		ctx.Add(ir.Resource{
			Type:        binding.Type,
			Name:        binding.Name,
			SourceNode:  node.ID,
			SourceLabel: node.Name,
			Args: ir.Attrs{
				ir.A("name", ir.R(svcID, ir.Field("name"))),
				ir.A("location", ir.R(svcID, ir.Field("location"))),
				ir.A("role", ir.Str(invokerRole)),
				ir.A("member", ir.Str("allUsers")),
			},
		})
		ctx.AddOutput(ir.Output{
			Name:        local + "_url",
			Description: fmt.Sprintf("Public URL of the %s service", node.Name),
			Value:       url,
		})
	}

	h := &resolve.Handle{
		Node:    node,
		Primary: svcID,
		Env:     resolve.SortedEnv(p.Env),
		Exports: resolve.CloudRunExports{
			Service:        ir.R(svcID, ir.Field("name")),
			Location:       ir.R(svcID, ir.Field("location")),
			URL:            url,
			ServiceAccount: ir.R(accountID, ir.Field("email")),
		},
	}
	h.Finalise = func() {
		if len(h.Env) > 0 {
			container.Set("env", containerEnv(h.Env))
		}
		if h.NeedsNetwork {
			network := ensureNetwork(ctx)
			template.Set("vpc_access", ir.B(ir.Attrs{
				ir.A("connector", ir.R(network.Connector, ir.Field("id"))),
				ir.A("egress", ir.Str("PRIVATE_RANGES_ONLY")),
			}))
		}
		template.Set("containers", ir.B(container))
		svc.Args.Set("template", ir.B(template))
	}
	return h
}

// A public service already answers to allUsers, so a route to it adds the output alone.
func publicBindingID(ctx *resolve.Context, node ir.Node) ir.ID {
	return ir.ID{Type: "google_cloud_run_v2_service_iam_member", Name: ctx.Local(node.Name)}
}

func containerEnv(env ir.Attrs) ir.Block {
	out := make(ir.Block, 0, len(env))
	for _, a := range env {
		out = append(out, ir.Attrs{ir.A("name", ir.Str(a.Key)), ir.A("value", a.Value)})
	}
	return out
}
