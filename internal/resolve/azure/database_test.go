package azure

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/mooncitizen/togen/internal/emit/hcl"
	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

func databaseNode(t *testing.T, id, name string, p ir.DatabaseProps) ir.Node {
	t.Helper()
	return ir.Node{ID: id, Type: ir.NodeDatabase, Name: name, Properties: props(t, p)}
}

func setupDatabase(t *testing.T, p ir.DatabaseProps) (*resolve.Context, *resolve.Handle) {
	t.Helper()
	ctx := newContext(t, []ir.Node{databaseNode(t, "n3", "main-db", p)})
	return ctx, resolveDatabase(ctx, ctx.Project.Nodes[0])
}

// Alignment padding depends on the longest key in a block, so lines are compared without it.
func unaligned(file []byte) string {
	return strings.Join(strings.Fields(string(file)), " ")
}

func grouped(name string, args ...ir.Attr) ir.Attrs {
	return append(ir.Attrs{
		ir.A("name", ir.Str(name)),
		ir.A("resource_group_name", ir.R(groupID, ir.Field("name"))),
	}, args...)
}

var (
	smallPostgres = ir.DatabaseProps{Engine: ir.EnginePostgres, Size: ir.SizeSmall, StorageGB: 20}
	smallMySQL    = ir.DatabaseProps{Engine: ir.EngineMySQL, Size: ir.SizeSmall, StorageGB: 20}

	postgresID     = ir.ID{Type: "azurerm_postgresql_flexible_server", Name: "main_db"}
	mysqlID        = ir.ID{Type: "azurerm_mysql_flexible_server", Name: "main_db"}
	passwordID     = ir.ID{Type: "random_password", Name: "main_db"}
	zoneID         = ir.ID{Type: "azurerm_private_dns_zone", Name: "main_db"}
	linkID         = ir.ID{Type: "azurerm_private_dns_zone_virtual_network_link", Name: "main_db"}
	vnetID         = ir.ID{Type: "azurerm_virtual_network", Name: "main"}
	postgresSubnet = ir.ID{Type: "azurerm_subnet", Name: "postgres"}
	mysqlSubnet    = ir.ID{Type: "azurerm_subnet", Name: "mysql"}

	wantRandom = ir.Provider{Name: "random", Source: "hashicorp/random", Version: "~> 3.6"}
)

var engineServers = []struct {
	engine ir.Engine
	server string
}{
	{ir.EnginePostgres, "azurerm_postgresql_flexible_server"},
	{ir.EngineMySQL, "azurerm_mysql_flexible_server"},
}

func serverArgs(name string, version, sku string, subnet ir.ID, engine ...ir.Attr) ir.Attrs {
	return located(name, append([]ir.Attr{
		ir.A("version", ir.Str(version)),
		ir.A("sku_name", ir.Str(sku)),
		ir.A("administrator_login", ir.Str("app")),
		ir.A("administrator_password", ir.R(passwordID, ir.Field("result"))),
		ir.A("delegated_subnet_id", ir.R(subnet, ir.Field("id"))),
		ir.A("private_dns_zone_id", ir.R(zoneID, ir.Field("id"))),
	}, engine...)...)
}

func checkDatabaseScaffolding(t *testing.T, ctx *resolve.Context, domain string) {
	t.Helper()
	wantPassword := ir.Attrs{
		ir.A("length", ir.Num(32)),
		ir.A("special", ir.Bool(false)),
		ir.A("min_lower", ir.Num(1)),
		ir.A("min_upper", ir.Num(1)),
		ir.A("min_numeric", ir.Num(1)),
	}
	if diff := cmp.Diff(wantPassword, firstOfType(t, ctx, "random_password").Args); diff != "" {
		t.Errorf("random_password args (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]ir.Provider{wantRandom}, ctx.Providers()); diff != "" {
		t.Errorf("providers (-want +got):\n%s", diff)
	}
	wantZone := grouped("shop-dev-main-db." + domain)
	if diff := cmp.Diff(wantZone, firstOfType(t, ctx, "azurerm_private_dns_zone").Args); diff != "" {
		t.Errorf("private dns zone args (-want +got):\n%s", diff)
	}
	wantLink := ir.Attrs{
		ir.A("name", ir.Str("shop-dev-main-db")),
		ir.A("private_dns_zone_id", ir.R(zoneID, ir.Field("id"))),
		ir.A("virtual_network_id", ir.R(vnetID, ir.Field("id"))),
	}
	if diff := cmp.Diff(wantLink, firstOfType(t, ctx, "azurerm_private_dns_zone_virtual_network_link").Args); diff != "" {
		t.Errorf("virtual network link args (-want +got):\n%s", diff)
	}
	for _, r := range ctx.Resources() {
		if r.SourceLabel == groupLabel || r.SourceLabel == networkLabel {
			continue
		}
		if r.Name != "main_db" || r.SourceNode != "n3" || r.SourceLabel != "main-db" {
			t.Errorf("%s = %q %q %q", r.Type, r.Name, r.SourceNode, r.SourceLabel)
		}
	}
	if got := countOfType(ctx, "azurerm_virtual_network"); got != 1 {
		t.Errorf("virtual networks = %d", got)
	}
}

func TestDatabaseEmitsAPostgresFlexibleServerOnThePrivateNetwork(t *testing.T) {
	ctx, handle := setupDatabase(t, smallPostgres)
	if len(ctx.Errors) != 0 {
		t.Fatalf("errors = %v", ctx.Errors)
	}

	server := firstOfType(t, ctx, "azurerm_postgresql_flexible_server")
	want := serverArgs("shop-dev-main-db", "17", "B_Standard_B1ms", postgresSubnet,
		ir.A("public_network_access_enabled", ir.Bool(false)),
		ir.A("storage_mb", ir.Num(32768)),
	)
	if diff := cmp.Diff(want, server.Args); diff != "" {
		t.Errorf("server args (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]ir.ID{linkID}, server.DependsOn); diff != "" {
		t.Errorf("server depends_on (-want +got):\n%s", diff)
	}
	wantDatabase := ir.Attrs{
		ir.A("name", ir.Str("main_db")),
		ir.A("server_id", ir.R(postgresID, ir.Field("id"))),
	}
	if diff := cmp.Diff(wantDatabase, firstOfType(t, ctx, "azurerm_postgresql_flexible_server_database").Args); diff != "" {
		t.Errorf("database args (-want +got):\n%s", diff)
	}
	checkDatabaseScaffolding(t, ctx, "postgres.database.azure.com")
	if _, ok := ctx.Resource(mysqlSubnet); ok {
		t.Error("a postgres database made the mysql subnet")
	}

	wantExports := resolve.DatabaseExports{
		Host:     ir.R(postgresID, ir.Field("fqdn")),
		Port:     ir.Num(5432),
		Name:     ir.Str("main_db"),
		User:     ir.Str("app"),
		Password: ir.R(passwordID, ir.Field("result")),
	}
	if diff := cmp.Diff(wantExports, handle.Exports); diff != "" {
		t.Errorf("exports (-want +got):\n%s", diff)
	}
	if handle.Primary != postgresID {
		t.Errorf("primary = %v", handle.Primary)
	}
	wantOutputs := []ir.Output{{
		Name:        "main_db_fqdn",
		Description: "FQDN of the main-db database",
		Value:       ir.R(postgresID, ir.Field("fqdn")),
	}}
	if diff := cmp.Diff(wantOutputs, ctx.Outputs); diff != "" {
		t.Errorf("outputs (-want +got):\n%s", diff)
	}
}

func TestDatabaseEmitsAMySQLFlexibleServerOnItsOwnSubnet(t *testing.T) {
	ctx, handle := setupDatabase(t, smallMySQL)
	if len(ctx.Errors) != 0 {
		t.Fatalf("errors = %v", ctx.Errors)
	}

	server := firstOfType(t, ctx, "azurerm_mysql_flexible_server")
	want := serverArgs("shop-dev-main-db", "8.0.21", "B_Standard_B1ms", mysqlSubnet,
		ir.A("public_network_access", ir.Str("Disabled")),
		ir.A("storage", ir.B(ir.Attrs{ir.A("size_gb", ir.Num(20))})),
	)
	if diff := cmp.Diff(want, server.Args); diff != "" {
		t.Errorf("server args (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]ir.ID{linkID}, server.DependsOn); diff != "" {
		t.Errorf("server depends_on (-want +got):\n%s", diff)
	}
	wantDatabase := grouped("main_db",
		ir.A("server_name", ir.R(mysqlID, ir.Field("name"))),
		ir.A("charset", ir.Str("utf8mb4")),
		ir.A("collation", ir.Str("utf8mb4_unicode_ci")),
	)
	if diff := cmp.Diff(wantDatabase, firstOfType(t, ctx, "azurerm_mysql_flexible_database").Args); diff != "" {
		t.Errorf("database args (-want +got):\n%s", diff)
	}
	checkDatabaseScaffolding(t, ctx, "mysql.database.azure.com")
	if _, ok := ctx.Resource(mysqlSubnet); !ok {
		t.Error("no mysql subnet")
	}
	if got := countOfType(ctx, "azurerm_subnet"); got != 3 {
		t.Errorf("subnets = %d", got)
	}

	wantExports := resolve.DatabaseExports{
		Host:     ir.R(mysqlID, ir.Field("fqdn")),
		Port:     ir.Num(3306),
		Name:     ir.Str("main_db"),
		User:     ir.Str("app"),
		Password: ir.R(passwordID, ir.Field("result")),
	}
	if diff := cmp.Diff(wantExports, handle.Exports); diff != "" {
		t.Errorf("exports (-want +got):\n%s", diff)
	}
	if handle.Primary != mysqlID {
		t.Errorf("primary = %v", handle.Primary)
	}
	if len(ctx.Outputs) != 1 || ctx.Outputs[0].Name != "main_db_fqdn" {
		t.Errorf("outputs = %+v", ctx.Outputs)
	}
}

func TestDatabaseMapsSizesOntoTheSameSkusForBothEngines(t *testing.T) {
	skus := map[ir.Size]string{
		ir.SizeSmall:  "B_Standard_B1ms",
		ir.SizeMedium: "GP_Standard_D2ds_v4",
		ir.SizeLarge:  "GP_Standard_D4ds_v4",
	}
	for _, es := range engineServers {
		for _, size := range ir.Sizes {
			ctx, _ := setupDatabase(t, ir.DatabaseProps{Engine: es.engine, Size: size, StorageGB: 20})
			got, _ := firstOfType(t, ctx, es.server).Args.Get("sku_name")
			if diff := cmp.Diff(ir.Value(ir.Str(skus[size])), got); diff != "" {
				t.Errorf("%s %s sku_name (-want +got):\n%s", es.engine, size, diff)
			}
		}
	}
}

func TestDatabaseDefaultsToTheNewestVersionAndTakesAnExplicitOne(t *testing.T) {
	for _, tc := range []struct {
		engine  ir.Engine
		server  string
		version string
		want    string
	}{
		{ir.EnginePostgres, "azurerm_postgresql_flexible_server", "", "17"},
		{ir.EnginePostgres, "azurerm_postgresql_flexible_server", "15", "15"},
		{ir.EngineMySQL, "azurerm_mysql_flexible_server", "", "8.0.21"},
		{ir.EngineMySQL, "azurerm_mysql_flexible_server", "5.7", "5.7"},
	} {
		ctx, _ := setupDatabase(t, ir.DatabaseProps{Engine: tc.engine, Version: tc.version, Size: ir.SizeSmall, StorageGB: 20})
		if len(ctx.Errors) != 0 {
			t.Errorf("%s %q: errors = %v", tc.engine, tc.version, ctx.Errors)
		}
		got, _ := firstOfType(t, ctx, tc.server).Args.Get("version")
		if diff := cmp.Diff(ir.Value(ir.Str(tc.want)), got); diff != "" {
			t.Errorf("%s %q version (-want +got):\n%s", tc.engine, tc.version, diff)
		}
	}
}

func TestDatabaseReportsAnUnsupportedEngineVersion(t *testing.T) {
	ctx, handle := setupDatabase(t, ir.DatabaseProps{
		Engine:    ir.EngineMySQL,
		Version:   "8.0",
		Size:      ir.SizeSmall,
		StorageGB: 20,
	})
	if handle == nil {
		t.Fatal("want a handle even with a bad version")
	}
	want := ir.Errors{{
		NodeID:  "n3",
		Message: "database 'main-db': mysql version '8.0' is not supported on azure (use one of 8.0.21, 5.7)",
	}}
	if diff := cmp.Diff(want, ctx.Errors); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
	got, _ := firstOfType(t, ctx, "azurerm_mysql_flexible_server").Args.Get("version")
	if diff := cmp.Diff(ir.Value(ir.Str("8.0.21")), got); diff != "" {
		t.Errorf("version (-want +got):\n%s", diff)
	}
}

func TestDatabaseRoundsPostgresStorageUpToATier(t *testing.T) {
	for _, tc := range []struct {
		gb   int
		want float64
	}{
		{20, 32768},
		{32, 32768},
		{33, 65536},
		{100, 131072},
		{4096, 4194304},
		{32767, 33553408},
	} {
		ctx, _ := setupDatabase(t, ir.DatabaseProps{Engine: ir.EnginePostgres, Size: ir.SizeSmall, StorageGB: tc.gb})
		if len(ctx.Errors) != 0 {
			t.Errorf("%d GB: errors = %v", tc.gb, ctx.Errors)
		}
		got, _ := firstOfType(t, ctx, "azurerm_postgresql_flexible_server").Args.Get("storage_mb")
		if diff := cmp.Diff(ir.Value(ir.Num(tc.want)), got); diff != "" {
			t.Errorf("%d GB storage_mb (-want +got):\n%s", tc.gb, diff)
		}
	}
}

func TestDatabaseReportsPostgresStorageAboveTheLargestTier(t *testing.T) {
	ctx, _ := setupDatabase(t, ir.DatabaseProps{Engine: ir.EnginePostgres, Size: ir.SizeSmall, StorageGB: 40000})
	want := ir.Errors{{
		NodeID:  "n3",
		Message: "database 'main-db': storageGb 40000 is more than the 32767 GB a postgres flexible server holds",
	}}
	if diff := cmp.Diff(want, ctx.Errors); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
}

func TestDatabaseWritesMySQLStorageInGigabytes(t *testing.T) {
	ctx, _ := setupDatabase(t, ir.DatabaseProps{Engine: ir.EngineMySQL, Size: ir.SizeLarge, StorageGB: 100})
	server := firstOfType(t, ctx, "azurerm_mysql_flexible_server")
	got, _ := server.Args.Get("storage")
	if diff := cmp.Diff(ir.Value(ir.B(ir.Attrs{ir.A("size_gb", ir.Num(100))})), got); diff != "" {
		t.Errorf("storage (-want +got):\n%s", diff)
	}
	if _, ok := server.Args.Get("storage_mb"); ok {
		t.Error("a mysql server was given storage_mb")
	}
}

func TestDatabaseHighAvailabilityIsZoneRedundant(t *testing.T) {
	for _, es := range engineServers {
		for _, size := range []ir.Size{ir.SizeMedium, ir.SizeLarge} {
			ctx, _ := setupDatabase(t, ir.DatabaseProps{Engine: es.engine, Size: size, StorageGB: 20, HighAvailability: true})
			if len(ctx.Errors) != 0 {
				t.Errorf("%s %s: errors = %v", es.engine, size, ctx.Errors)
			}
			args := firstOfType(t, ctx, es.server).Args
			got, _ := args.Get("high_availability")
			want := ir.B(ir.Attrs{ir.A("mode", ir.Str("ZoneRedundant"))})
			if diff := cmp.Diff(ir.Value(want), got); diff != "" {
				t.Errorf("%s %s high_availability (-want +got):\n%s", es.engine, size, diff)
			}
			if args[len(args)-1].Key != "high_availability" {
				t.Errorf("%s %s: the high_availability block is not last: %v", es.engine, size, args[len(args)-1].Key)
			}
		}
	}
	ctx, _ := setupDatabase(t, smallPostgres)
	if _, ok := firstOfType(t, ctx, "azurerm_postgresql_flexible_server").Args.Get("high_availability"); ok {
		t.Error("a database without highAvailability got a high_availability block")
	}
}

func TestDatabaseRefusesHighAvailabilityOnTheBurstableTier(t *testing.T) {
	for _, es := range engineServers {
		p := ir.DatabaseProps{Engine: es.engine, Size: ir.SizeSmall, StorageGB: 20, HighAvailability: true}
		project := newProject(t, []ir.Node{databaseNode(t, "n3", "main-db", p)})
		want := ir.Errors{{
			NodeID:  "n3",
			Message: "database 'main-db': highAvailability needs a medium or large size on azure, the burstable tier has no standby",
		}}
		if diff := cmp.Diff(want, runErrors(t, project)); diff != "" {
			t.Errorf("%s errors (-want +got):\n%s", es.engine, diff)
		}
	}
}

func TestTwoDatabasesShareOneRandomProviderEntry(t *testing.T) {
	p := newProject(t, []ir.Node{
		databaseNode(t, "n3", "main-db", smallPostgres),
		databaseNode(t, "n4", "reports", smallMySQL),
	})
	g, err := resolve.Run(p, New())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if errs := ir.ValidateGraph(g); len(errs) > 0 {
		t.Errorf("graph is not valid:\n%s", errs.Error())
	}
	if diff := cmp.Diff([]ir.Provider{New().ProviderBlock(p), wantRandom}, g.Providers); diff != "" {
		t.Errorf("providers (-want +got):\n%s", diff)
	}
	var passwords, subnets int
	for _, r := range g.Resources {
		switch r.Type {
		case "random_password":
			passwords++
		case "azurerm_subnet":
			subnets++
		}
	}
	if passwords != 2 || subnets != 3 {
		t.Errorf("random passwords = %d, subnets = %d", passwords, subnets)
	}

	files, err := hcl.Emit(g)
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	providers := string(files["providers.tf"])
	if strings.Count(providers, "hashicorp/random") != 1 {
		t.Errorf("providers.tf:\n%s", providers)
	}
	main := unaligned(files["main.tf"])
	for _, want := range []string{
		`resource "random_password" "main_db"`,
		`administrator_password = random_password.main_db.result`,
		`depends_on = [azurerm_private_dns_zone_virtual_network_link.main_db]`,
		`depends_on = [azurerm_private_dns_zone_virtual_network_link.reports]`,
		`delegated_subnet_id = azurerm_subnet.mysql.id`,
	} {
		if !strings.Contains(main, want) {
			t.Errorf("main.tf lacks %s:\n%s", want, files["main.tf"])
		}
	}
}
