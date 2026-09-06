package azure

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

func props(t *testing.T, v any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal properties: %v", err)
	}
	return raw
}

func functionNode(t *testing.T, id, name string, p ir.FunctionProps) ir.Node {
	t.Helper()
	return ir.Node{ID: id, Type: ir.NodeFunction, Name: name, Properties: props(t, p)}
}

func setupFunction(t *testing.T, p ir.FunctionProps) (*resolve.Context, *resolve.Handle) {
	t.Helper()
	ctx := newContext(t, []ir.Node{functionNode(t, "n2", "handler", p)})
	return ctx, resolveFunction(ctx, ctx.Project.Nodes[0])
}

var defaultFunction = ir.FunctionProps{
	Runtime:        ir.RuntimeNode,
	Handler:        "index.handler",
	Size:           ir.SizeSmall,
	TimeoutSeconds: 30,
}

var (
	fnID      = ir.ID{Type: "azurerm_linux_function_app", Name: "handler"}
	planID    = ir.ID{Type: "azurerm_service_plan", Name: "handler"}
	storageID = ir.ID{Type: "azurerm_storage_account", Name: "handler"}
)

func located(name string, args ...ir.Attr) ir.Attrs {
	return append(ir.Attrs{
		ir.A("name", ir.Str(name)),
		ir.A("resource_group_name", ir.R(groupID, ir.Field("name"))),
		ir.A("location", ir.R(groupID, ir.Field("location"))),
	}, args...)
}

func TestFunctionEmitsAFunctionAppOnAConsumptionPlanWithAStorageAccount(t *testing.T) {
	p := defaultFunction
	p.Env = map[string]string{"LOG_LEVEL": "info", "APP_NAME": "shop"}
	ctx, handle := setupFunction(t, p)
	handle.Finalise()

	wantPlan := located("shop-dev-handler",
		ir.A("os_type", ir.Str("Linux")),
		ir.A("sku_name", ir.Str("Y1")),
	)
	if diff := cmp.Diff(wantPlan, firstOfType(t, ctx, "azurerm_service_plan").Args); diff != "" {
		t.Errorf("plan args (-want +got):\n%s", diff)
	}
	wantStorage := located("shopdevhandler",
		ir.A("account_tier", ir.Str("Standard")),
		ir.A("account_replication_type", ir.Str("LRS")),
	)
	if diff := cmp.Diff(wantStorage, firstOfType(t, ctx, "azurerm_storage_account").Args); diff != "" {
		t.Errorf("storage account args (-want +got):\n%s", diff)
	}

	fn := firstOfType(t, ctx, "azurerm_linux_function_app")
	wantFn := located("shop-dev-handler",
		ir.A("service_plan_id", ir.R(planID, ir.Field("id"))),
		ir.A("storage_account_name", ir.R(storageID, ir.Field("name"))),
		ir.A("storage_account_access_key", ir.R(storageID, ir.Field("primary_access_key"))),
		ir.A("https_only", ir.Bool(true)),
		ir.A("site_config", ir.B(ir.Attrs{
			ir.A("application_stack", ir.B(ir.Attrs{ir.A("node_version", ir.Str("22"))})),
		})),
		ir.A("identity", ir.B(ir.Attrs{ir.A("type", ir.Str("SystemAssigned"))})),
		ir.A("app_settings", ir.M(
			ir.A("APP_NAME", ir.Str("shop")),
			ir.A("LOG_LEVEL", ir.Str("info")),
			ir.A("AzureFunctionsJobHost__functionTimeout", ir.Str("00:00:30")),
		)),
	)
	if diff := cmp.Diff(wantFn, fn.Args); diff != "" {
		t.Errorf("function app args (-want +got):\n%s", diff)
	}
	for _, r := range ctx.Resources() {
		if r.Type != "azurerm_resource_group" && (r.SourceNode != "n2" || r.SourceLabel != "handler") {
			t.Errorf("%s source = %q/%q", r.Type, r.SourceNode, r.SourceLabel)
		}
	}
	if got := countOfType(ctx, "azurerm_resource_group"); got != 1 {
		t.Errorf("resource groups = %d", got)
	}
	if got := countOfType(ctx, "azurerm_virtual_network"); got != 0 {
		t.Error("a function app on the consumption plan joined a network")
	}
	if len(ctx.Variables) != 0 || len(ctx.Outputs) != 0 {
		t.Errorf("variables = %+v, outputs = %+v", ctx.Variables, ctx.Outputs)
	}

	wantExports := resolve.FunctionExports{
		FunctionName: ir.R(fnID, ir.Field("name")),
		URL:          ir.C(ir.Str("https://"), ir.R(fnID, ir.Field("default_hostname"))),
		PrincipalID:  ir.R(fnID, ir.Field("identity"), ir.Index(0), ir.Field("principal_id")),
	}
	if diff := cmp.Diff(wantExports, handle.Exports); diff != "" {
		t.Errorf("exports (-want +got):\n%s", diff)
	}
	if handle.Primary != fnID {
		t.Errorf("primary = %v", handle.Primary)
	}
}

func TestFunctionMapsEachRuntimeOntoTheApplicationStack(t *testing.T) {
	for _, tc := range []struct {
		runtime ir.Runtime
		want    ir.Attr
	}{
		{ir.RuntimeNode, ir.A("node_version", ir.Str("22"))},
		{ir.RuntimePython, ir.A("python_version", ir.Str("3.12"))},
		{ir.RuntimeGo, ir.A("use_custom_runtime", ir.Bool(true))},
	} {
		p := defaultFunction
		p.Runtime = tc.runtime
		ctx, handle := setupFunction(t, p)
		handle.Finalise()
		got, _ := firstOfType(t, ctx, "azurerm_linux_function_app").Args.Get("site_config")
		want := ir.B(ir.Attrs{ir.A("application_stack", ir.B(ir.Attrs{tc.want}))})
		if diff := cmp.Diff(ir.Value(want), got); diff != "" {
			t.Errorf("%s site_config (-want +got):\n%s", tc.runtime, diff)
		}
	}
}

// Y1 is pay per execution with one memory tier, so size changes nothing.
func TestFunctionSizeMapsToNothingOnTheConsumptionPlan(t *testing.T) {
	var apps []ir.Attrs
	for _, size := range ir.Sizes {
		p := defaultFunction
		p.Size = size
		ctx, handle := setupFunction(t, p)
		handle.Finalise()
		sku, _ := firstOfType(t, ctx, "azurerm_service_plan").Args.Get("sku_name")
		if diff := cmp.Diff(ir.Value(ir.Str("Y1")), sku); diff != "" {
			t.Errorf("%s sku (-want +got):\n%s", size, diff)
		}
		apps = append(apps, firstOfType(t, ctx, "azurerm_linux_function_app").Args)
	}
	for i := 1; i < len(apps); i++ {
		if diff := cmp.Diff(apps[0], apps[i]); diff != "" {
			t.Errorf("%s function app differs from %s (-small +got):\n%s", ir.Sizes[i], ir.Sizes[0], diff)
		}
	}
}

func TestFunctionWritesTheTimeoutAsAHostSetting(t *testing.T) {
	for _, tc := range []struct {
		seconds int
		want    string
	}{
		{1, "00:00:01"},
		{125, "00:02:05"},
		{600, "00:10:00"},
	} {
		p := defaultFunction
		p.TimeoutSeconds = tc.seconds
		ctx, handle := setupFunction(t, p)
		handle.Finalise()
		if len(ctx.Errors) != 0 {
			t.Errorf("%d seconds: errors = %v", tc.seconds, ctx.Errors)
		}
		settings, _ := firstOfType(t, ctx, "azurerm_linux_function_app").Args.Get("app_settings")
		want := ir.M(ir.A("AzureFunctionsJobHost__functionTimeout", ir.Str(tc.want)))
		if diff := cmp.Diff(ir.Value(want), settings); diff != "" {
			t.Errorf("%d seconds app_settings (-want +got):\n%s", tc.seconds, diff)
		}
	}
}

func TestFunctionRefusesATimeoutTheConsumptionPlanCannotHonour(t *testing.T) {
	p := defaultFunction
	p.TimeoutSeconds = 601
	project := newProject(t, []ir.Node{functionNode(t, "n2", "handler", p)})
	want := ir.Errors{{
		NodeID:  "n2",
		Message: "function 'handler' has a timeout of 601 seconds, the consumption plan allows at most 600",
	}}
	if diff := cmp.Diff(want, runErrors(t, project)); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
}

func TestFunctionKeepsTheStorageAccountNameWithinTheLimit(t *testing.T) {
	project := newProject(t, []ir.Node{functionNode(t, "n2", "orders", defaultFunction)})
	project.Name = "a-project-name-that-is-long-too"
	g, err := resolve.Run(project, New())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, r := range g.Resources {
		if r.Type != "azurerm_storage_account" {
			continue
		}
		name, _ := r.Args.Get("name")
		if diff := cmp.Diff(ir.Value(ir.Str("aprojectnamethadevorders")), name); diff != "" {
			t.Errorf("storage account name (-want +got):\n%s", diff)
		}
		if s, ok := name.(ir.String); !ok || !storageAccountNamePattern.MatchString(string(s)) {
			t.Errorf("storage account name %v is not 3 to 24 lowercase alphanumerics", name)
		}
	}
	for _, r := range g.Resources {
		if name, ok := r.Args.Get("name"); ok && r.Type != "azurerm_storage_account" {
			if s, ok := name.(ir.String); ok && !strings.HasPrefix(string(s), "a-project-name-that-is-long-too-dev") {
				t.Errorf("%s name = %q", r.Type, s)
			}
		}
	}
}
