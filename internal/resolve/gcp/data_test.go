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

func TestAServiceReadingADatabaseGetsTheConnectionAndTheConnector(t *testing.T) {
	ctx, project := newContext(t, []ir.Node{
		serviceNode(t, "n4", "web", defaultService),
		databaseNode(t, "n3", "main-db", defaultDatabase),
	}, []ir.Edge{{ID: "e1", From: "n4", To: "n3", Relation: ir.RelReads}})
	svc := resolveService(ctx, project.Nodes[0])
	db := resolveDatabase(ctx, project.Nodes[1])
	resolveDataAccess(ctx, project.Edges[0], svc, db)
	svc.Finalise()

	if diff := cmp.Diff(databaseEnv, svc.Env); diff != "" {
		t.Errorf("env (-want +got):\n%s", diff)
	}
	if !svc.NeedsNetwork {
		t.Error("the service did not join the network")
	}
	access, _ := template(t, ctx).Get("vpc_access")
	want := ir.B(ir.Attrs{
		ir.A("connector", ir.R(connectorID, ir.Field("id"))),
		ir.A("egress", ir.Str("PRIVATE_RANGES_ONLY")),
	})
	if diff := cmp.Diff(ir.Value(want), access); diff != "" {
		t.Errorf("vpc_access (-want +got):\n%s", diff)
	}
	if len(ctx.Errors) != 0 {
		t.Errorf("errors = %v", ctx.Errors)
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

func setupFunctionBucketAccess(t *testing.T, relations ...ir.Relation) (*resolve.Context, *resolve.Handle) {
	t.Helper()
	edges := make([]ir.Edge, len(relations))
	for i, r := range relations {
		edges[i] = ir.Edge{ID: "e" + string(rune('1'+i)), From: "n2", To: "n7", Relation: r}
	}
	ctx, project := newContext(t, []ir.Node{
		functionNode(t, "n2", "handler", defaultFunction),
		bucketNode("n7", "uploads", "{}"),
	}, edges)
	fn := resolveFunction(ctx, project.Nodes[0])
	bucket := resolveBucket(ctx, project.Nodes[1])
	for _, e := range project.Edges {
		resolveDataAccess(ctx, e, fn, bucket)
	}
	fn.Finalise()
	return ctx, fn
}

func setupServiceBucketAccess(t *testing.T, relations ...ir.Relation) (*resolve.Context, *resolve.Handle) {
	t.Helper()
	edges := make([]ir.Edge, len(relations))
	for i, r := range relations {
		edges[i] = ir.Edge{ID: "e" + string(rune('1'+i)), From: "n4", To: "n7", Relation: r}
	}
	ctx, project := newContext(t, []ir.Node{
		serviceNode(t, "n4", "web", defaultService),
		bucketNode("n7", "uploads", "{}"),
	}, edges)
	svc := resolveService(ctx, project.Nodes[0])
	bucket := resolveBucket(ctx, project.Nodes[1])
	for _, e := range project.Edges {
		resolveDataAccess(ctx, e, svc, bucket)
	}
	svc.Finalise()
	return ctx, svc
}

func bucketGrant(role string, account ir.ID) ir.Attrs {
	return ir.Attrs{
		ir.A("bucket", bucketRef),
		ir.A("role", ir.Str(role)),
		ir.A("member", ir.C(ir.Str("serviceAccount:"), ir.R(account, ir.Field("email")))),
	}
}

var bucketEnv = ir.Attrs{ir.A("UPLOADS_BUCKET", bucketName)}

func TestReadsFromABucketGrantsObjectViewerAndInjectsTheName(t *testing.T) {
	ctx, fn := setupFunctionBucketAccess(t, ir.RelReads)

	grant := named(t, ctx, ir.ID{Type: "google_storage_bucket_iam_member", Name: "uploads_reads_from_handler"})
	if grant.SourceNode != "n7" || grant.SourceLabel != "uploads" {
		t.Errorf("grant source = %q/%q", grant.SourceNode, grant.SourceLabel)
	}
	if diff := cmp.Diff(bucketGrant("roles/storage.objectViewer", fnAccountID), grant.Args); diff != "" {
		t.Errorf("grant args (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(bucketEnv, fn.Env); diff != "" {
		t.Errorf("env (-want +got):\n%s", diff)
	}
	env, _ := serviceConfig(t, ctx).Get("environment_variables")
	if diff := cmp.Diff(ir.Value(ir.Map(bucketEnv)), env); diff != "" {
		t.Errorf("environment_variables (-want +got):\n%s", diff)
	}
	if fn.NeedsNetwork || countOfType(ctx, "google_compute_network") != 0 {
		t.Error("a bucket reader was put on the network")
	}
	if len(ctx.Errors) != 0 {
		t.Errorf("errors = %v", ctx.Errors)
	}
}

func TestWritesToABucketGrantsObjectUser(t *testing.T) {
	ctx, fn := setupFunctionBucketAccess(t, ir.RelWrites)

	grant := named(t, ctx, ir.ID{Type: "google_storage_bucket_iam_member", Name: "uploads_writes_from_handler"})
	if diff := cmp.Diff(bucketGrant("roles/storage.objectUser", fnAccountID), grant.Args); diff != "" {
		t.Errorf("grant args (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(bucketEnv, fn.Env); diff != "" {
		t.Errorf("env (-want +got):\n%s", diff)
	}
	if got := countOfType(ctx, "google_storage_bucket_iam_member"); got != 1 {
		t.Errorf("grants = %d", got)
	}
}

func TestReadsAndWritesToOneBucketGrantBothRolesAndSetTheNameOnce(t *testing.T) {
	ctx, fn := setupFunctionBucketAccess(t, ir.RelReads, ir.RelWrites)

	wantTypes := []string{
		"google_storage_bucket",
		"google_service_account",
		"google_storage_bucket_object",
		"google_cloudfunctions2_function",
		"google_storage_bucket",
		"google_storage_bucket_iam_member",
		"google_storage_bucket_iam_member",
	}
	if diff := cmp.Diff(wantTypes, resourceTypes(ctx)); diff != "" {
		t.Errorf("resources (-want +got):\n%s", diff)
	}
	named(t, ctx, ir.ID{Type: "google_storage_bucket_iam_member", Name: "uploads_reads_from_handler"})
	named(t, ctx, ir.ID{Type: "google_storage_bucket_iam_member", Name: "uploads_writes_from_handler"})
	if diff := cmp.Diff(bucketEnv, fn.Env); diff != "" {
		t.Errorf("env (-want +got):\n%s", diff)
	}
}

func TestAServiceReadingAndWritingABucketIsGrantedOnItsOwnAccount(t *testing.T) {
	ctx, svc := setupServiceBucketAccess(t, ir.RelReads, ir.RelWrites)

	reads := named(t, ctx, ir.ID{Type: "google_storage_bucket_iam_member", Name: "uploads_reads_from_web"})
	if diff := cmp.Diff(bucketGrant("roles/storage.objectViewer", svcAccountID), reads.Args); diff != "" {
		t.Errorf("reads grant (-want +got):\n%s", diff)
	}
	writes := named(t, ctx, ir.ID{Type: "google_storage_bucket_iam_member", Name: "uploads_writes_from_web"})
	if diff := cmp.Diff(bucketGrant("roles/storage.objectUser", svcAccountID), writes.Args); diff != "" {
		t.Errorf("writes grant (-want +got):\n%s", diff)
	}
	env, _ := container(t, ctx).Get("env")
	want := ir.B(ir.Attrs{ir.A("name", ir.Str("UPLOADS_BUCKET")), ir.A("value", bucketName)})
	if diff := cmp.Diff(ir.Value(want), env); diff != "" {
		t.Errorf("container env (-want +got):\n%s", diff)
	}
	if svc.NeedsNetwork {
		t.Error("a bucket writer was put on the network")
	}
	if len(ctx.Errors) != 0 {
		t.Errorf("errors = %v", ctx.Errors)
	}
}

func setupFunctionCacheAccess(t *testing.T, relations ...ir.Relation) (*resolve.Context, *resolve.Handle) {
	t.Helper()
	edges := make([]ir.Edge, len(relations))
	for i, r := range relations {
		edges[i] = ir.Edge{ID: "e" + string(rune('1'+i)), From: "n2", To: "n8", Relation: r}
	}
	ctx, project := newContext(t, []ir.Node{
		functionNode(t, "n2", "handler", defaultFunction),
		cacheNode(t, "n8", "sessions", ir.CacheProps{Size: ir.SizeSmall}),
	}, edges)
	fn := resolveFunction(ctx, project.Nodes[0])
	cache := resolveCache(ctx, project.Nodes[1])
	for _, e := range project.Edges {
		resolveDataAccess(ctx, e, fn, cache)
	}
	fn.Finalise()
	return ctx, fn
}

var cacheEnv = ir.Attrs{
	ir.A("SESSIONS_HOST", cacheHost),
	ir.A("SESSIONS_PORT", ir.Str("6379")),
}

func TestReadsFromACacheInjectsTheEndpointAndJoinsTheConnector(t *testing.T) {
	ctx, fn := setupFunctionCacheAccess(t, ir.RelReads)

	if diff := cmp.Diff(cacheEnv, fn.Env); diff != "" {
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
	if diff := cmp.Diff(ir.Value(ir.Map(cacheEnv)), env); diff != "" {
		t.Errorf("environment_variables (-want +got):\n%s", diff)
	}
	if len(ctx.Errors) != 0 {
		t.Errorf("errors = %v", ctx.Errors)
	}

	// Nothing to open and nobody to grant to: the edge adds no resource.
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
		"google_redis_instance",
	}
	if diff := cmp.Diff(wantTypes, resourceTypes(ctx)); diff != "" {
		t.Errorf("resources (-want +got):\n%s", diff)
	}
}

func TestWritesToACacheWiresTheSameAsReads(t *testing.T) {
	_, reads := setupFunctionCacheAccess(t, ir.RelReads)
	_, writes := setupFunctionCacheAccess(t, ir.RelWrites)
	if diff := cmp.Diff(reads.Env, writes.Env); diff != "" {
		t.Errorf("env (-reads +writes):\n%s", diff)
	}
	if !writes.NeedsNetwork {
		t.Error("the writer did not join the network")
	}
}

func TestCacheAccessDoesNotDuplicateWiringForReadsAndWrites(t *testing.T) {
	ctx, fn := setupFunctionCacheAccess(t, ir.RelReads, ir.RelWrites)
	if diff := cmp.Diff(cacheEnv, fn.Env); diff != "" {
		t.Errorf("env (-want +got):\n%s", diff)
	}
	if got := countOfType(ctx, "google_redis_instance"); got != 1 {
		t.Errorf("instances = %d", got)
	}
}

func TestAServiceReadingAndWritingACacheGetsTheEndpointAndTheConnector(t *testing.T) {
	ctx, project := newContext(t, []ir.Node{
		serviceNode(t, "n4", "web", defaultService),
		cacheNode(t, "n8", "sessions", ir.CacheProps{Size: ir.SizeSmall}),
	}, []ir.Edge{
		{ID: "e1", From: "n4", To: "n8", Relation: ir.RelReads},
		{ID: "e2", From: "n4", To: "n8", Relation: ir.RelWrites},
	})
	svc := resolveService(ctx, project.Nodes[0])
	cache := resolveCache(ctx, project.Nodes[1])
	for _, e := range project.Edges {
		resolveDataAccess(ctx, e, svc, cache)
	}
	svc.Finalise()

	if diff := cmp.Diff(cacheEnv, svc.Env); diff != "" {
		t.Errorf("env (-want +got):\n%s", diff)
	}
	if !svc.NeedsNetwork {
		t.Error("the service did not join the network")
	}
	access, _ := template(t, ctx).Get("vpc_access")
	want := ir.B(ir.Attrs{
		ir.A("connector", ir.R(connectorID, ir.Field("id"))),
		ir.A("egress", ir.Str("PRIVATE_RANGES_ONLY")),
	})
	if diff := cmp.Diff(ir.Value(want), access); diff != "" {
		t.Errorf("vpc_access (-want +got):\n%s", diff)
	}
	env, _ := container(t, ctx).Get("env")
	wantEnv := ir.B(
		ir.Attrs{ir.A("name", ir.Str("SESSIONS_HOST")), ir.A("value", cacheHost)},
		ir.Attrs{ir.A("name", ir.Str("SESSIONS_PORT")), ir.A("value", ir.Str("6379"))},
	)
	if diff := cmp.Diff(ir.Value(wantEnv), env); diff != "" {
		t.Errorf("container env (-want +got):\n%s", diff)
	}
	if len(ctx.Errors) != 0 {
		t.Errorf("errors = %v", ctx.Errors)
	}
}
