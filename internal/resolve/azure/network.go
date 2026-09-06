package azure

import (
	"slices"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

type Network struct {
	VirtualNetwork ir.ID
	AppsSubnet     ir.ID
	PostgresSubnet ir.ID
}

const networkLabel = "network"

const joinAction = "Microsoft.Network/virtualNetworks/subnets/join/action"

// Container Apps environments and flexible servers sit on delegated subnets. A function app on
// the consumption plan and Cache for Redis do not join the network.
func needsNetwork(p *ir.Project) bool {
	return slices.ContainsFunc(p.Nodes, func(n ir.Node) bool {
		return n.Type == ir.NodeService || n.Type == ir.NodeDatabase
	})
}

func ensureNetwork(ctx *resolve.Context) *Network {
	if n, ok := ctx.Scratch[networkLabel].(*Network); ok {
		return n
	}
	n := createNetwork(ctx)
	ctx.Scratch[networkLabel] = n
	return n
}

func createNetwork(ctx *resolve.Context) *Network {
	group := ensureGroup(ctx)
	vnet := ctx.Add(ir.Resource{
		Type:        "azurerm_virtual_network",
		Name:        "main",
		SourceLabel: networkLabel,
		Args: group.Located(ctx.Named("vnet"),
			ir.A("address_space", ir.L(ir.Str("10.0.0.0/16"))),
		),
	})
	vnetID := ir.ID{Type: vnet.Type, Name: vnet.Name}
	// A /23 is the floor for a consumption-only Container Apps environment and well above the
	// /27 a workload profiles one needs. A subnet carries one delegation, so a MySQL flexible
	// server will need its own, at 10.0.3.0/24.
	apps := delegatedSubnet(ctx, group, vnetID, "apps", "10.0.0.0/23", "Microsoft.App/environments")
	postgres := delegatedSubnet(ctx, group, vnetID, "postgres", "10.0.2.0/24", "Microsoft.DBforPostgreSQL/flexibleServers")
	return &Network{VirtualNetwork: vnetID, AppsSubnet: apps, PostgresSubnet: postgres}
}

func delegatedSubnet(ctx *resolve.Context, group *Group, vnet ir.ID, name, prefix, service string) ir.ID {
	r := ctx.Add(ir.Resource{
		Type:        "azurerm_subnet",
		Name:        name,
		SourceLabel: networkLabel,
		Args: group.Grouped(ctx.Named(name),
			ir.A("virtual_network_name", ir.R(vnet, ir.Field("name"))),
			ir.A("address_prefixes", ir.L(ir.Str(prefix))),
			ir.A("delegation", ir.B(ir.Attrs{
				ir.A("name", ir.Str(name)),
				ir.A("service_delegation", ir.B(ir.Attrs{
					ir.A("name", ir.Str(service)),
					ir.A("actions", ir.L(ir.Str(joinAction))),
				})),
			})),
		),
	})
	return ir.ID{Type: r.Type, Name: r.Name}
}
