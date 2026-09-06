package azure

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/mooncitizen/togen/internal/emit/hcl"
	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

func cacheNode(t *testing.T, id, name string, p ir.CacheProps) ir.Node {
	t.Helper()
	return ir.Node{ID: id, Type: ir.NodeCache, Name: name, Properties: props(t, p)}
}

func setupCache(t *testing.T, p ir.CacheProps) (*resolve.Context, *resolve.Handle) {
	t.Helper()
	ctx := newContext(t, []ir.Node{cacheNode(t, "n8", "sessions", p)})
	return ctx, resolveCache(ctx, ctx.Project.Nodes[0])
}

var (
	cacheID   = ir.ID{Type: "azurerm_redis_cache", Name: "sessions"}
	cacheHost = ir.R(cacheID, ir.Field("hostname"))
	cachePort = ir.R(cacheID, ir.Field("ssl_port"))
	cacheKey  = ir.R(cacheID, ir.Field("primary_access_key"))
)

func cacheArgs(capacity float64, family, sku string) ir.Attrs {
	return located("shop-dev-sessions",
		ir.A("capacity", ir.Num(capacity)),
		ir.A("family", ir.Str(family)),
		ir.A("sku_name", ir.Str(sku)),
		ir.A("non_ssl_port_enabled", ir.Bool(false)),
		ir.A("minimum_tls_version", ir.Str("1.2")),
		ir.A("redis_version", ir.Str("6")),
	)
}

func TestCacheEmitsATLSOnlyRedisCacheInTheResourceGroup(t *testing.T) {
	ctx, handle := setupCache(t, ir.CacheProps{Size: ir.SizeSmall})
	if len(ctx.Errors) != 0 {
		t.Fatalf("errors = %v", ctx.Errors)
	}

	cache := firstOfType(t, ctx, "azurerm_redis_cache")
	if cache.Name != "sessions" || cache.SourceNode != "n8" || cache.SourceLabel != "sessions" {
		t.Errorf("cache = %q %q %q", cache.Name, cache.SourceNode, cache.SourceLabel)
	}
	if diff := cmp.Diff(cacheArgs(0, "C", "Basic"), cache.Args); diff != "" {
		t.Errorf("cache args (-want +got):\n%s", diff)
	}
	if got := len(ctx.Resources()); got != 2 {
		t.Errorf("resources = %d, want the group and the cache", got)
	}
	if got := countOfType(ctx, "azurerm_virtual_network"); got != 0 {
		t.Error("a cache pulled in the network")
	}

	wantExports := resolve.CacheExports{Host: cacheHost, Port: cachePort, Password: cacheKey}
	if diff := cmp.Diff(resolve.Exports(wantExports), handle.Exports); diff != "" {
		t.Errorf("exports (-want +got):\n%s", diff)
	}
	if handle.Primary != cacheID {
		t.Errorf("primary = %v", handle.Primary)
	}
	wantOutputs := []ir.Output{{
		Name:        "sessions_hostname",
		Description: "Hostname of the sessions cache",
		Value:       cacheHost,
	}}
	if diff := cmp.Diff(wantOutputs, ctx.Outputs); diff != "" {
		t.Errorf("outputs (-want +got):\n%s", diff)
	}
}

func TestCacheMapsSizesOntoValidSkuTriples(t *testing.T) {
	for _, tc := range []struct {
		size     ir.Size
		capacity float64
		family   string
		sku      string
	}{
		{ir.SizeSmall, 0, "C", "Basic"},
		{ir.SizeMedium, 1, "C", "Standard"},
		{ir.SizeLarge, 3, "C", "Standard"},
	} {
		ctx, _ := setupCache(t, ir.CacheProps{Size: tc.size})
		if diff := cmp.Diff(cacheArgs(tc.capacity, tc.family, tc.sku), firstOfType(t, ctx, "azurerm_redis_cache").Args); diff != "" {
			t.Errorf("%s cache args (-want +got):\n%s", tc.size, diff)
		}
	}
}

func TestCacheEmitsValidHCL(t *testing.T) {
	p := newProject(t, []ir.Node{
		functionNode(t, "n2", "handler", defaultFunction),
		cacheNode(t, "n8", "sessions", ir.CacheProps{Size: ir.SizeSmall}),
	})
	p.Edges = []ir.Edge{{ID: "e1", From: "n2", To: "n8", Relation: ir.RelReads}}
	g, err := resolve.Run(p, New())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if errs := ir.ValidateGraph(g); len(errs) > 0 {
		t.Errorf("graph is not valid:\n%s", errs.Error())
	}
	files, err := hcl.Emit(g)
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	main := unaligned(files["main.tf"])
	for _, want := range []string{
		`resource "azurerm_redis_cache" "sessions"`,
		`non_ssl_port_enabled = false`,
		`SESSIONS_HOST = azurerm_redis_cache.sessions.hostname`,
		`SESSIONS_PORT = azurerm_redis_cache.sessions.ssl_port`,
		`SESSIONS_PASSWORD = azurerm_redis_cache.sessions.primary_access_key`,
	} {
		if !strings.Contains(main, want) {
			t.Errorf("main.tf lacks %s:\n%s", want, files["main.tf"])
		}
	}
	if strings.Contains(main, "azurerm_virtual_network") {
		t.Errorf("a cache edge pulled in the network:\n%s", files["main.tf"])
	}
}

func TestResolveRejectsCacheNamesThatExceedTheLimit(t *testing.T) {
	p := newProject(t, []ir.Node{cacheNode(t, "n8", strings.Repeat("s", 60), ir.CacheProps{Size: ir.SizeSmall})})
	errs := runErrors(t, p)
	if len(errs) != 1 {
		t.Fatalf("errors = %v", errs)
	}
	if errs[0].NodeID != "n8" || !strings.Contains(errs[0].Message, "63") {
		t.Errorf("error = %+v", errs[0])
	}
}
