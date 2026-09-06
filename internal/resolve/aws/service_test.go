package aws

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

func serviceNode(t *testing.T, id, name string, p ir.ServiceProps) ir.Node {
	t.Helper()
	return ir.Node{ID: id, Type: ir.NodeService, Name: name, Properties: props(t, p)}
}

func setupService(t *testing.T, p ir.ServiceProps) (*resolve.Context, *resolve.Handle) {
	t.Helper()
	ctx, project := newContext(t, []ir.Node{serviceNode(t, "n4", "web", p)}, nil)
	return ctx, resolveService(ctx, project.Nodes[0])
}

func named(t *testing.T, ctx *resolve.Context, id ir.ID) ir.Resource {
	t.Helper()
	for _, r := range ctx.Resources() {
		if r.Type == id.Type && r.Name == id.Name {
			return r
		}
	}
	t.Fatalf("no %s in the graph", id)
	return ir.Resource{}
}

var defaultService = ir.ServiceProps{
	Image:       "nginx:1.27",
	Port:        8080,
	Size:        ir.SizeSmall,
	MinReplicas: ir.Ptr(1),
	MaxReplicas: 2,
}

var (
	clusterID    = ir.ID{Type: "aws_ecs_cluster", Name: "main"}
	svcID        = ir.ID{Type: "aws_ecs_service", Name: "web"}
	svcTaskDef   = ir.ID{Type: "aws_ecs_task_definition", Name: "web"}
	svcLogsID    = ir.ID{Type: "aws_cloudwatch_log_group", Name: "web"}
	svcRoleID    = ir.ID{Type: "aws_iam_role", Name: "web"}
	svcExecID    = ir.ID{Type: "aws_iam_role", Name: "web_execution"}
	svcSGID      = ir.ID{Type: "aws_security_group", Name: "web"}
	albID        = ir.ID{Type: "aws_lb", Name: "web"}
	albSGID      = ir.ID{Type: "aws_security_group", Name: "web_alb"}
	albTargets   = ir.ID{Type: "aws_lb_target_group", Name: "web"}
	albListener  = ir.ID{Type: "aws_lb_listener", Name: "web"}
	ecsTaskTrust = ir.J(ir.M(
		ir.A("Version", ir.Str("2012-10-17")),
		ir.A("Statement", ir.L(ir.M(
			ir.A("Effect", ir.Str("Allow")),
			ir.A("Principal", ir.M(ir.A("Service", ir.Str("ecs-tasks.amazonaws.com")))),
			ir.A("Action", ir.Str("sts:AssumeRole")),
		))),
	))
)

func TestServiceEmitsAClusterLogGroupRolesAndAFargateService(t *testing.T) {
	p := defaultService
	p.Env = map[string]string{"LOG_LEVEL": "info", "APP_NAME": "shop"}
	ctx, handle := setupService(t, p)
	handle.Finalise()

	cluster := named(t, ctx, clusterID)
	if cluster.SourceNode != "" || cluster.SourceLabel != "cluster" {
		t.Errorf("cluster source = %q/%q", cluster.SourceNode, cluster.SourceLabel)
	}
	wantCluster := ir.Attrs{
		ir.A("name", ir.Str("shop-dev")),
		ir.A("setting", ir.B(ir.Attrs{
			ir.A("name", ir.Str("containerInsights")),
			ir.A("value", ir.Str("enabled")),
		})),
	}
	if diff := cmp.Diff(wantCluster, cluster.Args); diff != "" {
		t.Errorf("cluster args (-want +got):\n%s", diff)
	}

	wantLogs := ir.Attrs{
		ir.A("name", ir.Str("/ecs/shop-dev-web")),
		ir.A("retention_in_days", ir.Num(30)),
	}
	if diff := cmp.Diff(wantLogs, named(t, ctx, svcLogsID).Args); diff != "" {
		t.Errorf("log group args (-want +got):\n%s", diff)
	}

	wantExecution := ir.Attrs{
		ir.A("name", ir.Str("shop-dev-web-execution")),
		ir.A("assume_role_policy", ecsTaskTrust),
	}
	if diff := cmp.Diff(wantExecution, named(t, ctx, svcExecID).Args); diff != "" {
		t.Errorf("execution role args (-want +got):\n%s", diff)
	}
	wantAttachment := ir.Attrs{
		ir.A("role", ir.R(svcExecID, ir.Field("name"))),
		ir.A("policy_arn", ir.Str("arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy")),
	}
	attachment := named(t, ctx, ir.ID{Type: "aws_iam_role_policy_attachment", Name: "web_execution"})
	if diff := cmp.Diff(wantAttachment, attachment.Args); diff != "" {
		t.Errorf("policy attachment args (-want +got):\n%s", diff)
	}

	wantTaskRole := ir.Attrs{
		ir.A("name", ir.Str("shop-dev-web")),
		ir.A("assume_role_policy", ecsTaskTrust),
	}
	if diff := cmp.Diff(wantTaskRole, named(t, ctx, svcRoleID).Args); diff != "" {
		t.Errorf("task role args (-want +got):\n%s", diff)
	}

	wantSG := ir.Attrs{
		ir.A("name", ir.Str("shop-dev-web")),
		ir.A("description", ir.Str("Outbound access for web")),
		ir.A("vpc_id", ir.R(ir.ID{Type: "aws_vpc", Name: "main"}, ir.Field("id"))),
	}
	if diff := cmp.Diff(wantSG, named(t, ctx, svcSGID).Args); diff != "" {
		t.Errorf("security group args (-want +got):\n%s", diff)
	}
	wantEgress := ir.Attrs{
		ir.A("security_group_id", ir.R(svcSGID, ir.Field("id"))),
		ir.A("ip_protocol", ir.Str("-1")),
		ir.A("cidr_ipv4", ir.Str("0.0.0.0/0")),
	}
	egress := named(t, ctx, ir.ID{Type: "aws_vpc_security_group_egress_rule", Name: "web_all"})
	if diff := cmp.Diff(wantEgress, egress.Args); diff != "" {
		t.Errorf("egress rule args (-want +got):\n%s", diff)
	}
	if !handle.NeedsNetwork || handle.SecurityGroup == nil || *handle.SecurityGroup != svcSGID {
		t.Errorf("handle network wiring = %v/%v", handle.NeedsNetwork, handle.SecurityGroup)
	}

	wantDefinition := ir.Attrs{
		ir.A("family", ir.Str("shop-dev-web")),
		ir.A("requires_compatibilities", ir.L(ir.Str("FARGATE"))),
		ir.A("network_mode", ir.Str("awsvpc")),
		ir.A("cpu", ir.Str("256")),
		ir.A("memory", ir.Str("512")),
		ir.A("execution_role_arn", ir.R(svcExecID, ir.Field("arn"))),
		ir.A("task_role_arn", ir.R(svcRoleID, ir.Field("arn"))),
		ir.A("container_definitions", ir.J(ir.L(ir.M(
			ir.A("name", ir.Str("web")),
			ir.A("image", ir.Str("nginx:1.27")),
			ir.A("essential", ir.Bool(true)),
			ir.A("portMappings", ir.L(ir.M(
				ir.A("containerPort", ir.Num(8080)),
				ir.A("protocol", ir.Str("tcp")),
			))),
			ir.A("environment", ir.L(
				ir.M(ir.A("name", ir.Str("APP_NAME")), ir.A("value", ir.Str("shop"))),
				ir.M(ir.A("name", ir.Str("LOG_LEVEL")), ir.A("value", ir.Str("info"))),
			)),
			ir.A("logConfiguration", ir.M(
				ir.A("logDriver", ir.Str("awslogs")),
				ir.A("options", ir.M(
					ir.A("awslogs-group", ir.R(svcLogsID, ir.Field("name"))),
					ir.A("awslogs-region", ir.Str("eu-west-2")),
					ir.A("awslogs-stream-prefix", ir.Str("web")),
				)),
			)),
		)))),
	}
	if diff := cmp.Diff(wantDefinition, named(t, ctx, svcTaskDef).Args); diff != "" {
		t.Errorf("task definition args (-want +got):\n%s", diff)
	}

	svc := named(t, ctx, svcID)
	wantService := ir.Attrs{
		ir.A("name", ir.Str("shop-dev-web")),
		ir.A("cluster", ir.R(clusterID, ir.Field("id"))),
		ir.A("task_definition", ir.R(svcTaskDef, ir.Field("arn"))),
		ir.A("desired_count", ir.Num(1)),
		ir.A("launch_type", ir.Str("FARGATE")),
		ir.A("network_configuration", ir.B(ir.Attrs{
			ir.A("subnets", privateSub),
			ir.A("security_groups", ir.L(ir.R(svcSGID, ir.Field("id")))),
			ir.A("assign_public_ip", ir.Bool(false)),
		})),
	}
	if diff := cmp.Diff(wantService, svc.Args); diff != "" {
		t.Errorf("service args (-want +got):\n%s", diff)
	}
	if len(svc.DependsOn) != 0 {
		t.Errorf("depends_on = %v", svc.DependsOn)
	}

	for _, typ := range []string{"aws_lb", "aws_lb_listener", "aws_lb_target_group"} {
		if countOfType(ctx, typ) != 0 {
			t.Errorf("a private service created %s", typ)
		}
	}
	if len(ctx.Outputs) != 0 {
		t.Errorf("outputs = %v", ctx.Outputs)
	}
	if countOfType(ctx, "aws_iam_role_policy") != 0 {
		t.Error("a role policy was created without statements")
	}

	wantExports := resolve.ServiceExports{Port: ir.Num(8080)}
	if diff := cmp.Diff(wantExports, handle.Exports); diff != "" {
		t.Errorf("exports (-want +got):\n%s", diff)
	}
}

func TestServiceHonoursReplicasAndTheContainerImage(t *testing.T) {
	p := defaultService
	p.Image = "ghcr.io/shop/web:1.2.3"
	p.MinReplicas = ir.Ptr(3)
	ctx, handle := setupService(t, p)
	handle.Finalise()

	got, _ := named(t, ctx, svcID).Args.Get("desired_count")
	if diff := cmp.Diff(ir.Value(ir.Num(3)), got); diff != "" {
		t.Errorf("desired_count (-want +got):\n%s", diff)
	}
	definitions, _ := named(t, ctx, svcTaskDef).Args.Get("container_definitions")
	image := definitions.(ir.JSON).Value.(ir.List)[0].(ir.Map)
	if got, _ := ir.Attrs(image).Get("image"); got != ir.Str("ghcr.io/shop/web:1.2.3") {
		t.Errorf("image = %v", got)
	}
}

func TestServiceMapsSizesToFargateCpuAndMemoryPairs(t *testing.T) {
	for _, c := range []struct {
		size        ir.Size
		cpu, memory string
	}{
		{ir.SizeSmall, "256", "512"},
		{ir.SizeMedium, "1024", "2048"},
		{ir.SizeLarge, "2048", "4096"},
	} {
		t.Run(string(c.size), func(t *testing.T) {
			p := defaultService
			p.Size = c.size
			ctx, handle := setupService(t, p)
			handle.Finalise()
			definition := named(t, ctx, svcTaskDef)
			cpu, _ := definition.Args.Get("cpu")
			memory, _ := definition.Args.Get("memory")
			if cpu != ir.Str(c.cpu) || memory != ir.Str(c.memory) {
				t.Errorf("cpu/memory = %v/%v, want %s/%s", cpu, memory, c.cpu, c.memory)
			}
		})
	}
}

func TestServiceCreatesTheClusterOnce(t *testing.T) {
	ctx, project := newContext(t, []ir.Node{
		serviceNode(t, "n4", "web", defaultService),
		serviceNode(t, "n5", "admin", defaultService),
	}, nil)
	for _, n := range project.Nodes {
		resolveService(ctx, n).Finalise()
	}
	if countOfType(ctx, "aws_ecs_cluster") != 1 {
		t.Errorf("clusters = %d", countOfType(ctx, "aws_ecs_cluster"))
	}
	if countOfType(ctx, "aws_ecs_service") != 2 {
		t.Errorf("services = %d", countOfType(ctx, "aws_ecs_service"))
	}
}

func TestPublicServiceGetsALoadBalancerListenerAndTargetGroup(t *testing.T) {
	p := defaultService
	p.Port = 80
	p.Public = true
	ctx, handle := setupService(t, p)
	handle.Finalise()

	wantALBSG := ir.Attrs{
		ir.A("name", ir.Str("shop-dev-web-alb")),
		ir.A("description", ir.Str("Public access to web")),
		ir.A("vpc_id", ir.R(ir.ID{Type: "aws_vpc", Name: "main"}, ir.Field("id"))),
	}
	if diff := cmp.Diff(wantALBSG, named(t, ctx, albSGID).Args); diff != "" {
		t.Errorf("alb security group args (-want +got):\n%s", diff)
	}
	wantHTTP := ir.Attrs{
		ir.A("security_group_id", ir.R(albSGID, ir.Field("id"))),
		ir.A("cidr_ipv4", ir.Str("0.0.0.0/0")),
		ir.A("from_port", ir.Num(80)),
		ir.A("to_port", ir.Num(80)),
		ir.A("ip_protocol", ir.Str("tcp")),
		ir.A("description", ir.Str("Internet to web")),
	}
	http := named(t, ctx, ir.ID{Type: "aws_vpc_security_group_ingress_rule", Name: "web_alb_http"})
	if diff := cmp.Diff(wantHTTP, http.Args); diff != "" {
		t.Errorf("alb ingress rule args (-want +got):\n%s", diff)
	}
	wantALBEgress := ir.Attrs{
		ir.A("security_group_id", ir.R(albSGID, ir.Field("id"))),
		ir.A("ip_protocol", ir.Str("-1")),
		ir.A("cidr_ipv4", ir.Str("0.0.0.0/0")),
	}
	albEgress := named(t, ctx, ir.ID{Type: "aws_vpc_security_group_egress_rule", Name: "web_alb_all"})
	if diff := cmp.Diff(wantALBEgress, albEgress.Args); diff != "" {
		t.Errorf("alb egress rule args (-want +got):\n%s", diff)
	}

	wantLB := ir.Attrs{
		ir.A("name", ir.Str("shop-dev-web")),
		ir.A("load_balancer_type", ir.Str("application")),
		ir.A("internal", ir.Bool(false)),
		ir.A("subnets", ir.L(
			ir.R(ir.ID{Type: "aws_subnet", Name: "public_a"}, ir.Field("id")),
			ir.R(ir.ID{Type: "aws_subnet", Name: "public_b"}, ir.Field("id")),
		)),
		ir.A("security_groups", ir.L(ir.R(albSGID, ir.Field("id")))),
	}
	if diff := cmp.Diff(wantLB, named(t, ctx, albID).Args); diff != "" {
		t.Errorf("load balancer args (-want +got):\n%s", diff)
	}
	wantTargets := ir.Attrs{
		ir.A("name", ir.Str("shop-dev-web")),
		ir.A("port", ir.Num(80)),
		ir.A("protocol", ir.Str("HTTP")),
		ir.A("vpc_id", ir.R(ir.ID{Type: "aws_vpc", Name: "main"}, ir.Field("id"))),
		ir.A("target_type", ir.Str("ip")),
		ir.A("health_check", ir.B(ir.Attrs{ir.A("path", ir.Str("/"))})),
	}
	if diff := cmp.Diff(wantTargets, named(t, ctx, albTargets).Args); diff != "" {
		t.Errorf("target group args (-want +got):\n%s", diff)
	}
	wantListener := ir.Attrs{
		ir.A("load_balancer_arn", ir.R(albID, ir.Field("arn"))),
		ir.A("port", ir.Num(80)),
		ir.A("protocol", ir.Str("HTTP")),
		ir.A("default_action", ir.B(ir.Attrs{
			ir.A("type", ir.Str("forward")),
			ir.A("target_group_arn", ir.R(albTargets, ir.Field("arn"))),
		})),
	}
	if diff := cmp.Diff(wantListener, named(t, ctx, albListener).Args); diff != "" {
		t.Errorf("listener args (-want +got):\n%s", diff)
	}

	wantFromALB := ir.Attrs{
		ir.A("security_group_id", ir.R(svcSGID, ir.Field("id"))),
		ir.A("referenced_security_group_id", ir.R(albSGID, ir.Field("id"))),
		ir.A("from_port", ir.Num(80)),
		ir.A("to_port", ir.Num(80)),
		ir.A("ip_protocol", ir.Str("tcp")),
		ir.A("description", ir.Str("Load balancer to web")),
	}
	fromALB := named(t, ctx, ir.ID{Type: "aws_vpc_security_group_ingress_rule", Name: "web_from_alb"})
	if diff := cmp.Diff(wantFromALB, fromALB.Args); diff != "" {
		t.Errorf("service ingress rule args (-want +got):\n%s", diff)
	}

	svc := named(t, ctx, svcID)
	got, _ := svc.Args.Get("load_balancer")
	wantBlock := ir.B(ir.Attrs{
		ir.A("target_group_arn", ir.R(albTargets, ir.Field("arn"))),
		ir.A("container_name", ir.Str("web")),
		ir.A("container_port", ir.Num(80)),
	})
	if diff := cmp.Diff(ir.Value(wantBlock), got); diff != "" {
		t.Errorf("load_balancer block (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]ir.ID{albListener}, svc.DependsOn); diff != "" {
		t.Errorf("depends_on (-want +got):\n%s", diff)
	}

	url := ir.C(ir.Str("http://"), ir.R(albID, ir.Field("dns_name")))
	wantOutputs := []ir.Output{{
		Name:        "web_url",
		Description: "Public URL of the web service",
		Value:       url,
	}}
	if diff := cmp.Diff(wantOutputs, ctx.Outputs); diff != "" {
		t.Errorf("outputs (-want +got):\n%s", diff)
	}
	wantExports := resolve.ServiceExports{
		Port:        ir.Num(80),
		Public:      true,
		URL:         url,
		ListenerARN: ir.R(albListener, ir.Field("arn")),
	}
	if diff := cmp.Diff(wantExports, handle.Exports); diff != "" {
		t.Errorf("exports (-want +got):\n%s", diff)
	}
}

func TestLoadBalancerIsCreatedOnce(t *testing.T) {
	p := defaultService
	p.Port = 80
	p.Public = true
	ctx, handle := setupService(t, p)
	handle.Finalise()

	albSG, exports := ensureLoadBalancer(ctx, handle, false)
	if albSG != albSGID {
		t.Errorf("alb security group = %s", albSG)
	}
	if diff := cmp.Diff(handle.Exports, exports); diff != "" {
		t.Errorf("returned exports (-handle +returned):\n%s", diff)
	}
	for _, typ := range []string{"aws_lb", "aws_lb_listener", "aws_lb_target_group"} {
		if got := countOfType(ctx, typ); got != 1 {
			t.Errorf("%s = %d, want 1", typ, got)
		}
	}
	if len(ctx.Outputs) != 1 {
		t.Errorf("outputs = %v", ctx.Outputs)
	}
	if diff := cmp.Diff([]ir.ID{albListener}, named(t, ctx, svcID).DependsOn); diff != "" {
		t.Errorf("depends_on (-want +got):\n%s", diff)
	}
}

func TestServiceWritesStatementsOntoTheTaskRole(t *testing.T) {
	ctx, handle := setupService(t, defaultService)
	statement := ir.M(
		ir.A("Effect", ir.Str("Allow")),
		ir.A("Action", ir.L(ir.Str("s3:GetObject"))),
		ir.A("Resource", ir.Str("arn:x")),
	)
	handle.AddStatement(statement)
	handle.Finalise()

	want := ir.Attrs{
		ir.A("name", ir.Str("shop-dev-web")),
		ir.A("role", ir.R(svcRoleID, ir.Field("id"))),
		ir.A("policy", ir.J(ir.M(
			ir.A("Version", ir.Str("2012-10-17")),
			ir.A("Statement", ir.L(statement)),
		))),
	}
	policy := firstOfType(t, ctx, "aws_iam_role_policy")
	if diff := cmp.Diff(want, policy.Args); diff != "" {
		t.Errorf("role policy args (-want +got):\n%s", diff)
	}
}

func TestServiceWithZeroMinReplicasRunsNoTasks(t *testing.T) {
	p := defaultService
	p.MinReplicas = ir.Ptr(0)
	ctx, handle := setupService(t, p)
	handle.Finalise()

	got, _ := named(t, ctx, svcID).Args.Get("desired_count")
	if diff := cmp.Diff(ir.Value(ir.Num(0)), got); diff != "" {
		t.Errorf("desired_count (-want +got):\n%s", diff)
	}
}
