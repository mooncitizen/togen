package azure

import (
	"fmt"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

type provider struct{}

func New() resolve.Provider { return provider{} }

func (provider) Name() ir.CloudProvider { return ir.ProviderAzure }

// No subscription or region here: the provider reads ARM_SUBSCRIPTION_ID, and every resource
// takes its location from the resource group.
func (provider) ProviderBlock(*ir.Project) ir.Provider {
	return ir.Provider{
		Name:    "azurerm",
		Source:  "hashicorp/azurerm",
		Version: "~> 5.0",
		Config:  ir.Attrs{ir.A("features", ir.B(ir.Attrs{}))},
	}
}

func (provider) NameLimits() []resolve.NameLimit {
	return []resolve.NameLimit{
		{Type: "azurerm_resource_group", Arg: "name", Max: 90},
		{Type: "azurerm_virtual_network", Arg: "name", Max: 64},
		{Type: "azurerm_subnet", Arg: "name", Max: 80},
		{Type: "azurerm_storage_account", Arg: "name", Max: 24},
		{Type: "azurerm_storage_container", Arg: "name", Max: 63},
		{Type: "azurerm_container_app", Arg: "name", Max: 32},
		{Type: "azurerm_log_analytics_workspace", Arg: "name", Max: 63},
		{Type: "azurerm_service_plan", Arg: "name", Max: 60},
		{Type: "azurerm_linux_function_app", Arg: "name", Max: 60},
		{Type: "azurerm_postgresql_flexible_server", Arg: "name", Max: 63},
		{Type: "azurerm_mysql_flexible_server", Arg: "name", Max: 63},
		{Type: "azurerm_servicebus_namespace", Arg: "name", Max: 50},
		{Type: "azurerm_servicebus_queue", Arg: "name", Max: 260},
		{Type: "azurerm_redis_cache", Arg: "name", Max: 63},
	}
}

func (provider) ResolveNode(ctx *resolve.Context, n ir.Node) (*resolve.Handle, bool) {
	ctx.Report(ir.ValidationError{
		NodeID:  n.ID,
		Message: fmt.Sprintf("node type '%s' is not supported by the azure resolver yet", n.Type),
	})
	return nil, false
}

func (provider) ResolveEdge(ctx *resolve.Context, e ir.Edge, _, _ *resolve.Handle) {
	ctx.Report(ir.ValidationError{
		EdgeID:  e.ID,
		Message: fmt.Sprintf("'%s' edges are not supported by the azure resolver yet", e.Relation),
	})
}
