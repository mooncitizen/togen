package aws

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

func setupFunction(t *testing.T, p ir.FunctionProps) (*resolve.Context, *resolve.Handle) {
	t.Helper()
	ctx, project := newContext(t, []ir.Node{{
		ID:         "n2",
		Type:       ir.NodeFunction,
		Name:       "handler",
		Properties: props(t, p),
	}}, nil)
	return ctx, resolveFunction(ctx, project.Nodes[0])
}

var defaultFunction = ir.FunctionProps{
	Runtime:        ir.RuntimeNode,
	Handler:        "index.handler",
	Size:           ir.SizeSmall,
	TimeoutSeconds: 30,
}

var (
	fnID       = ir.ID{Type: "aws_lambda_function", Name: "handler"}
	fnRoleID   = ir.ID{Type: "aws_iam_role", Name: "handler"}
	fnSGID     = ir.ID{Type: "aws_security_group", Name: "handler"}
	privateSub = ir.L(
		ir.R(ir.ID{Type: "aws_subnet", Name: "private_a"}, ir.Field("id")),
		ir.R(ir.ID{Type: "aws_subnet", Name: "private_b"}, ir.Field("id")),
	)
)

func TestFunctionEmitsALambdaWithRoleLogsAndAPackageVariable(t *testing.T) {
	p := defaultFunction
	p.Env = map[string]string{"LOG_LEVEL": "info", "APP_NAME": "shop"}
	ctx, handle := setupFunction(t, p)
	handle.Finalise()

	fn := firstOfType(t, ctx, "aws_lambda_function")
	want := ir.Attrs{
		ir.A("function_name", ir.Str("shop-dev-handler")),
		ir.A("role", ir.R(fnRoleID, ir.Field("arn"))),
		ir.A("runtime", ir.Str("nodejs22.x")),
		ir.A("handler", ir.Str("index.handler")),
		ir.A("filename", ir.V("handler_package")),
		ir.A("source_code_hash", ir.Fn("filebase64sha256", ir.V("handler_package"))),
		ir.A("memory_size", ir.Num(512)),
		ir.A("timeout", ir.Num(30)),
		ir.A("environment", ir.B(ir.Attrs{ir.A("variables", ir.M(
			ir.A("APP_NAME", ir.Str("shop")),
			ir.A("LOG_LEVEL", ir.Str("info")),
		))})),
	}
	if diff := cmp.Diff(want, fn.Args); diff != "" {
		t.Errorf("lambda args (-want +got):\n%s", diff)
	}
	wantDeps := []ir.ID{
		{Type: "aws_cloudwatch_log_group", Name: "handler"},
		{Type: "aws_iam_role_policy_attachment", Name: "handler_logs"},
	}
	if diff := cmp.Diff(wantDeps, fn.DependsOn); diff != "" {
		t.Errorf("depends_on (-want +got):\n%s", diff)
	}

	logGroup := firstOfType(t, ctx, "aws_cloudwatch_log_group")
	wantLogs := ir.Attrs{
		ir.A("name", ir.Str("/aws/lambda/shop-dev-handler")),
		ir.A("retention_in_days", ir.Num(30)),
	}
	if diff := cmp.Diff(wantLogs, logGroup.Args); diff != "" {
		t.Errorf("log group args (-want +got):\n%s", diff)
	}

	role := firstOfType(t, ctx, "aws_iam_role")
	wantRole := ir.Attrs{
		ir.A("name", ir.Str("shop-dev-handler")),
		ir.A("assume_role_policy", ir.J(ir.M(
			ir.A("Version", ir.Str("2012-10-17")),
			ir.A("Statement", ir.L(ir.M(
				ir.A("Effect", ir.Str("Allow")),
				ir.A("Principal", ir.M(ir.A("Service", ir.Str("lambda.amazonaws.com")))),
				ir.A("Action", ir.Str("sts:AssumeRole")),
			))),
		))),
	}
	if diff := cmp.Diff(wantRole, role.Args); diff != "" {
		t.Errorf("role args (-want +got):\n%s", diff)
	}

	wantVars := []ir.Variable{{
		Name:        "handler_package",
		Description: "Path to the deployment package for the handler function",
		Type:        "string",
		Default:     ir.Str("functions/handler.zip"),
	}}
	if diff := cmp.Diff(wantVars, ctx.Variables); diff != "" {
		t.Errorf("variables (-want +got):\n%s", diff)
	}
	if countOfType(ctx, "aws_vpc") != 0 {
		t.Error("a network was created without an edge asking for one")
	}
	if countOfType(ctx, "aws_iam_role_policy") != 0 {
		t.Error("a role policy was created without statements")
	}

	wantExports := resolve.FunctionExports{
		ARN:          ir.R(fnID, ir.Field("arn")),
		InvokeARN:    ir.R(fnID, ir.Field("invoke_arn")),
		FunctionName: ir.R(fnID, ir.Field("function_name")),
	}
	if diff := cmp.Diff(wantExports, handle.Exports); diff != "" {
		t.Errorf("exports (-want +got):\n%s", diff)
	}
}

func TestFunctionOmitsTheEnvironmentBlockWhenThereAreNoVariables(t *testing.T) {
	ctx, handle := setupFunction(t, defaultFunction)
	handle.Finalise()
	if _, ok := firstOfType(t, ctx, "aws_lambda_function").Args.Get("environment"); ok {
		t.Error("environment block present with no variables")
	}
}

func TestFunctionMapsRuntimesAndSizes(t *testing.T) {
	ctx, handle := setupFunction(t, ir.FunctionProps{
		Runtime:        ir.RuntimeGo,
		Handler:        "index.handler",
		Size:           ir.SizeLarge,
		TimeoutSeconds: 120,
	})
	handle.Finalise()
	fn := firstOfType(t, ctx, "aws_lambda_function")
	for _, c := range []struct {
		key  string
		want ir.Value
	}{
		{"runtime", ir.Str("provided.al2023")},
		{"handler", ir.Str("bootstrap")},
		{"memory_size", ir.Num(2048)},
		{"timeout", ir.Num(120)},
	} {
		got, _ := fn.Args.Get(c.key)
		if diff := cmp.Diff(c.want, got); diff != "" {
			t.Errorf("%s (-want +got):\n%s", c.key, diff)
		}
	}
}

func TestFunctionWritesStatementsIntoAnInlineRolePolicy(t *testing.T) {
	ctx, handle := setupFunction(t, defaultFunction)
	statement := ir.M(
		ir.A("Effect", ir.Str("Allow")),
		ir.A("Action", ir.L(ir.Str("s3:GetObject"))),
		ir.A("Resource", ir.Str("arn:x")),
	)
	handle.AddStatement(statement)
	handle.Finalise()

	policy := firstOfType(t, ctx, "aws_iam_role_policy")
	want := ir.Attrs{
		ir.A("name", ir.Str("shop-dev-handler")),
		ir.A("role", ir.R(fnRoleID, ir.Field("id"))),
		ir.A("policy", ir.J(ir.M(
			ir.A("Version", ir.Str("2012-10-17")),
			ir.A("Statement", ir.L(statement)),
		))),
	}
	if diff := cmp.Diff(want, policy.Args); diff != "" {
		t.Errorf("role policy args (-want +got):\n%s", diff)
	}
}

func TestFunctionJoinsTheVpcWhenAnEdgeAsksForIt(t *testing.T) {
	ctx, handle := setupFunction(t, defaultFunction)
	sg := ensureSecurityGroup(ctx, handle)
	if sg != fnSGID {
		t.Errorf("security group = %v", sg)
	}
	if !handle.NeedsNetwork {
		t.Error("NeedsNetwork is false")
	}
	handle.Finalise()

	got, _ := firstOfType(t, ctx, "aws_lambda_function").Args.Get("vpc_config")
	want := ir.B(ir.Attrs{
		ir.A("subnet_ids", privateSub),
		ir.A("security_group_ids", ir.L(ir.R(fnSGID, ir.Field("id")))),
	})
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("vpc_config (-want +got):\n%s", diff)
	}

	egress := firstOfType(t, ctx, "aws_vpc_security_group_egress_rule")
	if egress.Name != "handler_all" {
		t.Errorf("egress rule name = %q", egress.Name)
	}
	wantEgress := ir.Attrs{
		ir.A("security_group_id", ir.R(fnSGID, ir.Field("id"))),
		ir.A("ip_protocol", ir.Str("-1")),
		ir.A("cidr_ipv4", ir.Str("0.0.0.0/0")),
	}
	if diff := cmp.Diff(wantEgress, egress.Args); diff != "" {
		t.Errorf("egress rule args (-want +got):\n%s", diff)
	}

	var arns []ir.Value
	for _, r := range byType(ctx, "aws_iam_role_policy_attachment") {
		v, _ := r.Args.Get("policy_arn")
		arns = append(arns, v)
	}
	want4 := []ir.Value{
		ir.Str("arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"),
		ir.Str("arn:aws:iam::aws:policy/service-role/AWSLambdaVPCAccessExecutionRole"),
	}
	if diff := cmp.Diff(want4, arns); diff != "" {
		t.Errorf("policy attachments (-want +got):\n%s", diff)
	}
}

func TestEnsureSecurityGroupIsIdempotent(t *testing.T) {
	ctx, handle := setupFunction(t, defaultFunction)
	a := ensureSecurityGroup(ctx, handle)
	b := ensureSecurityGroup(ctx, handle)
	if a != b {
		t.Errorf("%v != %v", a, b)
	}
	if countOfType(ctx, "aws_security_group") != 1 {
		t.Error("a second security group was created")
	}
}
