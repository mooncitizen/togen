package azure

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

func setupDataAccess(t *testing.T, relations ...ir.Relation) (*resolve.Context, *resolve.Handle) {
	t.Helper()
	ctx := newContext(t, []ir.Node{
		functionNode(t, "n2", "handler", defaultFunction),
		databaseNode(t, "n3", "main-db", smallPostgres),
	})
	fn := resolveFunction(ctx, ctx.Project.Nodes[0])
	db := resolveDatabase(ctx, ctx.Project.Nodes[1])
	for i, r := range relations {
		edge := ir.Edge{ID: "e" + string(rune('1'+i)), From: "n2", To: "n3", Relation: r}
		resolveDataAccess(ctx, edge, fn, db)
	}
	fn.Finalise()
	return ctx, fn
}

var connectionSettings = ir.Attrs{
	ir.A("MAIN_DB_HOST", ir.R(postgresID, ir.Field("fqdn"))),
	ir.A("MAIN_DB_PORT", ir.Str("5432")),
	ir.A("MAIN_DB_NAME", ir.Str("main_db")),
	ir.A("MAIN_DB_USER", ir.Str("app")),
	ir.A("MAIN_DB_PASSWORD", ir.R(passwordID, ir.Field("result"))),
}

func TestReadsFromAFunctionSetTheConnectionAsAppSettings(t *testing.T) {
	ctx, fn := setupDataAccess(t, ir.RelReads)
	if len(ctx.Errors) != 0 {
		t.Fatalf("errors = %v", ctx.Errors)
	}
	settings, _ := firstOfType(t, ctx, "azurerm_linux_function_app").Args.Get("app_settings")
	timeout := ir.A("AzureFunctionsJobHost__functionTimeout", ir.Str("00:00:30"))
	want := ir.M(append(ir.Attrs{timeout}, connectionSettings...)...)
	if diff := cmp.Diff(ir.Value(want), settings); diff != "" {
		t.Errorf("app_settings (-want +got):\n%s", diff)
	}
	if !fn.NeedsNetwork {
		t.Error("the edge did not ask for the network")
	}
	// The group, the function's three, the network's three and the database's five.
	if got := len(ctx.Resources()); got != 12 {
		t.Errorf("the edge added resources: %d in total", got)
	}
}

func TestReadsAndWritesSetTheConnectionOnce(t *testing.T) {
	ctx, fn := setupDataAccess(t, ir.RelReads, ir.RelWrites)
	if len(ctx.Errors) != 0 {
		t.Fatalf("errors = %v", ctx.Errors)
	}
	if len(fn.Env) != 6 {
		t.Errorf("env = %v", fn.Env)
	}
}

func TestDataAccessFromAServiceWiresItTheSameWay(t *testing.T) {
	ctx := newContext(t, []ir.Node{serviceNode("n4", "web"), databaseNode(t, "n3", "main-db", smallPostgres)})
	svc := &resolve.Handle{Node: ctx.Project.Nodes[0], Exports: resolve.ServiceExports{}}
	db := resolveDatabase(ctx, ctx.Project.Nodes[1])
	resolveDataAccess(ctx, ir.Edge{ID: "e1", From: "n4", To: "n3", Relation: ir.RelWrites}, svc, db)

	if len(ctx.Errors) != 0 {
		t.Fatalf("errors = %v", ctx.Errors)
	}
	if diff := cmp.Diff(connectionSettings, svc.Env); diff != "" {
		t.Errorf("env (-want +got):\n%s", diff)
	}
	if !svc.NeedsNetwork {
		t.Error("the service was not asked to join the network")
	}
}

func TestDataAccessReportsUnsupportedEnds(t *testing.T) {
	ctx := newContext(t, []ir.Node{gatewayNode, databaseNode(t, "n3", "main-db", smallPostgres)})
	gateway := resolveGateway(ctx.Project.Nodes[0])
	db := resolveDatabase(ctx, ctx.Project.Nodes[1])
	resolveDataAccess(ctx, ir.Edge{ID: "e1", From: "n1", To: "n3", Relation: ir.RelReads}, gateway, db)

	want := ir.Errors{{
		EdgeID:  "e1",
		Message: "reads from a gateway is not supported by the azure resolver yet",
	}}
	if diff := cmp.Diff(want, ctx.Errors); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
}

func TestDataAccessReportsAnUnsupportedTarget(t *testing.T) {
	ctx := newContext(t, []ir.Node{functionNode(t, "n2", "handler", defaultFunction), gatewayNode})
	fn := resolveFunction(ctx, ctx.Project.Nodes[0])
	gateway := resolveGateway(ctx.Project.Nodes[1])
	resolveDataAccess(ctx, ir.Edge{ID: "e1", From: "n2", To: "n1", Relation: ir.RelWrites}, fn, gateway)

	want := ir.Errors{{
		EdgeID:  "e1",
		Message: "writes to a gateway is not supported by the azure resolver yet",
	}}
	if diff := cmp.Diff(want, ctx.Errors); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
	if len(fn.Env) != 1 || fn.NeedsNetwork {
		t.Errorf("the function was wired anyway: env = %v, needs network = %v", fn.Env, fn.NeedsNetwork)
	}
}
