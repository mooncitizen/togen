package gcp

import (
	"fmt"
	"slices"
	"strings"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

var engineNames = map[ir.Engine]string{
	ir.EnginePostgres: "POSTGRES",
	ir.EngineMySQL:    "MYSQL",
}

const (
	databaseUser   = "app"
	passwordLength = 32
)

var randomProvider = ir.Provider{Name: "random", Source: "hashicorp/random", Version: "~> 3.6"}

func resolveDatabase(ctx *resolve.Context, node ir.Node) *resolve.Handle {
	p := resolve.Props[ir.DatabaseProps](ctx, node)
	engine, ok := ir.Engines[ir.ProviderGCP][p.Engine]
	if !ok {
		ctx.Fail(fmt.Sprintf("database '%s' has an unknown engine '%s'", node.Name, p.Engine))
	}
	tier, ok := databaseTiers[p.Size]
	if !ok {
		ctx.Fail(fmt.Sprintf("database '%s' has an unknown size '%s'", node.Name, p.Size))
	}
	// ValidateProject checks this already. Kept so a project reaching the resolver another way
	// still gets a version Cloud SQL accepts.
	version := engine.Versions[0]
	switch {
	case p.Version == "":
	case slices.Contains(engine.Versions, p.Version):
		version = p.Version
	default:
		ctx.Report(ir.ValidationError{
			NodeID: node.ID,
			Message: fmt.Sprintf("database '%s': %s version '%s' is not supported on gcp (use one of %s)",
				node.Name, p.Engine, p.Version, strings.Join(engine.Versions, ", ")),
		})
	}

	local := ctx.Local(node.Name)
	network := ensureNetwork(ctx)
	ctx.RequireProvider(randomProvider)

	availability := "ZONAL"
	if p.HighAvailability {
		availability = "REGIONAL"
	}
	instance := ctx.Add(ir.Resource{
		Type:        "google_sql_database_instance",
		Name:        local,
		SourceNode:  node.ID,
		SourceLabel: node.Name,
		Args: ir.Attrs{
			ir.A("name", ir.Str(ctx.Named(node.Name))),
			ir.A("region", ir.Str(ctx.Project.Region)),
			ir.A("database_version", ir.Str(engineNames[p.Engine]+"_"+strings.ReplaceAll(version, ".", "_"))),
			ir.A("deletion_protection", ir.Bool(false)),
			ir.A("settings", ir.B(ir.Attrs{
				ir.A("tier", ir.Str(tier)),
				// Postgres 16 and later default to Enterprise Plus, which has no shared-core or custom tiers.
				ir.A("edition", ir.Str("ENTERPRISE")),
				ir.A("availability_type", ir.Str(availability)),
				ir.A("disk_size", ir.Num(float64(p.StorageGB))),
				ir.A("disk_autoresize", ir.Bool(false)),
				ir.A("ip_configuration", ir.B(ir.Attrs{
					ir.A("ipv4_enabled", ir.Bool(false)),
					ir.A("private_network", ir.R(network.VPC, ir.Field("id"))),
				})),
			})),
		},
		DependsOn: []ir.ID{network.Connection},
	})
	instanceID := ir.ID{Type: instance.Type, Name: instance.Name}

	ctx.Add(ir.Resource{
		Type:        "google_sql_database",
		Name:        local,
		SourceNode:  node.ID,
		SourceLabel: node.Name,
		Args: ir.Attrs{
			ir.A("name", ir.Str(local)),
			ir.A("instance", ir.R(instanceID, ir.Field("name"))),
		},
	})
	password := ctx.Add(ir.Resource{
		Type:        "random_password",
		Name:        local,
		SourceNode:  node.ID,
		SourceLabel: node.Name,
		Args: ir.Attrs{
			ir.A("length", ir.Num(passwordLength)),
			ir.A("special", ir.Bool(false)),
		},
	})
	passwordRef := ir.R(ir.ID{Type: password.Type, Name: password.Name}, ir.Field("result"))
	ctx.Add(ir.Resource{
		Type:        "google_sql_user",
		Name:        local,
		SourceNode:  node.ID,
		SourceLabel: node.Name,
		Args: ir.Attrs{
			ir.A("name", ir.Str(databaseUser)),
			ir.A("instance", ir.R(instanceID, ir.Field("name"))),
			ir.A("password", passwordRef),
		},
	})

	ctx.AddOutput(ir.Output{
		Name:        local + "_connection_name",
		Description: fmt.Sprintf("Connection name of the %s database, for the Cloud SQL Auth Proxy", node.Name),
		Value:       ir.R(instanceID, ir.Field("connection_name")),
	})

	return &resolve.Handle{
		Node:    node,
		Primary: instanceID,
		Exports: resolve.DatabaseExports{
			Host:     ir.R(instanceID, ir.Field("private_ip_address")),
			Port:     ir.Num(float64(engine.Port)),
			Name:     ir.Str(local),
			User:     ir.Str(databaseUser),
			Password: passwordRef,
		},
	}
}
