package azure

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/mooncitizen/togen/internal/emit/hcl"
	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

var defaultService = ir.ServiceProps{
	Image:       "nginx:1.27",
	Port:        8080,
	Size:        ir.SizeSmall,
	MinReplicas: 1,
	MaxReplicas: 2,
}

var (
	appID         = ir.ID{Type: "azurerm_container_app", Name: "web"}
	environmentID = ir.ID{Type: "azurerm_container_app_environment", Name: "main"}
	logsID        = ir.ID{Type: "azurerm_log_analytics_workspace", Name: "main"}
	appsSubnet    = ir.ID{Type: "azurerm_subnet", Name: "apps"}

	webURL = ir.C(ir.Str("https://"), ir.R(appID, ir.Field("ingress"), ir.Index(0), ir.Field("fqdn")))
)

func serviceNodeWith(t *testing.T, id, name string, p ir.ServiceProps) ir.Node {
	t.Helper()
	return ir.Node{ID: id, Type: ir.NodeService, Name: name, Properties: props(t, p)}
}

func setupService(t *testing.T, p ir.ServiceProps) (*resolve.Context, *resolve.Handle) {
	t.Helper()
	ctx := newContext(t, []ir.Node{serviceNodeWith(t, "n4", "web", p)})
	h := resolveService(ctx, ctx.Project.Nodes[0])
	h.Finalise()
	return ctx, h
}

func containerApp(t *testing.T, ctx *resolve.Context, id ir.ID) ir.Attrs {
	t.Helper()
	r, ok := ctx.Resource(id)
	if !ok {
		t.Fatalf("no %s", id)
	}
	return r.Args
}

func appIngress(t *testing.T, ctx *resolve.Context, id ir.ID) ir.Attrs {
	t.Helper()
	ingress, ok := containerApp(t, ctx, id).Get("ingress")
	if !ok {
		t.Fatal("no ingress block")
	}
	return ingress.(ir.Block)[0]
}

func appContainer(t *testing.T, ctx *resolve.Context, id ir.ID) ir.Attrs {
	t.Helper()
	template, ok := containerApp(t, ctx, id).Get("template")
	if !ok {
		t.Fatal("no template block")
	}
	container, _ := template.(ir.Block)[0].Get("container")
	return container.(ir.Block)[0]
}

func containerEnvOf(t *testing.T, ctx *resolve.Context, id ir.ID) ir.Attrs {
	t.Helper()
	env, ok := appContainer(t, ctx, id).Get("env")
	if !ok {
		return nil
	}
	var out ir.Attrs
	for _, entry := range env.(ir.Block) {
		name, _ := entry.Get("name")
		value, _ := entry.Get("value")
		out = append(out, ir.A(string(name.(ir.String)), value))
	}
	return out
}

func ingressBlock(external bool, port float64) ir.Block {
	return ir.B(ir.Attrs{
		ir.A("external_enabled", ir.Bool(external)),
		ir.A("target_port", ir.Num(port)),
		ir.A("traffic_weight", ir.B(ir.Attrs{
			ir.A("percentage", ir.Num(100)),
			ir.A("latest_revision", ir.Bool(true)),
		})),
	})
}

func TestServiceEmitsAContainerAppInAnEnvironmentOnTheAppsSubnet(t *testing.T) {
	p := defaultService
	p.Env = map[string]string{"LOG_LEVEL": "info", "APP_NAME": "shop"}
	ctx, handle := setupService(t, p)
	if len(ctx.Errors) != 0 {
		t.Fatalf("errors = %v", ctx.Errors)
	}

	wantLogs := located("shop-dev-logs",
		ir.A("sku", ir.Str("PerGB2018")),
		ir.A("retention_in_days", ir.Num(30)),
	)
	if diff := cmp.Diff(wantLogs, firstOfType(t, ctx, "azurerm_log_analytics_workspace").Args); diff != "" {
		t.Errorf("workspace args (-want +got):\n%s", diff)
	}
	wantEnvironment := located("shop-dev-apps",
		ir.A("log_analytics_workspace_id", ir.R(logsID, ir.Field("id"))),
		ir.A("infrastructure_subnet_id", ir.R(appsSubnet, ir.Field("id"))),
		ir.A("workload_profile", ir.B(ir.Attrs{
			ir.A("name", ir.Str("Consumption")),
			ir.A("workload_profile_type", ir.Str("Consumption")),
		})),
	)
	if diff := cmp.Diff(wantEnvironment, firstOfType(t, ctx, "azurerm_container_app_environment").Args); diff != "" {
		t.Errorf("environment args (-want +got):\n%s", diff)
	}
	for _, id := range []ir.ID{logsID, environmentID} {
		r, _ := ctx.Resource(id)
		if r.SourceNode != "" || r.SourceLabel != "container apps environment" {
			t.Errorf("%s source = %q/%q", id, r.SourceNode, r.SourceLabel)
		}
	}

	wantApp := grouped("shop-dev-web",
		ir.A("container_app_environment_id", ir.R(environmentID, ir.Field("id"))),
		ir.A("revision_mode", ir.Str("Single")),
		ir.A("workload_profile_name", ir.Str("Consumption")),
		ir.A("identity", ir.B(ir.Attrs{ir.A("type", ir.Str("SystemAssigned"))})),
		ir.A("template", ir.B(ir.Attrs{
			ir.A("min_replicas", ir.Num(1)),
			ir.A("max_replicas", ir.Num(2)),
			ir.A("container", ir.B(ir.Attrs{
				ir.A("name", ir.Str("web")),
				ir.A("image", ir.Str("nginx:1.27")),
				ir.A("cpu", ir.Num(0.25)),
				ir.A("memory", ir.Str("0.5Gi")),
				ir.A("env", ir.B(
					ir.Attrs{ir.A("name", ir.Str("APP_NAME")), ir.A("value", ir.Str("shop"))},
					ir.Attrs{ir.A("name", ir.Str("LOG_LEVEL")), ir.A("value", ir.Str("info"))},
				)),
			})),
		})),
		ir.A("ingress", ingressBlock(false, 8080)),
	)
	app := firstOfType(t, ctx, "azurerm_container_app")
	if diff := cmp.Diff(wantApp, app.Args); diff != "" {
		t.Errorf("container app args (-want +got):\n%s", diff)
	}
	if app.SourceNode != "n4" || app.SourceLabel != "web" {
		t.Errorf("app source = %q/%q", app.SourceNode, app.SourceLabel)
	}
	if got := countOfType(ctx, "azurerm_virtual_network"); got != 1 {
		t.Errorf("virtual networks = %d", got)
	}
	// The group, the network's three, the environment's two and the app.
	if got := len(ctx.Resources()); got != 7 {
		t.Errorf("resources = %d", got)
	}
	if len(ctx.Variables) != 0 || len(ctx.Outputs) != 0 {
		t.Errorf("a private service has variables %+v or outputs %+v", ctx.Variables, ctx.Outputs)
	}

	wantExports := resolve.ServiceExports{Port: ir.Num(8080), URL: webURL}
	if diff := cmp.Diff(wantExports, handle.Exports); diff != "" {
		t.Errorf("exports (-want +got):\n%s", diff)
	}
	if handle.Primary != appID {
		t.Errorf("primary = %v", handle.Primary)
	}
}

func TestServiceMapsSizesOntoTheConsumptionPairs(t *testing.T) {
	for _, tc := range []struct {
		size   ir.Size
		cpu    float64
		memory string
	}{
		{ir.SizeSmall, 0.25, "0.5Gi"},
		{ir.SizeMedium, 0.5, "1Gi"},
		{ir.SizeLarge, 1, "2Gi"},
	} {
		p := defaultService
		p.Size = tc.size
		ctx, _ := setupService(t, p)
		container := appContainer(t, ctx, appID)
		cpu, _ := container.Get("cpu")
		memory, _ := container.Get("memory")
		if diff := cmp.Diff(ir.Value(ir.Num(tc.cpu)), cpu); diff != "" {
			t.Errorf("%s cpu (-want +got):\n%s", tc.size, diff)
		}
		if diff := cmp.Diff(ir.Value(ir.Str(tc.memory)), memory); diff != "" {
			t.Errorf("%s memory (-want +got):\n%s", tc.size, diff)
		}
	}
}

func TestServiceIngressIsExternalOnlyWhenPublic(t *testing.T) {
	p := defaultService
	p.Public = true
	ctx, handle := setupService(t, p)

	if diff := cmp.Diff(ingressBlock(true, 8080)[0], appIngress(t, ctx, appID)); diff != "" {
		t.Errorf("ingress (-want +got):\n%s", diff)
	}
	wantExports := resolve.ServiceExports{Port: ir.Num(8080), Public: true, URL: webURL}
	if diff := cmp.Diff(wantExports, handle.Exports); diff != "" {
		t.Errorf("exports (-want +got):\n%s", diff)
	}
	wantOutputs := []ir.Output{{
		Name:        "web_url",
		Description: "Public URL of the web service",
		Value:       webURL,
	}}
	if diff := cmp.Diff(wantOutputs, ctx.Outputs); diff != "" {
		t.Errorf("outputs (-want +got):\n%s", diff)
	}
	if got := len(ctx.Resources()); got != 7 {
		t.Errorf("a public service added resources: %d in total", got)
	}

	ctx, _ = setupService(t, defaultService)
	if diff := cmp.Diff(ingressBlock(false, 8080)[0], appIngress(t, ctx, appID)); diff != "" {
		t.Errorf("private ingress (-want +got):\n%s", diff)
	}
}

func TestServiceWritesTheReplicasAndThePortAsGiven(t *testing.T) {
	p := defaultService
	p.Port = 3000
	p.MinReplicas = 2
	p.MaxReplicas = 5
	ctx, _ := setupService(t, p)

	template, _ := containerApp(t, ctx, appID).Get("template")
	got := template.(ir.Block)[0]
	minReplicas, _ := got.Get("min_replicas")
	maxReplicas, _ := got.Get("max_replicas")
	if diff := cmp.Diff(ir.Value(ir.Num(2)), minReplicas); diff != "" {
		t.Errorf("min_replicas (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(ir.Value(ir.Num(5)), maxReplicas); diff != "" {
		t.Errorf("max_replicas (-want +got):\n%s", diff)
	}
	port, _ := appIngress(t, ctx, appID).Get("target_port")
	if diff := cmp.Diff(ir.Value(ir.Num(3000)), port); diff != "" {
		t.Errorf("target_port (-want +got):\n%s", diff)
	}
	if _, ok := appContainer(t, ctx, appID).Get("env"); ok {
		t.Error("a service without env got env blocks")
	}
}

func TestTwoServicesShareOneEnvironment(t *testing.T) {
	p := newProject(t, []ir.Node{
		serviceNodeWith(t, "n4", "web", defaultService),
		serviceNodeWith(t, "n5", "admin-ui", defaultService),
	})
	g, err := resolve.Run(p, New())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if errs := ir.ValidateGraph(g); len(errs) > 0 {
		t.Errorf("graph is not valid:\n%s", errs.Error())
	}
	var types []string
	for _, r := range g.Resources {
		types = append(types, r.Type)
	}
	want := []string{
		"azurerm_resource_group",
		"azurerm_virtual_network",
		"azurerm_subnet",
		"azurerm_subnet",
		"azurerm_log_analytics_workspace",
		"azurerm_container_app_environment",
		"azurerm_container_app",
		"azurerm_container_app",
	}
	if diff := cmp.Diff(want, types); diff != "" {
		t.Errorf("resources (-want +got):\n%s", diff)
	}

	files, err := hcl.Emit(g)
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	main := unaligned(files["main.tf"])
	for _, want := range []string{
		`resource "azurerm_container_app" "admin_ui" { name = "shop-dev-admin-ui"`,
		`container_app_environment_id = azurerm_container_app_environment.main.id`,
		`infrastructure_subnet_id = azurerm_subnet.apps.id`,
		`container { name = "admin-ui" image = "nginx:1.27" cpu = 0.25 memory = "0.5Gi" }`,
	} {
		if !strings.Contains(main, want) {
			t.Errorf("main.tf lacks %s:\n%s", want, files["main.tf"])
		}
	}
}
