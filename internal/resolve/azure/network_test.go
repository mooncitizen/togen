package azure

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

func newProject(t *testing.T, nodes []ir.Node) *ir.Project {
	t.Helper()
	p, err := ir.ApplyDefaults(&ir.Project{
		Version:     1,
		Name:        "shop",
		Provider:    ir.ProviderAzure,
		Region:      "uksouth",
		Environment: "dev",
		Nodes:       nodes,
	})
	if err != nil {
		t.Fatalf("apply defaults: %v", err)
	}
	return p
}

func newContext(t *testing.T, nodes []ir.Node) *resolve.Context {
	t.Helper()
	return resolve.NewContext(newProject(t, nodes))
}

func byType(ctx *resolve.Context, typ string) []ir.Resource {
	var out []ir.Resource
	for _, r := range ctx.Resources() {
		if r.Type == typ {
			out = append(out, r)
		}
	}
	return out
}

func firstOfType(t *testing.T, ctx *resolve.Context, typ string) ir.Resource {
	t.Helper()
	rs := byType(ctx, typ)
	if len(rs) == 0 {
		t.Fatalf("no %s resource", typ)
	}
	return rs[0]
}

func countOfType(ctx *resolve.Context, typ string) int { return len(byType(ctx, typ)) }

var groupID = ir.ID{Type: "azurerm_resource_group", Name: "main"}

func serviceNode(id, name string) ir.Node {
	return ir.Node{
		ID:         id,
		Type:       ir.NodeService,
		Name:       name,
		Properties: json.RawMessage(`{"image":"nginx:1.27","port":80}`),
	}
}

func TestNetworkCreatesAVirtualNetworkInTheResourceGroup(t *testing.T) {
	ctx := newContext(t, nil)
	net := ensureNetwork(ctx)

	want := &Network{
		VirtualNetwork: ir.ID{Type: "azurerm_virtual_network", Name: "main"},
		AppsSubnet:     ir.ID{Type: "azurerm_subnet", Name: "apps"},
		PostgresSubnet: ir.ID{Type: "azurerm_subnet", Name: "postgres"},
	}
	if diff := cmp.Diff(want, net); diff != "" {
		t.Errorf("network (-want +got):\n%s", diff)
	}

	vnet := firstOfType(t, ctx, "azurerm_virtual_network")
	wantArgs := ir.Attrs{
		ir.A("name", ir.Str("shop-dev-vnet")),
		ir.A("resource_group_name", ir.R(groupID, ir.Field("name"))),
		ir.A("location", ir.R(groupID, ir.Field("location"))),
		ir.A("address_space", ir.L(ir.Str("10.0.0.0/16"))),
	}
	if diff := cmp.Diff(wantArgs, vnet.Args); diff != "" {
		t.Errorf("vnet args (-want +got):\n%s", diff)
	}
	if vnet.SourceLabel != "network" || vnet.SourceNode != "" {
		t.Errorf("source = %q/%q", vnet.SourceNode, vnet.SourceLabel)
	}
	if got := countOfType(ctx, "azurerm_resource_group"); got != 1 {
		t.Errorf("resource groups = %d", got)
	}
}

func TestNetworkDelegatesEachSubnetToItsService(t *testing.T) {
	ctx := newContext(t, nil)
	ensureNetwork(ctx).mysqlSubnet(ctx)

	vnetName := ir.R(ir.ID{Type: "azurerm_virtual_network", Name: "main"}, ir.Field("name"))
	for _, tc := range []struct{ subnet, prefix, service string }{
		{"apps", "10.0.0.0/23", "Microsoft.App/environments"},
		{"postgres", "10.0.2.0/24", "Microsoft.DBforPostgreSQL/flexibleServers"},
		{"mysql", "10.0.3.0/24", "Microsoft.DBforMySQL/flexibleServers"},
	} {
		r, ok := ctx.Resource(ir.ID{Type: "azurerm_subnet", Name: tc.subnet})
		if !ok {
			t.Fatalf("no %s subnet", tc.subnet)
		}
		want := ir.Attrs{
			ir.A("name", ir.Str("shop-dev-"+tc.subnet)),
			ir.A("resource_group_name", ir.R(groupID, ir.Field("name"))),
			ir.A("virtual_network_name", vnetName),
			ir.A("address_prefixes", ir.L(ir.Str(tc.prefix))),
			ir.A("delegation", ir.B(ir.Attrs{
				ir.A("name", ir.Str(tc.subnet)),
				ir.A("service_delegation", ir.B(ir.Attrs{
					ir.A("name", ir.Str(tc.service)),
					ir.A("actions", ir.L(ir.Str("Microsoft.Network/virtualNetworks/subnets/join/action"))),
				})),
			})),
		}
		if diff := cmp.Diff(want, r.Args); diff != "" {
			t.Errorf("%s subnet args (-want +got):\n%s", tc.subnet, diff)
		}
		if r.SourceLabel != "network" {
			t.Errorf("%s subnet label = %q", tc.subnet, r.SourceLabel)
		}
	}
}

func TestNetworkIsCreatedOnceForTwoNodesThatNeedIt(t *testing.T) {
	ctx := newContext(t, []ir.Node{serviceNode("n1", "web"), serviceNode("n2", "admin")})
	a := ensureNetwork(ctx)
	b := ensureNetwork(ctx)
	if a != b {
		t.Error("ensureNetwork built a second network")
	}
	if got := countOfType(ctx, "azurerm_virtual_network"); got != 1 {
		t.Errorf("virtual networks = %d", got)
	}
	if got := countOfType(ctx, "azurerm_subnet"); got != 2 {
		t.Errorf("subnets = %d", got)
	}
}

func TestNetworkMakesTheMySQLSubnetOnceAndOnlyWhenAsked(t *testing.T) {
	ctx := newContext(t, nil)
	net := ensureNetwork(ctx)
	if net.MySQLSubnet != (ir.ID{}) || countOfType(ctx, "azurerm_subnet") != 2 {
		t.Fatalf("the network came with a mysql subnet: %v, %d subnets", net.MySQLSubnet, countOfType(ctx, "azurerm_subnet"))
	}
	want := ir.ID{Type: "azurerm_subnet", Name: "mysql"}
	if got := net.mysqlSubnet(ctx); got != want {
		t.Errorf("mysqlSubnet = %v", got)
	}
	if got := net.mysqlSubnet(ctx); got != want {
		t.Errorf("second mysqlSubnet = %v", got)
	}
	if got := countOfType(ctx, "azurerm_subnet"); got != 3 {
		t.Errorf("subnets = %d", got)
	}
	if net.MySQLSubnet != want {
		t.Errorf("MySQLSubnet = %v", net.MySQLSubnet)
	}
}

func TestNetworkIsNeededByServicesAndDatabasesOnly(t *testing.T) {
	for _, tc := range []struct {
		node ir.Node
		want bool
	}{
		{serviceNode("n1", "web"), true},
		{ir.Node{ID: "n1", Type: ir.NodeDatabase, Name: "main-db"}, true},
		{ir.Node{ID: "n1", Type: ir.NodeGateway, Name: "api"}, false},
		{ir.Node{ID: "n1", Type: ir.NodeFunction, Name: "handler"}, false},
		{ir.Node{ID: "n1", Type: ir.NodeQueue, Name: "jobs"}, false},
		{ir.Node{ID: "n1", Type: ir.NodeBucket, Name: "uploads"}, false},
		{ir.Node{ID: "n1", Type: ir.NodeCache, Name: "sessions"}, false},
	} {
		if got := needsNetwork(newProject(t, []ir.Node{tc.node})); got != tc.want {
			t.Errorf("needsNetwork(%s) = %v, want %v", tc.node.Type, got, tc.want)
		}
	}
	if needsNetwork(newProject(t, nil)) {
		t.Error("an empty project needs a network")
	}
}
