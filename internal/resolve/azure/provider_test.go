package azure

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/mooncitizen/togen/internal/emit/hcl"
	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

func runErrors(t *testing.T, p *ir.Project) ir.Errors {
	t.Helper()
	_, err := resolve.Run(p, New())
	if err == nil {
		t.Fatal("want a ResolveError, got nil")
	}
	re, ok := err.(*resolve.ResolveError)
	if !ok {
		t.Fatalf("want a *resolve.ResolveError, got %T: %v", err, err)
	}
	return re.Errors
}

func TestProviderBlock(t *testing.T) {
	want := ir.Provider{
		Name:    "azurerm",
		Source:  "hashicorp/azurerm",
		Version: "~> 5.0",
		Config:  ir.Attrs{ir.A("features", ir.B(ir.Attrs{}))},
	}
	if diff := cmp.Diff(want, New().ProviderBlock(newProject(t, nil))); diff != "" {
		t.Errorf("provider block (-want +got):\n%s", diff)
	}
}

func TestProviderBlockEmitsTheFeaturesBlockAndNoSubscription(t *testing.T) {
	g, err := resolve.Run(newProject(t, nil), New())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	files, err := hcl.Emit(g)
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	providers := string(files["providers.tf"])
	if !regexp.MustCompile(`provider "azurerm" \{\n  features \{\s*\}\n\}`).MatchString(providers) {
		t.Errorf("providers.tf:\n%s", providers)
	}
	for _, absent := range []string{"subscription_id", "location", "region"} {
		if strings.Contains(providers, absent) {
			t.Errorf("providers.tf sets %s:\n%s", absent, providers)
		}
	}
}

func TestNameLimits(t *testing.T) {
	want := []resolve.NameLimit{
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
	if diff := cmp.Diff(want, New().NameLimits()); diff != "" {
		t.Errorf("name limits (-want +got):\n%s", diff)
	}
}

func TestResolveProducesAValidGraphForAnEmptyProject(t *testing.T) {
	g, err := resolve.Run(newProject(t, nil), New())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if errs := ir.ValidateGraph(g); len(errs) > 0 {
		t.Errorf("graph is not valid:\n%s", errs.Error())
	}
	if len(g.Providers) != 1 || g.Providers[0].Name != "azurerm" {
		t.Errorf("providers = %+v", g.Providers)
	}
	if len(g.Resources) != 0 || len(g.Data) != 0 {
		t.Errorf("nothing asked for a network, got resources %+v and data %+v", g.Resources, g.Data)
	}
}

func TestResolveProducesAValidGraphForAGatewayAndAFunction(t *testing.T) {
	p := newProject(t, []ir.Node{
		{ID: "n1", Type: ir.NodeGateway, Name: "api"},
		{ID: "n2", Type: ir.NodeFunction, Name: "orders"},
	})
	p.Edges = []ir.Edge{{
		ID: "e1", From: "n1", To: "n2", Relation: ir.RelRoutes,
		Properties: ir.EdgeProperties{Path: "/orders", Methods: []ir.Method{ir.MethodGet}},
	}}
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
		"azurerm_service_plan",
		"azurerm_storage_account",
		"azurerm_linux_function_app",
	}
	if diff := cmp.Diff(want, types); diff != "" {
		t.Errorf("resources (-want +got):\n%s", diff)
	}
	if len(g.Outputs) != 1 || g.Outputs[0].Name != "api_orders_url" {
		t.Errorf("outputs = %+v", g.Outputs)
	}
	files, err := hcl.Emit(g)
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	if !strings.Contains(string(files["main.tf"]), `resource "azurerm_linux_function_app" "orders"`) {
		t.Errorf("main.tf:\n%s", files["main.tf"])
	}
	wantURL := `"https://${azurerm_linux_function_app.orders.default_hostname}"`
	if !strings.Contains(string(files["outputs.tf"]), wantURL) {
		t.Errorf("outputs.tf:\n%s", files["outputs.tf"])
	}
}

func TestResolveProducesAValidGraphForAFunctionReadingADatabase(t *testing.T) {
	p := newProject(t, []ir.Node{
		{ID: "n1", Type: ir.NodeGateway, Name: "api"},
		{ID: "n2", Type: ir.NodeFunction, Name: "orders"},
		{ID: "n3", Type: ir.NodeDatabase, Name: "main-db"},
	})
	p.Edges = []ir.Edge{
		{ID: "e1", From: "n1", To: "n2", Relation: ir.RelRoutes},
		{ID: "e2", From: "n2", To: "n3", Relation: ir.RelReads},
	}
	g, err := resolve.Run(p, New())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if errs := ir.ValidateGraph(g); len(errs) > 0 {
		t.Errorf("graph is not valid:\n%s", errs.Error())
	}
	var providers []string
	for _, prov := range g.Providers {
		providers = append(providers, prov.Name)
	}
	if diff := cmp.Diff([]string{"azurerm", "random"}, providers); diff != "" {
		t.Errorf("providers (-want +got):\n%s", diff)
	}
	var types []string
	for _, r := range g.Resources {
		types = append(types, r.Type)
	}
	want := []string{
		"azurerm_resource_group",
		"azurerm_service_plan",
		"azurerm_storage_account",
		"azurerm_linux_function_app",
		"azurerm_virtual_network",
		"azurerm_subnet",
		"azurerm_subnet",
		"random_password",
		"azurerm_private_dns_zone",
		"azurerm_private_dns_zone_virtual_network_link",
		"azurerm_postgresql_flexible_server",
		"azurerm_postgresql_flexible_server_database",
	}
	if diff := cmp.Diff(want, types); diff != "" {
		t.Errorf("resources (-want +got):\n%s", diff)
	}
	var outputs []string
	for _, o := range g.Outputs {
		outputs = append(outputs, o.Name)
	}
	if diff := cmp.Diff([]string{"main_db_fqdn", "api_orders_url"}, outputs); diff != "" {
		t.Errorf("outputs (-want +got):\n%s", diff)
	}
	files, err := hcl.Emit(g)
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	main := unaligned(files["main.tf"])
	for _, want := range []string{
		`MAIN_DB_HOST = azurerm_postgresql_flexible_server.main_db.fqdn`,
		`MAIN_DB_PASSWORD = random_password.main_db.result`,
		`depends_on = [azurerm_private_dns_zone_virtual_network_link.main_db]`,
	} {
		if !strings.Contains(main, want) {
			t.Errorf("main.tf lacks %s:\n%s", want, files["main.tf"])
		}
	}
	if !strings.Contains(string(files["providers.tf"]), `source  = "hashicorp/random"`) {
		t.Errorf("providers.tf:\n%s", files["providers.tf"])
	}
}

func TestResolveProducesAValidGraphForAServiceBehindAGateway(t *testing.T) {
	p := newProject(t, []ir.Node{
		{ID: "n1", Type: ir.NodeGateway, Name: "api"},
		{ID: "n2", Type: ir.NodeFunction, Name: "orders"},
		serviceNodeWith(t, "n4", "web", defaultService),
		{ID: "n3", Type: ir.NodeDatabase, Name: "main-db"},
	})
	p.Edges = []ir.Edge{
		{ID: "e1", From: "n1", To: "n4", Relation: ir.RelRoutes},
		{ID: "e2", From: "n4", To: "n3", Relation: ir.RelReads},
		{ID: "e3", From: "n4", To: "n2", Relation: ir.RelCalls},
		{ID: "e4", From: "n2", To: "n4", Relation: ir.RelCalls},
	}
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
		"azurerm_service_plan",
		"azurerm_storage_account",
		"azurerm_linux_function_app",
		"azurerm_virtual_network",
		"azurerm_subnet",
		"azurerm_subnet",
		"azurerm_log_analytics_workspace",
		"azurerm_container_app_environment",
		"azurerm_container_app",
		"random_password",
		"azurerm_private_dns_zone",
		"azurerm_private_dns_zone_virtual_network_link",
		"azurerm_postgresql_flexible_server",
		"azurerm_postgresql_flexible_server_database",
	}
	if diff := cmp.Diff(want, types); diff != "" {
		t.Errorf("resources (-want +got):\n%s", diff)
	}
	var outputs []string
	for _, o := range g.Outputs {
		outputs = append(outputs, o.Name)
	}
	if diff := cmp.Diff([]string{"main_db_fqdn", "api_web_url"}, outputs); diff != "" {
		t.Errorf("outputs (-want +got):\n%s", diff)
	}
	files, err := hcl.Emit(g)
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	main := unaligned(files["main.tf"])
	for _, want := range []string{
		`external_enabled = true`,
		`env { name = "MAIN_DB_HOST" value = azurerm_postgresql_flexible_server.main_db.fqdn }`,
		`env { name = "ORDERS_URL" value = "https://${azurerm_linux_function_app.orders.default_hostname}" }`,
		`WEB_URL = "https://${azurerm_container_app.web.ingress[0].fqdn}"`,
	} {
		if !strings.Contains(main, want) {
			t.Errorf("main.tf lacks %s:\n%s", want, files["main.tf"])
		}
	}
}

func TestResolveRejectsOtherProviders(t *testing.T) {
	p := newProject(t, nil)
	p.Provider = ir.ProviderAWS
	want := ir.Errors{{Path: "provider", Message: "the azure resolver cannot resolve a 'aws' project"}}
	if diff := cmp.Diff(want, runErrors(t, p)); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
}

func TestResolveRefusesTheNodeTypesWithoutAResolverYet(t *testing.T) {
	for _, typ := range ir.NodeTypes {
		if typ == ir.NodeFunction || typ == ir.NodeGateway || typ == ir.NodeDatabase || typ == ir.NodeService {
			continue
		}
		ctx := newContext(t, nil)
		handle, ok := New().ResolveNode(ctx, ir.Node{ID: "n1", Type: typ, Name: "thing"})
		if ok || handle != nil {
			t.Errorf("ResolveNode(%s) = %v, %v", typ, handle, ok)
		}
		want := ir.Errors{{
			NodeID:  "n1",
			Message: fmt.Sprintf("node type '%s' is not supported by the azure resolver yet", typ),
		}}
		if diff := cmp.Diff(want, ctx.Errors); diff != "" {
			t.Errorf("%s errors (-want +got):\n%s", typ, diff)
		}
	}
}

func TestResolveReportsEveryNodeInFileOrder(t *testing.T) {
	p := newProject(t, []ir.Node{
		{ID: "n1", Type: ir.NodeQueue, Name: "jobs"},
		{ID: "n2", Type: ir.NodeBucket, Name: "uploads"},
	})
	want := ir.Errors{
		{NodeID: "n1", Message: "node type 'queue' is not supported by the azure resolver yet"},
		{NodeID: "n2", Message: "node type 'bucket' is not supported by the azure resolver yet"},
	}
	if diff := cmp.Diff(want, runErrors(t, p)); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
}

func TestResolveRefusesTheRelationsWithoutAResolverYet(t *testing.T) {
	for _, rel := range ir.Relations {
		if rel == ir.RelRoutes || rel == ir.RelCalls || rel == ir.RelReads || rel == ir.RelWrites {
			continue
		}
		ctx := newContext(t, nil)
		New().ResolveEdge(ctx, ir.Edge{ID: "e1", Relation: rel}, nil, nil)
		want := ir.Errors{{
			EdgeID:  "e1",
			Message: fmt.Sprintf("'%s' edges are not supported by the azure resolver yet", rel),
		}}
		if diff := cmp.Diff(want, ctx.Errors); diff != "" {
			t.Errorf("%s errors (-want +got):\n%s", rel, diff)
		}
	}
}
