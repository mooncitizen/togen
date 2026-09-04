package aws

import (
	"fmt"

	"togen/internal/ir"
	"togen/internal/resolve"
)

type Network struct {
	VPC            ir.ID
	PublicSubnets  []ir.ID
	PrivateSubnets []ir.ID
}

const networkLabel = "network"

var zones = []string{"a", "b"}

func ensureNetwork(ctx *resolve.Context) *Network {
	if n, ok := ctx.Scratch[networkLabel].(*Network); ok {
		return n
	}
	n := createNetwork(ctx)
	ctx.Scratch[networkLabel] = n
	return n
}

func createNetwork(ctx *resolve.Context) *Network {
	azs := ctx.AddData(ir.DataSource{
		Type:        "aws_availability_zones",
		Name:        "available",
		Args:        ir.Attrs{ir.A("state", ir.Str("available"))},
		SourceLabel: networkLabel,
	})
	azID := ir.ID{Type: azs.Type, Name: azs.Name}

	vpc := ctx.Add(ir.Resource{
		Type:        "aws_vpc",
		Name:        "main",
		SourceLabel: networkLabel,
		Args: ir.Attrs{
			ir.A("cidr_block", ir.Str("10.0.0.0/16")),
			ir.A("enable_dns_support", ir.Bool(true)),
			ir.A("enable_dns_hostnames", ir.Bool(true)),
			ir.A("tags", nameTag(ctx.Named("vpc"))),
		},
	})
	vpcID := ir.ID{Type: vpc.Type, Name: vpc.Name}
	vpcRef := ir.R(vpcID, ir.Field("id"))

	igw := ctx.Add(ir.Resource{
		Type:        "aws_internet_gateway",
		Name:        "main",
		SourceLabel: networkLabel,
		Args: ir.Attrs{
			ir.A("vpc_id", vpcRef),
			ir.A("tags", nameTag(ctx.Named("igw"))),
		},
	})
	igwID := ir.ID{Type: igw.Type, Name: igw.Name}

	var public, private []ir.ID
	for i, zone := range zones {
		az := ir.D(azID, ir.Field("names"), ir.Index(i))
		pub := ctx.Add(ir.Resource{
			Type:        "aws_subnet",
			Name:        "public_" + zone,
			SourceLabel: networkLabel,
			Args: ir.Attrs{
				ir.A("vpc_id", vpcRef),
				ir.A("cidr_block", ir.Str(fmt.Sprintf("10.0.%d.0/24", i))),
				ir.A("availability_zone", az),
				ir.A("map_public_ip_on_launch", ir.Bool(true)),
				ir.A("tags", nameTag(ctx.Named("public-"+zone))),
			},
		})
		public = append(public, ir.ID{Type: pub.Type, Name: pub.Name})
		priv := ctx.Add(ir.Resource{
			Type:        "aws_subnet",
			Name:        "private_" + zone,
			SourceLabel: networkLabel,
			Args: ir.Attrs{
				ir.A("vpc_id", vpcRef),
				ir.A("cidr_block", ir.Str(fmt.Sprintf("10.0.%d.0/24", 10+i))),
				ir.A("availability_zone", az),
				ir.A("tags", nameTag(ctx.Named("private-"+zone))),
			},
		})
		private = append(private, ir.ID{Type: priv.Type, Name: priv.Name})
	}

	eip := ctx.Add(ir.Resource{
		Type:        "aws_eip",
		Name:        "nat",
		SourceLabel: networkLabel,
		Args: ir.Attrs{
			ir.A("domain", ir.Str("vpc")),
			ir.A("tags", nameTag(ctx.Named("nat"))),
		},
	})
	nat := ctx.Add(ir.Resource{
		Type:        "aws_nat_gateway",
		Name:        "main",
		SourceLabel: networkLabel,
		Args: ir.Attrs{
			ir.A("allocation_id", ir.R(ir.ID{Type: eip.Type, Name: eip.Name}, ir.Field("id"))),
			ir.A("subnet_id", ir.R(public[0], ir.Field("id"))),
			ir.A("tags", nameTag(ctx.Named("nat"))),
		},
		DependsOn: []ir.ID{igwID},
	})

	publicRT := ctx.Add(ir.Resource{
		Type:        "aws_route_table",
		Name:        "public",
		SourceLabel: networkLabel,
		Args: ir.Attrs{
			ir.A("vpc_id", vpcRef),
			ir.A("route", ir.B(ir.Attrs{
				ir.A("cidr_block", ir.Str("0.0.0.0/0")),
				ir.A("gateway_id", ir.R(igwID, ir.Field("id"))),
			})),
			ir.A("tags", nameTag(ctx.Named("public"))),
		},
	})
	privateRT := ctx.Add(ir.Resource{
		Type:        "aws_route_table",
		Name:        "private",
		SourceLabel: networkLabel,
		Args: ir.Attrs{
			ir.A("vpc_id", vpcRef),
			ir.A("route", ir.B(ir.Attrs{
				ir.A("cidr_block", ir.Str("0.0.0.0/0")),
				ir.A("nat_gateway_id", ir.R(ir.ID{Type: nat.Type, Name: nat.Name}, ir.Field("id"))),
			})),
			ir.A("tags", nameTag(ctx.Named("private"))),
		},
	})
	publicRTRef := ir.R(ir.ID{Type: publicRT.Type, Name: publicRT.Name}, ir.Field("id"))
	privateRTRef := ir.R(ir.ID{Type: privateRT.Type, Name: privateRT.Name}, ir.Field("id"))

	for i, zone := range zones {
		ctx.Add(ir.Resource{
			Type:        "aws_route_table_association",
			Name:        "public_" + zone,
			SourceLabel: networkLabel,
			Args: ir.Attrs{
				ir.A("subnet_id", ir.R(public[i], ir.Field("id"))),
				ir.A("route_table_id", publicRTRef),
			},
		})
		ctx.Add(ir.Resource{
			Type:        "aws_route_table_association",
			Name:        "private_" + zone,
			SourceLabel: networkLabel,
			Args: ir.Attrs{
				ir.A("subnet_id", ir.R(private[i], ir.Field("id"))),
				ir.A("route_table_id", privateRTRef),
			},
		})
	}

	return &Network{VPC: vpcID, PublicSubnets: public, PrivateSubnets: private}
}

func nameTag(name string) ir.Value {
	return ir.M(ir.A("Name", ir.Str(name)))
}

func subnetRefs(ids []ir.ID) ir.List {
	out := make(ir.List, len(ids))
	for i, id := range ids {
		out[i] = ir.R(id, ir.Field("id"))
	}
	return out
}
