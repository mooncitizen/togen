package aws

import (
	"fmt"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

func resolveBucket(ctx *resolve.Context, node ir.Node) *resolve.Handle {
	p := resolve.Props[ir.BucketProps](ctx, node)
	local := ctx.Local(node.Name)

	// S3 names are global, so a fixed name collides the second time anyone generates the same
	// sketch. Terraform appends a unique suffix to the prefix instead.
	bucket := ctx.Add(ir.Resource{
		Type:        "aws_s3_bucket",
		Name:        local,
		SourceNode:  node.ID,
		SourceLabel: node.Name,
		Args: ir.Attrs{
			ir.A("bucket_prefix", ir.Str(ctx.Named(node.Name)+"-")),
			ir.A("force_destroy", ir.Bool(true)),
		},
	})
	bucketID := ir.ID{Type: bucket.Type, Name: bucket.Name}
	name := ir.R(bucketID, ir.Field("id"))

	// A new bucket is unversioned, so versioning off emits nothing rather than suspending
	// versioning that was never enabled.
	if p.Versioning {
		ctx.Add(ir.Resource{
			Type:        "aws_s3_bucket_versioning",
			Name:        local,
			SourceNode:  node.ID,
			SourceLabel: node.Name,
			Args: ir.Attrs{
				ir.A("bucket", name),
				ir.A("versioning_configuration", ir.B(ir.Attrs{
					ir.A("status", ir.Str("Enabled")),
				})),
			},
		})
	}

	ctx.Add(ir.Resource{
		Type:        "aws_s3_bucket_server_side_encryption_configuration",
		Name:        local,
		SourceNode:  node.ID,
		SourceLabel: node.Name,
		Args: ir.Attrs{
			ir.A("bucket", name),
			ir.A("rule", ir.B(ir.Attrs{
				ir.A("apply_server_side_encryption_by_default", ir.B(ir.Attrs{
					ir.A("sse_algorithm", ir.Str("AES256")),
				})),
			})),
		},
	})

	// Public access is granted by a bucket policy, never by an ACL, so the ACL flags stay on
	// whichever way the node is set.
	access := ctx.Add(ir.Resource{
		Type:        "aws_s3_bucket_public_access_block",
		Name:        local,
		SourceNode:  node.ID,
		SourceLabel: node.Name,
		Args: ir.Attrs{
			ir.A("bucket", name),
			ir.A("block_public_acls", ir.Bool(true)),
			ir.A("block_public_policy", ir.Bool(!p.Public)),
			ir.A("ignore_public_acls", ir.Bool(true)),
			ir.A("restrict_public_buckets", ir.Bool(!p.Public)),
		},
	})

	if p.Public {
		ctx.Add(ir.Resource{
			Type:        "aws_s3_bucket_policy",
			Name:        local,
			SourceNode:  node.ID,
			SourceLabel: node.Name,
			Args: ir.Attrs{
				ir.A("bucket", name),
				ir.A("policy", ir.J(ir.M(
					ir.A("Version", ir.Str("2012-10-17")),
					ir.A("Statement", ir.L(ir.M(
						ir.A("Sid", ir.Str("PublicRead")),
						ir.A("Effect", ir.Str("Allow")),
						ir.A("Principal", ir.Str("*")),
						ir.A("Action", ir.Str("s3:GetObject")),
						ir.A("Resource", ir.C(ir.R(bucketID, ir.Field("arn")), ir.Str("/*"))),
					))),
				))),
			},
			// AWS rejects a public policy while the public access block is still in force.
			DependsOn: []ir.ID{{Type: access.Type, Name: access.Name}},
		})
	}

	ctx.AddOutput(ir.Output{
		Name:        local + "_bucket",
		Description: fmt.Sprintf("Name of the %s bucket", node.Name),
		Value:       name,
	})

	return &resolve.Handle{
		Node:    node,
		Primary: bucketID,
		Exports: resolve.BucketExports{
			ARN:  ir.R(bucketID, ir.Field("arn")),
			Name: name,
		},
	}
}
