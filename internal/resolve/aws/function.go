package aws

import (
	"fmt"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

type runtimeSpec struct {
	Runtime string
	Handler func(string) string
}

var runtimes = map[ir.Runtime]runtimeSpec{
	ir.RuntimeNode:   {Runtime: "nodejs22.x", Handler: func(h string) string { return h }},
	ir.RuntimePython: {Runtime: "python3.12", Handler: func(h string) string { return h }},
	ir.RuntimeGo:     {Runtime: "provided.al2023", Handler: func(string) string { return "bootstrap" }},
}

const (
	logsPolicy = "arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"
	vpcPolicy  = "arn:aws:iam::aws:policy/service-role/AWSLambdaVPCAccessExecutionRole"
)

func resolveFunction(ctx *resolve.Context, node ir.Node) *resolve.Handle {
	p := resolve.Props[ir.FunctionProps](ctx, node)
	rt, ok := runtimes[p.Runtime]
	if !ok {
		ctx.Fail(fmt.Sprintf("function '%s' has an unknown runtime '%s'", node.Name, p.Runtime))
	}
	local := ctx.Local(node.Name)

	role := ctx.Add(ir.Resource{
		Type:        "aws_iam_role",
		Name:        local,
		SourceNode:  node.ID,
		SourceLabel: node.Name,
		Args: ir.Attrs{
			ir.A("name", ir.Str(ctx.Named(node.Name))),
			ir.A("assume_role_policy", assumeRolePolicy("lambda.amazonaws.com")),
		},
	})
	roleID := ir.ID{Type: role.Type, Name: role.Name}

	logsAttachment := ctx.Add(ir.Resource{
		Type:        "aws_iam_role_policy_attachment",
		Name:        local + "_logs",
		SourceNode:  node.ID,
		SourceLabel: node.Name,
		Args: ir.Attrs{
			ir.A("role", ir.R(roleID, ir.Field("name"))),
			ir.A("policy_arn", ir.Str(logsPolicy)),
		},
	})
	logGroup := ctx.Add(ir.Resource{
		Type:        "aws_cloudwatch_log_group",
		Name:        local,
		SourceNode:  node.ID,
		SourceLabel: node.Name,
		Args: ir.Attrs{
			ir.A("name", ir.Str("/aws/lambda/"+ctx.Named(node.Name))),
			ir.A("retention_in_days", ir.Num(30)),
		},
	})

	packageVar := local + "_package"
	ctx.AddVariable(ir.Variable{
		Name:        packageVar,
		Description: fmt.Sprintf("Path to the deployment package for the %s function", node.Name),
		Type:        "string",
		Default:     ir.Str(fmt.Sprintf("functions/%s.zip", node.Name)),
	})

	fn := ctx.Add(ir.Resource{
		Type:        "aws_lambda_function",
		Name:        local,
		SourceNode:  node.ID,
		SourceLabel: node.Name,
		Args: ir.Attrs{
			ir.A("function_name", ir.Str(ctx.Named(node.Name))),
			ir.A("role", ir.R(roleID, ir.Field("arn"))),
			ir.A("runtime", ir.Str(rt.Runtime)),
			ir.A("handler", ir.Str(rt.Handler(p.Handler))),
			ir.A("filename", ir.V(packageVar)),
			ir.A("source_code_hash", ir.Fn("filebase64sha256", ir.V(packageVar))),
			ir.A("memory_size", ir.Num(lambdaMemory[p.Size])),
			ir.A("timeout", ir.Num(float64(p.TimeoutSeconds))),
		},
		DependsOn: []ir.ID{
			{Type: logGroup.Type, Name: logGroup.Name},
			{Type: logsAttachment.Type, Name: logsAttachment.Name},
		},
	})
	fnID := ir.ID{Type: fn.Type, Name: fn.Name}

	h := &resolve.Handle{
		Node:    node,
		Primary: fnID,
		Env:     resolve.SortedEnv(p.Env),
		Exports: resolve.FunctionExports{
			ARN:          ir.R(fnID, ir.Field("arn")),
			InvokeARN:    ir.R(fnID, ir.Field("invoke_arn")),
			FunctionName: ir.R(fnID, ir.Field("function_name")),
		},
	}
	h.Finalise = func() {
		if len(h.Env) > 0 {
			fn.Args.Set("environment", ir.B(ir.Attrs{ir.A("variables", ir.Map(h.Env))}))
		}
		if len(h.Statements) > 0 {
			ctx.Add(ir.Resource{
				Type:        "aws_iam_role_policy",
				Name:        local,
				SourceNode:  node.ID,
				SourceLabel: node.Name,
				Args: ir.Attrs{
					ir.A("name", ir.Str(ctx.Named(node.Name))),
					ir.A("role", ir.R(roleID, ir.Field("id"))),
					ir.A("policy", ir.J(ir.M(
						ir.A("Version", ir.Str("2012-10-17")),
						ir.A("Statement", ir.L(h.Statements...)),
					))),
				},
			})
		}
		if h.NeedsNetwork {
			sg := ensureSecurityGroup(ctx, h)
			network := ensureNetwork(ctx)
			fn.Args.Set("vpc_config", ir.B(ir.Attrs{
				ir.A("subnet_ids", subnetRefs(network.PrivateSubnets)),
				ir.A("security_group_ids", ir.L(ir.R(sg, ir.Field("id")))),
			}))
			ctx.Add(ir.Resource{
				Type:        "aws_iam_role_policy_attachment",
				Name:        local + "_vpc",
				SourceNode:  node.ID,
				SourceLabel: node.Name,
				Args: ir.Attrs{
					ir.A("role", ir.R(roleID, ir.Field("name"))),
					ir.A("policy_arn", ir.Str(vpcPolicy)),
				},
			})
		}
	}
	return h
}

func ensureSecurityGroup(ctx *resolve.Context, h *resolve.Handle) ir.ID {
	if h.SecurityGroup != nil {
		return *h.SecurityGroup
	}
	local := ctx.Local(h.Node.Name)
	network := ensureNetwork(ctx)
	sg := ctx.Add(ir.Resource{
		Type:        "aws_security_group",
		Name:        local,
		SourceNode:  h.Node.ID,
		SourceLabel: h.Node.Name,
		Args: ir.Attrs{
			ir.A("name", ir.Str(ctx.Named(h.Node.Name))),
			ir.A("description", ir.Str("Outbound access for "+h.Node.Name)),
			ir.A("vpc_id", ir.R(network.VPC, ir.Field("id"))),
		},
	})
	sgID := ir.ID{Type: sg.Type, Name: sg.Name}
	ctx.Add(ir.Resource{
		Type:        "aws_vpc_security_group_egress_rule",
		Name:        local + "_all",
		SourceNode:  h.Node.ID,
		SourceLabel: h.Node.Name,
		Args: ir.Attrs{
			ir.A("security_group_id", ir.R(sgID, ir.Field("id"))),
			ir.A("ip_protocol", ir.Str("-1")),
			ir.A("cidr_ipv4", ir.Str("0.0.0.0/0")),
		},
	})
	h.SecurityGroup = &sgID
	h.NeedsNetwork = true
	return sgID
}
