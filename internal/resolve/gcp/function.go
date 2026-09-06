package gcp

import (
	"fmt"
	"strings"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

var runtimes = map[ir.Runtime]string{
	ir.RuntimeNode:   "nodejs22",
	ir.RuntimePython: "python313",
	ir.RuntimeGo:     "go126",
}

const (
	functionsLabel  = "functions"
	accountIDMax    = 30
	functionNameMax = 63
)

func resolveFunction(ctx *resolve.Context, node ir.Node) *resolve.Handle {
	p := resolve.Props[ir.FunctionProps](ctx, node)
	runtime, ok := runtimes[p.Runtime]
	if !ok {
		ctx.Fail(fmt.Sprintf("function '%s' has an unknown runtime '%s'", node.Name, p.Runtime))
	}
	memory, ok := functionMemory[p.Size]
	if !ok {
		ctx.Fail(fmt.Sprintf("function '%s' has an unknown size '%s'", node.Name, p.Size))
	}
	entry := entryPoint(p.Handler)
	if entry == "" {
		ctx.Report(ir.ValidationError{
			NodeID:  node.ID,
			Message: fmt.Sprintf("function '%s' has a handler '%s' that names no function", node.Name, p.Handler),
		})
	}
	local := ctx.Local(node.Name)
	bucket := ensureSourceBucket(ctx)

	account := ctx.Add(ir.Resource{
		Type:        "google_service_account",
		Name:        local,
		SourceNode:  node.ID,
		SourceLabel: node.Name,
		Args: ir.Attrs{
			ir.A("account_id", ir.Str(ctx.Named(node.Name))),
			ir.A("display_name", ir.Str(ctx.Named(node.Name))),
		},
	})
	accountID := ir.ID{Type: account.Type, Name: account.Name}

	packageVar := local + "_package"
	ctx.AddVariable(ir.Variable{
		Name:        packageVar,
		Description: fmt.Sprintf("Path to the deployment package for the %s function", node.Name),
		Type:        "string",
		Default:     ir.Str(fmt.Sprintf("functions/%s.zip", node.Name)),
	})
	object := ctx.Add(ir.Resource{
		Type:        "google_storage_bucket_object",
		Name:        local,
		SourceNode:  node.ID,
		SourceLabel: node.Name,
		Args: ir.Attrs{
			ir.A("name", ir.Str(node.Name+".zip")),
			ir.A("bucket", ir.R(bucket, ir.Field("name"))),
			ir.A("source", ir.V(packageVar)),
		},
	})
	objectID := ir.ID{Type: object.Type, Name: object.Name}

	serviceConfig := ir.Attrs{
		ir.A("available_memory", ir.Str(memory)),
		ir.A("timeout_seconds", ir.Num(float64(p.TimeoutSeconds))),
		ir.A("service_account_email", ir.R(accountID, ir.Field("email"))),
	}
	fn := ctx.Add(ir.Resource{
		Type:        "google_cloudfunctions2_function",
		Name:        local,
		SourceNode:  node.ID,
		SourceLabel: node.Name,
		Args: ir.Attrs{
			ir.A("name", ir.Str(ctx.Named(node.Name))),
			ir.A("location", ir.Str(ctx.Project.Region)),
			ir.A("build_config", ir.B(ir.Attrs{
				ir.A("runtime", ir.Str(runtime)),
				ir.A("entry_point", ir.Str(entry)),
				ir.A("source", ir.B(ir.Attrs{
					// A new zip is a new generation, so the function is rebuilt the way
					// source_code_hash rebuilds a lambda.
					ir.A("storage_source", ir.B(ir.Attrs{
						ir.A("bucket", ir.R(bucket, ir.Field("name"))),
						ir.A("object", ir.R(objectID, ir.Field("name"))),
						ir.A("generation", ir.R(objectID, ir.Field("generation"))),
					})),
				})),
			})),
			ir.A("service_config", ir.B(serviceConfig)),
		},
	})
	fnID := ir.ID{Type: fn.Type, Name: fn.Name}

	h := &resolve.Handle{
		Node:    node,
		Primary: fnID,
		Env:     resolve.SortedEnv(p.Env),
		Exports: resolve.CloudRunExports{
			Service:        ir.R(fnID, ir.Field("name")),
			Location:       ir.R(fnID, ir.Field("location")),
			URL:            ir.R(fnID, ir.Field("service_config"), ir.Index(0), ir.Field("uri")),
			ServiceAccount: ir.R(accountID, ir.Field("email")),
		},
	}
	h.Finalise = func() {
		if len(h.Env) > 0 {
			serviceConfig.Set("environment_variables", ir.Map(h.Env))
		}
		if h.NeedsNetwork {
			network := ensureNetwork(ctx)
			serviceConfig.Set("vpc_connector", ir.R(network.Connector, ir.Field("id")))
			serviceConfig.Set("vpc_connector_egress_settings", ir.Str("PRIVATE_RANGES_ONLY"))
		}
		fn.Args.Set("service_config", ir.B(serviceConfig))
	}
	return h
}

// Cloud Functions looks the entry point up by name in the source's main module, so the file
// in a Lambda style handler such as index.handler has nothing to say.
func entryPoint(handler string) string {
	if i := strings.LastIndex(handler, "."); i >= 0 {
		return handler[i+1:]
	}
	return handler
}

// Bucket names are global, so the GCP project id goes in front of the prefix.
func ensureSourceBucket(ctx *resolve.Context) ir.ID {
	if id, ok := ctx.Scratch[functionsLabel].(ir.ID); ok {
		return id
	}
	bucket := ctx.Add(ir.Resource{
		Type:        "google_storage_bucket",
		Name:        ctx.Local(ctx.Prefix()) + "_" + functionsLabel,
		SourceLabel: functionsLabel,
		Args: ir.Attrs{
			ir.A("name", ir.C(ir.V(projectVar), ir.Str("-"+ctx.Prefix()+"-"+functionsLabel))),
			ir.A("location", ir.Str(ctx.Project.Region)),
			ir.A("uniform_bucket_level_access", ir.Bool(true)),
			ir.A("force_destroy", ir.Bool(true)),
		},
	})
	id := ir.ID{Type: bucket.Type, Name: bucket.Name}
	ctx.Scratch[functionsLabel] = id
	return id
}
