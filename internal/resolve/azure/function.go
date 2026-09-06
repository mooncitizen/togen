package azure

import (
	"fmt"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

// Go has no worker on Azure Functions. It runs as a custom handler, an executable the code's
// host.json names, which is why the handler property maps to nothing here.
var runtimes = map[ir.Runtime]ir.Attr{
	ir.RuntimeNode:   ir.A("node_version", ir.Str("22")),
	ir.RuntimePython: ir.A("python_version", ir.Str("3.12")),
	ir.RuntimeGo:     ir.A("use_custom_runtime", ir.Bool(true)),
}

const (
	timeoutSetting = "AzureFunctionsJobHost__functionTimeout"
	// The consumption plan runs a function for ten minutes at most.
	consumptionTimeoutMax = 600
)

func resolveFunction(ctx *resolve.Context, node ir.Node) *resolve.Handle {
	p := resolve.Props[ir.FunctionProps](ctx, node)
	stack, ok := runtimes[p.Runtime]
	if !ok {
		ctx.Fail(fmt.Sprintf("function '%s' has an unknown runtime '%s'", node.Name, p.Runtime))
	}
	if p.TimeoutSeconds > consumptionTimeoutMax {
		ctx.Report(ir.ValidationError{
			NodeID: node.ID,
			Message: fmt.Sprintf("function '%s' has a timeout of %d seconds, the consumption plan allows at most %d",
				node.Name, p.TimeoutSeconds, consumptionTimeoutMax),
		})
	}
	group := ensureGroup(ctx)
	local := ctx.Local(node.Name)

	plan := ctx.Add(ir.Resource{
		Type:        "azurerm_service_plan",
		Name:        local,
		SourceNode:  node.ID,
		SourceLabel: node.Name,
		Args: group.Located(ctx.Named(node.Name),
			ir.A("os_type", ir.Str("Linux")),
			ir.A("sku_name", ir.Str("Y1")),
		),
	})
	planID := ir.ID{Type: plan.Type, Name: plan.Name}

	storage := ctx.Add(ir.Resource{
		Type:        "azurerm_storage_account",
		Name:        local,
		SourceNode:  node.ID,
		SourceLabel: node.Name,
		Args: group.Located(storageAccountName(ctx, node.Name),
			ir.A("account_tier", ir.Str("Standard")),
			ir.A("account_replication_type", ir.Str("LRS")),
		),
	})
	storageID := ir.ID{Type: storage.Type, Name: storage.Name}

	fn := ctx.Add(ir.Resource{
		Type:        "azurerm_linux_function_app",
		Name:        local,
		SourceNode:  node.ID,
		SourceLabel: node.Name,
		Args: group.Located(ctx.Named(node.Name),
			ir.A("service_plan_id", ir.R(planID, ir.Field("id"))),
			ir.A("storage_account_name", ir.R(storageID, ir.Field("name"))),
			ir.A("storage_account_access_key", ir.R(storageID, ir.Field("primary_access_key"))),
			ir.A("https_only", ir.Bool(true)),
			ir.A("site_config", ir.B(ir.Attrs{
				ir.A("application_stack", ir.B(ir.Attrs{stack})),
			})),
			ir.A("identity", ir.B(ir.Attrs{ir.A("type", ir.Str("SystemAssigned"))})),
		),
	})
	fnID := ir.ID{Type: fn.Type, Name: fn.Name}

	h := &resolve.Handle{
		Node:    node,
		Primary: fnID,
		Env:     resolve.SortedEnv(p.Env),
		Exports: resolve.FunctionExports{
			FunctionName: ir.R(fnID, ir.Field("name")),
			URL:          ir.C(ir.Str("https://"), ir.R(fnID, ir.Field("default_hostname"))),
			PrincipalID:  ir.R(fnID, ir.Field("identity"), ir.Index(0), ir.Field("principal_id")),
		},
	}
	h.SetEnv(timeoutSetting, ir.Str(timeSpan(p.TimeoutSeconds)))
	h.Finalise = func() {
		fn.Args.Set("app_settings", ir.Map(h.Env))
	}
	return h
}

func timeSpan(seconds int) string {
	return fmt.Sprintf("%02d:%02d:%02d", seconds/3600, seconds%3600/60, seconds%60)
}
