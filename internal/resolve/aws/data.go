package aws

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

func resolveDataAccess(ctx *resolve.Context, edge ir.Edge, from, to *resolve.Handle) {
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
	case resolve.DatabaseExports:
		connectDatabase(ctx, from, to, target)
	case resolve.BucketExports:
		grantBucket(ctx, edge, from, to, target)
	default:
		ctx.Report(ir.ValidationError{
			EdgeID:  edge.ID,
			Message: fmt.Sprintf("%s to a %s is not supported by the aws resolver yet", edge.Relation, to.Node.Type),
		})
	}
}

func connectDatabase(ctx *resolve.Context, from, to *resolve.Handle, target resolve.DatabaseExports) {
	if to.SecurityGroup == nil {
		ctx.Fail(fmt.Sprintf("database '%s' has no security group", to.Node.Name))
	}
	port, ok := target.Port.(ir.Number)
	if !ok {
		ctx.Fail(fmt.Sprintf("database '%s' exports a port that is not a number", to.Node.Name))
	}

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

	prefix := strings.ToUpper(ctx.Local(to.Node.Name))
	from.SetEnv(prefix+"_HOST", target.Host)
	from.SetEnv(prefix+"_PORT", ir.Str(strconv.FormatFloat(float64(port), 'f', -1, 64)))
	from.SetEnv(prefix+"_NAME", target.Name)
	from.SetEnv(prefix+"_SECRET_ARN", target.SecretARN)

	from.AddStatement(ir.M(
		ir.A("Effect", ir.Str("Allow")),
		ir.A("Action", ir.L(ir.Str("secretsmanager:GetSecretValue"))),
		ir.A("Resource", target.SecretARN),
	))
}

// A bucket is reached over the public S3 endpoint, so nothing here joins the VPC.
func grantBucket(ctx *resolve.Context, edge ir.Edge, from, to *resolve.Handle, target resolve.BucketExports) {
	objects := ir.C(target.ARN, ir.Str("/*"))
	if edge.Relation == ir.RelReads {
		from.AddStatement(ir.M(
			ir.A("Effect", ir.Str("Allow")),
			ir.A("Action", ir.L(ir.Str("s3:GetObject"))),
			ir.A("Resource", objects),
		))
		from.AddStatement(ir.M(
			ir.A("Effect", ir.Str("Allow")),
			ir.A("Action", ir.L(ir.Str("s3:ListBucket"))),
			ir.A("Resource", target.ARN),
		))
	} else {
		from.AddStatement(ir.M(
			ir.A("Effect", ir.Str("Allow")),
			ir.A("Action", ir.L(ir.Str("s3:PutObject"), ir.Str("s3:DeleteObject"))),
			ir.A("Resource", objects),
		))
	}
	from.SetEnv(strings.ToUpper(ctx.Local(to.Node.Name))+"_BUCKET", target.Name)
}
