package aws

import (
	"fmt"
	"strings"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

const discoveryLabel = "discovery"

func resolveCalls(ctx *resolve.Context, edge ir.Edge, from, to *resolve.Handle) {
	switch from.Exports.(type) {
	case resolve.FunctionExports, resolve.ServiceExports:
	default:
		ctx.Report(ir.ValidationError{
			EdgeID:  edge.ID,
			Message: fmt.Sprintf("%s from a %s is not supported by the aws resolver yet", edge.Relation, from.Node.Type),
		})
		return
	}
	switch target := to.Exports.(type) {
	case resolve.FunctionExports:
		callFunction(ctx, from, to, target)
	case resolve.ServiceExports:
		callService(ctx, from, to, target)
	default:
		ctx.Report(ir.ValidationError{
			EdgeID:  edge.ID,
			Message: fmt.Sprintf("%s to a %s is not supported by the aws resolver yet", edge.Relation, to.Node.Type),
		})
	}
}

// A lambda is invoked over the AWS API rather than the network, so nothing here joins the VPC.
func callFunction(ctx *resolve.Context, from, to *resolve.Handle, target resolve.FunctionExports) {
	from.AddStatement(ir.M(
		ir.A("Effect", ir.Str("Allow")),
		ir.A("Action", ir.L(ir.Str("lambda:InvokeFunction"))),
		ir.A("Resource", target.ARN),
	))
	from.SetEnv(strings.ToUpper(ctx.Local(to.Node.Name))+"_FUNCTION_NAME", target.FunctionName)
}

// A private service has no load balancer, so callers reach it through Cloud Map private DNS. A
// public one is called by the same name, which keeps internal traffic inside the VPC.
func callService(ctx *resolve.Context, from, to *resolve.Handle, target resolve.ServiceExports) {
	if to.SecurityGroup == nil {
		ctx.Fail(fmt.Sprintf("%s '%s' has no security group", to.Node.Type, to.Node.Name))
	}
	port, ok := target.Port.(ir.Number)
	if !ok {
		ctx.Fail(fmt.Sprintf("%s '%s' exports a port that is not a number", to.Node.Type, to.Node.Name))
	}
	registerService(ctx, to)

	sourceSG := ensureSecurityGroup(ctx, from)
	ruleID := ir.ID{
		Type: "aws_vpc_security_group_ingress_rule",
		Name: ctx.Local(to.Node.Name) + "_from_" + ctx.Local(from.Node.Name),
	}
	if !ctx.HasResource(ruleID) {
		ctx.Add(ir.Resource{
			Type:        ruleID.Type,
			Name:        ruleID.Name,
			SourceNode:  to.Node.ID,
			SourceLabel: to.Node.Name,
			Args: ir.Attrs{
				ir.A("security_group_id", ir.R(*to.SecurityGroup, ir.Field("id"))),
				ir.A("referenced_security_group_id", ir.R(sourceSG, ir.Field("id"))),
				ir.A("from_port", port),
				ir.A("to_port", port),
				ir.A("ip_protocol", ir.Str("tcp")),
				ir.A("description", ir.Str(from.Node.Name+" to "+to.Node.Name)),
			},
		})
	}

	url := "http://" + serviceHost(ctx, to.Node.Name) + ":" + portString(port)
	from.SetEnv(strings.ToUpper(ctx.Local(to.Node.Name))+"_URL", ir.Str(url))
}

func serviceHost(ctx *resolve.Context, node string) string {
	return node + "." + ctx.Prefix() + ".local"
}

func registerService(ctx *resolve.Context, to *resolve.Handle) {
	discovery := ir.ID{Type: "aws_service_discovery_service", Name: ctx.Local(to.Node.Name)}
	if ctx.HasResource(discovery) {
		return
	}
	namespace := ensureNamespace(ctx)
	ctx.Add(ir.Resource{
		Type:        discovery.Type,
		Name:        discovery.Name,
		SourceNode:  to.Node.ID,
		SourceLabel: to.Node.Name,
		Args: ir.Attrs{
			ir.A("name", ir.Str(to.Node.Name)),
			ir.A("dns_config", ir.B(ir.Attrs{
				ir.A("namespace_id", ir.R(namespace, ir.Field("id"))),
				ir.A("routing_policy", ir.Str("MULTIVALUE")),
				ir.A("dns_records", ir.B(ir.Attrs{
					ir.A("ttl", ir.Num(10)),
					ir.A("type", ir.Str("A")),
				})),
			})),
			// The block tells Cloud Map that ECS reports task health. Its one argument,
			// failure_threshold, is deprecated and always 1, so it is left out.
			ir.A("health_check_custom_config", ir.B(ir.Attrs{})),
		},
	})

	svc, ok := ctx.Resource(to.Primary)
	if !ok {
		ctx.Fail(fmt.Sprintf("service '%s' has no %s to register", to.Node.Name, to.Primary.Type))
	}
	svc.Args.Set("service_registries", ir.B(ir.Attrs{
		ir.A("registry_arn", ir.R(discovery, ir.Field("arn"))),
	}))
}

func ensureNamespace(ctx *resolve.Context) ir.ID {
	if id, ok := ctx.Scratch[discoveryLabel].(ir.ID); ok {
		return id
	}
	network := ensureNetwork(ctx)
	ns := ctx.Add(ir.Resource{
		Type:        "aws_service_discovery_private_dns_namespace",
		Name:        "main",
		SourceLabel: networkLabel,
		Args: ir.Attrs{
			ir.A("name", ir.Str(ctx.Prefix()+".local")),
			ir.A("description", ir.Str("Private DNS for "+ctx.Prefix())),
			ir.A("vpc", ir.R(network.VPC, ir.Field("id"))),
		},
	})
	id := ir.ID{Type: ns.Type, Name: ns.Name}
	ctx.Scratch[discoveryLabel] = id
	return id
}
