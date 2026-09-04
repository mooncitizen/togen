package aws

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

func setupCache(t *testing.T, p ir.CacheProps) (*resolve.Context, *resolve.Handle) {
	t.Helper()
	ctx, project := newContext(t, []ir.Node{{
		ID:         "n8",
		Type:       ir.NodeCache,
		Name:       "sessions",
		Properties: props(t, p),
	}}, nil)
	return ctx, resolveCache(ctx, project.Nodes[0])
}

var (
	cacheID   = ir.ID{Type: "aws_elasticache_replication_group", Name: "sessions"}
	cacheSGID = ir.ID{Type: "aws_security_group", Name: "sessions"}
	cacheHost = ir.R(cacheID, ir.Field("primary_endpoint_address"))
)

func TestCacheEmitsAnEncryptedSingleNodeRedisGroup(t *testing.T) {
	ctx, handle := setupCache(t, ir.CacheProps{Size: ir.SizeSmall})

	group := named(t, ctx, cacheID)
	if group.SourceNode != "n8" || group.SourceLabel != "sessions" {
		t.Errorf("group source = %q/%q", group.SourceNode, group.SourceLabel)
	}
	want := ir.Attrs{
		ir.A("replication_group_id", ir.Str("shop-dev-sessions")),
		ir.A("description", ir.Str("sessions cache")),
		ir.A("engine", ir.Str("redis")),
		ir.A("engine_version", ir.Str("7.1")),
		ir.A("node_type", ir.Str("cache.t4g.micro")),
		ir.A("num_cache_clusters", ir.Num(1)),
		ir.A("port", ir.Num(6379)),
		ir.A("subnet_group_name", ir.R(ir.ID{Type: "aws_elasticache_subnet_group", Name: "sessions"}, ir.Field("name"))),
		ir.A("security_group_ids", ir.L(ir.R(cacheSGID, ir.Field("id")))),
		ir.A("at_rest_encryption_enabled", ir.Bool(true)),
		ir.A("transit_encryption_enabled", ir.Bool(true)),
		ir.A("automatic_failover_enabled", ir.Bool(false)),
		ir.A("apply_immediately", ir.Bool(true)),
	}
	if diff := cmp.Diff(want, group.Args); diff != "" {
		t.Errorf("replication group args (-want +got):\n%s", diff)
	}

	subnetGroup := firstOfType(t, ctx, "aws_elasticache_subnet_group")
	wantGroup := ir.Attrs{
		ir.A("name", ir.Str("shop-dev-sessions")),
		ir.A("subnet_ids", ir.L(
			ir.R(ir.ID{Type: "aws_subnet", Name: "private_a"}, ir.Field("id")),
			ir.R(ir.ID{Type: "aws_subnet", Name: "private_b"}, ir.Field("id")),
		)),
	}
	if diff := cmp.Diff(wantGroup, subnetGroup.Args); diff != "" {
		t.Errorf("subnet group args (-want +got):\n%s", diff)
	}

	wantSG := ir.Attrs{
		ir.A("name", ir.Str("shop-dev-sessions")),
		ir.A("description", ir.Str("Access to sessions")),
		ir.A("vpc_id", ir.R(ir.ID{Type: "aws_vpc", Name: "main"}, ir.Field("id"))),
	}
	if diff := cmp.Diff(wantSG, named(t, ctx, cacheSGID).Args); diff != "" {
		t.Errorf("security group args (-want +got):\n%s", diff)
	}
	if countOfType(ctx, "aws_vpc_security_group_ingress_rule") != 0 {
		t.Error("the cache security group was given a rule of its own")
	}
	if countOfType(ctx, "aws_vpc") != 1 {
		t.Error("no implicit network")
	}

	wantOutputs := []ir.Output{{
		Name:        "sessions_endpoint",
		Description: "Endpoint of the sessions cache",
		Value:       cacheHost,
	}}
	if diff := cmp.Diff(wantOutputs, ctx.Outputs); diff != "" {
		t.Errorf("outputs (-want +got):\n%s", diff)
	}

	if diff := cmp.Diff(&cacheSGID, handle.SecurityGroup); diff != "" {
		t.Errorf("handle security group (-want +got):\n%s", diff)
	}
	wantExports := resolve.CacheExports{Host: cacheHost, Port: ir.Num(6379)}
	if diff := cmp.Diff(resolve.Exports(wantExports), handle.Exports); diff != "" {
		t.Errorf("exports (-want +got):\n%s", diff)
	}
	if handle.Primary != cacheID {
		t.Errorf("primary = %s", handle.Primary)
	}
}

func TestCacheMapsSizesToNodeTypes(t *testing.T) {
	for _, c := range []struct {
		size ir.Size
		want ir.Value
	}{
		{ir.SizeSmall, ir.Str("cache.t4g.micro")},
		{ir.SizeMedium, ir.Str("cache.t4g.medium")},
		{ir.SizeLarge, ir.Str("cache.r7g.large")},
	} {
		t.Run(string(c.size), func(t *testing.T) {
			ctx, _ := setupCache(t, ir.CacheProps{Size: c.size})
			got, _ := named(t, ctx, cacheID).Args.Get("node_type")
			if diff := cmp.Diff(c.want, got); diff != "" {
				t.Errorf("node_type (-want +got):\n%s", diff)
			}
		})
	}
}

func TestResolveRejectsCacheNamesThatExceedTheReplicationGroupLimit(t *testing.T) {
	p := example()
	p.Nodes = []ir.Node{{ID: "n8", Type: ir.NodeCache, Name: strings.Repeat("s", 32)}}
	p.Edges = nil
	errs := runErrors(t, p)
	if len(errs) != 1 {
		t.Fatalf("errors = %v", errs)
	}
	if errs[0].NodeID != "n8" || !strings.Contains(errs[0].Message, "40") {
		t.Errorf("error = %+v", errs[0])
	}
}
