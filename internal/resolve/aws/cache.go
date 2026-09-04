package aws

import (
	"fmt"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

const cachePort = 6379

func resolveCache(ctx *resolve.Context, node ir.Node) *resolve.Handle {
	p := nodeProps[ir.CacheProps](ctx, node)
	local := ctx.Local(node.Name)
	network := ensureNetwork(ctx)

	subnetGroup := ctx.Add(ir.Resource{
		Type:        "aws_elasticache_subnet_group",
		Name:        local,
		SourceNode:  node.ID,
		SourceLabel: node.Name,
		Args: ir.Attrs{
			ir.A("name", ir.Str(ctx.Named(node.Name))),
			ir.A("subnet_ids", subnetRefs(network.PrivateSubnets)),
		},
	})
	sg := ctx.Add(ir.Resource{
		Type:        "aws_security_group",
		Name:        local,
		SourceNode:  node.ID,
		SourceLabel: node.Name,
		Args: ir.Attrs{
			ir.A("name", ir.Str(ctx.Named(node.Name))),
			ir.A("description", ir.Str("Access to "+node.Name)),
			ir.A("vpc_id", ir.R(network.VPC, ir.Field("id"))),
		},
	})
	sgID := ir.ID{Type: sg.Type, Name: sg.Name}

	group := ctx.Add(ir.Resource{
		Type:        "aws_elasticache_replication_group",
		Name:        local,
		SourceNode:  node.ID,
		SourceLabel: node.Name,
		Args: ir.Attrs{
			ir.A("replication_group_id", ir.Str(ctx.Named(node.Name))),
			ir.A("description", ir.Str(node.Name+" cache")),
			ir.A("engine", ir.Str("redis")),
			ir.A("engine_version", ir.Str("7.1")),
			ir.A("node_type", ir.Str(cacheSizes[p.Size])),
			ir.A("num_cache_clusters", ir.Num(1)),
			ir.A("port", ir.Num(cachePort)),
			ir.A("subnet_group_name", ir.R(ir.ID{Type: subnetGroup.Type, Name: subnetGroup.Name}, ir.Field("name"))),
			ir.A("security_group_ids", ir.L(ir.R(sgID, ir.Field("id")))),
			ir.A("at_rest_encryption_enabled", ir.Bool(true)),
			ir.A("transit_encryption_enabled", ir.Bool(true)),
			// Failing over wants a second cluster, which a scaffold should not be paying for.
			ir.A("automatic_failover_enabled", ir.Bool(false)),
			ir.A("apply_immediately", ir.Bool(true)),
		},
	})
	groupID := ir.ID{Type: group.Type, Name: group.Name}
	host := ir.R(groupID, ir.Field("primary_endpoint_address"))

	ctx.AddOutput(ir.Output{
		Name:        local + "_endpoint",
		Description: fmt.Sprintf("Endpoint of the %s cache", node.Name),
		Value:       host,
	})

	return &resolve.Handle{
		Node:          node,
		Primary:       groupID,
		SecurityGroup: &sgID,
		Exports:       resolve.CacheExports{Host: host, Port: ir.Num(cachePort)},
	}
}
