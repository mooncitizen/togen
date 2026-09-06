package gcp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

func bucketNode(id, name, properties string) ir.Node {
	return ir.Node{ID: id, Type: ir.NodeBucket, Name: name, Properties: json.RawMessage(properties)}
}

// Properties are given raw because versioning defaults to true and omitempty would drop a
// false written through BucketProps.
func setupBucket(t *testing.T, properties string) (*resolve.Context, *resolve.Handle) {
	t.Helper()
	ctx, project := newContext(t, []ir.Node{bucketNode("n7", "uploads", properties)}, nil)
	return ctx, resolveBucket(ctx, project.Nodes[0])
}

var (
	bucketID   = ir.ID{Type: "google_storage_bucket", Name: "uploads"}
	bucketRef  = ir.R(bucketID, ir.Field("id"))
	bucketName = ir.R(bucketID, ir.Field("name"))
)

func TestBucketEmitsAVersionedBucketWithUniformAccessNamedAfterTheProject(t *testing.T) {
	ctx, handle := setupBucket(t, "{}")

	if diff := cmp.Diff([]string{"google_storage_bucket"}, resourceTypes(ctx)); diff != "" {
		t.Errorf("resources (-want +got):\n%s", diff)
	}
	bucket := named(t, ctx, bucketID)
	if bucket.SourceNode != "n7" || bucket.SourceLabel != "uploads" {
		t.Errorf("bucket source = %q/%q", bucket.SourceNode, bucket.SourceLabel)
	}
	want := ir.Attrs{
		ir.A("name", ir.C(ir.V("project"), ir.Str("-shop-dev-uploads"))),
		ir.A("location", ir.Str("europe-west2")),
		ir.A("uniform_bucket_level_access", ir.Bool(true)),
		ir.A("force_destroy", ir.Bool(false)),
		ir.A("versioning", ir.B(ir.Attrs{ir.A("enabled", ir.Bool(true))})),
	}
	if diff := cmp.Diff(want, bucket.Args); diff != "" {
		t.Errorf("bucket args (-want +got):\n%s", diff)
	}
	if countOfType(ctx, "google_compute_network") != 0 {
		t.Error("a bucket pulled in the network")
	}
	if len(ctx.Errors) != 0 {
		t.Errorf("errors = %v", ctx.Errors)
	}

	wantOutputs := []ir.Output{{
		Name:        "uploads_bucket",
		Description: "Name of the uploads bucket",
		Value:       bucketName,
	}}
	if diff := cmp.Diff(wantOutputs, ctx.Outputs); diff != "" {
		t.Errorf("outputs (-want +got):\n%s", diff)
	}
	wantExports := resolve.BucketExports{ID: bucketRef, Name: bucketName}
	if diff := cmp.Diff(resolve.Exports(wantExports), handle.Exports); diff != "" {
		t.Errorf("exports (-want +got):\n%s", diff)
	}
	if handle.Primary != bucketID {
		t.Errorf("primary = %s", handle.Primary)
	}
}

func TestBucketWithoutVersioningEmitsNoVersioningBlock(t *testing.T) {
	ctx, _ := setupBucket(t, `{"versioning":false}`)
	if _, ok := named(t, ctx, bucketID).Args.Get("versioning"); ok {
		t.Error("an unversioned bucket was given a versioning block")
	}
}

func TestPublicBucketGrantsObjectViewerToAllUsers(t *testing.T) {
	ctx, _ := setupBucket(t, `{"public":true}`)

	grant := named(t, ctx, ir.ID{Type: "google_storage_bucket_iam_member", Name: "uploads_public"})
	if grant.SourceNode != "n7" || grant.SourceLabel != "uploads" {
		t.Errorf("grant source = %q/%q", grant.SourceNode, grant.SourceLabel)
	}
	want := ir.Attrs{
		ir.A("bucket", bucketRef),
		ir.A("role", ir.Str("roles/storage.objectViewer")),
		ir.A("member", ir.Str("allUsers")),
	}
	if diff := cmp.Diff(want, grant.Args); diff != "" {
		t.Errorf("grant args (-want +got):\n%s", diff)
	}
}

func TestPrivateBucketHasNoPublicGrant(t *testing.T) {
	ctx, _ := setupBucket(t, "{}")
	if got := countOfType(ctx, "google_storage_bucket_iam_member"); got != 0 {
		t.Errorf("grants = %d", got)
	}
}

func TestBucketNameLeavesRoomForTheLongestProjectID(t *testing.T) {
	p := newProject(t, []ir.Node{bucketNode("n7", strings.Repeat("u", 20), "{}")}, nil)
	p.Name = "a-project-name-that-is-long-too"
	ctx := resolve.NewContext(p)
	resolveBucket(ctx, p.Nodes[0])

	want := ir.Errors{{
		NodeID: "n7",
		Message: "google_storage_bucket name 'a-project-name-that-is-long-too-dev-uuuuuuuuuuuuuuuuuuuu' is 56 " +
			"characters after the project id, the limit is 32. Shorten the project, environment or node name.",
	}}
	if diff := cmp.Diff(want, ctx.Errors); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
}

func TestBucketNameAtTheLimitIsAccepted(t *testing.T) {
	p := newProject(t, []ir.Node{bucketNode("n7", strings.Repeat("u", 32-len("shop-dev-")), "{}")}, nil)
	ctx := resolve.NewContext(p)
	resolveBucket(ctx, p.Nodes[0])
	if len(ctx.Errors) != 0 {
		t.Errorf("errors = %v", ctx.Errors)
	}
}
