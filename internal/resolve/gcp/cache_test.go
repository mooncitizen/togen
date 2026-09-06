package gcp

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

var (
	cacheID   = ir.ID{Type: "google_redis_instance", Name: "sessions"}
	cacheHost = ir.R(cacheID, ir.Field("host"))
)

func cacheNode(t *testing.T, id, name string, p ir.CacheProps) ir.Node {
	t.Helper()
	return ir.Node{ID: id, Type: ir.NodeCache, Name: name, Properties: props(t, p)}
}

func setupCache(t *testing.T, p ir.CacheProps) (*resolve.Context, *resolve.Handle) {
	t.Helper()
	ctx, project := newContext(t, []ir.Node{cacheNode(t, "n8", "sessions", p)}, nil)
	return ctx, resolveCache(ctx, project.Nodes[0])
}

func TestCacheEmitsARedisInstanceOnPrivateServiceAccess(t *testing.T) {
	ctx, handle := setupCache(t, ir.CacheProps{Size: ir.SizeSmall})

	wantTypes := []string{
		"google_compute_network",
		"google_compute_subnetwork",
		"google_vpc_access_connector",
		"google_compute_global_address",
		"google_service_networking_connection",
		"google_redis_instance",
	}
	if diff := cmp.Diff(wantTypes, resourceTypes(ctx)); diff != "" {
		t.Errorf("resources (-want +got):\n%s", diff)
	}

	instance := named(t, ctx, cacheID)
	if instance.SourceNode != "n8" || instance.SourceLabel != "sessions" {
		t.Errorf("instance source = %q/%q", instance.SourceNode, instance.SourceLabel)
	}
	want := ir.Attrs{
		ir.A("name", ir.Str("shop-dev-sessions")),
		ir.A("region", ir.Str("europe-west2")),
		ir.A("tier", ir.Str("BASIC")),
		ir.A("memory_size_gb", ir.Num(1)),
		ir.A("redis_version", ir.Str("REDIS_7_2")),
		ir.A("authorized_network", ir.R(vpcID, ir.Field("id"))),
		ir.A("connect_mode", ir.Str("PRIVATE_SERVICE_ACCESS")),
		ir.A("transit_encryption_mode", ir.Str("DISABLED")),
	}
	if diff := cmp.Diff(want, instance.Args); diff != "" {
		t.Errorf("instance args (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]ir.ID{connectionID}, instance.DependsOn); diff != "" {
		t.Errorf("depends_on (-want +got):\n%s", diff)
	}

	wantExports := resolve.CacheExports{Host: cacheHost, Port: ir.Num(6379)}
	if diff := cmp.Diff(wantExports, handle.Exports); diff != "" {
		t.Errorf("exports (-want +got):\n%s", diff)
	}
	if handle.Primary != cacheID {
		t.Errorf("primary = %v", handle.Primary)
	}
	if len(ctx.Providers()) != 0 {
		t.Errorf("providers = %v", ctx.Providers())
	}

	wantOutputs := []ir.Output{{
		Name:        "sessions_host",
		Description: "Private address of the sessions cache",
		Value:       cacheHost,
	}}
	if diff := cmp.Diff(wantOutputs, ctx.Outputs); diff != "" {
		t.Errorf("outputs (-want +got):\n%s", diff)
	}
}

func TestCacheMapsSizesToTiersAndMemory(t *testing.T) {
	for _, c := range []struct {
		size   ir.Size
		tier   string
		memory float64
	}{
		{ir.SizeSmall, "BASIC", 1},
		{ir.SizeMedium, "BASIC", 2},
		{ir.SizeLarge, "STANDARD_HA", 5},
	} {
		ctx, _ := setupCache(t, ir.CacheProps{Size: c.size})
		tier, _ := named(t, ctx, cacheID).Args.Get("tier")
		memory, _ := named(t, ctx, cacheID).Args.Get("memory_size_gb")
		got := ir.Attrs{ir.A("tier", tier), ir.A("memory_size_gb", memory)}
		want := ir.Attrs{ir.A("tier", ir.Str(c.tier)), ir.A("memory_size_gb", ir.Num(c.memory))}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("%s (-want +got):\n%s", c.size, diff)
		}
	}
}

func TestACacheAndADatabaseShareTheNetwork(t *testing.T) {
	ctx, project := newContext(t, []ir.Node{
		databaseNode(t, "n3", "main-db", defaultDatabase),
		cacheNode(t, "n8", "sessions", ir.CacheProps{Size: ir.SizeSmall}),
	}, nil)
	resolveDatabase(ctx, project.Nodes[0])
	resolveCache(ctx, project.Nodes[1])

	if got := countOfType(ctx, "google_compute_network"); got != 1 {
		t.Errorf("networks = %d, want 1", got)
	}
	if got := countOfType(ctx, "google_service_networking_connection"); got != 1 {
		t.Errorf("connections = %d, want 1", got)
	}
}
