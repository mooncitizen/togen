package aws

import (
	"fmt"
	"slices"
	"strings"

	"togen/internal/ir"
	"togen/internal/resolve"
)

func resolveDatabase(ctx *resolve.Context, node ir.Node) *resolve.Handle {
	p := nodeProps[ir.DatabaseProps](ctx, node)
	engine, ok := ir.Engines[ir.ProviderAWS][p.Engine]
	if !ok {
		ctx.Fail(fmt.Sprintf("database '%s' has an unknown engine '%s'", node.Name, p.Engine))
	}
	// ValidateProject checks this already. Kept so a project reaching the resolver another way
	// still gets a supported version rather than one RDS will reject.
	version := engine.Versions[0]
	switch {
	case p.Version == "":
	case slices.Contains(engine.Versions, p.Version):
		version = p.Version
	default:
		ctx.Report(ir.ValidationError{
			NodeID: node.ID,
			Message: fmt.Sprintf("database '%s': %s version '%s' is not supported on aws (use one of %s)",
				node.Name, p.Engine, p.Version, strings.Join(engine.Versions, ", ")),
		})
	}

	local := ctx.Local(node.Name)
	network := ensureNetwork(ctx)

	subnetGroup := ctx.Add(ir.Resource{
		Type:        "aws_db_subnet_group",
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

	db := ctx.Add(ir.Resource{
		Type:        "aws_db_instance",
		Name:        local,
		SourceNode:  node.ID,
		SourceLabel: node.Name,
		Args: ir.Attrs{
			ir.A("identifier", ir.Str(ctx.Named(node.Name))),
			ir.A("engine", ir.Str(string(p.Engine))),
			ir.A("engine_version", ir.Str(version)),
			ir.A("instance_class", ir.Str(dbSizes[p.Size])),
			ir.A("allocated_storage", ir.Num(float64(p.StorageGB))),
			ir.A("db_name", ir.Str(local)),
			ir.A("username", ir.Str("app")),
			ir.A("manage_master_user_password", ir.Bool(true)),
			ir.A("db_subnet_group_name", ir.R(ir.ID{Type: subnetGroup.Type, Name: subnetGroup.Name}, ir.Field("name"))),
			ir.A("vpc_security_group_ids", ir.L(ir.R(sgID, ir.Field("id")))),
			ir.A("multi_az", ir.Bool(p.HighAvailability)),
			ir.A("storage_encrypted", ir.Bool(true)),
			ir.A("publicly_accessible", ir.Bool(false)),
			ir.A("backup_retention_period", ir.Num(7)),
			ir.A("skip_final_snapshot", ir.Bool(true)),
			ir.A("deletion_protection", ir.Bool(false)),
		},
	})
	dbID := ir.ID{Type: db.Type, Name: db.Name}

	ctx.AddOutput(ir.Output{
		Name:        local + "_endpoint",
		Description: fmt.Sprintf("Endpoint of the %s database", node.Name),
		Value:       ir.R(dbID, ir.Field("endpoint")),
	})

	return &resolve.Handle{
		Node:          node,
		Primary:       dbID,
		SecurityGroup: &sgID,
		Exports: resolve.DatabaseExports{
			Host:      ir.R(dbID, ir.Field("address")),
			Port:      ir.Num(float64(engine.Port)),
			Name:      ir.Str(local),
			SecretARN: ir.R(dbID, ir.Field("master_user_secret"), ir.Index(0), ir.Field("secret_arn")),
		},
	}
}
