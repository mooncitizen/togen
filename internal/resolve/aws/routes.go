package aws

import (
	"fmt"
	"regexp"
	"strings"

	"togen/internal/ir"
	"togen/internal/resolve"
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

const routesLabel = "routes"

type routeKey struct {
	gateway string
	method  ir.Method
	path    string
}

func routeOwners(ctx *resolve.Context) map[routeKey]string {
	owners, ok := ctx.Scratch[routesLabel].(map[routeKey]string)
	if !ok {
		owners = map[routeKey]string{}
		ctx.Scratch[routesLabel] = owners
	}
	return owners
}

func resolveRoutes(ctx *resolve.Context, edge ir.Edge, from, to *resolve.Handle) {
	gateway, ok := from.Exports.(resolve.GatewayExports)
	if !ok {
		ctx.Fail(fmt.Sprintf("edge '%s' routes from a %s, which has no gateway exports", edge.ID, from.Node.Type))
	}
	target, ok := to.Exports.(resolve.FunctionExports)
	if !ok {
		ctx.Report(ir.ValidationError{
			EdgeID:  edge.ID,
			Message: fmt.Sprintf("routes to a %s are not supported by the aws resolver yet", to.Node.Type),
		})
		return
	}

	source := from.Node.ID
	label := from.Node.Name
	base := ctx.Local(from.Node.Name) + "_" + ctx.Local(to.Node.Name)

	integrationID := ir.ID{Type: "aws_apigatewayv2_integration", Name: base}
	if !ctx.HasResource(integrationID) {
		ctx.Add(ir.Resource{
			Type:        integrationID.Type,
			Name:        integrationID.Name,
			SourceNode:  source,
			SourceLabel: label,
			Args: ir.Attrs{
				ir.A("api_id", gateway.APIID),
				ir.A("integration_type", ir.Str("AWS_PROXY")),
				ir.A("integration_uri", target.InvokeARN),
				ir.A("integration_method", ir.Str("POST")),
				ir.A("payload_format_version", ir.Str("2.0")),
			},
		})
	}

	owners := routeOwners(ctx)
	for _, method := range edge.Properties.Methods {
		key := routeKey{gateway: source, method: method, path: edge.Properties.Path}
		if owner, taken := owners[key]; taken {
			ctx.Report(ir.ValidationError{
				EdgeID: edge.ID,
				Message: fmt.Sprintf("route '%s %s' on gateway '%s' is already used by edge '%s'",
					method, edge.Properties.Path, from.Node.Name, owner),
			})
			continue
		}
		owners[key] = edge.ID
		name := uniqueRouteName(ctx, fmt.Sprintf("%s_%s_%s", base, strings.ToLower(string(method)), pathSlug(edge.Properties.Path)))
		ctx.Add(ir.Resource{
			Type:        "aws_apigatewayv2_route",
			Name:        name,
			SourceNode:  source,
			SourceLabel: label,
			Args: ir.Attrs{
				ir.A("api_id", gateway.APIID),
				ir.A("route_key", ir.Str(fmt.Sprintf("%s %s", method, edge.Properties.Path))),
				ir.A("target", ir.C(ir.Str("integrations/"), ir.R(integrationID, ir.Field("id")))),
			},
		})
	}

	permissionID := ir.ID{Type: "aws_lambda_permission", Name: base}
	if !ctx.HasResource(permissionID) {
		ctx.Add(ir.Resource{
			Type:        permissionID.Type,
			Name:        permissionID.Name,
			SourceNode:  source,
			SourceLabel: label,
			Args: ir.Attrs{
				ir.A("statement_id", ir.Str("AllowInvokeFromApiGateway")),
				ir.A("action", ir.Str("lambda:InvokeFunction")),
				ir.A("function_name", target.FunctionName),
				ir.A("principal", ir.Str("apigateway.amazonaws.com")),
				ir.A("source_arn", ir.C(gateway.ExecutionARN, ir.Str("/*/*"))),
			},
		})
	}
}
