package aws

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

func setupDataAccess(t *testing.T, relations ...ir.Relation) *resolve.Context {
	t.Helper()
	edges := make([]ir.Edge, len(relations))
	for i, r := range relations {
		edges[i] = ir.Edge{ID: "e" + string(rune('0'+i)), From: "n2", To: "n3", Relation: r}
	}
	ctx, project := newContext(t, []ir.Node{
		{ID: "n2", Type: ir.NodeFunction, Name: "handler", Properties: props(t, defaultFunction)},
		{ID: "n3", Type: ir.NodeDatabase, Name: "main-db", Properties: props(t, ir.DatabaseProps{
			Engine: ir.EnginePostgres, Size: ir.SizeSmall, StorageGB: 20,
		})},
	}, edges)
	fn := resolveFunction(ctx, project.Nodes[0])
	db := resolveDatabase(ctx, project.Nodes[1])
	ctx.SetHandle("n2", fn)
	ctx.SetHandle("n3", db)
	for _, e := range project.Edges {
		resolveDataAccess(ctx, e, fn, db)
	}
	return ctx
}

func TestDataAccessOpensThePortAndInjectsConnectionDetails(t *testing.T) {
	ctx := setupDataAccess(t, ir.RelReads)

	rule := firstOfType(t, ctx, "aws_vpc_security_group_ingress_rule")
	if rule.Name != "main_db_from_handler" {
		t.Errorf("rule name = %q", rule.Name)
	}
	if rule.SourceNode != "n3" || rule.SourceLabel != "main-db" {
		t.Errorf("rule source = %q/%q", rule.SourceNode, rule.SourceLabel)
	}
	want := ir.Attrs{
		ir.A("security_group_id", ir.R(ir.ID{Type: "aws_security_group", Name: "main_db"}, ir.Field("id"))),
		ir.A("referenced_security_group_id", ir.R(fnSGID, ir.Field("id"))),
		ir.A("from_port", ir.Num(5432)),
		ir.A("to_port", ir.Num(5432)),
		ir.A("ip_protocol", ir.Str("tcp")),
		ir.A("description", ir.Str("handler to main-db")),
	}
	if diff := cmp.Diff(want, rule.Args); diff != "" {
		t.Errorf("ingress rule args (-want +got):\n%s", diff)
	}

	fn, _ := ctx.Handle("n2")
	if !fn.NeedsNetwork {
		t.Error("the function did not join the vpc")
	}
	wantEnv := ir.Attrs{
		ir.A("MAIN_DB_HOST", ir.R(dbID, ir.Field("address"))),
		ir.A("MAIN_DB_PORT", ir.Str("5432")),
		ir.A("MAIN_DB_NAME", ir.Str("main_db")),
		ir.A("MAIN_DB_SECRET_ARN", ir.R(dbID, ir.Field("master_user_secret"), ir.Index(0), ir.Field("secret_arn"))),
	}
	if diff := cmp.Diff(wantEnv, fn.Env); diff != "" {
		t.Errorf("env (-want +got):\n%s", diff)
	}
	wantStatements := []ir.Value{ir.M(
		ir.A("Effect", ir.Str("Allow")),
		ir.A("Action", ir.L(ir.Str("secretsmanager:GetSecretValue"))),
		ir.A("Resource", ir.R(dbID, ir.Field("master_user_secret"), ir.Index(0), ir.Field("secret_arn"))),
	)}
	if diff := cmp.Diff(wantStatements, fn.Statements); diff != "" {
		t.Errorf("statements (-want +got):\n%s", diff)
	}
}

func TestDataAccessDoesNotDuplicateWiringForReadsAndWrites(t *testing.T) {
	ctx := setupDataAccess(t, ir.RelReads, ir.RelWrites)
	if countOfType(ctx, "aws_vpc_security_group_ingress_rule") != 1 {
		t.Error("the ingress rule was created twice")
	}
	fn, _ := ctx.Handle("n2")
	if len(fn.Statements) != 1 {
		t.Errorf("statements = %v", fn.Statements)
	}
	if len(fn.Env) != 4 {
		t.Errorf("env = %v", fn.Env)
	}
}

func TestDataAccessReportsUnsupportedEnds(t *testing.T) {
	ctx, project := newContext(t, []ir.Node{
		{ID: "n1", Type: ir.NodeGateway, Name: "api"},
		{ID: "n3", Type: ir.NodeDatabase, Name: "main-db"},
	}, []ir.Edge{{ID: "e1", From: "n1", To: "n3", Relation: ir.RelReads}})
	gateway := resolveGateway(ctx, project.Nodes[0])
	db := resolveDatabase(ctx, project.Nodes[1])
	resolveDataAccess(ctx, project.Edges[0], gateway, db)

	want := ir.Errors{{
		EdgeID:  "e1",
		Message: "reads from a gateway is not supported by the aws resolver yet",
	}}
	if diff := cmp.Diff(want, ctx.Errors); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
}

func TestDataAccessReportsAnUnsupportedTarget(t *testing.T) {
	ctx, project := newContext(t, []ir.Node{
		{ID: "n2", Type: ir.NodeFunction, Name: "handler", Properties: props(t, defaultFunction)},
		{ID: "n1", Type: ir.NodeGateway, Name: "api"},
	}, []ir.Edge{{ID: "e1", From: "n2", To: "n1", Relation: ir.RelWrites}})
	fn := resolveFunction(ctx, project.Nodes[0])
	gateway := resolveGateway(ctx, project.Nodes[1])
	resolveDataAccess(ctx, project.Edges[0], fn, gateway)

	want := ir.Errors{{
		EdgeID:  "e1",
		Message: "writes to a gateway is not supported by the aws resolver yet",
	}}
	if diff := cmp.Diff(want, ctx.Errors); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
}

func setupBucketAccess(t *testing.T, relations ...ir.Relation) (*resolve.Context, *resolve.Handle) {
	t.Helper()
	edges := make([]ir.Edge, len(relations))
	for i, r := range relations {
		edges[i] = ir.Edge{ID: "e" + string(rune('0'+i)), From: "n2", To: "n7", Relation: r}
	}
	ctx, project := newContext(t, []ir.Node{
		{ID: "n2", Type: ir.NodeFunction, Name: "handler", Properties: props(t, defaultFunction)},
		{ID: "n7", Type: ir.NodeBucket, Name: "uploads"},
	}, edges)
	fn := resolveFunction(ctx, project.Nodes[0])
	bucket := resolveBucket(ctx, project.Nodes[1])
	for _, e := range project.Edges {
		resolveDataAccess(ctx, e, fn, bucket)
	}
	return ctx, fn
}

var (
	bucketARN     = ir.R(bucketID, ir.Field("arn"))
	bucketObjects = ir.C(bucketARN, ir.Str("/*"))
	readObjects   = ir.M(
		ir.A("Effect", ir.Str("Allow")),
		ir.A("Action", ir.L(ir.Str("s3:GetObject"))),
		ir.A("Resource", bucketObjects),
	)
	listBucket = ir.M(
		ir.A("Effect", ir.Str("Allow")),
		ir.A("Action", ir.L(ir.Str("s3:ListBucket"))),
		ir.A("Resource", bucketARN),
	)
	writeObjects = ir.M(
		ir.A("Effect", ir.Str("Allow")),
		ir.A("Action", ir.L(ir.Str("s3:PutObject"), ir.Str("s3:DeleteObject"))),
		ir.A("Resource", bucketObjects),
	)
	bucketEnv = ir.Attrs{ir.A("UPLOADS_BUCKET", ir.R(bucketID, ir.Field("id")))}
)

func TestReadsFromABucketGrantsGetAndList(t *testing.T) {
	_, fn := setupBucketAccess(t, ir.RelReads)

	if diff := cmp.Diff([]ir.Value{readObjects, listBucket}, fn.Statements); diff != "" {
		t.Errorf("statements (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(bucketEnv, fn.Env); diff != "" {
		t.Errorf("env (-want +got):\n%s", diff)
	}
}

func TestWritesToABucketGrantsPutAndDelete(t *testing.T) {
	_, fn := setupBucketAccess(t, ir.RelWrites)

	if diff := cmp.Diff([]ir.Value{writeObjects}, fn.Statements); diff != "" {
		t.Errorf("statements (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(bucketEnv, fn.Env); diff != "" {
		t.Errorf("env (-want +got):\n%s", diff)
	}
}

func TestBucketAccessKeepsTheReaderOutOfTheVPC(t *testing.T) {
	ctx, fn := setupBucketAccess(t, ir.RelReads, ir.RelWrites, ir.RelReads)

	if diff := cmp.Diff([]ir.Value{readObjects, listBucket, writeObjects}, fn.Statements); diff != "" {
		t.Errorf("statements (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(bucketEnv, fn.Env); diff != "" {
		t.Errorf("env (-want +got):\n%s", diff)
	}
	if fn.NeedsNetwork || fn.SecurityGroup != nil {
		t.Error("a bucket pulled the function into the vpc")
	}
	for _, typ := range []string{"aws_security_group", "aws_vpc", "aws_vpc_security_group_ingress_rule"} {
		if got := countOfType(ctx, typ); got != 0 {
			t.Errorf("%s = %d", typ, got)
		}
	}
}

func TestReadsFromAServiceToABucketGrantsTheTaskRole(t *testing.T) {
	ctx, project := newContext(t, []ir.Node{
		serviceNode(t, "n4", "web", defaultService),
		{ID: "n7", Type: ir.NodeBucket, Name: "uploads"},
	}, []ir.Edge{{ID: "e1", From: "n4", To: "n7", Relation: ir.RelReads}})
	svc := resolveService(ctx, project.Nodes[0])
	bucket := resolveBucket(ctx, project.Nodes[1])
	resolveDataAccess(ctx, project.Edges[0], svc, bucket)
	svc.Finalise()

	policy := named(t, ctx, ir.ID{Type: "aws_iam_role_policy", Name: "web"})
	statements, _ := policy.Args.Get("policy")
	want := ir.J(ir.M(
		ir.A("Version", ir.Str("2012-10-17")),
		ir.A("Statement", ir.L(readObjects, listBucket)),
	))
	if diff := cmp.Diff(ir.Value(want), statements); diff != "" {
		t.Errorf("policy statements (-want +got):\n%s", diff)
	}
	if countOfType(ctx, "aws_vpc_security_group_ingress_rule") != 0 {
		t.Error("a bucket was given an ingress rule")
	}
}

func TestDataAccessFromAServiceWiresTheTaskRoleAndSecurityGroup(t *testing.T) {
	ctx, project := newContext(t, []ir.Node{
		serviceNode(t, "n4", "web", defaultService),
		{ID: "n3", Type: ir.NodeDatabase, Name: "main-db", Properties: props(t, ir.DatabaseProps{
			Engine: ir.EnginePostgres, Size: ir.SizeSmall, StorageGB: 20,
		})},
	}, []ir.Edge{{ID: "e1", From: "n4", To: "n3", Relation: ir.RelReads}})
	svc := resolveService(ctx, project.Nodes[0])
	db := resolveDatabase(ctx, project.Nodes[1])
	resolveDataAccess(ctx, project.Edges[0], svc, db)
	svc.Finalise()

	rule := named(t, ctx, ir.ID{Type: "aws_vpc_security_group_ingress_rule", Name: "main_db_from_web"})
	want := ir.Attrs{
		ir.A("security_group_id", ir.R(ir.ID{Type: "aws_security_group", Name: "main_db"}, ir.Field("id"))),
		ir.A("referenced_security_group_id", ir.R(svcSGID, ir.Field("id"))),
		ir.A("from_port", ir.Num(5432)),
		ir.A("to_port", ir.Num(5432)),
		ir.A("ip_protocol", ir.Str("tcp")),
		ir.A("description", ir.Str("web to main-db")),
	}
	if diff := cmp.Diff(want, rule.Args); diff != "" {
		t.Errorf("ingress rule args (-want +got):\n%s", diff)
	}

	definitions, _ := named(t, ctx, svcTaskDef).Args.Get("container_definitions")
	container := ir.Attrs(definitions.(ir.JSON).Value.(ir.List)[0].(ir.Map))
	env, _ := container.Get("environment")
	secret := ir.R(dbID, ir.Field("master_user_secret"), ir.Index(0), ir.Field("secret_arn"))
	wantEnv := ir.L(
		ir.M(ir.A("name", ir.Str("MAIN_DB_HOST")), ir.A("value", ir.R(dbID, ir.Field("address")))),
		ir.M(ir.A("name", ir.Str("MAIN_DB_NAME")), ir.A("value", ir.Str("main_db"))),
		ir.M(ir.A("name", ir.Str("MAIN_DB_PORT")), ir.A("value", ir.Str("5432"))),
		ir.M(ir.A("name", ir.Str("MAIN_DB_SECRET_ARN")), ir.A("value", secret)),
	)
	if diff := cmp.Diff(ir.Value(wantEnv), env); diff != "" {
		t.Errorf("container environment (-want +got):\n%s", diff)
	}

	policy := firstOfType(t, ctx, "aws_iam_role_policy")
	role, _ := policy.Args.Get("role")
	if diff := cmp.Diff(ir.Value(ir.R(svcRoleID, ir.Field("id"))), role); diff != "" {
		t.Errorf("policy role (-want +got):\n%s", diff)
	}
	statements, _ := policy.Args.Get("policy")
	wantStatements := ir.J(ir.M(
		ir.A("Version", ir.Str("2012-10-17")),
		ir.A("Statement", ir.L(ir.M(
			ir.A("Effect", ir.Str("Allow")),
			ir.A("Action", ir.L(ir.Str("secretsmanager:GetSecretValue"))),
			ir.A("Resource", secret),
		))),
	))
	if diff := cmp.Diff(ir.Value(wantStatements), statements); diff != "" {
		t.Errorf("policy statements (-want +got):\n%s", diff)
	}
}

func setupCacheAccess(t *testing.T, relations ...ir.Relation) (*resolve.Context, *resolve.Handle) {
	t.Helper()
	edges := make([]ir.Edge, len(relations))
	for i, r := range relations {
		edges[i] = ir.Edge{ID: "e" + string(rune('0'+i)), From: "n2", To: "n8", Relation: r}
	}
	ctx, project := newContext(t, []ir.Node{
		{ID: "n2", Type: ir.NodeFunction, Name: "handler", Properties: props(t, defaultFunction)},
		{ID: "n8", Type: ir.NodeCache, Name: "sessions", Properties: props(t, ir.CacheProps{Size: ir.SizeSmall})},
	}, edges)
	fn := resolveFunction(ctx, project.Nodes[0])
	cache := resolveCache(ctx, project.Nodes[1])
	for _, e := range project.Edges {
		resolveDataAccess(ctx, e, fn, cache)
	}
	return ctx, fn
}

var cacheEnv = ir.Attrs{
	ir.A("SESSIONS_HOST", cacheHost),
	ir.A("SESSIONS_PORT", ir.Str("6379")),
}

func TestReadsFromACacheOpensRedisAndInjectsTheEndpoint(t *testing.T) {
	ctx, fn := setupCacheAccess(t, ir.RelReads)

	rule := named(t, ctx, ir.ID{Type: "aws_vpc_security_group_ingress_rule", Name: "sessions_from_handler"})
	if rule.SourceNode != "n8" || rule.SourceLabel != "sessions" {
		t.Errorf("rule source = %q/%q", rule.SourceNode, rule.SourceLabel)
	}
	want := ir.Attrs{
		ir.A("security_group_id", ir.R(cacheSGID, ir.Field("id"))),
		ir.A("referenced_security_group_id", ir.R(fnSGID, ir.Field("id"))),
		ir.A("from_port", ir.Num(6379)),
		ir.A("to_port", ir.Num(6379)),
		ir.A("ip_protocol", ir.Str("tcp")),
		ir.A("description", ir.Str("handler to sessions")),
	}
	if diff := cmp.Diff(want, rule.Args); diff != "" {
		t.Errorf("ingress rule args (-want +got):\n%s", diff)
	}

	if diff := cmp.Diff(cacheEnv, fn.Env); diff != "" {
		t.Errorf("env (-want +got):\n%s", diff)
	}
	if !fn.NeedsNetwork {
		t.Error("the function did not join the vpc")
	}
	if len(fn.Statements) != 0 {
		t.Errorf("statements = %v", fn.Statements)
	}
}

func TestCacheAccessDoesNotDuplicateWiringForReadsAndWrites(t *testing.T) {
	ctx, fn := setupCacheAccess(t, ir.RelReads, ir.RelWrites)

	if countOfType(ctx, "aws_vpc_security_group_ingress_rule") != 1 {
		t.Error("the ingress rule was created twice")
	}
	if diff := cmp.Diff(cacheEnv, fn.Env); diff != "" {
		t.Errorf("env (-want +got):\n%s", diff)
	}
}

func TestReadsFromAServiceToACacheUsesTheServiceSecurityGroup(t *testing.T) {
	ctx, project := newContext(t, []ir.Node{
		serviceNode(t, "n4", "web", defaultService),
		{ID: "n8", Type: ir.NodeCache, Name: "sessions", Properties: props(t, ir.CacheProps{Size: ir.SizeSmall})},
	}, []ir.Edge{{ID: "e1", From: "n4", To: "n8", Relation: ir.RelReads}})
	svc := resolveService(ctx, project.Nodes[0])
	cache := resolveCache(ctx, project.Nodes[1])
	resolveDataAccess(ctx, project.Edges[0], svc, cache)
	svc.Finalise()

	rule := named(t, ctx, ir.ID{Type: "aws_vpc_security_group_ingress_rule", Name: "sessions_from_web"})
	referenced, _ := rule.Args.Get("referenced_security_group_id")
	if diff := cmp.Diff(ir.Value(ir.R(svcSGID, ir.Field("id"))), referenced); diff != "" {
		t.Errorf("referenced security group (-want +got):\n%s", diff)
	}

	definitions, _ := named(t, ctx, svcTaskDef).Args.Get("container_definitions")
	container := ir.Attrs(definitions.(ir.JSON).Value.(ir.List)[0].(ir.Map))
	env, _ := container.Get("environment")
	wantEnv := ir.L(
		ir.M(ir.A("name", ir.Str("SESSIONS_HOST")), ir.A("value", cacheHost)),
		ir.M(ir.A("name", ir.Str("SESSIONS_PORT")), ir.A("value", ir.Str("6379"))),
	)
	if diff := cmp.Diff(ir.Value(wantEnv), env); diff != "" {
		t.Errorf("container environment (-want +got):\n%s", diff)
	}
	if countOfType(ctx, "aws_iam_role_policy") != 0 {
		t.Error("a cache produced an iam policy")
	}
}
