package aws

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

var (
	pathBraces = regexp.MustCompile(`[{}]`)
	pathRuns   = regexp.MustCompile(`[^a-zA-Z0-9]+`)
	pathTrim   = regexp.MustCompile(`^_+|_+$`)
)

func pathSlug(path string) string {
	if path == "/" {
		return "root"
	}
	s := pathBraces.ReplaceAllString(path, "")
	s = pathRuns.ReplaceAllString(s, "_")
	s = pathTrim.ReplaceAllString(s, "")
	return strings.ToLower(s)
}

func uniqueRouteName(ctx *resolve.Context, name string) string {
	if !ctx.HasResource(ir.ID{Type: "aws_apigatewayv2_route", Name: name}) {
		return name
	}
	for n := 2; ; n++ {
		candidate := fmt.Sprintf("%s_%d", name, n)
		if !ctx.HasResource(ir.ID{Type: "aws_apigatewayv2_route", Name: candidate}) {
			return candidate
		}
	}
}

func resolveRoutes(ctx *resolve.Context, edge ir.Edge, from, to *resolve.Handle) {
	gateway, ok := from.Exports.(resolve.GatewayExports)
	if !ok {
		ctx.Fail(fmt.Sprintf("edge '%s' routes from a %s, which has no gateway exports", edge.ID, from.Node.Type))
	}
	base := ctx.Local(from.Node.Name) + "_" + ctx.Local(to.Node.Name)
	integrationID := ir.ID{Type: "aws_apigatewayv2_integration", Name: base}

	switch target := to.Exports.(type) {
	case resolve.FunctionExports:
		proxyToFunction(ctx, from, gateway, target, integrationID)
		addRoutes(ctx, edge, from, gateway, integrationID, base)
		allowGatewayToInvoke(ctx, from, gateway, target, base)
	case resolve.ServiceExports:
		proxyToService(ctx, from, to, gateway, target, integrationID)
		addRoutes(ctx, edge, from, gateway, integrationID, base)
	default:
		ctx.Report(ir.ValidationError{
			EdgeID:  edge.ID,
			Message: fmt.Sprintf("routes to a %s are not supported by the aws resolver yet", to.Node.Type),
		})
	}
}

func proxyToFunction(
	ctx *resolve.Context,
	from *resolve.Handle,
	gateway resolve.GatewayExports,
	target resolve.FunctionExports,
	integrationID ir.ID,
) {
	if ctx.HasResource(integrationID) {
		return
	}
	ctx.Add(ir.Resource{
		Type:        integrationID.Type,
		Name:        integrationID.Name,
		SourceNode:  from.Node.ID,
		SourceLabel: from.Node.Name,
		Args: ir.Attrs{
			ir.A("api_id", gateway.APIID),
			ir.A("integration_type", ir.Str("AWS_PROXY")),
			ir.A("integration_uri", target.InvokeARN),
			ir.A("integration_method", ir.Str("POST")),
			ir.A("payload_format_version", ir.Str("2.0")),
		},
	})
}

// A service is reached over the network, not the AWS API, so the gateway needs a listener to point at and a VPC link into the private subnets.
func proxyToService(
	ctx *resolve.Context,
	from, to *resolve.Handle,
	gateway resolve.GatewayExports,
	target resolve.ServiceExports,
	integrationID ir.ID,
) {
	albSG, balanced := ensureLoadBalancer(ctx, to, !target.Public)
	link := ensureVPCLink(ctx)

	ingressID := ir.ID{
		Type: "aws_vpc_security_group_ingress_rule",
		Name: ctx.Local(to.Node.Name) + "_alb_from_vpc_link",
	}
	if !ctx.HasResource(ingressID) {
		ctx.Add(ir.Resource{
			Type:        ingressID.Type,
			Name:        ingressID.Name,
			SourceNode:  to.Node.ID,
			SourceLabel: to.Node.Name,
			Args: ir.Attrs{
				ir.A("security_group_id", ir.R(albSG, ir.Field("id"))),
				ir.A("referenced_security_group_id", ir.R(link.SecurityGroup, ir.Field("id"))),
				ir.A("from_port", ir.Num(80)),
				ir.A("to_port", ir.Num(80)),
				ir.A("ip_protocol", ir.Str("tcp")),
				ir.A("description", ir.Str("Gateway to "+to.Node.Name)),
			},
		})
	}

	if ctx.HasResource(integrationID) {
		return
	}
	ctx.Add(ir.Resource{
		Type:        integrationID.Type,
		Name:        integrationID.Name,
		SourceNode:  from.Node.ID,
		SourceLabel: from.Node.Name,
		Args: ir.Attrs{
			ir.A("api_id", gateway.APIID),
			ir.A("integration_type", ir.Str("HTTP_PROXY")),
			ir.A("integration_uri", balanced.ListenerARN),
			ir.A("integration_method", ir.Str("ANY")),
			ir.A("connection_type", ir.Str("VPC_LINK")),
			ir.A("connection_id", ir.R(link.Link, ir.Field("id"))),
			ir.A("payload_format_version", ir.Str("1.0")),
		},
	})
}

func addRoutes(
	ctx *resolve.Context,
	edge ir.Edge,
	from *resolve.Handle,
	gateway resolve.GatewayExports,
	integrationID ir.ID,
	base string,
) {
	for _, method := range edge.Properties.Methods {
		if owner, ok := ctx.ClaimRoute(from.Node.ID, method, edge.Properties.Path, edge.ID); !ok {
			ctx.Report(ir.ValidationError{
				EdgeID: edge.ID,
				Message: fmt.Sprintf("route '%s %s' on gateway '%s' is already used by edge '%s'",
					method, edge.Properties.Path, from.Node.Name, owner),
			})
			continue
		}
		name := uniqueRouteName(ctx, fmt.Sprintf("%s_%s_%s", base, strings.ToLower(string(method)), pathSlug(edge.Properties.Path)))
		ctx.Add(ir.Resource{
			Type:        "aws_apigatewayv2_route",
			Name:        name,
			SourceNode:  from.Node.ID,
			SourceLabel: from.Node.Name,
			Args: ir.Attrs{
				ir.A("api_id", gateway.APIID),
				ir.A("route_key", ir.Str(fmt.Sprintf("%s %s", method, edge.Properties.Path))),
				ir.A("target", ir.C(ir.Str("integrations/"), ir.R(integrationID, ir.Field("id")))),
			},
		})
	}
}

func allowGatewayToInvoke(
	ctx *resolve.Context,
	from *resolve.Handle,
	gateway resolve.GatewayExports,
	target resolve.FunctionExports,
	base string,
) {
	permissionID := ir.ID{Type: "aws_lambda_permission", Name: base}
	if ctx.HasResource(permissionID) {
		return
	}
	ctx.Add(ir.Resource{
		Type:        permissionID.Type,
		Name:        permissionID.Name,
		SourceNode:  from.Node.ID,
		SourceLabel: from.Node.Name,
		Args: ir.Attrs{
			ir.A("statement_id", ir.Str("AllowInvokeFromApiGateway")),
			ir.A("action", ir.Str("lambda:InvokeFunction")),
			ir.A("function_name", target.FunctionName),
			ir.A("principal", ir.Str("apigateway.amazonaws.com")),
			ir.A("source_arn", ir.C(gateway.ExecutionARN, ir.Str("/*/*"))),
		},
	})
}

type vpcLink struct{ Link, SecurityGroup ir.ID }

const vpcLinkLabel = "vpc_link"

func ensureVPCLink(ctx *resolve.Context) vpcLink {
	if l, ok := ctx.Scratch[vpcLinkLabel].(vpcLink); ok {
		return l
	}
	network := ensureNetwork(ctx)
	local := ctx.Local(ctx.Prefix()) + "_vpc_link"
	sg := ctx.Add(ir.Resource{
		Type:        "aws_security_group",
		Name:        local,
		SourceLabel: networkLabel,
		Args: ir.Attrs{
			ir.A("name", ir.Str(ctx.Named("vpc-link"))),
			ir.A("description", ir.Str("Gateway access to load balancers in "+ctx.Prefix())),
			ir.A("vpc_id", ir.R(network.VPC, ir.Field("id"))),
		},
	})
	sgID := ir.ID{Type: sg.Type, Name: sg.Name}
	ctx.Add(ir.Resource{
		Type:        "aws_vpc_security_group_egress_rule",
		Name:        local + "_all",
		SourceLabel: networkLabel,
		Args: ir.Attrs{
			ir.A("security_group_id", ir.R(sgID, ir.Field("id"))),
			ir.A("ip_protocol", ir.Str("-1")),
			ir.A("cidr_ipv4", ir.Str("0.0.0.0/0")),
		},
	})
	link := ctx.Add(ir.Resource{
		Type:        "aws_apigatewayv2_vpc_link",
		Name:        "main",
		SourceLabel: networkLabel,
		Args: ir.Attrs{
			ir.A("name", ir.Str(ctx.Prefix())),
			ir.A("subnet_ids", subnetRefs(network.PrivateSubnets)),
			ir.A("security_group_ids", ir.L(ir.R(sgID, ir.Field("id")))),
		},
	})
	l := vpcLink{Link: ir.ID{Type: link.Type, Name: link.Name}, SecurityGroup: sgID}
	ctx.Scratch[vpcLinkLabel] = l
	return l
}
