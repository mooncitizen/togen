package gcp

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

var defaultDatabase = ir.DatabaseProps{Engine: ir.EnginePostgres, Size: ir.SizeSmall, StorageGB: 20}

var (
	dbID                = ir.ID{Type: "google_sql_database_instance", Name: "main_db"}
	dbPasswordID        = ir.ID{Type: "random_password", Name: "main_db"}
	vpcID               = ir.ID{Type: "google_compute_network", Name: "main"}
	connectionID        = ir.ID{Type: "google_service_networking_connection", Name: "main"}
	randomProviderBlock = ir.Provider{Name: "random", Source: "hashicorp/random", Version: "~> 3.6"}
)

func databaseNode(t *testing.T, id, name string, p ir.DatabaseProps) ir.Node {
	t.Helper()
	return ir.Node{ID: id, Type: ir.NodeDatabase, Name: name, Properties: props(t, p)}
}

func setupDatabase(t *testing.T, p ir.DatabaseProps) (*resolve.Context, *resolve.Handle) {
	t.Helper()
	ctx, project := newContext(t, []ir.Node{databaseNode(t, "n3", "main-db", p)}, nil)
	return ctx, resolveDatabase(ctx, project.Nodes[0])
}

func instanceSettings(t *testing.T, ctx *resolve.Context) ir.Attrs {
	t.Helper()
	v, ok := firstOfType(t, ctx, "google_sql_database_instance").Args.Get("settings")
	if !ok {
		t.Fatal("the instance has no settings")
	}
	block, ok := v.(ir.Block)
	if !ok || len(block) != 1 {
		t.Fatalf("settings = %#v", v)
	}
	return block[0]
}

func TestDatabaseEmitsAPrivateInstanceADatabaseAndAUser(t *testing.T) {
	ctx, handle := setupDatabase(t, defaultDatabase)

	wantTypes := []string{
		"google_compute_network",
		"google_compute_subnetwork",
		"google_vpc_access_connector",
		"google_compute_global_address",
		"google_service_networking_connection",
		"google_sql_database_instance",
		"google_sql_database",
		"random_password",
		"google_sql_user",
	}
	if diff := cmp.Diff(wantTypes, resourceTypes(ctx)); diff != "" {
		t.Errorf("resources (-want +got):\n%s", diff)
	}

	instance := firstOfType(t, ctx, "google_sql_database_instance")
	want := ir.Attrs{
		ir.A("name", ir.Str("shop-dev-main-db")),
		ir.A("region", ir.Str("europe-west2")),
		ir.A("database_version", ir.Str("POSTGRES_17")),
		ir.A("deletion_protection", ir.Bool(false)),
		ir.A("settings", ir.B(ir.Attrs{
			ir.A("tier", ir.Str("db-f1-micro")),
			ir.A("edition", ir.Str("ENTERPRISE")),
			ir.A("availability_type", ir.Str("ZONAL")),
			ir.A("disk_size", ir.Num(20)),
			ir.A("disk_autoresize", ir.Bool(false)),
			ir.A("ip_configuration", ir.B(ir.Attrs{
				ir.A("ipv4_enabled", ir.Bool(false)),
				ir.A("private_network", ir.R(vpcID, ir.Field("id"))),
			})),
		})),
	}
	if diff := cmp.Diff(want, instance.Args); diff != "" {
		t.Errorf("instance args (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]ir.ID{connectionID}, instance.DependsOn); diff != "" {
		t.Errorf("depends_on (-want +got):\n%s", diff)
	}

	database := firstOfType(t, ctx, "google_sql_database")
	wantDatabase := ir.Attrs{
		ir.A("name", ir.Str("main_db")),
		ir.A("instance", ir.R(dbID, ir.Field("name"))),
	}
	if diff := cmp.Diff(wantDatabase, database.Args); diff != "" {
		t.Errorf("database args (-want +got):\n%s", diff)
	}

	password := firstOfType(t, ctx, "random_password")
	wantPassword := ir.Attrs{
		ir.A("length", ir.Num(32)),
		ir.A("special", ir.Bool(false)),
	}
	if diff := cmp.Diff(wantPassword, password.Args); diff != "" {
		t.Errorf("password args (-want +got):\n%s", diff)
	}

	user := firstOfType(t, ctx, "google_sql_user")
	wantUser := ir.Attrs{
		ir.A("name", ir.Str("app")),
		ir.A("instance", ir.R(dbID, ir.Field("name"))),
		ir.A("password", ir.R(dbPasswordID, ir.Field("result"))),
	}
	if diff := cmp.Diff(wantUser, user.Args); diff != "" {
		t.Errorf("user args (-want +got):\n%s", diff)
	}
	for _, r := range []ir.Resource{instance, database, password, user} {
		if r.Name != "main_db" || r.SourceNode != "n3" || r.SourceLabel != "main-db" {
			t.Errorf("%s = %q, source %q/%q", r.Type, r.Name, r.SourceNode, r.SourceLabel)
		}
	}

	if diff := cmp.Diff([]ir.Provider{randomProviderBlock}, ctx.Providers()); diff != "" {
		t.Errorf("providers (-want +got):\n%s", diff)
	}
	wantExports := resolve.DatabaseExports{
		Host:     ir.R(dbID, ir.Field("private_ip_address")),
		Port:     ir.Num(5432),
		Name:     ir.Str("main_db"),
		User:     ir.Str("app"),
		Password: ir.R(dbPasswordID, ir.Field("result")),
	}
	if diff := cmp.Diff(wantExports, handle.Exports); diff != "" {
		t.Errorf("exports (-want +got):\n%s", diff)
	}
	if handle.Primary != dbID {
		t.Errorf("primary = %v", handle.Primary)
	}

	wantOutputs := []ir.Output{{
		Name:        "main_db_connection_name",
		Description: "Connection name of the main-db database, for the Cloud SQL Auth Proxy",
		Value:       ir.R(dbID, ir.Field("connection_name")),
	}}
	if diff := cmp.Diff(wantOutputs, ctx.Outputs); diff != "" {
		t.Errorf("outputs (-want +got):\n%s", diff)
	}
}

func TestDatabaseMapsEnginesAndSizesToVersionsTiersAndPorts(t *testing.T) {
	for _, c := range []struct {
		engine  ir.Engine
		size    ir.Size
		version string
		tier    string
		port    float64
	}{
		{ir.EnginePostgres, ir.SizeSmall, "POSTGRES_17", "db-f1-micro", 5432},
		{ir.EnginePostgres, ir.SizeMedium, "POSTGRES_17", "db-custom-2-7680", 5432},
		{ir.EnginePostgres, ir.SizeLarge, "POSTGRES_17", "db-custom-4-15360", 5432},
		{ir.EngineMySQL, ir.SizeSmall, "MYSQL_8_4", "db-f1-micro", 3306},
		{ir.EngineMySQL, ir.SizeMedium, "MYSQL_8_4", "db-custom-2-7680", 3306},
		{ir.EngineMySQL, ir.SizeLarge, "MYSQL_8_4", "db-custom-4-15360", 3306},
	} {
		p := defaultDatabase
		p.Engine = c.engine
		p.Size = c.size
		ctx, handle := setupDatabase(t, p)

		version, _ := firstOfType(t, ctx, "google_sql_database_instance").Args.Get("database_version")
		tier, _ := instanceSettings(t, ctx).Get("tier")
		got := ir.Attrs{
			ir.A("database_version", version),
			ir.A("tier", tier),
			ir.A("port", handle.Exports.(resolve.DatabaseExports).Port),
		}
		want := ir.Attrs{
			ir.A("database_version", ir.Str(c.version)),
			ir.A("tier", ir.Str(c.tier)),
			ir.A("port", ir.Num(c.port)),
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("%s %s (-want +got):\n%s", c.engine, c.size, diff)
		}
	}
}

func TestDatabaseTakesAnExplicitVersionAndDefaultsToTheNewest(t *testing.T) {
	for _, c := range []struct {
		engine  ir.Engine
		version string
		want    string
	}{
		{ir.EnginePostgres, "", "POSTGRES_17"},
		{ir.EnginePostgres, "15", "POSTGRES_15"},
		{ir.EngineMySQL, "", "MYSQL_8_4"},
		{ir.EngineMySQL, "8.0", "MYSQL_8_0"},
	} {
		p := defaultDatabase
		p.Engine = c.engine
		p.Version = c.version
		ctx, _ := setupDatabase(t, p)
		got, _ := firstOfType(t, ctx, "google_sql_database_instance").Args.Get("database_version")
		if diff := cmp.Diff(ir.Value(ir.Str(c.want)), got); diff != "" {
			t.Errorf("%s %q (-want +got):\n%s", c.engine, c.version, diff)
		}
		if len(ctx.Errors) != 0 {
			t.Errorf("%s %q errors = %v", c.engine, c.version, ctx.Errors)
		}
	}
}

func TestDatabaseReportsAnUnsupportedEngineVersion(t *testing.T) {
	p := defaultDatabase
	p.Version = "9.6"
	ctx, handle := setupDatabase(t, p)
	if handle == nil {
		t.Fatal("want a handle even with a bad version")
	}
	want := ir.Errors{{
		NodeID:  "n3",
		Message: "database 'main-db': postgres version '9.6' is not supported on gcp (use one of 17, 16, 15)",
	}}
	if diff := cmp.Diff(want, ctx.Errors); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
	got, _ := firstOfType(t, ctx, "google_sql_database_instance").Args.Get("database_version")
	if diff := cmp.Diff(ir.Value(ir.Str("POSTGRES_17")), got); diff != "" {
		t.Errorf("database_version (-want +got):\n%s", diff)
	}
}

func TestDatabaseSetsHighAvailabilityAndStorage(t *testing.T) {
	p := defaultDatabase
	p.HighAvailability = true
	p.StorageGB = 100
	ctx, _ := setupDatabase(t, p)
	settings := instanceSettings(t, ctx)
	for _, c := range []struct {
		key  string
		want ir.Value
	}{
		{"availability_type", ir.Str("REGIONAL")},
		{"disk_size", ir.Num(100)},
	} {
		got, _ := settings.Get(c.key)
		if diff := cmp.Diff(c.want, got); diff != "" {
			t.Errorf("%s (-want +got):\n%s", c.key, diff)
		}
	}
}

func TestTwoDatabasesShareTheRandomProviderAndTheNetwork(t *testing.T) {
	ctx, project := newContext(t, []ir.Node{
		databaseNode(t, "n3", "main-db", defaultDatabase),
		databaseNode(t, "n4", "reports-db", ir.DatabaseProps{Engine: ir.EngineMySQL, Size: ir.SizeSmall, StorageGB: 20}),
	}, nil)
	resolveDatabase(ctx, project.Nodes[0])
	resolveDatabase(ctx, project.Nodes[1])

	if diff := cmp.Diff([]ir.Provider{randomProviderBlock}, ctx.Providers()); diff != "" {
		t.Errorf("providers (-want +got):\n%s", diff)
	}
	if got := countOfType(ctx, "google_compute_network"); got != 1 {
		t.Errorf("networks = %d, want 1", got)
	}
	var passwords []string
	for _, r := range byType(ctx, "random_password") {
		passwords = append(passwords, r.Name)
	}
	if diff := cmp.Diff([]string{"main_db", "reports_db"}, passwords); diff != "" {
		t.Errorf("passwords (-want +got):\n%s", diff)
	}
}
