package gcp

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

var defaultService = ir.ServiceProps{
	Image:       "nginx:1.27",
	Port:        8080,
	Size:        ir.SizeSmall,
	MinReplicas: 1,
	MaxReplicas: 2,
}

var (
	svcID        = ir.ID{Type: "google_cloud_run_v2_service", Name: "web"}
	svcAccountID = ir.ID{Type: "google_service_account", Name: "web"}
	svcBindingID = ir.ID{Type: "google_cloud_run_v2_service_iam_member", Name: "web"}
)

func serviceNode(t *testing.T, id, name string, p ir.ServiceProps) ir.Node {
	t.Helper()
	return ir.Node{ID: id, Type: ir.NodeService, Name: name, Properties: props(t, p)}
}

func setupService(t *testing.T, p ir.ServiceProps) (*resolve.Context, *resolve.Handle) {
	t.Helper()
	ctx, project := newContext(t, []ir.Node{serviceNode(t, "n4", "web", p)}, nil)
	return ctx, resolveService(ctx, project.Nodes[0])
}

func named(t *testing.T, ctx *resolve.Context, id ir.ID) ir.Resource {
	t.Helper()
	r, ok := ctx.Resource(id)
	if !ok {
		t.Fatalf("no %s in the graph", id)
	}
	return *r
}

func template(t *testing.T, ctx *resolve.Context) ir.Attrs {
	t.Helper()
	v, ok := named(t, ctx, svcID).Args.Get("template")
	if !ok {
		t.Fatal("the service has no template")
	}
	block, ok := v.(ir.Block)
	if !ok || len(block) != 1 {
		t.Fatalf("template = %#v", v)
	}
	return block[0]
}

func container(t *testing.T, ctx *resolve.Context) ir.Attrs {
	t.Helper()
	v, ok := template(t, ctx).Get("containers")
	if !ok {
		t.Fatal("the template has no containers")
	}
	block, ok := v.(ir.Block)
	if !ok || len(block) != 1 {
		t.Fatalf("containers = %#v", v)
	}
	return block[0]
}

func TestServiceEmitsAnAccountAndAPrivateCloudRunService(t *testing.T) {
	ctx, handle := setupService(t, defaultService)
	handle.Finalise()

	if diff := cmp.Diff([]string{"google_service_account", "google_cloud_run_v2_service"}, resourceTypes(ctx)); diff != "" {
		t.Errorf("resources (-want +got):\n%s", diff)
	}
	svc := named(t, ctx, svcID)
	want := ir.Attrs{
		ir.A("name", ir.Str("shop-dev-web")),
		ir.A("location", ir.Str("europe-west2")),
		ir.A("deletion_protection", ir.Bool(false)),
		ir.A("ingress", ir.Str("INGRESS_TRAFFIC_INTERNAL_ONLY")),
		ir.A("template", ir.B(ir.Attrs{
			ir.A("service_account", ir.R(svcAccountID, ir.Field("email"))),
			ir.A("scaling", ir.B(ir.Attrs{
				ir.A("min_instance_count", ir.Num(1)),
				ir.A("max_instance_count", ir.Num(2)),
			})),
			ir.A("containers", ir.B(ir.Attrs{
				ir.A("image", ir.Str("nginx:1.27")),
				ir.A("ports", ir.B(ir.Attrs{ir.A("container_port", ir.Num(8080))})),
				ir.A("resources", ir.B(ir.Attrs{
					ir.A("limits", ir.M(ir.A("cpu", ir.Str("1")), ir.A("memory", ir.Str("512Mi")))),
				})),
			})),
		})),
	}
	if diff := cmp.Diff(want, svc.Args); diff != "" {
		t.Errorf("service args (-want +got):\n%s", diff)
	}
	if svc.SourceNode != "n4" || svc.SourceLabel != "web" {
		t.Errorf("service source = %q/%q", svc.SourceNode, svc.SourceLabel)
	}

	wantAccount := ir.Attrs{
		ir.A("account_id", ir.Str("shop-dev-web")),
		ir.A("display_name", ir.Str("shop-dev-web")),
	}
	if diff := cmp.Diff(wantAccount, named(t, ctx, svcAccountID).Args); diff != "" {
		t.Errorf("service account args (-want +got):\n%s", diff)
	}
	if len(ctx.Outputs) != 0 {
		t.Errorf("a private service produced outputs %+v", ctx.Outputs)
	}
	if countOfType(ctx, "google_compute_network") != 0 {
		t.Error("a network was created without an edge asking for one")
	}

	wantExports := resolve.CloudRunExports{
		Service:        ir.R(svcID, ir.Field("name")),
		Location:       ir.R(svcID, ir.Field("location")),
		URL:            ir.R(svcID, ir.Field("uri")),
		ServiceAccount: ir.R(svcAccountID, ir.Field("email")),
	}
	if diff := cmp.Diff(wantExports, handle.Exports); diff != "" {
		t.Errorf("exports (-want +got):\n%s", diff)
	}
	if handle.Primary != svcID {
		t.Errorf("primary = %v", handle.Primary)
	}
}

func TestServiceMapsSizesToCPUAndMemoryLimits(t *testing.T) {
	for size, want := range map[ir.Size]ir.Map{
		ir.SizeSmall:  ir.M(ir.A("cpu", ir.Str("1")), ir.A("memory", ir.Str("512Mi"))),
		ir.SizeMedium: ir.M(ir.A("cpu", ir.Str("1")), ir.A("memory", ir.Str("1Gi"))),
		ir.SizeLarge:  ir.M(ir.A("cpu", ir.Str("2")), ir.A("memory", ir.Str("2Gi"))),
	} {
		p := defaultService
		p.Size = size
		ctx, handle := setupService(t, p)
		handle.Finalise()
		v, _ := container(t, ctx).Get("resources")
		limits, _ := v.(ir.Block)[0].Get("limits")
		if diff := cmp.Diff(ir.Value(want), limits); diff != "" {
			t.Errorf("%s limits (-want +got):\n%s", size, diff)
		}
	}
}

func TestPublicServiceOpensIngressBindsAllUsersAndOutputsItsURL(t *testing.T) {
	p := defaultService
	p.Public = true
	ctx, handle := setupService(t, p)
	handle.Finalise()

	ingress, _ := named(t, ctx, svcID).Args.Get("ingress")
	if diff := cmp.Diff(ir.Value(ir.Str("INGRESS_TRAFFIC_ALL")), ingress); diff != "" {
		t.Errorf("ingress (-want +got):\n%s", diff)
	}
	binding := named(t, ctx, svcBindingID)
	if binding.SourceNode != "n4" || binding.SourceLabel != "web" {
		t.Errorf("binding source = %q/%q", binding.SourceNode, binding.SourceLabel)
	}
	want := ir.Attrs{
		ir.A("name", ir.R(svcID, ir.Field("name"))),
		ir.A("location", ir.R(svcID, ir.Field("location"))),
		ir.A("role", ir.Str("roles/run.invoker")),
		ir.A("member", ir.Str("allUsers")),
	}
	if diff := cmp.Diff(want, binding.Args); diff != "" {
		t.Errorf("binding args (-want +got):\n%s", diff)
	}
	wantOutputs := []ir.Output{{
		Name:        "web_url",
		Description: "Public URL of the web service",
		Value:       ir.R(svcID, ir.Field("uri")),
	}}
	if diff := cmp.Diff(wantOutputs, ctx.Outputs); diff != "" {
		t.Errorf("outputs (-want +got):\n%s", diff)
	}
}

func TestPrivateServiceHasNoBindingButStillExportsItsURL(t *testing.T) {
	ctx, handle := setupService(t, defaultService)
	handle.Finalise()
	if countOfType(ctx, svcBindingID.Type) != 0 {
		t.Error("a private service was opened to allUsers")
	}
	url := handle.Exports.(resolve.CloudRunExports).URL
	if diff := cmp.Diff(ir.Value(ir.R(svcID, ir.Field("uri"))), url); diff != "" {
		t.Errorf("url (-want +got):\n%s", diff)
	}
}

func TestServiceSetsReplicasOnTheScalingBlock(t *testing.T) {
	p := defaultService
	p.MinReplicas = 2
	p.MaxReplicas = 5
	ctx, handle := setupService(t, p)
	handle.Finalise()
	scaling, _ := template(t, ctx).Get("scaling")
	want := ir.B(ir.Attrs{
		ir.A("min_instance_count", ir.Num(2)),
		ir.A("max_instance_count", ir.Num(5)),
	})
	if diff := cmp.Diff(ir.Value(want), scaling); diff != "" {
		t.Errorf("scaling (-want +got):\n%s", diff)
	}
}

func TestServiceSetsTheImageAndPort(t *testing.T) {
	p := defaultService
	p.Image = "ghcr.io/shop/web:2.1"
	p.Port = 3000
	ctx, handle := setupService(t, p)
	handle.Finalise()
	c := container(t, ctx)
	image, _ := c.Get("image")
	if diff := cmp.Diff(ir.Value(ir.Str("ghcr.io/shop/web:2.1")), image); diff != "" {
		t.Errorf("image (-want +got):\n%s", diff)
	}
	ports, _ := c.Get("ports")
	if diff := cmp.Diff(ir.Value(ir.B(ir.Attrs{ir.A("container_port", ir.Num(3000))})), ports); diff != "" {
		t.Errorf("ports (-want +got):\n%s", diff)
	}
}

func TestServiceEnvIsSortedAndTakesWhatEdgesAdd(t *testing.T) {
	p := defaultService
	p.Env = map[string]string{"LOG_LEVEL": "info", "APP_NAME": "shop"}
	ctx, handle := setupService(t, p)
	handle.SetEnv("JOBS_TOPIC", ir.Str("jobs"))
	handle.Finalise()

	env, _ := container(t, ctx).Get("env")
	want := ir.B(
		ir.Attrs{ir.A("name", ir.Str("APP_NAME")), ir.A("value", ir.Str("shop"))},
		ir.Attrs{ir.A("name", ir.Str("LOG_LEVEL")), ir.A("value", ir.Str("info"))},
		ir.Attrs{ir.A("name", ir.Str("JOBS_TOPIC")), ir.A("value", ir.Str("jobs"))},
	)
	if diff := cmp.Diff(ir.Value(want), env); diff != "" {
		t.Errorf("env (-want +got):\n%s", diff)
	}
}

func TestServiceOmitsEnvWhenThereIsNone(t *testing.T) {
	ctx, handle := setupService(t, defaultService)
	handle.Finalise()
	if _, ok := container(t, ctx).Get("env"); ok {
		t.Error("env present with no variables")
	}
	if _, ok := template(t, ctx).Get("vpc_access"); ok {
		t.Error("vpc_access present with no edge asking for the network")
	}
}

func TestServiceJoinsTheConnectorWhenAnEdgeAsksForIt(t *testing.T) {
	ctx, handle := setupService(t, defaultService)
	handle.NeedsNetwork = true
	handle.Finalise()

	access, _ := template(t, ctx).Get("vpc_access")
	want := ir.B(ir.Attrs{
		ir.A("connector", ir.R(connectorID, ir.Field("id"))),
		ir.A("egress", ir.Str("PRIVATE_RANGES_ONLY")),
	})
	if diff := cmp.Diff(ir.Value(want), access); diff != "" {
		t.Errorf("vpc_access (-want +got):\n%s", diff)
	}
	if countOfType(ctx, "google_compute_network") != 1 {
		t.Error("the network was not created")
	}
}

func TestServiceNameIsCheckedAgainstItsLimit(t *testing.T) {
	p := newProject(t, []ir.Node{serviceNode(t, "n4", "web", defaultService)}, nil)
	p.Name = "a-project-name-that-is-long-too"
	want := ir.Errors{{
		NodeID: "n4",
		Message: "google_service_account name 'a-project-name-that-is-long-too-dev-web' is 39 characters, " +
			"the limit is 30. Shorten the project, environment or node name.",
	}}
	if diff := cmp.Diff(want, runErrors(t, p)); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
}
