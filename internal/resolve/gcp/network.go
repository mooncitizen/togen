package gcp

import (
	"strings"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

// Connection is the service networking peering; Cloud SQL and Memorystore must depend on it
// explicitly because Terraform does not infer that from their arguments.
type Network struct {
	VPC        ir.ID
	Subnet     ir.ID
	Connector  ir.ID
	Connection ir.ID
	Router     ir.ID
	NAT        ir.ID
}

const (
	networkLabel     = "network"
	connectorNameMax = 25
)

func ensureNetwork(ctx *resolve.Context) *Network {
	if n, ok := ctx.Scratch[networkLabel].(*Network); ok {
		return n
	}
	n := createNetwork(ctx)
	ctx.Scratch[networkLabel] = n
	return n
}

func createNetwork(ctx *resolve.Context) *Network {
	vpc := ctx.Add(ir.Resource{
		Type:        "google_compute_network",
		Name:        "main",
		SourceLabel: networkLabel,
		Args: ir.Attrs{
			ir.A("name", ir.Str(ctx.Named("network"))),
			ir.A("auto_create_subnetworks", ir.Bool(false)),
		},
	})
	vpcID := ir.ID{Type: vpc.Type, Name: vpc.Name}
	vpcRef := ir.R(vpcID, ir.Field("id"))

	subnet := ctx.Add(ir.Resource{
		Type:        "google_compute_subnetwork",
		Name:        "main",
		SourceLabel: networkLabel,
		Args: ir.Attrs{
			ir.A("name", ir.Str(ctx.Named("subnet"))),
			ir.A("network", vpcRef),
			ir.A("region", ir.Str(ctx.Project.Region)),
			ir.A("ip_cidr_range", ir.Str("10.0.0.0/24")),
			ir.A("private_ip_google_access", ir.Bool(true)),
		},
	})

	connector := ctx.Add(ir.Resource{
		Type:        "google_vpc_access_connector",
		Name:        "main",
		SourceLabel: networkLabel,
		Args: ir.Attrs{
			ir.A("name", ir.Str(connectorName(ctx))),
			ir.A("network", ir.R(vpcID, ir.Field("name"))),
			ir.A("region", ir.Str(ctx.Project.Region)),
			ir.A("ip_cidr_range", ir.Str("10.8.0.0/28")),
			ir.A("min_instances", ir.Num(2)),
			ir.A("max_instances", ir.Num(3)),
		},
	})

	peering := ctx.Add(ir.Resource{
		Type:        "google_compute_global_address",
		Name:        "peering",
		SourceLabel: networkLabel,
		Args: ir.Attrs{
			ir.A("name", ir.Str(ctx.Named("peering"))),
			ir.A("purpose", ir.Str("VPC_PEERING")),
			ir.A("address_type", ir.Str("INTERNAL")),
			ir.A("prefix_length", ir.Num(16)),
			ir.A("network", vpcRef),
		},
	})

	connection := ctx.Add(ir.Resource{
		Type:        "google_service_networking_connection",
		Name:        "main",
		SourceLabel: networkLabel,
		Args: ir.Attrs{
			ir.A("network", vpcRef),
			ir.A("service", ir.Str("servicenetworking.googleapis.com")),
			ir.A("reserved_peering_ranges", ir.L(ir.R(ir.ID{Type: peering.Type, Name: peering.Name}, ir.Field("name")))),
		},
	})

	return &Network{
		VPC:        vpcID,
		Subnet:     ir.ID{Type: subnet.Type, Name: subnet.Name},
		Connector:  ir.ID{Type: connector.Type, Name: connector.Name},
		Connection: ir.ID{Type: connection.Type, Name: connection.Name},
	}
}

// A caller that sends all its egress through the connector loses the internet unless the
// network translates for it, so the router and NAT appear with the first such caller.
func ensureNAT(ctx *resolve.Context) *Network {
	n := ensureNetwork(ctx)
	if n.NAT != (ir.ID{}) {
		return n
	}
	router := ctx.Add(ir.Resource{
		Type:        "google_compute_router",
		Name:        "main",
		SourceLabel: networkLabel,
		Args: ir.Attrs{
			ir.A("name", ir.Str(ctx.Named("router"))),
			ir.A("network", ir.R(n.VPC, ir.Field("id"))),
			ir.A("region", ir.Str(ctx.Project.Region)),
		},
	})
	n.Router = ir.ID{Type: router.Type, Name: router.Name}
	nat := ctx.Add(ir.Resource{
		Type:        "google_compute_router_nat",
		Name:        "main",
		SourceLabel: networkLabel,
		Args: ir.Attrs{
			ir.A("name", ir.Str(ctx.Named("nat"))),
			ir.A("router", ir.R(n.Router, ir.Field("name"))),
			ir.A("region", ir.Str(ctx.Project.Region)),
			ir.A("nat_ip_allocate_option", ir.Str("AUTO_ONLY")),
			ir.A("source_subnetwork_ip_ranges_to_nat", ir.Str("ALL_SUBNETWORKS_ALL_IP_RANGES")),
		},
	})
	n.NAT = ir.ID{Type: nat.Type, Name: nat.Name}
	return n
}

// A long project and environment give way to the suffix, so the name still says what it is.
func connectorName(ctx *resolve.Context) string {
	const suffix = "-connector"
	prefix := ctx.Prefix()
	if room := connectorNameMax - len(suffix); len(prefix) > room {
		prefix = strings.TrimRight(prefix[:room], "-")
	}
	return prefix + suffix
}
