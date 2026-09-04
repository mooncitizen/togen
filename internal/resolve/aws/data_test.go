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
