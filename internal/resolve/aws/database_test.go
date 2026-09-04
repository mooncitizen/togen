package aws

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

func setupDatabase(t *testing.T, p ir.DatabaseProps) (*resolve.Context, *resolve.Handle) {
	t.Helper()
	ctx, project := newContext(t, []ir.Node{{
		ID:         "n3",
		Type:       ir.NodeDatabase,
		Name:       "main-db",
		Properties: props(t, p),
	}}, nil)
	return ctx, resolveDatabase(ctx, project.Nodes[0])
}

var dbID = ir.ID{Type: "aws_db_instance", Name: "main_db"}

func TestDatabaseEmitsAnInstanceWithManagedCredentials(t *testing.T) {
	ctx, handle := setupDatabase(t, ir.DatabaseProps{Engine: ir.EnginePostgres, Size: ir.SizeSmall, StorageGB: 20})

	db := firstOfType(t, ctx, "aws_db_instance")
	if db.Name != "main_db" || db.SourceNode != "n3" || db.SourceLabel != "main-db" {
		t.Errorf("db = %q %q %q", db.Name, db.SourceNode, db.SourceLabel)
	}
	want := ir.Attrs{
		ir.A("identifier", ir.Str("shop-dev-main-db")),
		ir.A("engine", ir.Str("postgres")),
		ir.A("engine_version", ir.Str("17")),
		ir.A("instance_class", ir.Str("db.t4g.micro")),
		ir.A("allocated_storage", ir.Num(20)),
		ir.A("db_name", ir.Str("main_db")),
		ir.A("username", ir.Str("app")),
		ir.A("manage_master_user_password", ir.Bool(true)),
		ir.A("db_subnet_group_name", ir.R(ir.ID{Type: "aws_db_subnet_group", Name: "main_db"}, ir.Field("name"))),
		ir.A("vpc_security_group_ids", ir.L(ir.R(ir.ID{Type: "aws_security_group", Name: "main_db"}, ir.Field("id")))),
		ir.A("multi_az", ir.Bool(false)),
		ir.A("storage_encrypted", ir.Bool(true)),
		ir.A("publicly_accessible", ir.Bool(false)),
		ir.A("backup_retention_period", ir.Num(7)),
		ir.A("skip_final_snapshot", ir.Bool(true)),
		ir.A("deletion_protection", ir.Bool(false)),
	}
	if diff := cmp.Diff(want, db.Args); diff != "" {
		t.Errorf("db args (-want +got):\n%s", diff)
	}

	subnetGroup := firstOfType(t, ctx, "aws_db_subnet_group")
	wantGroup := ir.Attrs{
		ir.A("name", ir.Str("shop-dev-main-db")),
		ir.A("subnet_ids", ir.L(
			ir.R(ir.ID{Type: "aws_subnet", Name: "private_a"}, ir.Field("id")),
			ir.R(ir.ID{Type: "aws_subnet", Name: "private_b"}, ir.Field("id")),
		)),
	}
	if diff := cmp.Diff(wantGroup, subnetGroup.Args); diff != "" {
		t.Errorf("subnet group args (-want +got):\n%s", diff)
	}

	sg := firstOfType(t, ctx, "aws_security_group")
	wantSG := ir.Attrs{
		ir.A("name", ir.Str("shop-dev-main-db")),
		ir.A("description", ir.Str("Access to main-db")),
		ir.A("vpc_id", ir.R(ir.ID{Type: "aws_vpc", Name: "main"}, ir.Field("id"))),
	}
	if sg.Name != "main_db" {
		t.Errorf("security group name = %q", sg.Name)
	}
	if diff := cmp.Diff(wantSG, sg.Args); diff != "" {
		t.Errorf("security group args (-want +got):\n%s", diff)
	}
	if countOfType(ctx, "aws_vpc") != 1 {
		t.Error("no implicit network")
	}

	if diff := cmp.Diff(&ir.ID{Type: "aws_security_group", Name: "main_db"}, handle.SecurityGroup); diff != "" {
		t.Errorf("handle security group (-want +got):\n%s", diff)
	}
	wantExports := resolve.DatabaseExports{
		Host:      ir.R(dbID, ir.Field("address")),
		Port:      ir.Num(5432),
		Name:      ir.Str("main_db"),
		SecretARN: ir.R(dbID, ir.Field("master_user_secret"), ir.Index(0), ir.Field("secret_arn")),
	}
	if diff := cmp.Diff(wantExports, handle.Exports); diff != "" {
		t.Errorf("exports (-want +got):\n%s", diff)
	}
	if handle.Primary != dbID {
		t.Errorf("primary = %v", handle.Primary)
	}

	wantOutputs := []ir.Output{{
		Name:        "main_db_endpoint",
		Description: "Endpoint of the main-db database",
		Value:       ir.R(dbID, ir.Field("endpoint")),
	}}
	if diff := cmp.Diff(wantOutputs, ctx.Outputs); diff != "" {
		t.Errorf("outputs (-want +got):\n%s", diff)
	}
}

func TestDatabaseMapsSizesEnginesAndHighAvailability(t *testing.T) {
	ctx, _ := setupDatabase(t, ir.DatabaseProps{
		Engine:           ir.EngineMySQL,
		Size:             ir.SizeLarge,
		StorageGB:        100,
		HighAvailability: true,
	})
	db := firstOfType(t, ctx, "aws_db_instance")
	for _, c := range []struct {
		key  string
		want ir.Value
	}{
		{"engine", ir.Str("mysql")},
		{"engine_version", ir.Str("8.4")},
		{"instance_class", ir.Str("db.r6g.large")},
		{"multi_az", ir.Bool(true)},
		{"allocated_storage", ir.Num(100)},
	} {
		got, _ := db.Args.Get(c.key)
		if diff := cmp.Diff(c.want, got); diff != "" {
			t.Errorf("%s (-want +got):\n%s", c.key, diff)
		}
	}
}

func TestDatabaseReportsAnUnsupportedEngineVersion(t *testing.T) {
	ctx, handle := setupDatabase(t, ir.DatabaseProps{
		Engine:    ir.EnginePostgres,
		Version:   "9.6",
		Size:      ir.SizeSmall,
		StorageGB: 20,
	})
	if handle == nil {
		t.Fatal("want a handle even with a bad version")
	}
	if len(ctx.Errors) != 1 {
		t.Fatalf("errors = %v", ctx.Errors)
	}
	if ctx.Errors[0].NodeID != "n3" || !strings.Contains(ctx.Errors[0].Message, "9.6") {
		t.Errorf("error = %+v", ctx.Errors[0])
	}
	got, _ := firstOfType(t, ctx, "aws_db_instance").Args.Get("engine_version")
	if diff := cmp.Diff(ir.Str("17"), got); diff != "" {
		t.Errorf("engine_version (-want +got):\n%s", diff)
	}
}
