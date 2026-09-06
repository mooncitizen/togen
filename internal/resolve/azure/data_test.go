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

func TestDataAccessFromAServiceSetsTheConnectionAsContainerEnv(t *testing.T) {
	ctx := newContext(t, []ir.Node{serviceNodeWith(t, "n4", "web", defaultService), databaseNode(t, "n3", "main-db", smallPostgres)})
	svc := resolveService(ctx, ctx.Project.Nodes[0])
	db := resolveDatabase(ctx, ctx.Project.Nodes[1])
	resolveDataAccess(ctx, ir.Edge{ID: "e1", From: "n4", To: "n3", Relation: ir.RelWrites}, svc, db)
	svc.Finalise()

	if len(ctx.Errors) != 0 {
		t.Fatalf("errors = %v", ctx.Errors)
	}
	if diff := cmp.Diff(connectionSettings, containerEnvOf(t, ctx, appID)); diff != "" {
		t.Errorf("container env (-want +got):\n%s", diff)
	}
	if !svc.NeedsNetwork {
		t.Error("the service was not asked to join the network")
	}
	if got := countOfType(ctx, "azurerm_virtual_network"); got != 1 {
		t.Errorf("virtual networks = %d", got)
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

func setupBucketAccess(t *testing.T, relations ...ir.Relation) (*resolve.Context, *resolve.Handle) {
	t.Helper()
	ctx := newContext(t, []ir.Node{
		functionNode(t, "n2", "handler", defaultFunction),
		bucketNode("n7", "uploads", "{}"),
	})
	fn := resolveFunction(ctx, ctx.Project.Nodes[0])
	bucket := resolveBucket(ctx, ctx.Project.Nodes[1])
	for i, r := range relations {
		edge := ir.Edge{ID: "e" + string(rune('1'+i)), From: "n2", To: "n7", Relation: r}
		resolveDataAccess(ctx, edge, fn, bucket)
	}
	fn.Finalise()
	return ctx, fn
}

var (
	handlerPrincipal = ir.R(fnID, ir.Field("identity"), ir.Index(0), ir.Field("principal_id"))
	bucketEnv        = ir.Attrs{
		ir.A("UPLOADS_ACCOUNT", ir.R(accountID, ir.Field("name"))),
		ir.A("UPLOADS_CONTAINER", ir.R(containerID, ir.Field("name"))),
	}
)

func blobAssignment(role string, principal ir.Value) ir.Attrs {
	return ir.Attrs{
		ir.A("scope", ir.R(accountID, ir.Field("id"))),
		ir.A("role_definition_name", ir.Str(role)),
		ir.A("principal_id", principal),
		ir.A("principal_type", ir.Str("ServicePrincipal")),
	}
}

func TestReadsFromABucketGrantsBlobReaderOnTheAccount(t *testing.T) {
	ctx, fn := setupBucketAccess(t, ir.RelReads)
	if len(ctx.Errors) != 0 {
		t.Fatalf("errors = %v", ctx.Errors)
	}

	assignment := firstOfType(t, ctx, "azurerm_role_assignment")
	if assignment.Name != "handler_uploads_read" || assignment.SourceNode != "n2" || assignment.SourceLabel != "handler" {
		t.Errorf("assignment = %q %q %q", assignment.Name, assignment.SourceNode, assignment.SourceLabel)
	}
	if diff := cmp.Diff(blobAssignment("Storage Blob Data Reader", handlerPrincipal), assignment.Args); diff != "" {
		t.Errorf("assignment args (-want +got):\n%s", diff)
	}
	settings, _ := firstOfType(t, ctx, "azurerm_linux_function_app").Args.Get("app_settings")
	timeout := ir.A("AzureFunctionsJobHost__functionTimeout", ir.Str("00:00:30"))
	want := ir.M(append(ir.Attrs{timeout}, bucketEnv...)...)
	if diff := cmp.Diff(ir.Value(want), settings); diff != "" {
		t.Errorf("app_settings (-want +got):\n%s", diff)
	}
	if fn.NeedsNetwork {
		t.Error("a reader asked for the network")
	}
	if got := countOfType(ctx, "azurerm_virtual_network"); got != 0 {
		t.Error("a bucket edge pulled in the network")
	}
}

func TestWritesToABucketGrantsBlobContributorOnTheAccount(t *testing.T) {
	ctx, fn := setupBucketAccess(t, ir.RelWrites)
	if len(ctx.Errors) != 0 {
		t.Fatalf("errors = %v", ctx.Errors)
	}

	assignment := firstOfType(t, ctx, "azurerm_role_assignment")
	if assignment.Name != "handler_uploads_write" {
		t.Errorf("assignment name = %q", assignment.Name)
	}
	if diff := cmp.Diff(blobAssignment("Storage Blob Data Contributor", handlerPrincipal), assignment.Args); diff != "" {
		t.Errorf("assignment args (-want +got):\n%s", diff)
	}
	// The timeout, the account and the container.
	if len(fn.Env) != 3 {
		t.Errorf("env = %v", fn.Env)
	}
}

func TestReadsAndWritesToOneBucketGrantBothRolesAndSetTheNamesOnce(t *testing.T) {
	ctx, fn := setupBucketAccess(t, ir.RelReads, ir.RelWrites, ir.RelReads)

	var names []string
	for _, r := range byType(ctx, "azurerm_role_assignment") {
		names = append(names, r.Name)
	}
	if diff := cmp.Diff([]string{"handler_uploads_read", "handler_uploads_write"}, names); diff != "" {
		t.Errorf("role assignments (-want +got):\n%s", diff)
	}
	if len(fn.Env) != 3 {
		t.Errorf("env = %v", fn.Env)
	}
}

func TestBucketAccessFromAServiceGrantsTheRoleAndSetsContainerEnv(t *testing.T) {
	for _, tc := range []struct {
		relation ir.Relation
		name     string
		role     string
	}{
		{ir.RelReads, "web_uploads_read", "Storage Blob Data Reader"},
		{ir.RelWrites, "web_uploads_write", "Storage Blob Data Contributor"},
	} {
		ctx := newContext(t, []ir.Node{serviceNodeWith(t, "n4", "web", defaultService), bucketNode("n7", "uploads", "{}")})
		svc := resolveService(ctx, ctx.Project.Nodes[0])
		bucket := resolveBucket(ctx, ctx.Project.Nodes[1])
		resolveDataAccess(ctx, ir.Edge{ID: "e1", From: "n4", To: "n7", Relation: tc.relation}, svc, bucket)
		svc.Finalise()

		if len(ctx.Errors) != 0 {
			t.Fatalf("%s: errors = %v", tc.relation, ctx.Errors)
		}
		assignment := firstOfType(t, ctx, "azurerm_role_assignment")
		if assignment.Name != tc.name {
			t.Errorf("%s: assignment name = %q", tc.relation, assignment.Name)
		}
		if diff := cmp.Diff(blobAssignment(tc.role, webPrincipal), assignment.Args); diff != "" {
			t.Errorf("%s: assignment args (-want +got):\n%s", tc.relation, diff)
		}
		if diff := cmp.Diff(bucketEnv, containerEnvOf(t, ctx, appID)); diff != "" {
			t.Errorf("%s: container env (-want +got):\n%s", tc.relation, diff)
		}
		if svc.NeedsNetwork {
			t.Errorf("%s: the service was asked to join the network", tc.relation)
		}
	}
}

func setupCacheAccess(t *testing.T, relations ...ir.Relation) (*resolve.Context, *resolve.Handle) {
	t.Helper()
	ctx := newContext(t, []ir.Node{
		functionNode(t, "n2", "handler", defaultFunction),
		cacheNode(t, "n8", "sessions", ir.CacheProps{Size: ir.SizeSmall}),
	})
	fn := resolveFunction(ctx, ctx.Project.Nodes[0])
	cache := resolveCache(ctx, ctx.Project.Nodes[1])
	for i, r := range relations {
		edge := ir.Edge{ID: "e" + string(rune('1'+i)), From: "n2", To: "n8", Relation: r}
		resolveDataAccess(ctx, edge, fn, cache)
	}
	fn.Finalise()
	return ctx, fn
}

var cacheSettings = ir.Attrs{
	ir.A("SESSIONS_HOST", cacheHost),
	ir.A("SESSIONS_PORT", cachePort),
	ir.A("SESSIONS_PASSWORD", cacheKey),
}

func TestReadsFromACacheSetTheHostPortAndKeyAsAppSettings(t *testing.T) {
	for _, relation := range []ir.Relation{ir.RelReads, ir.RelWrites} {
		ctx, fn := setupCacheAccess(t, relation)
		if len(ctx.Errors) != 0 {
			t.Fatalf("%s: errors = %v", relation, ctx.Errors)
		}
		settings, _ := firstOfType(t, ctx, "azurerm_linux_function_app").Args.Get("app_settings")
		timeout := ir.A("AzureFunctionsJobHost__functionTimeout", ir.Str("00:00:30"))
		want := ir.M(append(ir.Attrs{timeout}, cacheSettings...)...)
		if diff := cmp.Diff(ir.Value(want), settings); diff != "" {
			t.Errorf("%s: app_settings (-want +got):\n%s", relation, diff)
		}
		if fn.NeedsNetwork {
			t.Errorf("%s: a cache edge asked for the network", relation)
		}
		// The group, the function's three and the cache.
		if got := len(ctx.Resources()); got != 5 {
			t.Errorf("%s: the edge added resources: %d in total", relation, got)
		}
	}
}

func TestReadsAndWritesToOneCacheSetTheSettingsOnce(t *testing.T) {
	ctx, fn := setupCacheAccess(t, ir.RelReads, ir.RelWrites)
	if len(ctx.Errors) != 0 {
		t.Fatalf("errors = %v", ctx.Errors)
	}
	if len(fn.Env) != 4 {
		t.Errorf("env = %v", fn.Env)
	}
}

func TestCacheAccessFromAServiceSetsTheSettingsAsContainerEnv(t *testing.T) {
	for _, relation := range []ir.Relation{ir.RelReads, ir.RelWrites} {
		ctx := newContext(t, []ir.Node{
			serviceNodeWith(t, "n4", "web", defaultService),
			cacheNode(t, "n8", "sessions", ir.CacheProps{Size: ir.SizeMedium}),
		})
		svc := resolveService(ctx, ctx.Project.Nodes[0])
		cache := resolveCache(ctx, ctx.Project.Nodes[1])
		resolveDataAccess(ctx, ir.Edge{ID: "e1", From: "n4", To: "n8", Relation: relation}, svc, cache)
		svc.Finalise()

		if len(ctx.Errors) != 0 {
			t.Fatalf("%s: errors = %v", relation, ctx.Errors)
		}
		if diff := cmp.Diff(cacheSettings, containerEnvOf(t, ctx, appID)); diff != "" {
			t.Errorf("%s: container env (-want +got):\n%s", relation, diff)
		}
		if svc.NeedsNetwork {
			t.Errorf("%s: the service was asked to join the network", relation)
		}
	}
}
