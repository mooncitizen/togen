package aws

import (
	"fmt"
	"slices"
	"strings"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

const (
	clusterLabel        = "cluster"
	ecsTasksPrincipal   = "ecs-tasks.amazonaws.com"
	taskExecutionPolicy = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
)

func resolveService(ctx *resolve.Context, node ir.Node) *resolve.Handle {
	p := resolve.Props[ir.ServiceProps](ctx, node)
	size, ok := fargateSizes[p.Size]
	if !ok {
		ctx.Fail(fmt.Sprintf("service '%s' has an unknown size '%s'", node.Name, p.Size))
	}
	local := ctx.Local(node.Name)
	cluster := ensureCluster(ctx)

	logGroup := ctx.Add(ir.Resource{
		Type:        "aws_cloudwatch_log_group",
		Name:        local,
		SourceNode:  node.ID,
		SourceLabel: node.Name,
		Args: ir.Attrs{
			ir.A("name", ir.Str("/ecs/"+ctx.Named(node.Name))),
			ir.A("retention_in_days", ir.Num(30)),
		},
	})
	logGroupID := ir.ID{Type: logGroup.Type, Name: logGroup.Name}

	execution := ctx.Add(ir.Resource{
		Type:        "aws_iam_role",
		Name:        local + "_execution",
		SourceNode:  node.ID,
		SourceLabel: node.Name,
		Args: ir.Attrs{
			ir.A("name", ir.Str(ctx.Named(node.Name)+"-execution")),
			ir.A("assume_role_policy", assumeRolePolicy(ecsTasksPrincipal)),
		},
	})
	executionID := ir.ID{Type: execution.Type, Name: execution.Name}
	ctx.Add(ir.Resource{
		Type:        "aws_iam_role_policy_attachment",
		Name:        local + "_execution",
		SourceNode:  node.ID,
		SourceLabel: node.Name,
		Args: ir.Attrs{
			ir.A("role", ir.R(executionID, ir.Field("name"))),
			ir.A("policy_arn", ir.Str(taskExecutionPolicy)),
		},
	})

	task := ctx.Add(ir.Resource{
		Type:        "aws_iam_role",
		Name:        local,
		SourceNode:  node.ID,
		SourceLabel: node.Name,
		Args: ir.Attrs{
			ir.A("name", ir.Str(ctx.Named(node.Name))),
			ir.A("assume_role_policy", assumeRolePolicy(ecsTasksPrincipal)),
		},
	})
	taskRoleID := ir.ID{Type: task.Type, Name: task.Name}

	// A service always runs in the VPC, so the security group is created up front rather
	// than waiting for an edge to ask for one.
	h := &resolve.Handle{Node: node, Env: resolve.SortedEnv(p.Env)}
	sgID := ensureSecurityGroup(ctx, h)
	network := ensureNetwork(ctx)

	definition := ctx.Add(ir.Resource{
		Type:        "aws_ecs_task_definition",
		Name:        local,
		SourceNode:  node.ID,
		SourceLabel: node.Name,
		Args: ir.Attrs{
			ir.A("family", ir.Str(ctx.Named(node.Name))),
			ir.A("requires_compatibilities", ir.L(ir.Str("FARGATE"))),
			ir.A("network_mode", ir.Str("awsvpc")),
			ir.A("cpu", ir.Str(size.CPU)),
			ir.A("memory", ir.Str(size.Memory)),
			ir.A("execution_role_arn", ir.R(executionID, ir.Field("arn"))),
			ir.A("task_role_arn", ir.R(taskRoleID, ir.Field("arn"))),
		},
	})
	definitionID := ir.ID{Type: definition.Type, Name: definition.Name}

	svc := ctx.Add(ir.Resource{
		Type:        "aws_ecs_service",
		Name:        local,
		SourceNode:  node.ID,
		SourceLabel: node.Name,
		Args: ir.Attrs{
			ir.A("name", ir.Str(ctx.Named(node.Name))),
			ir.A("cluster", ir.R(cluster, ir.Field("id"))),
			ir.A("task_definition", ir.R(definitionID, ir.Field("arn"))),
			ir.A("desired_count", ir.Num(float64(p.MinReplicas))),
			ir.A("launch_type", ir.Str("FARGATE")),
			ir.A("network_configuration", ir.B(ir.Attrs{
				ir.A("subnets", subnetRefs(network.PrivateSubnets)),
				ir.A("security_groups", ir.L(ir.R(sgID, ir.Field("id")))),
				ir.A("assign_public_ip", ir.Bool(false)),
			})),
		},
	})

	h.Primary = ir.ID{Type: svc.Type, Name: svc.Name}
	h.Exports = resolve.ServiceExports{
		Port:   ir.Num(float64(p.Port)),
		Public: p.Public,
	}
	if p.Public {
		ensureLoadBalancer(ctx, h, false)
	}

	h.Finalise = func() {
		definition.Args.Set("container_definitions", ir.J(ir.L(ir.M(
			ir.A("name", ir.Str(local)),
			ir.A("image", ir.Str(p.Image)),
			ir.A("essential", ir.Bool(true)),
			ir.A("portMappings", ir.L(ir.M(
				ir.A("containerPort", ir.Num(float64(p.Port))),
				ir.A("protocol", ir.Str("tcp")),
			))),
			ir.A("environment", containerEnv(h.Env)),
			ir.A("logConfiguration", ir.M(
				ir.A("logDriver", ir.Str("awslogs")),
				ir.A("options", ir.M(
					ir.A("awslogs-group", ir.R(logGroupID, ir.Field("name"))),
					ir.A("awslogs-region", ir.Str(ctx.Project.Region)),
					ir.A("awslogs-stream-prefix", ir.Str(local)),
				)),
			)),
		))))
		if len(h.Statements) > 0 {
			ctx.Add(ir.Resource{
				Type:        "aws_iam_role_policy",
				Name:        local,
				SourceNode:  node.ID,
				SourceLabel: node.Name,
				Args: ir.Attrs{
					ir.A("name", ir.Str(ctx.Named(node.Name))),
					ir.A("role", ir.R(taskRoleID, ir.Field("id"))),
					ir.A("policy", ir.J(ir.M(
						ir.A("Version", ir.Str("2012-10-17")),
						ir.A("Statement", ir.L(h.Statements...)),
					))),
				},
			})
		}
	}
	return h
}

// A public service asks for this at node time and a routed private one at edge time, so it must be idempotent.
func ensureLoadBalancer(ctx *resolve.Context, h *resolve.Handle, internal bool) (ir.ID, resolve.ServiceExports) {
	node := h.Node
	local := ctx.Local(node.Name)
	albSGID := ir.ID{Type: "aws_security_group", Name: local + "_alb"}
	exports, ok := h.Exports.(resolve.ServiceExports)
	if !ok {
		ctx.Fail(fmt.Sprintf("%s '%s' has no service exports to put a load balancer in front of", node.Type, node.Name))
	}
	if ctx.HasResource(ir.ID{Type: "aws_lb", Name: local}) {
		return albSGID, exports
	}
	if h.SecurityGroup == nil {
		ctx.Fail(fmt.Sprintf("service '%s' has no security group", node.Name))
	}
	network := ensureNetwork(ctx)
	subnets, reach := network.PublicSubnets, "Public access to "+node.Name
	if internal {
		subnets, reach = network.PrivateSubnets, "Internal access to "+node.Name
	}

	ctx.Add(ir.Resource{
		Type:        albSGID.Type,
		Name:        albSGID.Name,
		SourceNode:  node.ID,
		SourceLabel: node.Name,
		Args: ir.Attrs{
			ir.A("name", ir.Str(ctx.Named(node.Name)+"-alb")),
			ir.A("description", ir.Str(reach)),
			ir.A("vpc_id", ir.R(network.VPC, ir.Field("id"))),
		},
	})
	if !internal {
		ctx.Add(ir.Resource{
			Type:        "aws_vpc_security_group_ingress_rule",
			Name:        local + "_alb_http",
			SourceNode:  node.ID,
			SourceLabel: node.Name,
			Args: ir.Attrs{
				ir.A("security_group_id", ir.R(albSGID, ir.Field("id"))),
				ir.A("cidr_ipv4", ir.Str("0.0.0.0/0")),
				ir.A("from_port", ir.Num(80)),
				ir.A("to_port", ir.Num(80)),
				ir.A("ip_protocol", ir.Str("tcp")),
				ir.A("description", ir.Str("Internet to "+node.Name)),
			},
		})
	}
	ctx.Add(ir.Resource{
		Type:        "aws_vpc_security_group_egress_rule",
		Name:        local + "_alb_all",
		SourceNode:  node.ID,
		SourceLabel: node.Name,
		Args: ir.Attrs{
			ir.A("security_group_id", ir.R(albSGID, ir.Field("id"))),
			ir.A("ip_protocol", ir.Str("-1")),
			ir.A("cidr_ipv4", ir.Str("0.0.0.0/0")),
		},
	})

	lb := ctx.Add(ir.Resource{
		Type:        "aws_lb",
		Name:        local,
		SourceNode:  node.ID,
		SourceLabel: node.Name,
		Args: ir.Attrs{
			ir.A("name", ir.Str(ctx.Named(node.Name))),
			ir.A("load_balancer_type", ir.Str("application")),
			ir.A("internal", ir.Bool(internal)),
			ir.A("subnets", subnetRefs(subnets)),
			ir.A("security_groups", ir.L(ir.R(albSGID, ir.Field("id")))),
		},
	})
	lbID := ir.ID{Type: lb.Type, Name: lb.Name}

	targets := ctx.Add(ir.Resource{
		Type:        "aws_lb_target_group",
		Name:        local,
		SourceNode:  node.ID,
		SourceLabel: node.Name,
		Args: ir.Attrs{
			ir.A("name", ir.Str(ctx.Named(node.Name))),
			ir.A("port", exports.Port),
			ir.A("protocol", ir.Str("HTTP")),
			ir.A("vpc_id", ir.R(network.VPC, ir.Field("id"))),
			ir.A("target_type", ir.Str("ip")),
			ir.A("health_check", ir.B(ir.Attrs{ir.A("path", ir.Str("/"))})),
		},
	})
	targetsID := ir.ID{Type: targets.Type, Name: targets.Name}

	listener := ctx.Add(ir.Resource{
		Type:        "aws_lb_listener",
		Name:        local,
		SourceNode:  node.ID,
		SourceLabel: node.Name,
		Args: ir.Attrs{
			ir.A("load_balancer_arn", ir.R(lbID, ir.Field("arn"))),
			ir.A("port", ir.Num(80)),
			ir.A("protocol", ir.Str("HTTP")),
			ir.A("default_action", ir.B(ir.Attrs{
				ir.A("type", ir.Str("forward")),
				ir.A("target_group_arn", ir.R(targetsID, ir.Field("arn"))),
			})),
		},
	})
	listenerID := ir.ID{Type: listener.Type, Name: listener.Name}

	ctx.Add(ir.Resource{
		Type:        "aws_vpc_security_group_ingress_rule",
		Name:        local + "_from_alb",
		SourceNode:  node.ID,
		SourceLabel: node.Name,
		Args: ir.Attrs{
			ir.A("security_group_id", ir.R(*h.SecurityGroup, ir.Field("id"))),
			ir.A("referenced_security_group_id", ir.R(albSGID, ir.Field("id"))),
			ir.A("from_port", exports.Port),
			ir.A("to_port", exports.Port),
			ir.A("ip_protocol", ir.Str("tcp")),
			ir.A("description", ir.Str("Load balancer to "+node.Name)),
		},
	})

	svc, ok := ctx.Resource(h.Primary)
	if !ok {
		ctx.Fail(fmt.Sprintf("service '%s' has no %s to put behind a load balancer", node.Name, h.Primary.Type))
	}
	svc.Args.Set("load_balancer", ir.B(ir.Attrs{
		ir.A("target_group_arn", ir.R(targetsID, ir.Field("arn"))),
		ir.A("container_name", ir.Str(local)),
		ir.A("container_port", exports.Port),
	}))
	svc.DependsOn = append(svc.DependsOn, listenerID)

	exports.ListenerARN = ir.R(listenerID, ir.Field("arn"))
	if !internal {
		exports.URL = ir.C(ir.Str("http://"), ir.R(lbID, ir.Field("dns_name")))
		ctx.AddOutput(ir.Output{
			Name:        local + "_url",
			Description: fmt.Sprintf("Public URL of the %s service", node.Name),
			Value:       exports.URL,
		})
	}
	h.Exports = exports
	return albSGID, exports
}

func ensureCluster(ctx *resolve.Context) ir.ID {
	if id, ok := ctx.Scratch[clusterLabel].(ir.ID); ok {
		return id
	}
	cluster := ctx.Add(ir.Resource{
		Type:        "aws_ecs_cluster",
		Name:        "main",
		SourceLabel: clusterLabel,
		Args: ir.Attrs{
			ir.A("name", ir.Str(ctx.Prefix())),
			ir.A("setting", ir.B(ir.Attrs{
				ir.A("name", ir.Str("containerInsights")),
				ir.A("value", ir.Str("enabled")),
			})),
		},
	})
	id := ir.ID{Type: cluster.Type, Name: cluster.Name}
	ctx.Scratch[clusterLabel] = id
	return id
}

// containerEnv rewrites the handle's environment into the name/value pairs a container
// definition expects, alphabetically so the generated JSON does not churn.
func containerEnv(env ir.Attrs) ir.List {
	sorted := slices.Clone(env)
	slices.SortFunc(sorted, func(a, b ir.Attr) int { return strings.Compare(a.Key, b.Key) })
	out := make(ir.List, 0, len(sorted))
	for _, a := range sorted {
		out = append(out, ir.M(ir.A("name", ir.Str(a.Key)), ir.A("value", a.Value)))
	}
	return out
}

func assumeRolePolicy(principal string) ir.Value {
	return ir.J(ir.M(
		ir.A("Version", ir.Str("2012-10-17")),
		ir.A("Statement", ir.L(ir.M(
			ir.A("Effect", ir.Str("Allow")),
			ir.A("Principal", ir.M(ir.A("Service", ir.Str(principal)))),
			ir.A("Action", ir.Str("sts:AssumeRole")),
		))),
	))
}
