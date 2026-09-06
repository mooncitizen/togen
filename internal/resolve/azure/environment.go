package azure

import (
	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

const (
	environmentLabel   = "container apps environment"
	consumptionProfile = "Consumption"
)

// One environment per project, made by the first service. It sits on the delegated apps subnet
// so the apps in it can reach the flexible servers, and its Consumption profile is the one the
// size table is written for.
func ensureEnvironment(ctx *resolve.Context) ir.ID {
	if id, ok := ctx.Scratch[environmentLabel].(ir.ID); ok {
		return id
	}
	group := ensureGroup(ctx)
	network := ensureNetwork(ctx)

	logs := ctx.Add(ir.Resource{
		Type:        "azurerm_log_analytics_workspace",
		Name:        "main",
		SourceLabel: environmentLabel,
		Args: group.Located(ctx.Named("logs"),
			ir.A("sku", ir.Str("PerGB2018")),
			ir.A("retention_in_days", ir.Num(30)),
		),
	})
	logsID := ir.ID{Type: logs.Type, Name: logs.Name}

	env := ctx.Add(ir.Resource{
		Type:        "azurerm_container_app_environment",
		Name:        "main",
		SourceLabel: environmentLabel,
		Args: group.Located(ctx.Named("apps"),
			ir.A("log_analytics_workspace_id", ir.R(logsID, ir.Field("id"))),
			ir.A("infrastructure_subnet_id", ir.R(network.AppsSubnet, ir.Field("id"))),
			ir.A("workload_profile", ir.B(ir.Attrs{
				ir.A("name", ir.Str(consumptionProfile)),
				ir.A("workload_profile_type", ir.Str(consumptionProfile)),
			})),
		),
	})
	id := ir.ID{Type: env.Type, Name: env.Name}
	ctx.Scratch[environmentLabel] = id
	return id
}
