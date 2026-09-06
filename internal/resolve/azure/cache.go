package azure

import (
	"fmt"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

// Version 4 is retired for new caches, so 6 is the one the provider still creates.
const redisVersion = "6"

func resolveCache(ctx *resolve.Context, node ir.Node) *resolve.Handle {
	p := resolve.Props[ir.CacheProps](ctx, node)
	sku, ok := cacheSizes[p.Size]
	if !ok {
		ctx.Fail(fmt.Sprintf("cache '%s' has an unknown size '%s'", node.Name, p.Size))
	}
	local := ctx.Local(node.Name)
	group := ensureGroup(ctx)

	cache := ctx.Add(ir.Resource{
		Type:        "azurerm_redis_cache",
		Name:        local,
		SourceNode:  node.ID,
		SourceLabel: node.Name,
		Args: group.Located(ctx.Named(node.Name),
			ir.A("capacity", ir.Num(sku.capacity)),
			ir.A("family", ir.Str(sku.family)),
			ir.A("sku_name", ir.Str(sku.name)),
			ir.A("non_ssl_port_enabled", ir.Bool(false)),
			ir.A("minimum_tls_version", ir.Str("1.2")),
			ir.A("redis_version", ir.Str(redisVersion)),
		),
	})
	cacheID := ir.ID{Type: cache.Type, Name: cache.Name}
	host := ir.R(cacheID, ir.Field("hostname"))

	ctx.AddOutput(ir.Output{
		Name:        local + "_hostname",
		Description: fmt.Sprintf("Hostname of the %s cache", node.Name),
		Value:       host,
	})

	return &resolve.Handle{
		Node:    node,
		Primary: cacheID,
		Exports: resolve.CacheExports{
			Host:     host,
			Port:     ir.R(cacheID, ir.Field("ssl_port")),
			Password: ir.R(cacheID, ir.Field("primary_access_key")),
		},
	}
}
