package azure

import (
	"fmt"
	"slices"

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

// Every type the azure catalogue says it generates has a function here, and a test holds
// the two together.
var resolvers = map[ir.NodeType]func(*resolve.Context, ir.Node) *resolve.Handle{
	ir.NodeGateway:  func(_ *resolve.Context, n ir.Node) *resolve.Handle { return resolveGateway(n) },
	ir.NodeFunction: resolveFunction,
	ir.NodeDatabase: resolveDatabase,
	ir.NodeService:  resolveService,
	ir.NodeQueue:    resolveQueue,
	ir.NodeBucket:   resolveBucket,
	ir.NodeCache:    resolveCache,
}

func Resolvers() []ir.NodeType {
	out := make([]ir.NodeType, 0, len(resolvers))
	for t := range resolvers {
		out = append(out, t)
	}
	slices.Sort(out)
	return out
}

func (provider) ResolveNode(ctx *resolve.Context, n ir.Node) (*resolve.Handle, bool) {
	resolveNode, ok := resolvers[n.Type]
	if !ok {
		ctx.Report(ir.ValidationError{
			NodeID:  n.ID,
			Message: fmt.Sprintf("the azure resolver has no function for '%s'", n.Type),
		})
		return nil, false
	}
	return resolveNode(ctx, n), true
}

func (provider) ResolveEdge(ctx *resolve.Context, e ir.Edge, from, to *resolve.Handle) {
	switch e.Relation {
	case ir.RelRoutes:
		resolveRoutes(ctx, e, from, to)
	case ir.RelCalls:
		resolveCalls(ctx, e, from, to)
	case ir.RelReads, ir.RelWrites:
		resolveDataAccess(ctx, e, from, to)
	case ir.RelPublishes, ir.RelConsumes:
		resolveMessaging(ctx, e, from, to)
	default:
		ctx.Report(ir.ValidationError{
			EdgeID:  e.ID,
			Message: fmt.Sprintf("'%s' edges are not supported by the azure resolver yet", e.Relation),
		})
	}
}
