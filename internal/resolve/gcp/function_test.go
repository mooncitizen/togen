package gcp

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

func props(t *testing.T, v any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal properties: %v", err)
	}
	return raw
}

func functionNode(t *testing.T, id, name string, p ir.FunctionProps) ir.Node {
	t.Helper()
	return ir.Node{ID: id, Type: ir.NodeFunction, Name: name, Properties: props(t, p)}
}

func setupFunction(t *testing.T, p ir.FunctionProps) (*resolve.Context, *resolve.Handle) {
	t.Helper()
	ctx, project := newContext(t, []ir.Node{functionNode(t, "n2", "handler", p)}, nil)
	return ctx, resolveFunction(ctx, project.Nodes[0])
}

var defaultFunction = ir.FunctionProps{
	Runtime:        ir.RuntimeNode,
	Handler:        "index.handler",
	Size:           ir.SizeSmall,
	TimeoutSeconds: 30,
}

var (
	fnID        = ir.ID{Type: "google_cloudfunctions2_function", Name: "handler"}
	fnAccountID = ir.ID{Type: "google_service_account", Name: "handler"}
	fnObjectID  = ir.ID{Type: "google_storage_bucket_object", Name: "handler"}
	sourceID    = ir.ID{Type: "google_storage_bucket", Name: "shop_dev_functions"}
	connectorID = ir.ID{Type: "google_vpc_access_connector", Name: "main"}
)

func serviceConfig(t *testing.T, ctx *resolve.Context) ir.Attrs {
	t.Helper()
	v, ok := firstOfType(t, ctx, "google_cloudfunctions2_function").Args.Get("service_config")
	if !ok {
		t.Fatal("the function has no service_config")
	}
	block, ok := v.(ir.Block)
	if !ok || len(block) != 1 {
		t.Fatalf("service_config = %#v", v)
	}
	return block[0]
}

func TestFunctionEmitsAnAccountASourceObjectAndAFunction(t *testing.T) {
	p := defaultFunction
	p.Env = map[string]string{"LOG_LEVEL": "info", "APP_NAME": "shop"}
	ctx, handle := setupFunction(t, p)
	handle.Finalise()

	wantTypes := []string{
		"google_storage_bucket",
		"google_service_account",
		"google_storage_bucket_object",
		"google_cloudfunctions2_function",
	}
	if diff := cmp.Diff(wantTypes, resourceTypes(ctx)); diff != "" {
		t.Errorf("resources (-want +got):\n%s", diff)
	}

	fn := firstOfType(t, ctx, "google_cloudfunctions2_function")
	want := ir.Attrs{
		ir.A("name", ir.Str("shop-dev-handler")),
		ir.A("location", ir.Str("europe-west2")),
		ir.A("build_config", ir.B(ir.Attrs{
			ir.A("runtime", ir.Str("nodejs22")),
			ir.A("entry_point", ir.Str("handler")),
			ir.A("source", ir.B(ir.Attrs{
				ir.A("storage_source", ir.B(ir.Attrs{
					ir.A("bucket", ir.R(sourceID, ir.Field("name"))),
					ir.A("object", ir.R(fnObjectID, ir.Field("name"))),
					ir.A("generation", ir.R(fnObjectID, ir.Field("generation"))),
				})),
			})),
		})),
		ir.A("service_config", ir.B(ir.Attrs{
			ir.A("available_memory", ir.Str("512Mi")),
			ir.A("timeout_seconds", ir.Num(30)),
			ir.A("service_account_email", ir.R(fnAccountID, ir.Field("email"))),
			ir.A("environment_variables", ir.M(
				ir.A("APP_NAME", ir.Str("shop")),
				ir.A("LOG_LEVEL", ir.Str("info")),
			)),
		})),
	}
	if diff := cmp.Diff(want, fn.Args); diff != "" {
		t.Errorf("function args (-want +got):\n%s", diff)
	}
	if fn.SourceNode != "n2" || fn.SourceLabel != "handler" {
		t.Errorf("function source = %q/%q", fn.SourceNode, fn.SourceLabel)
	}

	account := firstOfType(t, ctx, "google_service_account")
	wantAccount := ir.Attrs{
		ir.A("account_id", ir.Str("shop-dev-handler")),
		ir.A("display_name", ir.Str("shop-dev-handler")),
	}
	if diff := cmp.Diff(wantAccount, account.Args); diff != "" {
		t.Errorf("service account args (-want +got):\n%s", diff)
	}

	bucket := firstOfType(t, ctx, "google_storage_bucket")
	if bucket.Name != sourceID.Name || bucket.SourceNode != "" || bucket.SourceLabel != "functions" {
		t.Errorf("bucket = %s, source %q/%q", bucket.Name, bucket.SourceNode, bucket.SourceLabel)
	}
	wantBucket := ir.Attrs{
		ir.A("name", ir.C(ir.V("project"), ir.Str("-shop-dev-functions"))),
		ir.A("location", ir.Str("europe-west2")),
		ir.A("uniform_bucket_level_access", ir.Bool(true)),
		ir.A("force_destroy", ir.Bool(true)),
	}
	if diff := cmp.Diff(wantBucket, bucket.Args); diff != "" {
		t.Errorf("bucket args (-want +got):\n%s", diff)
	}

	object := firstOfType(t, ctx, "google_storage_bucket_object")
	wantObject := ir.Attrs{
		ir.A("name", ir.Str("handler.zip")),
		ir.A("bucket", ir.R(sourceID, ir.Field("name"))),
		ir.A("source", ir.V("handler_package")),
	}
	if diff := cmp.Diff(wantObject, object.Args); diff != "" {
		t.Errorf("object args (-want +got):\n%s", diff)
	}

	wantVars := []ir.Variable{{
		Name:        "handler_package",
		Description: "Path to the deployment package for the handler function",
		Type:        "string",
		Default:     ir.Str("functions/handler.zip"),
	}}
	if diff := cmp.Diff(wantVars, ctx.Variables); diff != "" {
		t.Errorf("variables (-want +got):\n%s", diff)
	}
	if countOfType(ctx, "google_compute_network") != 0 {
		t.Error("a network was created without an edge asking for one")
	}
	if len(ctx.Outputs) != 0 {
		t.Errorf("outputs = %+v", ctx.Outputs)
	}

	wantExports := resolve.CloudRunExports{
		Service:        ir.R(fnID, ir.Field("name")),
		Location:       ir.R(fnID, ir.Field("location")),
		URL:            ir.R(fnID, ir.Field("service_config"), ir.Index(0), ir.Field("uri")),
		ServiceAccount: ir.R(fnAccountID, ir.Field("email")),
	}
	if diff := cmp.Diff(wantExports, handle.Exports); diff != "" {
		t.Errorf("exports (-want +got):\n%s", diff)
	}
	if handle.Primary != fnID {
		t.Errorf("primary = %v", handle.Primary)
	}
}

func TestFunctionOmitsEnvironmentVariablesWhenThereAreNone(t *testing.T) {
	ctx, handle := setupFunction(t, defaultFunction)
	handle.Finalise()
	if _, ok := serviceConfig(t, ctx).Get("environment_variables"); ok {
		t.Error("environment_variables present with no variables")
	}
}

func TestFunctionTakesVariablesEdgesAddAtFinalise(t *testing.T) {
	ctx, handle := setupFunction(t, defaultFunction)
	handle.SetEnv("JOBS_TOPIC", ir.Str("jobs"))
	handle.Finalise()
	got, _ := serviceConfig(t, ctx).Get("environment_variables")
	want := ir.M(ir.A("JOBS_TOPIC", ir.Str("jobs")))
	if diff := cmp.Diff(ir.Value(want), got); diff != "" {
		t.Errorf("environment_variables (-want +got):\n%s", diff)
	}
}

func TestFunctionMapsSizesToMemory(t *testing.T) {
	for size, want := range map[ir.Size]string{
		ir.SizeSmall:  "512Mi",
		ir.SizeMedium: "1Gi",
		ir.SizeLarge:  "2Gi",
	} {
		p := defaultFunction
		p.Size = size
		ctx, handle := setupFunction(t, p)
		handle.Finalise()
		got, _ := serviceConfig(t, ctx).Get("available_memory")
		if diff := cmp.Diff(ir.Value(ir.Str(want)), got); diff != "" {
			t.Errorf("%s memory (-want +got):\n%s", size, diff)
		}
	}
}

func TestFunctionMapsRuntimesAndEntryPoints(t *testing.T) {
	for _, c := range []struct {
		runtime ir.Runtime
		handler string
		want    ir.Attrs
	}{
		{ir.RuntimeNode, "index.handler", ir.Attrs{ir.A("runtime", ir.Str("nodejs22")), ir.A("entry_point", ir.Str("handler"))}},
		{ir.RuntimePython, "main.app", ir.Attrs{ir.A("runtime", ir.Str("python313")), ir.A("entry_point", ir.Str("app"))}},
		{ir.RuntimeGo, "Handle", ir.Attrs{ir.A("runtime", ir.Str("go126")), ir.A("entry_point", ir.Str("Handle"))}},
	} {
		p := defaultFunction
		p.Runtime = c.runtime
		p.Handler = c.handler
		ctx, handle := setupFunction(t, p)
		handle.Finalise()
		v, _ := firstOfType(t, ctx, "google_cloudfunctions2_function").Args.Get("build_config")
		got := v.(ir.Block)[0][:2]
		if diff := cmp.Diff(c.want, got); diff != "" {
			t.Errorf("%s %s (-want +got):\n%s", c.runtime, c.handler, diff)
		}
	}
}

func TestFunctionSetsTheTimeout(t *testing.T) {
	p := defaultFunction
	p.TimeoutSeconds = 120
	ctx, handle := setupFunction(t, p)
	handle.Finalise()
	got, _ := serviceConfig(t, ctx).Get("timeout_seconds")
	if diff := cmp.Diff(ir.Value(ir.Num(120)), got); diff != "" {
		t.Errorf("timeout_seconds (-want +got):\n%s", diff)
	}
}

func TestFunctionReportsAHandlerThatNamesNoFunction(t *testing.T) {
	p := defaultFunction
	p.Handler = "index."
	ctx, _ := setupFunction(t, p)
	want := ir.Errors{{NodeID: "n2", Message: "function 'handler' has a handler 'index.' that names no function"}}
	if diff := cmp.Diff(want, ctx.Errors); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
}

func TestFunctionJoinsTheConnectorWhenAnEdgeAsksForIt(t *testing.T) {
	ctx, handle := setupFunction(t, defaultFunction)
	handle.NeedsNetwork = true
	handle.Finalise()

	want := ir.Attrs{
		ir.A("available_memory", ir.Str("512Mi")),
		ir.A("timeout_seconds", ir.Num(30)),
		ir.A("service_account_email", ir.R(fnAccountID, ir.Field("email"))),
		ir.A("vpc_connector", ir.R(connectorID, ir.Field("id"))),
		ir.A("vpc_connector_egress_settings", ir.Str("PRIVATE_RANGES_ONLY")),
	}
	if diff := cmp.Diff(want, serviceConfig(t, ctx)); diff != "" {
		t.Errorf("service_config (-want +got):\n%s", diff)
	}
	if countOfType(ctx, "google_compute_network") != 1 {
		t.Error("the network was not created")
	}
}

func TestTwoFunctionsShareOneSourceBucket(t *testing.T) {
	ctx, project := newContext(t, []ir.Node{
		functionNode(t, "n2", "handler", defaultFunction),
		functionNode(t, "n3", "worker", defaultFunction),
	}, nil)
	resolveFunction(ctx, project.Nodes[0]).Finalise()
	resolveFunction(ctx, project.Nodes[1]).Finalise()

	if got := countOfType(ctx, "google_storage_bucket"); got != 1 {
		t.Errorf("buckets = %d, want 1", got)
	}
	var objects []string
	for _, r := range byType(ctx, "google_storage_bucket_object") {
		objects = append(objects, r.Name)
	}
	if diff := cmp.Diff([]string{"handler", "worker"}, objects); diff != "" {
		t.Errorf("objects (-want +got):\n%s", diff)
	}
}

func TestFunctionAccountIDIsCheckedAgainstItsLimit(t *testing.T) {
	p := newProject(t, []ir.Node{functionNode(t, "n2", "handler", defaultFunction)}, nil)
	p.Name = "a-project-name-that-is-long-too"
	want := ir.Errors{{
		NodeID: "n2",
		Message: "google_service_account name 'a-project-name-that-is-long-too-dev-handler' is 43 characters, " +
			"the limit is 30. Shorten the project, environment or node name.",
	}}
	if diff := cmp.Diff(want, runErrors(t, p)); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
}
