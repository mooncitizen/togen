package cost

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/mooncitizen/togen/internal/ir"
)

// Prices databases with an instance and a disk, gateways at rest on a request meter, and adds
// a nat when there is a database.
type fakeMatchers struct{}

func (fakeMatchers) Node(n ir.Node, region string) (Item, bool, error) {
	if n.Type == ir.NodeGateway {
		requests := lookup("requests", region)
		requests.Quantity, requests.Unit = 0, "requests"
		return Item{
			Name:     n.Name,
			Kind:     string(n.Type),
			Summary:  "HTTP API",
			Note:     "priced at rest, usage not set",
			Usage:    []Lookup{requests},
			Unpriced: []string{"requests", "data transfer"},
		}, true, nil
	}
	if n.Type != ir.NodeDatabase {
		return Item{}, false, nil
	}
	disk := lookup("disk", region)
	disk.Label, disk.Quantity, disk.Unit = "storage", 20, "GB"
	return Item{
		Name:     n.Name,
		Kind:     string(n.Type),
		Summary:  "small, 20 GB",
		Lookups:  []Lookup{lookup("db", region), disk},
		Unpriced: []string{"backups beyond 20 GB"},
	}, true, nil
}

func (fakeMatchers) Implicit(p *ir.Project) []Item {
	for _, n := range p.Nodes {
		if n.Type == ir.NodeDatabase {
			nat := lookup("nat", p.Region)
			nat.Filters = append(nat.Filters, Filter{Attribute: "usagetype", Value: `(\w+-)?NatGateway-Hours`, Pattern: true})
			return []Item{{Name: "network", Kind: "implicit VPC", Lookups: []Lookup{nat}, Unpriced: []string{"nat gateway data processed"}}}
		}
	}
	return nil
}

func (fakeMatchers) Catalogue(string) ([]Lookup, error) { return nil, nil }

func project(region string, nodes ...ir.Node) *ir.Project {
	return &ir.Project{Version: 1, Name: "shop", Provider: ir.ProviderAWS, Region: region, Environment: "dev", Nodes: nodes, Edges: []ir.Edge{}}
}

var (
	gateway  = ir.Node{ID: "n1", Type: ir.NodeGateway, Name: "api"}
	database = ir.Node{ID: "n2", Type: ir.NodeDatabase, Name: "orders-db"}
	queue    = ir.Node{ID: "n3", Type: ir.NodeQueue, Name: "jobs"}
	taken    = time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
)

func TestEstimatePricesEveryLineAndAddsThemUp(t *testing.T) {
	doc, err := Estimate(project("eu-west-2", gateway, database, queue), fakeMatchers{}, fixture(), taken)
	if err != nil {
		t.Fatal(err)
	}
	want := Document{
		Provider: ir.ProviderAWS,
		Region:   "eu-west-2",
		Currency: "USD",
		Items: []Priced{
			{
				Name: "api", Kind: "gateway", Summary: "HTTP API", Note: "priced at rest, usage not set",
				Lines: []Line{}, Subtotal: 0,
			},
			{
				Name: "orders-db", Kind: "database", Summary: "small, 20 GB",
				Lines: []Line{
					{Label: "db", Quantity: 730, Unit: "h", UnitPrice: 0.018, Amount: 13.14, SKU: "db-euw2"},
					{Label: "storage", Quantity: 20, Unit: "GB", UnitPrice: 0.133, Amount: 2.66, SKU: "disk-euw2"},
				},
				Subtotal: 15.80,
			},
			{
				Name: "network", Kind: "implicit VPC",
				Lines:    []Line{{Label: "nat", Quantity: 730, Unit: "h", UnitPrice: 0.05, Amount: 36.50, SKU: "nat-euw2"}},
				Subtotal: 36.50,
			},
		},
		NotPriced: []Omission{
			{Name: "jobs", Kind: "queue", Reason: "no aws prices for this node type yet"},
			{Name: "api", Kind: "gateway", Reason: "requests"},
			{Name: "api", Kind: "gateway", Reason: "data transfer"},
			{Name: "orders-db", Kind: "database", Reason: "backups beyond 20 GB"},
			{Name: "network", Kind: "implicit VPC", Reason: "nat gateway data processed"},
		},
		Total:        52.30,
		SnapshotDate: "2026-09-05",
		Note:         "list prices from 2026-09-05, estimate not a quote",
	}
	if diff := cmp.Diff(want, doc); diff != "" {
		t.Errorf("document (-want +got):\n%s", diff)
	}
}

func TestEstimateFollowsTheProjectRegion(t *testing.T) {
	doc, err := Estimate(project("us-east-1", database), fakeMatchers{}, fixture(), taken)
	if err != nil {
		t.Fatal(err)
	}
	if got := doc.Items[0].Lines[0]; got.SKU != "db-use1" || got.Amount != 11.68 {
		t.Errorf("instance line = %+v", got)
	}
	if got := doc.Items[1].Lines[0]; got.SKU != "nat-use1" || got.Amount != 32.85 {
		t.Errorf("nat line = %+v", got)
	}
}

func TestEstimateRefusesARegionTheSnapshotLacks(t *testing.T) {
	_, err := Estimate(project("eu-west-9", database), fakeMatchers{}, fixture(), taken)
	if err == nil || err.Error() != "no aws prices are bundled for region 'eu-west-9'" {
		t.Errorf("error = %v", err)
	}
}

func TestEstimateReportsALookupThatFindsNothing(t *testing.T) {
	snapshot := fixture()
	snapshot.SKUs = slices.DeleteFunc(snapshot.SKUs, func(sku SKU) bool { return sku.Attributes["kind"] == "disk" })
	_, err := Estimate(project("eu-west-2", database), fakeMatchers{}, snapshot, taken)
	if err == nil || !strings.Contains(err.Error(), "orders-db: no aws sku matches storage") {
		t.Errorf("error = %v", err)
	}
}

func TestEstimateCarriesTheStalenessWarning(t *testing.T) {
	doc, err := Estimate(project("eu-west-2", database), fakeMatchers{}, fixture(), taken.Add(100*24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if doc.Warning != "the bundled aws prices are 100 days old (taken 2026-09-05)" {
		t.Errorf("warning = %q", doc.Warning)
	}
}

func TestEstimatePricesNothingWithoutMatchers(t *testing.T) {
	p := project("europe-west2", gateway, ir.Node{ID: "q1", Type: ir.NodeQueue, Name: "jobs"})
	p.Provider = ir.ProviderGCP
	doc, err := Estimate(p, nil, nil, taken)
	if err != nil {
		t.Fatal(err)
	}
	want := Document{
		Provider: ir.ProviderGCP,
		Region:   "europe-west2",
		Currency: "USD",
		Items:    []Priced{},
		NotPriced: []Omission{
			{Name: "api", Kind: "gateway", Reason: "no gcp prices are bundled yet"},
			{Name: "jobs", Kind: "queue", Reason: "no gcp prices are bundled yet"},
		},
		Note: "no gcp prices are bundled yet",
	}
	if diff := cmp.Diff(want, doc); diff != "" {
		t.Errorf("document (-want +got):\n%s", diff)
	}
}

func TestEstimateNeedsASnapshotWhenThereAreMatchers(t *testing.T) {
	_, err := Estimate(project("eu-west-2", database), fakeMatchers{}, nil, taken)
	if err == nil || err.Error() != "no aws price snapshot is bundled" {
		t.Errorf("error = %v", err)
	}
}

func TestTableLinesUpTheColumns(t *testing.T) {
	doc, err := Estimate(project("eu-west-2", gateway, database, queue), fakeMatchers{}, fixture(), taken.Add(100*24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"api        gateway       HTTP API",
		"  priced at rest, usage not set",
		"                                     0.00",
		"orders-db  database      small, 20 GB",
		"  db       730 h   x  0.0180 USD    13.14",
		"  storage   20 GB  x  0.1330 USD     2.66",
		"                                    15.80",
		"network    implicit VPC",
		"  nat      730 h   x  0.0500 USD    36.50",
		"                                    36.50",
		"not priced",
		"  jobs       queue         no aws prices for this node type yet",
		"  api        gateway       requests",
		"  api        gateway       data transfer",
		"  orders-db  database      backups beyond 20 GB",
		"  network    implicit VPC  nat gateway data processed",
		"",
		"warning: the bundled aws prices are 100 days old (taken 2026-09-05)",
		"total  52.30 USD/month  eu-west-2, list prices from 2026-09-05, estimate not a quote",
	}
	if diff := cmp.Diff(want, doc.Table()); diff != "" {
		t.Errorf("table (-want +got):\n%s", diff)
	}
}

func TestTablePutsTheSubtotalPastTheNoteWhenNothingHasLines(t *testing.T) {
	doc, err := Estimate(project("eu-west-2", gateway), fakeMatchers{}, fixture(), taken)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"api  gateway  HTTP API",
		"  priced at rest, usage not set",
		"                                    0.00",
		"not priced",
		"  api  gateway  requests",
		"  api  gateway  data transfer",
		"",
		"total  0.00 USD/month  eu-west-2, list prices from 2026-09-05, estimate not a quote",
	}
	if diff := cmp.Diff(want, doc.Table()); diff != "" {
		t.Errorf("table (-want +got):\n%s", diff)
	}
}

func TestTableForAProviderWithoutPrices(t *testing.T) {
	p := project("europe-west2", gateway)
	p.Provider = ir.ProviderGCP
	doc, err := Estimate(p, nil, nil, taken)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"not priced",
		"  api  gateway  no gcp prices are bundled yet",
		"",
		"total  0.00 USD/month  europe-west2, no gcp prices are bundled yet",
	}
	if diff := cmp.Diff(want, doc.Table()); diff != "" {
		t.Errorf("table (-want +got):\n%s", diff)
	}
}
