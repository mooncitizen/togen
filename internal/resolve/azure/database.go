package azure

import (
	"fmt"
	"slices"
	"strings"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

var randomProvider = ir.Provider{Name: "random", Source: "hashicorp/random", Version: "~> 3.6"}

const administratorLogin = "app"

// Postgres storage comes in these tiers, so storageGb rounds up to the first one that holds it.
var postgresStorageTiers = []float64{
	32768, 65536, 131072, 262144, 524288, 1048576,
	2097152, 4193280, 4194304, 8388608, 16777216, 33553408,
}

type flexibleServer struct {
	server, database, domain string
}

var flexibleServers = map[ir.Engine]flexibleServer{
	ir.EnginePostgres: {
		server:   "azurerm_postgresql_flexible_server",
		database: "azurerm_postgresql_flexible_server_database",
		domain:   "postgres.database.azure.com",
	},
	ir.EngineMySQL: {
		server:   "azurerm_mysql_flexible_server",
		database: "azurerm_mysql_flexible_database",
		domain:   "mysql.database.azure.com",
	},
}

func resolveDatabase(ctx *resolve.Context, node ir.Node) *resolve.Handle {
	p := resolve.Props[ir.DatabaseProps](ctx, node)
	engine, ok := ir.Engines[ir.ProviderAzure][p.Engine]
	if !ok {
		ctx.Fail(fmt.Sprintf("database '%s' has an unknown engine '%s'", node.Name, p.Engine))
	}
	shape := flexibleServers[p.Engine]
	// ValidateProject checks this already. Kept so a project reaching the resolver another way
	// still gets a supported version rather than one the flexible server will reject.
	version := engine.Versions[0]
	switch {
	case p.Version == "":
	case slices.Contains(engine.Versions, p.Version):
		version = p.Version
	default:
		ctx.Report(ir.ValidationError{
			NodeID: node.ID,
			Message: fmt.Sprintf("database '%s': %s version '%s' is not supported on azure (use one of %s)",
				node.Name, p.Engine, p.Version, strings.Join(engine.Versions, ", ")),
		})
	}
	if p.HighAvailability && p.Size == ir.SizeSmall {
		ctx.Report(ir.ValidationError{
			NodeID: node.ID,
			Message: fmt.Sprintf("database '%s': highAvailability needs a medium or large size on azure, the burstable tier has no standby",
				node.Name),
		})
	}

	local := ctx.Local(node.Name)
	group := ensureGroup(ctx)
	network := ensureNetwork(ctx)
	ctx.RequireProvider(randomProvider)

	// Both servers want three character classes, and a 32 character draw from three classes
	// misses one about once in a hundred, so each class is guaranteed.
	password := ctx.Add(ir.Resource{
		Type:        "random_password",
		Name:        local,
		SourceNode:  node.ID,
		SourceLabel: node.Name,
		Args: ir.Attrs{
			ir.A("length", ir.Num(32)),
			ir.A("special", ir.Bool(false)),
			ir.A("min_lower", ir.Num(1)),
			ir.A("min_upper", ir.Num(1)),
			ir.A("min_numeric", ir.Num(1)),
		},
	})
	passwordID := ir.ID{Type: password.Type, Name: password.Name}
	zoneID, linkID := addPrivateZone(ctx, node, group, network, shape.domain)

	serverID := ir.ID{Type: shape.server, Name: local}
	var (
		subnet          ir.ID
		public, storage ir.Attr
		database        ir.Attrs
	)
	switch p.Engine {
	case ir.EnginePostgres:
		subnet = network.PostgresSubnet
		public = ir.A("public_network_access_enabled", ir.Bool(false))
		storage = ir.A("storage_mb", ir.Num(postgresStorageMB(ctx, node, p.StorageGB)))
		database = ir.Attrs{
			ir.A("name", ir.Str(local)),
			ir.A("server_id", ir.R(serverID, ir.Field("id"))),
		}
	case ir.EngineMySQL:
		subnet = network.mysqlSubnet(ctx)
		public = ir.A("public_network_access", ir.Str("Disabled"))
		storage = ir.A("storage", ir.B(ir.Attrs{ir.A("size_gb", ir.Num(float64(p.StorageGB)))}))
		database = group.Grouped(local,
			ir.A("server_name", ir.R(serverID, ir.Field("name"))),
			ir.A("charset", ir.Str("utf8mb4")),
			ir.A("collation", ir.Str("utf8mb4_unicode_ci")),
		)
	}

	args := group.Located(ctx.Named(node.Name),
		ir.A("version", ir.Str(version)),
		ir.A("sku_name", ir.Str(dbSizes[p.Size])),
		ir.A("administrator_login", ir.Str(administratorLogin)),
		ir.A("administrator_password", ir.R(passwordID, ir.Field("result"))),
		ir.A("delegated_subnet_id", ir.R(subnet, ir.Field("id"))),
		ir.A("private_dns_zone_id", ir.R(zoneID, ir.Field("id"))),
		public,
		storage,
	)
	if p.HighAvailability {
		args = append(args, ir.A("high_availability", ir.B(ir.Attrs{ir.A("mode", ir.Str("ZoneRedundant"))})))
	}
	// Azure refuses a server whose zone is not linked to the network yet, and nothing in the
	// server's arguments refers to the link.
	ctx.Add(ir.Resource{
		Type:        serverID.Type,
		Name:        serverID.Name,
		SourceNode:  node.ID,
		SourceLabel: node.Name,
		Args:        args,
		DependsOn:   []ir.ID{linkID},
	})
	ctx.Add(ir.Resource{
		Type:        shape.database,
		Name:        local,
		SourceNode:  node.ID,
		SourceLabel: node.Name,
		Args:        database,
	})

	ctx.AddOutput(ir.Output{
		Name:        local + "_fqdn",
		Description: fmt.Sprintf("FQDN of the %s database", node.Name),
		Value:       ir.R(serverID, ir.Field("fqdn")),
	})

	return &resolve.Handle{
		Node:    node,
		Primary: serverID,
		Exports: resolve.DatabaseExports{
			Host:     ir.R(serverID, ir.Field("fqdn")),
			Port:     ir.Num(float64(engine.Port)),
			Name:     ir.Str(local),
			User:     ir.Str(administratorLogin),
			Password: ir.R(passwordID, ir.Field("result")),
		},
	}
}

func addPrivateZone(ctx *resolve.Context, node ir.Node, group *Group, network *Network, domain string) (zone, link ir.ID) {
	local := ctx.Local(node.Name)
	z := ctx.Add(ir.Resource{
		Type:        "azurerm_private_dns_zone",
		Name:        local,
		SourceNode:  node.ID,
		SourceLabel: node.Name,
		Args:        group.Grouped(ctx.Named(node.Name) + "." + domain),
	})
	zone = ir.ID{Type: z.Type, Name: z.Name}
	l := ctx.Add(ir.Resource{
		Type:        "azurerm_private_dns_zone_virtual_network_link",
		Name:        local,
		SourceNode:  node.ID,
		SourceLabel: node.Name,
		Args: ir.Attrs{
			ir.A("name", ir.Str(ctx.Named(node.Name))),
			ir.A("private_dns_zone_id", ir.R(zone, ir.Field("id"))),
			ir.A("virtual_network_id", ir.R(network.VirtualNetwork, ir.Field("id"))),
		},
	})
	return zone, ir.ID{Type: l.Type, Name: l.Name}
}

func postgresStorageMB(ctx *resolve.Context, node ir.Node, gb int) float64 {
	need := float64(gb) * 1024
	i := slices.IndexFunc(postgresStorageTiers, func(tier float64) bool { return tier >= need })
	if i < 0 {
		last := postgresStorageTiers[len(postgresStorageTiers)-1]
		ctx.Report(ir.ValidationError{
			NodeID: node.ID,
			Message: fmt.Sprintf("database '%s': storageGb %d is more than the %d GB a postgres flexible server holds",
				node.Name, gb, int(last/1024)),
		})
		return last
	}
	return postgresStorageTiers[i]
}
