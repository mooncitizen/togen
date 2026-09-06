package gcp

import (
	"fmt"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

const (
	objectViewerRole = "roles/storage.objectViewer"
	objectUserRole   = "roles/storage.objectUser"
	bucketNameMax    = 63
	gcpProjectIDMax  = 30
)

func resolveBucket(ctx *resolve.Context, node ir.Node) *resolve.Handle {
	p := resolve.Props[ir.BucketProps](ctx, node)
	local := ctx.Local(node.Name)
	named := ctx.Named(node.Name)

	// Bucket names are global, so the GCP project id goes in front, as on the source bucket. The
	// id is not known here and can be 30 characters, so the rest is checked against what is left.
	if room := bucketNameMax - gcpProjectIDMax - 1; len(named) > room {
		ctx.Report(ir.ValidationError{
			NodeID: node.ID,
			Message: fmt.Sprintf(
				"google_storage_bucket name '%s' is %d characters after the project id, the limit is %d. Shorten the project, environment or node name.",
				named, len(named), room),
		})
	}

	args := ir.Attrs{
		ir.A("name", ir.C(ir.V(projectVar), ir.Str("-"+named))),
		ir.A("location", ir.Str(ctx.Project.Region)),
		ir.A("uniform_bucket_level_access", ir.Bool(true)),
		ir.A("force_destroy", ir.Bool(false)),
	}
	if p.Versioning {
		args = append(args, ir.A("versioning", ir.B(ir.Attrs{ir.A("enabled", ir.Bool(true))})))
	}
	bucket := ctx.Add(ir.Resource{
		Type:        "google_storage_bucket",
		Name:        local,
		SourceNode:  node.ID,
		SourceLabel: node.Name,
		Args:        args,
	})
	bucketID := ir.ID{Type: bucket.Type, Name: bucket.Name}
	id := ir.R(bucketID, ir.Field("id"))
	name := ir.R(bucketID, ir.Field("name"))

	if p.Public {
		ctx.Add(ir.Resource{
			Type:        "google_storage_bucket_iam_member",
			Name:        local + "_public",
			SourceNode:  node.ID,
			SourceLabel: node.Name,
			Args: ir.Attrs{
				ir.A("bucket", id),
				ir.A("role", ir.Str(objectViewerRole)),
				ir.A("member", ir.Str("allUsers")),
			},
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
		Exports: resolve.BucketExports{ID: id, Name: name},
	}
}
