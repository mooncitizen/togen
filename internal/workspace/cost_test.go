package workspace

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mooncitizen/togen/internal/cost"
	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/simulate"
)

func copyExample(t *testing.T, name, dir string) {
	t.Helper()
	src := os.DirFS(filepath.Join("..", "..", "examples", name))
	if err := os.CopyFS(dir, src); err != nil {
		t.Fatal(err)
	}
}

func gatewayID(t *testing.T, dir string) string {
	t.Helper()
	project, errs, err := Validate(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(errs) > 0 {
		t.Fatalf("errors = %v", errs)
	}
	for _, n := range project.Nodes {
		if n.Type == ir.NodeGateway {
			return n.ID
		}
	}
	t.Fatalf("examples/%s has no gateway node", filepath.Base(dir))
	return ""
}

// One request in five enqueues a job, which damps the async loop the queue closes.
func publishID(t *testing.T, dir string) string {
	t.Helper()
	project, _, err := Validate(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range project.Edges {
		if e.Relation == ir.RelPublishes {
			return e.ID
		}
	}
	t.Fatalf("examples/%s has no publishes edge", filepath.Base(dir))
	return ""
}

// The matchers a snapshot would be looked up with, once one is bundled. Until then Cost passes
// no matchers so the estimate says no prices are bundled, which the cli and server tests cover.
func TestCostMatchersPriceAGcpProjectWhenASnapshotIsThere(t *testing.T) {
	matchers, ok := costMatchers[ir.ProviderGCP]
	if !ok {
		t.Fatal("no gcp matchers are wired in")
	}
	sku := func(id, service, description, region, unit string, price float64) cost.SKU {
		return cost.SKU{ID: id, Service: service, Unit: unit, Price: price, Attributes: map[string]string{
			"description": description, "usageType": "OnDemand", "region": region,
		}}
	}
	snapshot := &cost.Snapshot{
		Provider: ir.ProviderGCP, Date: "2026-09-05", Currency: "USD",
		Versions: map[string]map[string]string{"Cloud SQL": {"europe-west2": "2026-08-01"}},
		SKUs: []cost.SKU{
			sku("sql-micro", "Cloud SQL", "Cloud SQL for PostgreSQL: Zonal - Micro instance in London", "europe-west2", "h", 0.0126),
			sku("sql-storage", "Cloud SQL", "Cloud SQL for PostgreSQL: Zonal - Standard storage in London", "europe-west2", "GiBy.mo", 0.204),
			sku("compute-core", "Compute Engine", "E2 Instance Core running in London", "europe-west2", "h", 0.028092),
			sku("compute-ram", "Compute Engine", "E2 Instance Ram running in London", "europe-west2", "GiBy.h", 0.003765),
		},
	}
	project := &ir.Project{
		Version: 1, Name: "shop", Provider: ir.ProviderGCP, Region: "europe-west2", Environment: "dev",
		Nodes: []ir.Node{{ID: "n1", Type: ir.NodeDatabase, Name: "main-db"}},
	}
	doc, err := cost.Estimate(project, matchers, snapshot, nil, time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if doc.Note == "no gcp prices are bundled yet" || doc.Total <= 0 {
		t.Fatalf("document = %+v", doc)
	}
	if len(doc.Items) != 2 || doc.Items[0].Name != "main-db" || doc.Items[1].Name != "network" {
		t.Fatalf("items = %+v", doc.Items)
	}
	if got := doc.Items[0].Lines; len(got) != 2 || got[0].SKU != "sql-micro" || got[1].SKU != "sql-storage" {
		t.Errorf("database = %+v", got)
	}
}

func TestCostPricesTheSimulation(t *testing.T) {
	dir := t.TempDir()
	copyExample(t, "aws-basic", dir)

	plain, _, err := Cost(dir)
	if err != nil {
		t.Fatal(err)
	}
	if plain.Simulated {
		t.Fatal("no simulation file, so nothing is simulated")
	}

	sim := simulate.Simulation{
		Version: simulate.Version,
		Sources: []simulate.Source{{ID: "mobile", Name: "Mobile app", Target: gatewayID(t, dir), Rate: "800/min"}},
		Edges:   map[string]float64{publishID(t, dir): 0.2},
	}
	if err := WriteSimulation(dir, sim); err != nil {
		t.Fatal(err)
	}
	simulated, _, err := Cost(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !simulated.Simulated {
		t.Fatal("the document should say the usage came from the simulation")
	}
	if simulated.Total <= plain.Total {
		t.Fatalf("traffic should cost more than none: %v vs %v", simulated.Total, plain.Total)
	}
}
