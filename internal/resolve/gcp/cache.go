package gcp

import (
	"fmt"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

const (
	cachePort         = 6379
	cacheRedisVersion = "REDIS_7_2"
	cacheNameMax      = 40
)

func resolveCache(ctx *resolve.Context, node ir.Node) *resolve.Handle {
	p := resolve.Props[ir.CacheProps](ctx, node)
	size, ok := cacheSizes[p.Size]
	if !ok {
		ctx.Fail(fmt.Sprintf("cache '%s' has an unknown size '%s'", node.Name, p.Size))
	}
	local := ctx.Local(node.Name)
	network := ensureNetwork(ctx)

	instance := ctx.Add(ir.Resource{
		Type:        "google_redis_instance",
		Name:        local,
		SourceNode:  node.ID,
		SourceLabel: node.Name,
		Args: ir.Attrs{
			ir.A("name", ir.Str(ctx.Named(node.Name))),
			ir.A("region", ir.Str(ctx.Project.Region)),
			ir.A("tier", ir.Str(size.Tier)),
			ir.A("memory_size_gb", ir.Num(float64(size.MemoryGB))),
			ir.A("redis_version", ir.Str(cacheRedisVersion)),
			ir.A("authorized_network", ir.R(network.VPC, ir.Field("id"))),
			ir.A("connect_mode", ir.Str("PRIVATE_SERVICE_ACCESS")),
			// Callers reach the instance inside the VPC. TLS would also mean fetching the
			// instance's server CA and handing it to every client, which a scaffold cannot do.
			ir.A("transit_encryption_mode", ir.Str("DISABLED")),
		},
		DependsOn: []ir.ID{network.Connection},
	})
	instanceID := ir.ID{Type: instance.Type, Name: instance.Name}
	host := ir.R(instanceID, ir.Field("host"))

	ctx.AddOutput(ir.Output{
		Name:        local + "_host",
		Description: fmt.Sprintf("Private address of the %s cache", node.Name),
		Value:       host,
	})

	return &resolve.Handle{
		Node:    node,
		Primary: instanceID,
		Exports: resolve.CacheExports{Host: host, Port: ir.Num(cachePort)},
	}
}
