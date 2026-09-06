package aws

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/mooncitizen/togen/internal/cost"
	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

func rdsInstance(id, region, instance, engine, deployment string, price float64) cost.SKU {
	return cost.SKU{ID: id, Service: "AmazonRDS", Unit: "Hrs", Price: price, Attributes: map[string]string{
		"productFamily": "Database Instance", "instanceType": instance, "databaseEngine": engine,
		"deploymentOption": deployment, "termType": "OnDemand", "regionCode": region,
	}}
}

func rdsStorage(id, region, volume, engine, deployment string, price float64) cost.SKU {
	return cost.SKU{ID: id, Service: "AmazonRDS", Unit: "GB-Mo", Price: price, Attributes: map[string]string{
		"productFamily": "Database Storage", "volumeType": volume, "databaseEngine": engine,
		"deploymentOption": deployment, "termType": "OnDemand", "regionCode": region,
	}}
}

func natGateway(id, region, usage string, price float64) cost.SKU {
	return cost.SKU{ID: id, Service: "AmazonEC2", Unit: "Hrs", Price: price, Attributes: map[string]string{
		"productFamily": "NAT Gateway", "usagetype": usage, "termType": "OnDemand", "regionCode": region,
	}}
}

// Two regions, both engines, every size, with the neighbours a loose filter would catch:
// reserved terms, gp3 and SSD-labelled storage, the regional NAT variant and its data charge.
func priceFixture() *cost.Snapshot {
	reserved := rdsInstance("rds-micro-pg-single-euw2-reserved", "eu-west-2", "db.t4g.micro", "PostgreSQL", "Single-AZ", 0.011)
	reserved.Attributes["termType"] = "Reserved"
	return &cost.Snapshot{
		Provider: ir.ProviderAWS,
		Date:     "2026-09-05",
		Currency: "USD",
		Versions: map[string]map[string]string{
			"AmazonRDS": {"eu-west-2": "v1", "us-east-1": "v1"},
			"AmazonEC2": {"eu-west-2": "v1", "us-east-1": "v1"},
		},
		SKUs: []cost.SKU{
			rdsInstance("rds-micro-pg-single-euw2", "eu-west-2", "db.t4g.micro", "PostgreSQL", "Single-AZ", 0.018),
			rdsInstance("rds-micro-pg-multi-euw2", "eu-west-2", "db.t4g.micro", "PostgreSQL", "Multi-AZ", 0.036),
			rdsInstance("rds-micro-mysql-single-euw2", "eu-west-2", "db.t4g.micro", "MySQL", "Single-AZ", 0.018),
			rdsInstance("rds-micro-mysql-multi-euw2", "eu-west-2", "db.t4g.micro", "MySQL", "Multi-AZ", 0.036),
			rdsInstance("rds-medium-pg-single-euw2", "eu-west-2", "db.t4g.medium", "PostgreSQL", "Single-AZ", 0.072),
			rdsInstance("rds-large-pg-single-euw2", "eu-west-2", "db.r6g.large", "PostgreSQL", "Single-AZ", 0.264),
			rdsInstance("rds-large-mysql-single-euw2", "eu-west-2", "db.r6g.large", "MySQL", "Single-AZ", 0.251),
			rdsInstance("rds-micro-maria-single-euw2", "eu-west-2", "db.t4g.micro", "MariaDB", "Single-AZ", 0.018),
			reserved,
			rdsStorage("rds-gp2-pg-single-euw2", "eu-west-2", "General Purpose", "PostgreSQL", "Single-AZ", 0.133),
			rdsStorage("rds-gp2-pg-multi-euw2", "eu-west-2", "General Purpose", "PostgreSQL", "Multi-AZ", 0.266),
			rdsStorage("rds-gp2-mysql-single-euw2", "eu-west-2", "General Purpose", "MySQL", "Single-AZ", 0.133),
			rdsStorage("rds-gp2-mysql-multi-euw2", "eu-west-2", "General Purpose", "MySQL", "Multi-AZ", 0.266),
			rdsStorage("rds-gp3-pg-single-euw2", "eu-west-2", "General Purpose-GP3", "PostgreSQL", "Single-AZ", 0.127),
			rdsStorage("rds-ssd-oracle-single-euw2", "eu-west-2", "General Purpose (SSD)", "Oracle", "Single-AZ", 0.133),
			natGateway("nat-euw2", "eu-west-2", "EUW2-NatGateway-Hours", 0.05),
			natGateway("nat-regional-euw2", "eu-west-2", "EUW2-RegionalNatGateway-Hours", 0.05),
			natGateway("nat-bytes-euw2", "eu-west-2", "EUW2-NatGateway-Bytes", 0.05),

			rdsInstance("rds-micro-pg-single-use1", "us-east-1", "db.t4g.micro", "PostgreSQL", "Single-AZ", 0.016),
			rdsStorage("rds-gp2-pg-single-use1", "us-east-1", "General Purpose", "PostgreSQL", "Single-AZ", 0.115),
			natGateway("nat-use1", "us-east-1", "NatGateway-Hours", 0.045),
			natGateway("nat-regional-use1", "us-east-1", "RegionalNatGateway-Hours", 0.045),
		},
	}
}

func databaseNode(t *testing.T, p ir.DatabaseProps) ir.Node {
	t.Helper()
	return ir.Node{ID: "n3", Type: ir.NodeDatabase, Name: "main-db", Properties: props(t, p)}
}

func estimate(t *testing.T, region string, nodes []ir.Node) cost.Document {
	t.Helper()
	p := newProject(t, nodes, nil)
	p.Region = region
	doc, err := cost.Estimate(p, Cost(), priceFixture(), time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestDatabaseCostPricesTheInstanceAndItsStorage(t *testing.T) {
	doc := estimate(t, "eu-west-2", []ir.Node{databaseNode(t, ir.DatabaseProps{})})
	want := cost.Priced{
		Name: "main-db", Kind: "database", Summary: "db.t4g.micro, postgres 17, single-AZ, 20 GB",
		Lines: []cost.Line{
			{Label: "instance", Quantity: 730, Unit: "h", UnitPrice: 0.018, Amount: 13.14, SKU: "rds-micro-pg-single-euw2"},
			{Label: "storage gp2", Quantity: 20, Unit: "GB", UnitPrice: 0.133, Amount: 2.66, SKU: "rds-gp2-pg-single-euw2"},
		},
		Subtotal: 15.80,
	}
	if diff := cmp.Diff(want, doc.Items[0]); diff != "" {
		t.Errorf("database (-want +got):\n%s", diff)
	}
	wantOmitted := []cost.Omission{
		{Name: "main-db", Kind: "database", Reason: "backups beyond 20 GB"},
		{Name: "network", Kind: "implicit VPC", Reason: "nat gateway data processed"},
	}
	if diff := cmp.Diff(wantOmitted, doc.NotPriced); diff != "" {
		t.Errorf("not priced (-want +got):\n%s", diff)
	}
}

func TestDatabaseCostDoublesForMultiAZ(t *testing.T) {
	doc := estimate(t, "eu-west-2", []ir.Node{databaseNode(t, ir.DatabaseProps{HighAvailability: true})})
	db := doc.Items[0]
	if db.Summary != "db.t4g.micro, postgres 17, multi-AZ, 20 GB" {
		t.Errorf("summary = %q", db.Summary)
	}
	if db.Lines[0].SKU != "rds-micro-pg-multi-euw2" || db.Lines[0].Amount != 26.28 {
		t.Errorf("instance = %+v", db.Lines[0])
	}
	if db.Lines[1].SKU != "rds-gp2-pg-multi-euw2" || db.Lines[1].Amount != 5.32 {
		t.Errorf("storage = %+v", db.Lines[1])
	}
}

func TestDatabaseCostFollowsTheEngineAndVersion(t *testing.T) {
	doc := estimate(t, "eu-west-2", []ir.Node{databaseNode(t, ir.DatabaseProps{Engine: ir.EngineMySQL, Version: "8.0", Size: ir.SizeLarge})})
	db := doc.Items[0]
	if db.Summary != "db.r6g.large, mysql 8.0, single-AZ, 20 GB" {
		t.Errorf("summary = %q", db.Summary)
	}
	if db.Lines[0].SKU != "rds-large-mysql-single-euw2" || db.Lines[0].UnitPrice != 0.251 {
		t.Errorf("instance = %+v", db.Lines[0])
	}
	if db.Lines[1].SKU != "rds-gp2-mysql-single-euw2" {
		t.Errorf("storage = %+v", db.Lines[1])
	}
}

func TestDatabaseCostScalesWithTheStorage(t *testing.T) {
	doc := estimate(t, "eu-west-2", []ir.Node{databaseNode(t, ir.DatabaseProps{StorageGB: 100})})
	db := doc.Items[0]
	if db.Summary != "db.t4g.micro, postgres 17, single-AZ, 100 GB" {
		t.Errorf("summary = %q", db.Summary)
	}
	if db.Lines[1].Quantity != 100 || db.Lines[1].Amount != 13.30 || db.Subtotal != 26.44 {
		t.Errorf("storage = %+v, subtotal = %v", db.Lines[1], db.Subtotal)
	}
	if doc.NotPriced[0].Reason != "backups beyond 100 GB" {
		t.Errorf("omission = %+v", doc.NotPriced[0])
	}
}

func TestCostFollowsTheProjectRegion(t *testing.T) {
	doc := estimate(t, "us-east-1", []ir.Node{databaseNode(t, ir.DatabaseProps{})})
	if got := doc.Items[0].Lines[0]; got.SKU != "rds-micro-pg-single-use1" || got.Amount != 11.68 {
		t.Errorf("instance = %+v", got)
	}
	if got := doc.Items[0].Lines[1]; got.SKU != "rds-gp2-pg-single-use1" || got.Amount != 2.30 {
		t.Errorf("storage = %+v", got)
	}
	if got := doc.Items[1].Lines[0]; got.SKU != "nat-use1" || got.Amount != 32.85 {
		t.Errorf("nat = %+v", got)
	}
	if doc.Total != 46.83 {
		t.Errorf("total = %v", doc.Total)
	}
}

func TestNetworkCostComesWithTheFirstNodeThatNeedsIt(t *testing.T) {
	serverless := []ir.Node{
		{ID: "n1", Type: ir.NodeGateway, Name: "api"},
		{ID: "n2", Type: ir.NodeFunction, Name: "handler"},
	}
	doc := estimate(t, "eu-west-2", serverless)
	if len(doc.Items) != 0 || doc.Total != 0 {
		t.Errorf("items = %+v", doc.Items)
	}
	wantOmitted := []cost.Omission{
		{Name: "api", Kind: "gateway", Reason: "no aws prices for this node type yet"},
		{Name: "handler", Kind: "function", Reason: "no aws prices for this node type yet"},
	}
	if diff := cmp.Diff(wantOmitted, doc.NotPriced); diff != "" {
		t.Errorf("not priced (-want +got):\n%s", diff)
	}

	doc = estimate(t, "eu-west-2", append(serverless, databaseNode(t, ir.DatabaseProps{})))
	want := cost.Priced{
		Name: "network", Kind: "implicit VPC",
		Lines:    []cost.Line{{Label: "nat gateway", Quantity: 730, Unit: "h", UnitPrice: 0.05, Amount: 36.50, SKU: "nat-euw2"}},
		Subtotal: 36.50,
	}
	if diff := cmp.Diff(want, doc.Items[1]); diff != "" {
		t.Errorf("network (-want +got):\n%s", diff)
	}
	if doc.Total != 52.30 {
		t.Errorf("total = %v", doc.Total)
	}
}

func TestNeedsNetworkAgreesWithTheResolver(t *testing.T) {
	gateway := ir.Node{ID: "g", Type: ir.NodeGateway, Name: "api"}
	function := ir.Node{ID: "f", Type: ir.NodeFunction, Name: "handler"}
	for _, c := range []struct {
		name  string
		nodes []ir.Node
		edges []ir.Edge
	}{
		{"nothing", nil, nil},
		{"gateway and function", []ir.Node{gateway, function}, []ir.Edge{{ID: "e", From: "g", To: "f", Relation: ir.RelRoutes}}},
		{"queue and bucket", []ir.Node{{ID: "q", Type: ir.NodeQueue, Name: "jobs"}, {ID: "b", Type: ir.NodeBucket, Name: "uploads"}}, nil},
		{"database", []ir.Node{databaseNode(t, ir.DatabaseProps{})}, nil},
		{"cache", []ir.Node{{ID: "c", Type: ir.NodeCache, Name: "sessions"}}, nil},
		{"service", []ir.Node{{ID: "s", Type: ir.NodeService, Name: "web", Properties: props(t, ir.ServiceProps{Image: "nginx"})}}, nil},
		{"function reading a database", []ir.Node{function, databaseNode(t, ir.DatabaseProps{})}, []ir.Edge{{ID: "e", From: "f", To: "n3", Relation: ir.RelReads}}},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := newProject(t, c.nodes, c.edges)
			graph, err := resolve.Run(p, New())
			if err != nil {
				t.Fatal(err)
			}
			hasVPC := false
			for _, r := range graph.Resources {
				hasVPC = hasVPC || r.Type == "aws_vpc"
			}
			if got := needsNetwork(p); got != hasVPC {
				t.Errorf("needsNetwork = %v, resolver made a vpc = %v", got, hasVPC)
			}
		})
	}
}

func TestCatalogueCoversEverySizeEngineAndDeployment(t *testing.T) {
	lookups, err := Cost().Catalogue("eu-west-2")
	if err != nil {
		t.Fatal(err)
	}
	distinct := map[string]bool{}
	for _, l := range lookups {
		distinct[l.Describe()] = true
	}
	// 3 sizes x 2 engines x 2 deployments of instance, 2 x 2 of storage, and the nat gateway.
	if len(distinct) != 12+4+1 {
		t.Errorf("distinct lookups = %d, want 17", len(distinct))
	}
}
