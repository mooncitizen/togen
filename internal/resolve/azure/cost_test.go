package azure

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/mooncitizen/togen/internal/cost"
	"github.com/mooncitizen/togen/internal/ir"
)

func meter(id, service, region, product, sku, name, measure, unit string, price, upTo float64) cost.SKU {
	return cost.SKU{ID: id, Service: service, Unit: unit, Price: price, UpTo: upTo, Attributes: map[string]string{
		"productName": product, "skuName": sku, "meterName": name, "armRegionName": region, "unitOfMeasure": measure,
	}}
}

func hourly(id, service, region, product, sku, name string, price float64) cost.SKU {
	return meter(id, service, region, product, sku, name, "1 Hour", "hour", price, 0)
}

// Two regions with every meter the matchers name and the neighbours a loose filter would
// catch: the single server and premium storage rows, the general purpose meter under both
// of its sku names, the whole-cache Standard meter, idle and dedicated Container Apps,
// Flex Consumption functions, the per-month base unit and Premium Service Bus, the other
// blob operations, and the Internet routing and inter-zone bandwidth rows.
func priceFixture() *cost.Snapshot {
	const (
		pg       = "Azure Database for PostgreSQL"
		mysql    = "Azure Database for MySQL"
		pgFlex   = "Azure Database for PostgreSQL Flexible Server "
		myFlex   = "Azure Database for MySQL Flexible Server "
		apps     = "Azure Container Apps"
		bus      = "Service Bus"
		blob     = "General Block Blob v2"
		mgn      = "Rtn Preference: MGN"
		internet = "Bandwidth - Routing Preference: Internet"
	)
	versions := map[string]string{"uksouth": "v", "eastus": "v"}
	return &cost.Snapshot{
		Provider: ir.ProviderAzure,
		Date:     "2026-09-06",
		Currency: "USD",
		Versions: map[string]map[string]string{
			pg: versions, mysql: versions, apps: versions, "Functions": versions, bus: versions,
			"Redis Cache": versions, "Storage": versions, "Bandwidth": versions,
		},
		SKUs: []cost.SKU{
			hourly("pg-b1ms-uks", pg, "uksouth", pgFlex+"Burstable BS Series Compute", "B1MS", "B1MS", 0.019),
			hourly("pg-b2s-uks", pg, "uksouth", pgFlex+"Burstable BS Series Compute", "B2S", "B2S", 0.076),
			hourly("pg-ddsv4-uks", pg, "uksouth", pgFlex+"General Purpose Ddsv4 Series Compute", "vCore", "vCore", 0.103),
			hourly("pg-ddsv5-uks", pg, "uksouth", pgFlex+"General Purpose Ddsv5 Series Compute", "vCore", "vCore", 0.099),
			meter("pg-storage-uks", pg, "uksouth", "Azure Database for PostgreSQL Flex Server Storage", "Storage", "Storage Data Stored", "1 GB/Month", "GB-month", 0.133, 0),
			meter("pg-ssdv2-uks", pg, "uksouth", "Azure Database for PostgreSQL Flex Server Storage", "Premium SSD v2 Storage", "Premium SSD v2 Storage Data Stored", "1 GiB/Month", "GiB-month", 0.133, 0),
			meter("pg-single-storage-uks", pg, "uksouth", "Azure Database for PostgreSQL Single Server General Purpose - Storage", "General Purpose", "General Purpose Data Stored", "1 GB/Month", "GB-month", 0.1334, 0),
			hourly("my-b1ms-uks", mysql, "uksouth", myFlex+"Burstable BS Series Compute", "B1MS", "B1MS", 0.019),
			hourly("my-basic-uks", mysql, "uksouth", myFlex+"Burstable BS Series Compute", "Basic", "Basic", 0.0095),
			hourly("my-gp-uks", mysql, "uksouth", myFlex+"General Purpose Series Compute", "1 vCore", "vCore", 0.099),
			hourly("my-ddsv5-uks", mysql, "uksouth", myFlex+"General Purpose Ddsv5 Series Compute", "vCore", "vCore", 0.099),
			meter("my-storage-uks", mysql, "uksouth", myFlex+"Storage", "Storage", "Storage Data Stored", "1 GB/Month", "GB-month", 0.133, 0),
			meter("my-storage-zrs-uks", mysql, "uksouth", myFlex+"Storage", "Storage ZRS", "Storage ZRS Data Stored", "1 GB/Month", "GB-month", 0.266, 0),

			hourly("redis-basic-c0-uks", "Redis Cache", "uksouth", "Azure Redis Cache Basic", "C0", "C0 Cache", 0.028),
			hourly("redis-standard-c0-uks", "Redis Cache", "uksouth", "Azure Redis Cache Standard", "C0", "C0 Cache", 0.069),
			hourly("redis-standard-c0-node-uks", "Redis Cache", "uksouth", "Azure Redis Cache Standard", "C0", "C0 Cache Instance", 0.0345),
			hourly("redis-standard-c1-uks", "Redis Cache", "uksouth", "Azure Redis Cache Standard", "C1", "C1 Cache", 0.173),
			hourly("redis-standard-c1-node-uks", "Redis Cache", "uksouth", "Azure Redis Cache Standard", "C1", "C1 Cache Instance", 0.0865),
			hourly("redis-basic-c3-uks", "Redis Cache", "uksouth", "Azure Redis Cache Basic", "C3", "C3 Cache", 0.225),
			hourly("redis-standard-c3-uks", "Redis Cache", "uksouth", "Azure Redis Cache Standard", "C3", "C3 Cache", 0.563),
			hourly("redis-standard-c3-node-uks", "Redis Cache", "uksouth", "Azure Redis Cache Standard", "C3", "C3 Cache Instance", 0.2815),

			meter("apps-vcpu-uks", apps, "uksouth", apps, "Standard", "Standard vCPU Active Usage", "1 Second", "hour", 0.1224, 0),
			meter("apps-vcpu-idle-uks", apps, "uksouth", apps, "Standard", "Standard vCPU Idle Usage", "1 Second", "hour", 0.0144, 0),
			meter("apps-memory-uks", apps, "uksouth", apps, "Standard", "Standard Memory Active Usage", "1 GiB Second", "GiB-hour", 0.0144, 0),
			meter("apps-memory-idle-uks", apps, "uksouth", apps, "Standard", "Standard Memory Idle Usage", "1 GiB Second", "GiB-hour", 0.0144, 0),
			meter("apps-requests-uks", apps, "uksouth", apps, "Standard", "Standard Requests", "1M", "each", 0.0000004, 0),
			hourly("apps-dedicated-vcpu-uks", apps, "uksouth", apps, "Dedicated", "Dedicated vCPU Usage", 0.080859),

			meter("fn-executions-uks", "Functions", "uksouth", "Functions", "Standard", "Standard Total Executions", "10", "each", 0.0000002, 0),
			meter("fn-duration-uks", "Functions", "uksouth", "Functions", "Standard", "Standard Execution Time", "1 GB Second", "GB-second", 0.000016, 0),
			meter("fn-flex-executions-uks", "Functions", "uksouth", "Flex Consumption", "On Demand", "On Demand Total Executions", "10", "each", 0.0000004, 0),
			meter("fn-flex-duration-uks", "Functions", "uksouth", "Flex Consumption", "On Demand", "On Demand Execution Time", "1 GB Second", "GB-second", 0.000037, 0),

			meter("bus-basic-ops-uks", bus, "uksouth", bus, "Basic", "Basic Messaging Operations", "1M", "each", 0.00000005, 0),
			meter("bus-standard-ops-uks", bus, "uksouth", bus, "Standard", "Standard Messaging Operations", "1M", "each", 0.0000008, 100000000),
			meter("bus-standard-base-uks", bus, "uksouth", bus, "Standard", "Standard Base Unit", "1/Month", "month", 10, 0),
			meter("bus-standard-base-hour-uks", bus, "uksouth", bus, "Standard", "Standard Base Unit", "1/Hour", "hour", 0.013441, 0),
			meter("bus-premium-uks", bus, "uksouth", bus, "Premium", "Premium Messaging Unit", "1/Hour", "hour", 0.9275, 0),

			meter("blob-storage-uks", "Storage", "uksouth", blob, "Hot LRS", "Hot LRS Data Stored", "1 GB/Month", "GB-month", 0.0192, 51200),
			meter("blob-write-uks", "Storage", "uksouth", blob, "Hot LRS", "Hot LRS Write Operations", "10K", "each", 0.0000059, 0),
			meter("blob-read-uks", "Storage", "uksouth", blob, "Hot LRS", "Hot Read Operations", "10K", "each", 0.00000047, 0),
			meter("blob-other-uks", "Storage", "uksouth", blob, "Hot LRS", "All Other Operations", "10K", "each", 0.00000047, 0),
			meter("blob-cool-storage-uks", "Storage", "uksouth", blob, "Cool LRS", "Cool LRS Data Stored", "1 GB/Month", "GB-month", 0.01, 0),
			meter("egress-uks", "Bandwidth", "uksouth", mgn, "Standard", "Standard Data Transfer Out", "1 GB", "GB", 0.087, 10335),
			meter("egress-internet-uks", "Bandwidth", "uksouth", internet, "Standard", "Standard Data Transfer Out", "1 GB", "GB", 0.08, 10100),
			meter("egress-zones-uks", "Bandwidth", "uksouth", mgn, "Standard", "Standard Inter-Availability Zone Data Transfer Out", "1 GB", "GB", 0.01, 0),

			hourly("pg-b1ms-eus", pg, "eastus", pgFlex+"Burstable BS Series Compute", "B1MS", "B1MS", 0.017),
			hourly("pg-ddsv4-eus", pg, "eastus", pgFlex+"General Purpose Ddsv4 Series Compute", "1 vCore", "vCore", 0.089),
			meter("pg-storage-eus", pg, "eastus", "Azure Database for PostgreSQL Flex Server Storage", "Storage", "Storage Data Stored", "1 GB/Month", "GB-month", 0.115, 0),
			hourly("redis-basic-c0-eus", "Redis Cache", "eastus", "Azure Redis Cache Basic", "C0", "C0 Cache", 0.022),
			meter("apps-vcpu-eus", apps, "eastus", apps, "Standard", "Standard vCPU Active Usage", "1 Second", "hour", 0.1152, 0),
			meter("apps-memory-eus", apps, "eastus", apps, "Standard", "Standard Memory Active Usage", "1 GiB Second", "GiB-hour", 0.0144, 0),
			meter("apps-requests-eus", apps, "eastus", apps, "Standard", "Standard Requests", "1M", "each", 0.0000004, 0),
			meter("fn-executions-eus", "Functions", "eastus", "Functions", "Standard", "Standard Total Executions", "10", "each", 0.0000002, 0),
			meter("fn-duration-eus", "Functions", "eastus", "Functions", "Standard", "Standard Execution Time", "1 GB Second", "GB-second", 0.000016, 0),
			meter("bus-basic-ops-eus", bus, "eastus", bus, "Basic", "Basic Messaging Operations", "1M", "each", 0.00000005, 0),
			meter("egress-eus", "Bandwidth", "eastus", mgn, "Standard", "Standard Data Transfer Out", "1 GB", "GB", 0.087, 10335),
		},
	}
}

func estimate(t *testing.T, region string, nodes []ir.Node) cost.Document {
	t.Helper()
	return estimateUsage(t, region, nodes, nil)
}

func estimateUsage(t *testing.T, region string, nodes []ir.Node, usage cost.Usage) cost.Document {
	t.Helper()
	p := newProject(t, nodes)
	p.Region = region
	doc, err := cost.Estimate(p, Cost(), priceFixture(), usage, time.Date(2026, 9, 6, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

// A pay-per-use item on defaults carries no lines, so its meters are checked by hand against
// the fixture.
func usageSKUs(t *testing.T, n ir.Node, region string) []string {
	t.Helper()
	p := newProject(t, []ir.Node{n})
	item, ok, err := Cost().Node(p.Nodes[0], region, cost.NodeUsage{})
	if err != nil || !ok {
		t.Fatalf("node = %+v, %v, %v", item, ok, err)
	}
	var ids []string
	for _, l := range item.Usage {
		sku, err := priceFixture().Find(l)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, sku.ID)
	}
	return ids
}

func TestDatabaseCostPricesTheServerAndItsStorageTier(t *testing.T) {
	doc := estimate(t, "uksouth", []ir.Node{databaseNode(t, "n3", "main-db", ir.DatabaseProps{})})
	want := cost.Priced{
		Name: "main-db", Kind: "database", Summary: "B_Standard_B1ms, postgres 17, single zone, 32 GB",
		Lines: []cost.Line{
			{Label: "compute", Quantity: 730, Unit: "h", UnitPrice: 0.019, Amount: 13.87, SKU: "pg-b1ms-uks"},
			{Label: "storage", Quantity: 32, Unit: "GB", UnitPrice: 0.133, Amount: 4.26, SKU: "pg-storage-uks"},
		},
		Subtotal: 18.13,
	}
	if diff := cmp.Diff(want, doc.Items[0]); diff != "" {
		t.Errorf("database (-want +got):\n%s", diff)
	}
	wantOmitted := []cost.Omission{{Name: "main-db", Kind: "database", Reason: "backups beyond 32 GB"}}
	if diff := cmp.Diff(wantOmitted, doc.NotPriced); diff != "" {
		t.Errorf("not priced (-want +got):\n%s", diff)
	}
	if len(doc.Items) != 1 || doc.Total != 18.13 {
		t.Errorf("items = %d, total = %v", len(doc.Items), doc.Total)
	}
}

func TestDatabaseCostPricesGeneralPurposePerVCoreAndDoublesForHighAvailability(t *testing.T) {
	doc := estimate(t, "uksouth", []ir.Node{databaseNode(t, "n3", "main-db", ir.DatabaseProps{Size: ir.SizeMedium, StorageGB: 50})})
	db := doc.Items[0]
	if db.Summary != "GP_Standard_D2ds_v4, postgres 17, single zone, 64 GB" {
		t.Errorf("summary = %q", db.Summary)
	}
	if got := db.Lines[0]; got.SKU != "pg-ddsv4-uks" || got.Quantity != 1460 || got.Unit != "vCore-h" || got.Amount != 150.38 {
		t.Errorf("compute = %+v", got)
	}
	if got := db.Lines[1]; got.Quantity != 64 || got.Amount != 8.51 {
		t.Errorf("storage = %+v", got)
	}

	doc = estimate(t, "uksouth", []ir.Node{databaseNode(t, "n3", "main-db", ir.DatabaseProps{Size: ir.SizeLarge, HighAvailability: true})})
	db = doc.Items[0]
	if db.Summary != "GP_Standard_D4ds_v4, postgres 17, zone redundant, 32 GB" {
		t.Errorf("summary = %q", db.Summary)
	}
	if got := db.Lines[0]; got.Quantity != 5840 || got.Amount != 601.52 {
		t.Errorf("compute = %+v", got)
	}
	if got := db.Lines[1]; got.Quantity != 64 || got.Amount != 8.51 {
		t.Errorf("storage = %+v", got)
	}
}

func TestDatabaseCostFollowsTheEngine(t *testing.T) {
	doc := estimate(t, "uksouth", []ir.Node{databaseNode(t, "n3", "main-db", ir.DatabaseProps{Engine: ir.EngineMySQL, Version: "5.7", StorageGB: 50})})
	db := doc.Items[0]
	if db.Summary != "B_Standard_B1ms, mysql 5.7, single zone, 50 GB" {
		t.Errorf("summary = %q", db.Summary)
	}
	if db.Lines[0].SKU != "my-b1ms-uks" || db.Lines[1].SKU != "my-storage-uks" || db.Lines[1].Quantity != 50 {
		t.Errorf("lines = %+v", db.Lines)
	}

	doc = estimate(t, "uksouth", []ir.Node{databaseNode(t, "n3", "main-db", ir.DatabaseProps{Engine: ir.EngineMySQL, Size: ir.SizeMedium})})
	if got := doc.Items[0].Lines[0]; got.SKU != "my-gp-uks" || got.Quantity != 1460 {
		t.Errorf("compute = %+v", got)
	}
}

func TestCostFollowsTheProjectRegion(t *testing.T) {
	doc := estimate(t, "eastus", []ir.Node{databaseNode(t, "n3", "main-db", ir.DatabaseProps{Size: ir.SizeMedium})})
	if got := doc.Items[0].Lines[0]; got.SKU != "pg-ddsv4-eus" || got.Amount != 129.94 {
		t.Errorf("compute = %+v", got)
	}
	if got := doc.Items[0].Lines[1]; got.SKU != "pg-storage-eus" || got.Amount != 3.68 {
		t.Errorf("storage = %+v", got)
	}
}

func TestCacheCostPricesTheWholeCacheOfEverySize(t *testing.T) {
	for _, c := range []struct {
		size    ir.Size
		summary string
		sku     string
		hours   float64
		price   float64
		amount  float64
	}{
		{ir.SizeSmall, "Basic C0, redis 6, 1 node", "redis-basic-c0-uks", 730, 0.028, 20.44},
		{ir.SizeMedium, "Standard C1, redis 6, 2 nodes", "redis-standard-c1-node-uks", 1460, 0.0865, 126.29},
		{ir.SizeLarge, "Standard C3, redis 6, 2 nodes", "redis-standard-c3-node-uks", 1460, 0.2815, 410.99},
	} {
		node := ir.Node{ID: "c1", Type: ir.NodeCache, Name: "sessions", Properties: props(t, ir.CacheProps{Size: c.size})}
		doc := estimate(t, "uksouth", []ir.Node{node})
		want := cost.Priced{
			Name: "sessions", Kind: "cache", Summary: c.summary,
			Lines:    []cost.Line{{Label: "node", Quantity: c.hours, Unit: "h", UnitPrice: c.price, Amount: c.amount, SKU: c.sku}},
			Subtotal: c.amount,
		}
		if diff := cmp.Diff(want, doc.Items[0]); diff != "" {
			t.Errorf("%s cache (-want +got):\n%s", c.size, diff)
		}
		if len(doc.NotPriced) != 0 {
			t.Errorf("not priced = %+v", doc.NotPriced)
		}
	}
	if got := estimate(t, "eastus", []ir.Node{{ID: "c1", Type: ir.NodeCache, Name: "sessions"}}).Items[0].Lines[0]; got.SKU != "redis-basic-c0-eus" || got.Amount != 16.06 {
		t.Errorf("eastus cache = %+v", got)
	}
}

func TestServiceCostPricesTheReplicasActiveByTheHour(t *testing.T) {
	node := serviceNodeWith(t, "s1", "web", ir.ServiceProps{Image: "nginx:1.27", Public: true})
	doc := estimate(t, "uksouth", []ir.Node{node})
	want := cost.Priced{
		Name: "web", Kind: "service", Summary: "0.25 vCPU, 0.5 GiB, 1 replica, public", Note: "priced on defaults, no usage set",
		Lines: []cost.Line{
			{Label: "vcpu", Quantity: 182.5, Unit: "vCPU-h", UnitPrice: 0.1224, Amount: 22.34, SKU: "apps-vcpu-uks"},
			{Label: "memory", Quantity: 365, Unit: "GiB-h", UnitPrice: 0.0144, Amount: 5.26, SKU: "apps-memory-uks"},
		},
		Subtotal: 27.60,
	}
	if diff := cmp.Diff(want, doc.Items[0]); diff != "" {
		t.Errorf("service (-want +got):\n%s", diff)
	}
	wantOmitted := []cost.Omission{
		{Name: "web", Kind: "service", Reason: "requests"},
		{Name: "web", Kind: "service", Reason: "egress"},
	}
	if diff := cmp.Diff(wantOmitted, doc.NotPriced); diff != "" {
		t.Errorf("not priced (-want +got):\n%s", diff)
	}
	if len(doc.Items) != 1 || doc.Total != 27.60 {
		t.Errorf("items = %d, total = %v", len(doc.Items), doc.Total)
	}
	if diff := cmp.Diff([]string{"apps-requests-uks", "egress-uks"}, usageSKUs(t, node, "uksouth")); diff != "" {
		t.Errorf("uksouth meters (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"apps-requests-eus", "egress-eus"}, usageSKUs(t, node, "eastus")); diff != "" {
		t.Errorf("eastus meters (-want +got):\n%s", diff)
	}
}

func TestServiceCostScalesWithTheSizeAndReplicas(t *testing.T) {
	for _, c := range []struct {
		size         ir.Size
		replicas     int
		summary      string
		vcpu, memory float64
		subtotal     float64
	}{
		{ir.SizeSmall, 3, "0.25 vCPU, 0.5 GiB, 3 replicas, private", 547.5, 1095, 82.78},
		{ir.SizeMedium, 2, "0.5 vCPU, 1 GiB, 2 replicas, private", 730, 1460, 110.37},
		{ir.SizeLarge, 1, "1 vCPU, 2 GiB, 1 replica, private", 730, 1460, 110.37},
		{ir.SizeMedium, 0, "0.5 vCPU, 1 GiB, 0 replicas, private", 0, 0, 0},
	} {
		t.Run(c.summary, func(t *testing.T) {
			node := serviceNodeWith(t, "s1", "web", ir.ServiceProps{Image: "nginx:1.27", Size: c.size, MinReplicas: ir.Ptr(c.replicas)})
			svc := estimate(t, "uksouth", []ir.Node{node}).Items[0]
			if svc.Summary != c.summary || len(svc.Lines) != 2 || svc.Subtotal != c.subtotal {
				t.Errorf("service = %+v", svc)
			}
			if svc.Lines[0].Quantity != c.vcpu || svc.Lines[1].Quantity != c.memory {
				t.Errorf("quantities = %v vCPU-h, %v GiB-h, want %v, %v", svc.Lines[0].Quantity, svc.Lines[1].Quantity, c.vcpu, c.memory)
			}
		})
	}
}

func TestServiceCostPricesRequestsAndEgressFromTheUsageBlock(t *testing.T) {
	node := serviceNodeWith(t, "s1", "web", ir.ServiceProps{Image: "nginx:1.27", Public: true})
	doc := estimateUsage(t, "uksouth", []ir.Node{node}, cost.Usage{"web": {Requests: "1M/month", EgressGb: 100}})
	svc := doc.Items[0]
	if svc.Note != "" || len(svc.Lines) != 4 {
		t.Fatalf("service = %+v", svc)
	}
	if got := svc.Lines[2]; got.Label != "requests" || got.Quantity != 1000000 || got.Unit != "requests" || got.Amount != 0.40 {
		t.Errorf("requests = %+v", got)
	}
	if got := svc.Lines[3]; got.Label != "egress" || got.Quantity != 100 || got.Amount != 8.70 || got.Note != "" {
		t.Errorf("egress = %+v", got)
	}
	if len(doc.NotPriced) != 0 {
		t.Errorf("not priced = %+v", doc.NotPriced)
	}
}

func TestFunctionCostIsPricedOnDefaultsOnTheConsumptionMeters(t *testing.T) {
	handler := functionNode(t, "f1", "handler", ir.FunctionProps{})
	doc := estimate(t, "uksouth", []ir.Node{handler})
	want := cost.Priced{
		Name: "handler", Kind: "function", Summary: "node, consumption plan, 512 MB", Note: "priced on defaults, no usage set",
		Lines: []cost.Line{}, Subtotal: 0,
	}
	if diff := cmp.Diff(want, doc.Items[0]); diff != "" {
		t.Errorf("function (-want +got):\n%s", diff)
	}
	wantOmitted := []cost.Omission{
		{Name: "handler", Kind: "function", Reason: "executions"},
		{Name: "handler", Kind: "function", Reason: "duration"},
	}
	if diff := cmp.Diff(wantOmitted, doc.NotPriced); diff != "" {
		t.Errorf("not priced (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"fn-executions-uks", "fn-duration-uks"}, usageSKUs(t, handler, "uksouth")); diff != "" {
		t.Errorf("uksouth meters (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"fn-executions-eus", "fn-duration-eus"}, usageSKUs(t, handler, "eastus")); diff != "" {
		t.Errorf("eastus meters (-want +got):\n%s", diff)
	}
}

func TestFunctionCostPricesExecutionsAndGBSecondsAtTheSizesMemory(t *testing.T) {
	for _, c := range []struct {
		props    ir.FunctionProps
		summary  string
		duration float64
		amount   float64
	}{
		{ir.FunctionProps{Size: ir.SizeSmall}, "node, consumption plan, 512 MB", 300000, 4.80},
		{ir.FunctionProps{Size: ir.SizeMedium, Runtime: ir.RuntimePython}, "python, consumption plan, 1024 MB", 600000, 9.60},
		{ir.FunctionProps{Size: ir.SizeLarge, Runtime: ir.RuntimeGo}, "go, consumption plan, 1536 MB", 900000, 14.40},
	} {
		handler := functionNode(t, "f1", "handler", c.props)
		doc := estimateUsage(t, "uksouth", []ir.Node{handler}, cost.Usage{"handler": {Invocations: "2M/month", DurationMs: 300}})
		fn := doc.Items[0]
		if fn.Summary != c.summary || fn.Note != "" || len(fn.Lines) != 2 {
			t.Errorf("function = %+v", fn)
			continue
		}
		if got := fn.Lines[0]; got.Label != "executions" || got.Quantity != 2000000 || got.Unit != "executions" || got.Amount != 0.40 || got.Note != "" {
			t.Errorf("executions = %+v", got)
		}
		if got := fn.Lines[1]; got.Label != "duration" || got.Quantity != c.duration || got.Unit != "GB-s" || got.Amount != c.amount || got.Note != "" {
			t.Errorf("duration = %+v", got)
		}
	}
}

func TestGatewayCostHasNothingToPrice(t *testing.T) {
	api := ir.Node{ID: "n1", Type: ir.NodeGateway, Name: "api"}
	doc := estimate(t, "uksouth", []ir.Node{api})
	want := cost.Priced{
		Name: "api", Kind: "gateway", Summary: "no resource, routes go to the targets' own URLs",
		Lines: []cost.Line{}, Subtotal: 0,
	}
	if diff := cmp.Diff(want, doc.Items[0]); diff != "" {
		t.Errorf("gateway (-want +got):\n%s", diff)
	}
	if len(doc.NotPriced) != 0 || doc.Total != 0 {
		t.Errorf("not priced = %+v, total = %v", doc.NotPriced, doc.Total)
	}
}

func TestQueueCostFollowsTheFifoPropertyToTheNamespaceTier(t *testing.T) {
	jobs := ir.Node{ID: "q1", Type: ir.NodeQueue, Name: "jobs", Properties: props(t, ir.QueueProps{})}
	doc := estimate(t, "uksouth", []ir.Node{jobs})
	want := cost.Priced{
		Name: "jobs", Kind: "queue", Summary: "standard, Basic namespace", Note: "priced on defaults, no usage set",
		Lines: []cost.Line{}, Subtotal: 0,
	}
	if diff := cmp.Diff(want, doc.Items[0]); diff != "" {
		t.Errorf("queue (-want +got):\n%s", diff)
	}
	wantOmitted := []cost.Omission{{Name: "jobs", Kind: "queue", Reason: "operations (3 per message)"}}
	if diff := cmp.Diff(wantOmitted, doc.NotPriced); diff != "" {
		t.Errorf("not priced (-want +got):\n%s", diff)
	}
	if len(doc.Items) != 1 {
		t.Errorf("items = %+v", doc.Items)
	}
	if diff := cmp.Diff([]string{"bus-basic-ops-uks"}, usageSKUs(t, jobs, "uksouth")); diff != "" {
		t.Errorf("uksouth meter (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"bus-basic-ops-eus"}, usageSKUs(t, jobs, "eastus")); diff != "" {
		t.Errorf("eastus meter (-want +got):\n%s", diff)
	}

	events := ir.Node{ID: "q2", Type: ir.NodeQueue, Name: "events", Properties: props(t, ir.QueueProps{FIFO: true})}
	doc = estimateUsage(t, "uksouth", []ir.Node{jobs, events}, cost.Usage{"events": {Messages: "1M/month"}})
	if len(doc.Items) != 3 {
		t.Fatalf("items = %+v", doc.Items)
	}
	queue := doc.Items[1]
	if queue.Summary != "FIFO (sessions), Standard namespace" || queue.Note != "" || len(queue.Lines) != 1 {
		t.Errorf("fifo queue = %+v", queue)
	}
	if got := queue.Lines[0]; got.SKU != "bus-standard-ops-uks" || got.Quantity != 3000000 || got.Unit != "operations" || got.Amount != 2.40 {
		t.Errorf("operations = %+v", got)
	}
	wantBus := cost.Priced{
		Name: "bus", Kind: "implicit namespace", Summary: "Service Bus Standard, shared by the queues",
		Lines:    []cost.Line{{Label: "base charge", Quantity: 730, Unit: "h", UnitPrice: 0.013441, Amount: 9.81, SKU: "bus-standard-base-hour-uks"}},
		Subtotal: 9.81,
	}
	if diff := cmp.Diff(wantBus, doc.Items[2]); diff != "" {
		t.Errorf("namespace (-want +got):\n%s", diff)
	}
	if doc.Total != 12.21 {
		t.Errorf("total = %v", doc.Total)
	}
}

func TestBucketCostPricesStorageOperationsAndEgressFromTheUsageBlock(t *testing.T) {
	uploads := ir.Node{ID: "b1", Type: ir.NodeBucket, Name: "uploads", Properties: props(t, ir.BucketProps{Versioning: true})}
	doc := estimate(t, "uksouth", []ir.Node{uploads})
	want := cost.Priced{
		Name: "uploads", Kind: "bucket", Summary: "hot block blob, LRS, versioned, private", Note: "priced on defaults, no usage set",
		Lines: []cost.Line{}, Subtotal: 0,
	}
	if diff := cmp.Diff(want, doc.Items[0]); diff != "" {
		t.Errorf("bucket (-want +got):\n%s", diff)
	}
	wantOmitted := []cost.Omission{
		{Name: "uploads", Kind: "bucket", Reason: "storage"},
		{Name: "uploads", Kind: "bucket", Reason: "put requests (1 in 10)"},
		{Name: "uploads", Kind: "bucket", Reason: "get requests (9 in 10)"},
		{Name: "uploads", Kind: "bucket", Reason: "egress"},
	}
	if diff := cmp.Diff(wantOmitted, doc.NotPriced); diff != "" {
		t.Errorf("not priced (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"blob-storage-uks", "blob-write-uks", "blob-read-uks", "egress-uks"}, usageSKUs(t, uploads, "uksouth")); diff != "" {
		t.Errorf("meters (-want +got):\n%s", diff)
	}

	doc = estimateUsage(t, "uksouth", []ir.Node{uploads}, cost.Usage{"uploads": {StorageGb: 200, EgressGb: 40, Requests: "1M/month"}})
	bucket := doc.Items[0]
	wantLines := []cost.Line{
		{Label: "storage", Quantity: 200, Unit: "GB", UnitPrice: 0.0192, Amount: 3.84, SKU: "blob-storage-uks"},
		{Label: "put requests (1 in 10)", Quantity: 100000, Unit: "requests", UnitPrice: 0.0000059, Amount: 0.59, SKU: "blob-write-uks"},
		{Label: "get requests (9 in 10)", Quantity: 900000, Unit: "requests", UnitPrice: 0.00000047, Amount: 0.42, SKU: "blob-read-uks"},
		{Label: "egress", Quantity: 40, Unit: "GB", UnitPrice: 0.087, Amount: 3.48, SKU: "egress-uks"},
	}
	if diff := cmp.Diff(wantLines, bucket.Lines); diff != "" {
		t.Errorf("lines (-want +got):\n%s", diff)
	}
	if bucket.Note != "" || bucket.Subtotal != 8.33 || len(doc.NotPriced) != 0 {
		t.Errorf("bucket = %+v, not priced = %+v", bucket, doc.NotPriced)
	}
}

func TestImplicitItemsAreOnlyTheStandardNamespace(t *testing.T) {
	nodes := []ir.Node{
		{ID: "n1", Type: ir.NodeGateway, Name: "api"},
		functionNode(t, "f1", "handler", ir.FunctionProps{}),
		databaseNode(t, "n3", "main-db", ir.DatabaseProps{}),
		serviceNodeWith(t, "s1", "web", ir.ServiceProps{Image: "nginx:1.27"}),
		{ID: "q1", Type: ir.NodeQueue, Name: "jobs"},
	}
	doc := estimate(t, "uksouth", nodes)
	var names []string
	for _, item := range doc.Items {
		names = append(names, item.Name)
	}
	if diff := cmp.Diff([]string{"api", "handler", "main-db", "web", "jobs"}, names); diff != "" {
		t.Errorf("items (-want +got):\n%s", diff)
	}
	if doc.Total != 45.73 {
		t.Errorf("total = %v", doc.Total)
	}
}

// Every lookup the catalogue lists finds one meter in the fixture, so a matcher that names a
// meter the fixture has no row for, or a loose one, fails here before the refresh does.
func TestCatalogueLookupsEachPickOneMeter(t *testing.T) {
	lookups, err := Cost().Catalogue("uksouth")
	if err != nil {
		t.Fatal(err)
	}
	if len(lookups) < 40 {
		t.Fatalf("lookups = %d", len(lookups))
	}
	for _, l := range lookups {
		if _, err := priceFixture().Find(l); err != nil {
			t.Error(err)
		}
	}
}
