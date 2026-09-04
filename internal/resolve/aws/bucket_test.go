package aws

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

// Properties are given raw because versioning defaults to true and omitempty would drop a
// false written through BucketProps.
func setupBucket(t *testing.T, properties string) (*resolve.Context, *resolve.Handle) {
	t.Helper()
	ctx, project := newContext(t, []ir.Node{{
		ID:         "n7",
		Type:       ir.NodeBucket,
		Name:       "uploads",
		Properties: json.RawMessage(properties),
	}}, nil)
	return ctx, resolveBucket(ctx, project.Nodes[0])
}

var (
	bucketID     = ir.ID{Type: "aws_s3_bucket", Name: "uploads"}
	bucketAccess = ir.ID{Type: "aws_s3_bucket_public_access_block", Name: "uploads"}
	bucketName   = ir.R(bucketID, ir.Field("id"))
)

func TestBucketEmitsAPrivateEncryptedVersionedBucket(t *testing.T) {
	ctx, handle := setupBucket(t, "{}")

	bucket := named(t, ctx, bucketID)
	if bucket.SourceNode != "n7" || bucket.SourceLabel != "uploads" {
		t.Errorf("bucket source = %q/%q", bucket.SourceNode, bucket.SourceLabel)
	}
	want := ir.Attrs{
		ir.A("bucket_prefix", ir.Str("shop-dev-uploads-")),
		ir.A("force_destroy", ir.Bool(true)),
	}
	if diff := cmp.Diff(want, bucket.Args); diff != "" {
		t.Errorf("bucket args (-want +got):\n%s", diff)
	}

	wantVersioning := ir.Attrs{
		ir.A("bucket", bucketName),
		ir.A("versioning_configuration", ir.B(ir.Attrs{ir.A("status", ir.Str("Enabled"))})),
	}
	versioning := named(t, ctx, ir.ID{Type: "aws_s3_bucket_versioning", Name: "uploads"})
	if diff := cmp.Diff(wantVersioning, versioning.Args); diff != "" {
		t.Errorf("versioning args (-want +got):\n%s", diff)
	}

	wantEncryption := ir.Attrs{
		ir.A("bucket", bucketName),
		ir.A("rule", ir.B(ir.Attrs{
			ir.A("apply_server_side_encryption_by_default", ir.B(ir.Attrs{
				ir.A("sse_algorithm", ir.Str("AES256")),
			})),
		})),
	}
	encryption := named(t, ctx, ir.ID{
		Type: "aws_s3_bucket_server_side_encryption_configuration",
		Name: "uploads",
	})
	if diff := cmp.Diff(wantEncryption, encryption.Args); diff != "" {
		t.Errorf("encryption args (-want +got):\n%s", diff)
	}

	wantAccess := ir.Attrs{
		ir.A("bucket", bucketName),
		ir.A("block_public_acls", ir.Bool(true)),
		ir.A("block_public_policy", ir.Bool(true)),
		ir.A("ignore_public_acls", ir.Bool(true)),
		ir.A("restrict_public_buckets", ir.Bool(true)),
	}
	if diff := cmp.Diff(wantAccess, named(t, ctx, bucketAccess).Args); diff != "" {
		t.Errorf("public access block args (-want +got):\n%s", diff)
	}

	if countOfType(ctx, "aws_s3_bucket_policy") != 0 {
		t.Error("a private bucket was given a policy")
	}
	if countOfType(ctx, "aws_vpc") != 0 {
		t.Error("a bucket pulled in the network")
	}

	wantOutputs := []ir.Output{{
		Name:        "uploads_bucket",
		Description: "Name of the uploads bucket",
		Value:       bucketName,
	}}
	if diff := cmp.Diff(wantOutputs, ctx.Outputs); diff != "" {
		t.Errorf("outputs (-want +got):\n%s", diff)
	}

	wantExports := resolve.BucketExports{ARN: ir.R(bucketID, ir.Field("arn")), Name: bucketName}
	if diff := cmp.Diff(resolve.Exports(wantExports), handle.Exports); diff != "" {
		t.Errorf("exports (-want +got):\n%s", diff)
	}
	if handle.Primary != bucketID {
		t.Errorf("primary = %s", handle.Primary)
	}
}

func TestBucketWithoutVersioningEmitsNoVersioningResource(t *testing.T) {
	ctx, _ := setupBucket(t, `{"versioning":false}`)

	if got := countOfType(ctx, "aws_s3_bucket_versioning"); got != 0 {
		t.Errorf("versioning resources = %d", got)
	}
	if got := countOfType(ctx, "aws_s3_bucket"); got != 1 {
		t.Errorf("buckets = %d", got)
	}
}

func TestPublicBucketOpensThePolicyAndKeepsACLsBlocked(t *testing.T) {
	ctx, _ := setupBucket(t, `{"public":true}`)

	wantAccess := ir.Attrs{
		ir.A("bucket", bucketName),
		ir.A("block_public_acls", ir.Bool(true)),
		ir.A("block_public_policy", ir.Bool(false)),
		ir.A("ignore_public_acls", ir.Bool(true)),
		ir.A("restrict_public_buckets", ir.Bool(false)),
	}
	if diff := cmp.Diff(wantAccess, named(t, ctx, bucketAccess).Args); diff != "" {
		t.Errorf("public access block args (-want +got):\n%s", diff)
	}

	policy := named(t, ctx, ir.ID{Type: "aws_s3_bucket_policy", Name: "uploads"})
	if policy.SourceNode != "n7" || policy.SourceLabel != "uploads" {
		t.Errorf("policy source = %q/%q", policy.SourceNode, policy.SourceLabel)
	}
	want := ir.Attrs{
		ir.A("bucket", bucketName),
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
	}
	if diff := cmp.Diff(want, policy.Args); diff != "" {
		t.Errorf("policy args (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]ir.ID{bucketAccess}, policy.DependsOn); diff != "" {
		t.Errorf("policy depends_on (-want +got):\n%s", diff)
	}
}

func TestResolveRejectsBucketNamesThatExceedTheS3PrefixLimit(t *testing.T) {
	p := example()
	p.Nodes = []ir.Node{{ID: "n7", Type: ir.NodeBucket, Name: strings.Repeat("u", 30)}}
	p.Edges = nil
	errs := runErrors(t, p)
	if len(errs) != 1 {
		t.Fatalf("errors = %v", errs)
	}
	if errs[0].NodeID != "n7" || !strings.Contains(errs[0].Message, "37") {
		t.Errorf("error = %+v", errs[0])
	}
}
