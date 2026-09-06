package gcp

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

func setupDataAccess(t *testing.T, relations ...ir.Relation) (*resolve.Context, *resolve.Handle) {
	t.Helper()
	edges := make([]ir.Edge, len(relations))
	for i, r := range relations {
		edges[i] = ir.Edge{ID: "e" + string(rune('0'+i)), From: "n2", To: "n3", Relation: r}
	}
	ctx, project := newContext(t, []ir.Node{
		functionNode(t, "n2", "handler", defaultFunction),
		databaseNode(t, "n3", "main-db", defaultDatabase),
	}, edges)
	fn := resolveFunction(ctx, project.Nodes[0])
	db := resolveDatabase(ctx, project.Nodes[1])
	ctx.SetHandle("n2", fn)
	ctx.SetHandle("n3", db)
	for _, e := range project.Edges {
		resolveDataAccess(ctx, e, fn, db)
	}
	fn.Finalise()
	return ctx, fn
}

var databaseEnv = ir.Attrs{
	ir.A("MAIN_DB_HOST", ir.R(dbID, ir.Field("private_ip_address"))),
	ir.A("MAIN_DB_PORT", ir.Str("5432")),
	ir.A("MAIN_DB_NAME", ir.Str("main_db")),
	ir.A("MAIN_DB_USER", ir.Str("app")),
	ir.A("MAIN_DB_PASSWORD", ir.R(dbPasswordID, ir.Field("result"))),
}

func TestReadsFromADatabaseInjectsTheConnectionAndJoinsTheConnector(t *testing.T) {
	ctx, fn := setupDataAccess(t, ir.RelReads)

	if diff := cmp.Diff(databaseEnv, fn.Env); diff != "" {
		t.Errorf("env (-want +got):\n%s", diff)
	}
	if !fn.NeedsNetwork {
		t.Error("the function did not join the network")
	}
	config := serviceConfig(t, ctx)
	connector, _ := config.Get("vpc_connector")
	if diff := cmp.Diff(ir.Value(ir.R(connectorID, ir.Field("id"))), connector); diff != "" {
		t.Errorf("vpc_connector (-want +got):\n%s", diff)
	}
	env, _ := config.Get("environment_variables")
	if diff := cmp.Diff(ir.Value(ir.Map(databaseEnv)), env); diff != "" {
		t.Errorf("environment_variables (-want +got):\n%s", diff)
	}
	if len(ctx.Errors) != 0 {
		t.Errorf("errors = %v", ctx.Errors)
	}

	// The edge adds nothing of its own: no port to open, no role to grant.
	wantTypes := []string{
		"google_storage_bucket",
		"google_service_account",
		"google_storage_bucket_object",
		"google_cloudfunctions2_function",
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
}

func TestWritesToADatabaseWiresTheSameAsReads(t *testing.T) {
	_, reads := setupDataAccess(t, ir.RelReads)
	_, writes := setupDataAccess(t, ir.RelWrites)
	if diff := cmp.Diff(reads.Env, writes.Env); diff != "" {
		t.Errorf("env (-reads +writes):\n%s", diff)
	}
	if !writes.NeedsNetwork {
		t.Error("the writer did not join the network")
	}
}

func TestDataAccessDoesNotDuplicateWiringForReadsAndWrites(t *testing.T) {
	_, fn := setupDataAccess(t, ir.RelReads, ir.RelWrites)
	if diff := cmp.Diff(databaseEnv, fn.Env); diff != "" {
		t.Errorf("env (-want +got):\n%s", diff)
	}
}

func TestDataAccessReportsUnsupportedEnds(t *testing.T) {
	ctx, project := newContext(t, []ir.Node{
		{ID: "n1", Type: ir.NodeGateway, Name: "api"},
		functionNode(t, "n2", "handler", defaultFunction),
		databaseNode(t, "n3", "main-db", defaultDatabase),
	}, []ir.Edge{
		{ID: "e1", From: "n1", To: "n3", Relation: ir.RelReads},
		{ID: "e2", From: "n2", To: "n1", Relation: ir.RelWrites},
	})
	gateway := resolveGateway(project.Nodes[0])
	fn := resolveFunction(ctx, project.Nodes[1])
	db := resolveDatabase(ctx, project.Nodes[2])
	resolveDataAccess(ctx, project.Edges[0], gateway, db)
	resolveDataAccess(ctx, project.Edges[1], fn, gateway)

	want := ir.Errors{
		{EdgeID: "e1", Message: "reads from a gateway is not supported by the gcp resolver yet"},
		{EdgeID: "e2", Message: "writes to a gateway is not supported by the gcp resolver yet"},
	}
	if diff := cmp.Diff(want, ctx.Errors); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
	if fn.NeedsNetwork || len(fn.Env) != 0 {
		t.Errorf("a refused edge wired the function: network %v, env %v", fn.NeedsNetwork, fn.Env)
	}
}
