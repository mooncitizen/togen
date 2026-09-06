package aws

import (
	"encoding/json"
	"strings"
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

func lambda(id, region, group, usage string, price, upTo float64) cost.SKU {
	sku := cost.SKU{ID: id, Service: "AWSLambda", Unit: "Requests", Price: price, UpTo: upTo, Attributes: map[string]string{
		"productFamily": "Serverless", "group": group, "usagetype": usage, "termType": "OnDemand",
	}}
	if region != "" {
		sku.Attributes["regionCode"] = region
	}
	return sku
}

func apiCalls(id, region, usage string, price, upTo float64) cost.SKU {
	return cost.SKU{ID: id, Service: "AmazonApiGateway", Unit: "Requests", Price: price, UpTo: upTo, Attributes: map[string]string{
		"productFamily": "API Calls", "usagetype": usage, "termType": "OnDemand", "regionCode": region,
	}}
}

func sqsRequests(id, region, queueType string, price float64) cost.SKU {
	sku := cost.SKU{ID: id, Service: "AWSQueueService", Unit: "Requests", Price: price, UpTo: 100000000000, Attributes: map[string]string{
		"productFamily": "API Request", "termType": "OnDemand",
	}}
	if queueType != "" {
		sku.Attributes["queueType"] = queueType
	}
	if region != "" {
		sku.Attributes["regionCode"] = region
	}
	return sku
}

func fargate(id, region, usage string, price float64) cost.SKU {
	return cost.SKU{ID: id, Service: "AmazonECS", Unit: "hours", Price: price, Attributes: map[string]string{
		"productFamily": "Compute", "usagetype": usage, "termType": "OnDemand", "regionCode": region,
	}}
}

func loadBalancer(id, region, family, usage, unit string, price float64) cost.SKU {
	return cost.SKU{ID: id, Service: "AWSELB", Unit: unit, Price: price, Attributes: map[string]string{
		"productFamily": family, "usagetype": usage, "termType": "OnDemand", "regionCode": region,
	}}
}

func cacheNode(id, region, instance, engine, usage string, price float64) cost.SKU {
	return cost.SKU{ID: id, Service: "AmazonElastiCache", Unit: "Hrs", Price: price, Attributes: map[string]string{
		"productFamily": "Cache Instance", "instanceType": instance, "cacheEngine": engine, "usagetype": usage,
		"termType": "OnDemand", "regionCode": region,
	}}
}

func s3Storage(id, region, volume, usage string, price float64) cost.SKU {
	return cost.SKU{ID: id, Service: "AmazonS3", Unit: "GB-Mo", Price: price, Attributes: map[string]string{
		"productFamily": "Storage", "storageClass": "General Purpose", "volumeType": volume, "usagetype": usage,
		"termType": "OnDemand", "regionCode": region,
	}}
}

func s3Requests(id, region, group, usage string, price float64) cost.SKU {
	return cost.SKU{ID: id, Service: "AmazonS3", Unit: "Requests", Price: price, Attributes: map[string]string{
		"productFamily": "API Request", "group": group, "usagetype": usage, "termType": "OnDemand", "regionCode": region,
	}}
}

func dataTransfer(id, region, transfer, to, usage string, price, upTo float64) cost.SKU {
	sku := cost.SKU{ID: id, Service: "AWSDataTransfer", Unit: "GB", Price: price, UpTo: upTo, Attributes: map[string]string{
		"productFamily": "Data Transfer", "transferType": transfer, "toLocation": to, "usagetype": usage, "termType": "OnDemand",
	}}
	if region != "" {
		sku.Attributes["regionCode"] = region
	}
	return sku
}

// Two regions, both engines, every size, with the neighbours a loose filter would catch:
// reserved terms, gp3 and SSD-labelled storage, the regional NAT variant and its data charge,
// the arm64, edge and storage meters of Lambda, the REST API and fair queue meters, the free
// tier rows that belong to no region, ARM and Windows Fargate, the trust store and network
// load balancer hours and the capacity units, other cache engines and extended support node
// hours, and the S3 annotation meters, and the data transfer rows that are not internet egress
// from the region: the global free allowance, inbound and between regions.
func priceFixture() *cost.Snapshot {
	reserved := rdsInstance("rds-micro-pg-single-euw2-reserved", "eu-west-2", "db.t4g.micro", "PostgreSQL", "Single-AZ", 0.011)
	reserved.Attributes["termType"] = "Reserved"
	return &cost.Snapshot{
		Provider: ir.ProviderAWS,
		Date:     "2026-09-05",
		Currency: "USD",
		Versions: map[string]map[string]string{
			"AmazonRDS":         {"eu-west-2": "v1", "us-east-1": "v1"},
			"AmazonEC2":         {"eu-west-2": "v1", "us-east-1": "v1"},
			"AmazonECS":         {"eu-west-2": "v1", "us-east-1": "v1"},
			"AWSELB":            {"eu-west-2": "v1", "us-east-1": "v1"},
			"AmazonElastiCache": {"eu-west-2": "v1", "us-east-1": "v1"},
			"AmazonS3":          {"eu-west-2": "v1", "us-east-1": "v1"},
			"AWSLambda":         {"eu-west-2": "v1", "us-east-1": "v1"},
			"AmazonApiGateway":  {"eu-west-2": "v1", "us-east-1": "v1"},
			"AWSQueueService":   {"eu-west-2": "v1", "us-east-1": "v1"},
			"AWSDataTransfer":   {"eu-west-2": "v1", "us-east-1": "v1"},
		},
		SKUs: []cost.SKU{
			lambda("lambda-request-euw2", "eu-west-2", "AWS-Lambda-Requests", "EUW2-Request", 0.0000002, 0),
			lambda("lambda-duration-euw2", "eu-west-2", "AWS-Lambda-Duration", "EUW2-Lambda-GB-Second", 0.0000166667, 6000000000),
			lambda("lambda-request-arm-euw2", "eu-west-2", "AWS-Lambda-Requests-ARM", "EUW2-Request-ARM", 0.0000002, 0),
			lambda("lambda-duration-arm-euw2", "eu-west-2", "AWS-Lambda-Duration-ARM", "EUW2-Lambda-GB-Second-ARM", 0.0000133334, 7500000000),
			lambda("lambda-edge-request-euw2", "eu-west-2", "AWS-Lambda-Edge-Requests", "EUW2-Lambda-Edge-Request", 0.0000006, 0),
			lambda("lambda-storage-euw2", "eu-west-2", "AWS-Lambda-Storage-Duration", "EUW2-Lambda-Storage-GB-Second", 0.0000000358, 0),
			lambda("lambda-free-requests", "", "AWS-Lambda-Requests", "Global-Request", 0, 1000000),
			lambda("lambda-free-duration", "", "AWS-Lambda-Duration", "Global-Lambda-GB-Second", 0, 400000),
			apiCalls("apigw-http-euw2", "eu-west-2", "EUW2-ApiGatewayHttpRequest", 0.00000116, 300000000),
			apiCalls("apigw-rest-euw2", "eu-west-2", "EUW2-ApiGatewayRequest", 0.0000035, 333000000),
			sqsRequests("sqs-standard-euw2", "eu-west-2", "Standard", 0.0000004),
			sqsRequests("sqs-fifo-euw2", "eu-west-2", "FIFO (first-in, first-out)", 0.0000005),
			sqsRequests("sqs-fair-euw2", "eu-west-2", "Fair", 0.0000001),
			sqsRequests("sqs-free-requests", "", "", 0),
			lambda("lambda-request-use1", "us-east-1", "AWS-Lambda-Requests", "Request", 0.0000002, 0),
			lambda("lambda-duration-use1", "us-east-1", "AWS-Lambda-Duration", "Lambda-GB-Second", 0.0000166667, 6000000000),
			apiCalls("apigw-http-use1", "us-east-1", "USE1-ApiGatewayHttpRequest", 0.000001, 300000000),
			sqsRequests("sqs-standard-use1", "us-east-1", "Standard", 0.0000004),
			sqsRequests("sqs-fifo-use1", "us-east-1", "FIFO (first-in, first-out)", 0.0000005),

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
			natGateway("nat-regional-bytes-euw2", "eu-west-2", "EUW2-RegionalNatGateway-Bytes", 0.05),
			fargate("fargate-vcpu-euw2", "eu-west-2", "EUW2-Fargate-vCPU-Hours:perCPU", 0.04656),
			fargate("fargate-gb-euw2", "eu-west-2", "EUW2-Fargate-GB-Hours", 0.00511),
			fargate("fargate-arm-vcpu-euw2", "eu-west-2", "EUW2-Fargate-ARM-vCPU-Hours:perCPU", 0.03725),
			fargate("fargate-arm-gb-euw2", "eu-west-2", "EUW2-Fargate-ARM-GB-Hours", 0.00409),
			fargate("fargate-windows-vcpu-euw2", "eu-west-2", "EUW2-Fargate-Windows-vCPU-Hours:perCPU", 0.053544),
			loadBalancer("alb-euw2", "eu-west-2", "Load Balancer-Application", "EUW2-LoadBalancerUsage", "Hrs", 0.02646),
			loadBalancer("alb-trust-store-euw2", "eu-west-2", "Load Balancer-Application", "EUW2-TS-LoadBalancerUsage", "Hrs", 0.0059),
			loadBalancer("alb-outposts-euw2", "eu-west-2", "Load Balancer-Application", "EUW2-Outposts-LoadBalancerUsage", "Hrs", 0.02646),
			loadBalancer("alb-lcu-euw2", "eu-west-2", "Load Balancer-Application", "EUW2-LCUUsage", "LCU-Hrs", 0.0084),
			loadBalancer("nlb-euw2", "eu-west-2", "Load Balancer-Network", "EUW2-LoadBalancerUsage", "Hrs", 0.02646),
			cacheNode("cache-micro-redis-euw2", "eu-west-2", "cache.t4g.micro", "Redis", "EUW2-NodeUsage:cache.t4g.micro", 0.018),
			cacheNode("cache-micro-valkey-euw2", "eu-west-2", "cache.t4g.micro", "Valkey", "EUW2-NodeUsage:cache.t4g.micro", 0.0144),
			cacheNode("cache-micro-redis-support-euw2", "eu-west-2", "cache.t4g.micro", "Redis", "EUW2-ExtendedSupportYr3-NodeUsage:cache.t4g.micro", 0.029),
			cacheNode("cache-medium-redis-euw2", "eu-west-2", "cache.t4g.medium", "Redis", "EUW2-NodeUsage:cache.t4g.medium", 0.072),
			cacheNode("cache-large-redis-euw2", "eu-west-2", "cache.r7g.large", "Redis", "EUW2-NodeUsage:cache.r7g.large", 0.256),
			cacheNode("cache-large-memcached-euw2", "eu-west-2", "cache.r7g.large", "Memcached", "EUW2-NodeUsage:cache.r7g.large", 0.256),
			s3Storage("s3-standard-euw2", "eu-west-2", "Standard", "EUW2-TimedStorage-ByteHrs", 0.024),
			s3Storage("s3-annotations-euw2", "eu-west-2", "Annotations", "EUW2-Annotation-TimedStorage-ByteHrs", 0.024),
			s3Requests("s3-put-euw2", "eu-west-2", "S3-API-Tier1", "EUW2-Requests-Tier1", 0.0000053),
			s3Requests("s3-get-euw2", "eu-west-2", "S3-API-Tier2", "EUW2-Requests-Tier2", 0.00000042),
			s3Requests("s3-put-annotation-euw2", "eu-west-2", "S3-API-Tier1", "EUW2-Requests-Annotation-Tier1", 0.0000053),
			dataTransfer("egress-euw2", "eu-west-2", "AWS Outbound", "External", "EUW2-DataTransfer-Out-Bytes", 0.09, 10240),
			dataTransfer("ingress-euw2", "eu-west-2", "AWS Inbound", "EU (London)", "EUW2-DataTransfer-In-Bytes", 0, 0),
			dataTransfer("interregion-euw2", "eu-west-2", "InterRegion Outbound", "EU (Ireland)", "EUW2-EU-AWS-Out-Bytes", 0.02, 0),
			dataTransfer("egress-free", "", "AWS Outbound", "External", "Global-DataTransfer-Out-Bytes", 0, 100),

			rdsInstance("rds-micro-pg-single-use1", "us-east-1", "db.t4g.micro", "PostgreSQL", "Single-AZ", 0.016),
			rdsStorage("rds-gp2-pg-single-use1", "us-east-1", "General Purpose", "PostgreSQL", "Single-AZ", 0.115),
			natGateway("nat-use1", "us-east-1", "NatGateway-Hours", 0.045),
			natGateway("nat-regional-use1", "us-east-1", "RegionalNatGateway-Hours", 0.045),
			natGateway("nat-bytes-use1", "us-east-1", "NatGateway-Bytes", 0.045),
			fargate("fargate-vcpu-use1", "us-east-1", "USE1-Fargate-vCPU-Hours:perCPU", 0.04048),
			fargate("fargate-gb-use1", "us-east-1", "USE1-Fargate-GB-Hours", 0.004445),
			loadBalancer("alb-use1", "us-east-1", "Load Balancer-Application", "LoadBalancerUsage", "Hrs", 0.0225),
			loadBalancer("alb-trust-store-use1", "us-east-1", "Load Balancer-Application", "TS-LoadBalancerUsage", "Hrs", 0.005),
			loadBalancer("alb-lcu-use1", "us-east-1", "Load Balancer-Application", "LCUUsage", "LCU-Hrs", 0.008),
			cacheNode("cache-micro-redis-use1", "us-east-1", "cache.t4g.micro", "Redis", "NodeUsage:cache.t4g.micro", 0.016),
			s3Storage("s3-standard-use1", "us-east-1", "Standard", "TimedStorage-ByteHrs", 0.023),
			s3Requests("s3-put-use1", "us-east-1", "S3-API-Tier1", "Requests-Tier1", 0.000005),
			s3Requests("s3-get-use1", "us-east-1", "S3-API-Tier2", "Requests-Tier2", 0.0000004),
			dataTransfer("egress-use1", "us-east-1", "AWS Outbound", "External", "DataTransfer-Out-Bytes", 0.09, 10240),
		},
	}
}

func databaseNode(t *testing.T, p ir.DatabaseProps) ir.Node {
	t.Helper()
	return ir.Node{ID: "n3", Type: ir.NodeDatabase, Name: "main-db", Properties: props(t, p)}
}

func estimate(t *testing.T, region string, nodes []ir.Node) cost.Document {
	t.Helper()
	return estimateGraph(t, region, nodes, nil)
}

func estimateGraph(t *testing.T, region string, nodes []ir.Node, edges []ir.Edge) cost.Document {
	t.Helper()
	return estimateUsage(t, region, nodes, edges, nil)
}

func estimateUsage(t *testing.T, region string, nodes []ir.Node, edges []ir.Edge, usage cost.Usage) cost.Document {
	t.Helper()
	p := newProject(t, nodes, edges)
	p.Region = region
	doc, err := cost.Estimate(p, Cost(), priceFixture(), usage, time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC))
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
		{Name: "network", Kind: "implicit VPC", Reason: "nat gateway data"},
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
	if len(doc.Items) != 2 || doc.Total != 0 {
		t.Errorf("items = %+v, total = %v", doc.Items, doc.Total)
	}
	wantOmitted := []cost.Omission{
		{Name: "api", Kind: "gateway", Reason: "requests"},
		{Name: "api", Kind: "gateway", Reason: "data transfer"},
		{Name: "handler", Kind: "function", Reason: "requests"},
		{Name: "handler", Kind: "function", Reason: "duration"},
	}
	if diff := cmp.Diff(wantOmitted, doc.NotPriced); diff != "" {
		t.Errorf("not priced (-want +got):\n%s", diff)
	}

	doc = estimate(t, "eu-west-2", append(serverless, databaseNode(t, ir.DatabaseProps{})))
	want := cost.Priced{
		Name: "network", Kind: "implicit VPC", Note: "priced on defaults, no usage set",
		Lines:    []cost.Line{{Label: "nat gateway", Quantity: 730, Unit: "h", UnitPrice: 0.05, Amount: 36.50, SKU: "nat-euw2"}},
		Subtotal: 36.50,
	}
	if diff := cmp.Diff(want, doc.Items[3]); diff != "" {
		t.Errorf("network (-want +got):\n%s", diff)
	}
	if doc.Total != 52.30 {
		t.Errorf("total = %v", doc.Total)
	}
}

func functionNode(t *testing.T, p ir.FunctionProps) ir.Node {
	t.Helper()
	return ir.Node{ID: "n2", Type: ir.NodeFunction, Name: "handler", Properties: props(t, p)}
}

// A pay-per-use item on defaults carries no lines, so its meters are checked by hand against
// the fixture.
func usageSKUs(t *testing.T, n ir.Node, region string) []string {
	t.Helper()
	p, err := ir.ApplyDefaults(newProject(t, []ir.Node{n}, nil))
	if err != nil {
		t.Fatal(err)
	}
	item, ok, err := Cost().Node(p.Nodes[0], region, cost.NodeUsage{})
	if err != nil || !ok {
		t.Fatalf("node = %+v, %v, %v", item, ok, err)
	}
	if len(item.Lookups) != 0 {
		t.Errorf("a pay-per-use item has fixed lookups: %+v", item.Lookups)
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

func TestFunctionCostIsPricedOnDefaultsOnTheX86Meters(t *testing.T) {
	doc := estimate(t, "eu-west-2", []ir.Node{functionNode(t, ir.FunctionProps{})})
	want := cost.Priced{
		Name: "handler", Kind: "function", Summary: "node, 512 MB, x86_64", Note: "priced on defaults, no usage set",
		Lines: []cost.Line{}, Subtotal: 0,
	}
	if diff := cmp.Diff(want, doc.Items[0]); diff != "" {
		t.Errorf("function (-want +got):\n%s", diff)
	}
	if len(doc.Items) != 1 || doc.Total != 0 {
		t.Errorf("items = %+v, total = %v", doc.Items, doc.Total)
	}
	wantOmitted := []cost.Omission{
		{Name: "handler", Kind: "function", Reason: "requests"},
		{Name: "handler", Kind: "function", Reason: "duration"},
	}
	if diff := cmp.Diff(wantOmitted, doc.NotPriced); diff != "" {
		t.Errorf("not priced (-want +got):\n%s", diff)
	}

	if diff := cmp.Diff([]string{"lambda-request-euw2", "lambda-duration-euw2"}, usageSKUs(t, functionNode(t, ir.FunctionProps{}), "eu-west-2")); diff != "" {
		t.Errorf("eu-west-2 meters (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"lambda-request-use1", "lambda-duration-use1"}, usageSKUs(t, functionNode(t, ir.FunctionProps{}), "us-east-1")); diff != "" {
		t.Errorf("us-east-1 meters (-want +got):\n%s", diff)
	}
}

func TestFunctionCostShowsTheRuntimeAndMemoryOfEverySize(t *testing.T) {
	for _, c := range []struct {
		props ir.FunctionProps
		want  string
	}{
		{ir.FunctionProps{Size: ir.SizeSmall}, "node, 512 MB, x86_64"},
		{ir.FunctionProps{Size: ir.SizeMedium, Runtime: ir.RuntimePython}, "python, 1024 MB, x86_64"},
		{ir.FunctionProps{Size: ir.SizeLarge, Runtime: ir.RuntimeGo}, "go, 2048 MB, x86_64"},
	} {
		doc := estimate(t, "eu-west-2", []ir.Node{functionNode(t, c.props)})
		if got := doc.Items[0].Summary; got != c.want {
			t.Errorf("summary = %q, want %q", got, c.want)
		}
		if diff := cmp.Diff([]string{"lambda-request-euw2", "lambda-duration-euw2"}, usageSKUs(t, functionNode(t, c.props), "eu-west-2")); diff != "" {
			t.Errorf("%s meters (-want +got):\n%s", c.props.Size, diff)
		}
	}
}

func TestGatewayCostIsPricedOnDefaultsOnTheHttpApiMeter(t *testing.T) {
	api := ir.Node{ID: "n1", Type: ir.NodeGateway, Name: "api"}
	doc := estimate(t, "eu-west-2", []ir.Node{api})
	want := cost.Priced{
		Name: "api", Kind: "gateway", Summary: "HTTP API", Note: "priced on defaults, no usage set",
		Lines: []cost.Line{}, Subtotal: 0,
	}
	if diff := cmp.Diff(want, doc.Items[0]); diff != "" {
		t.Errorf("gateway (-want +got):\n%s", diff)
	}
	wantOmitted := []cost.Omission{
		{Name: "api", Kind: "gateway", Reason: "requests"},
		{Name: "api", Kind: "gateway", Reason: "data transfer"},
	}
	if diff := cmp.Diff(wantOmitted, doc.NotPriced); diff != "" {
		t.Errorf("not priced (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"apigw-http-euw2"}, usageSKUs(t, api, "eu-west-2")); diff != "" {
		t.Errorf("eu-west-2 meter (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"apigw-http-use1"}, usageSKUs(t, api, "us-east-1")); diff != "" {
		t.Errorf("us-east-1 meter (-want +got):\n%s", diff)
	}
}

func TestQueueCostFollowsTheFifoProperty(t *testing.T) {
	for _, c := range []struct {
		fifo    bool
		summary string
		euw2    string
		use1    string
	}{
		{false, "standard", "sqs-standard-euw2", "sqs-standard-use1"},
		{true, "FIFO", "sqs-fifo-euw2", "sqs-fifo-use1"},
	} {
		jobs := ir.Node{ID: "q1", Type: ir.NodeQueue, Name: "jobs", Properties: props(t, ir.QueueProps{FIFO: c.fifo})}
		doc := estimate(t, "eu-west-2", []ir.Node{jobs})
		want := cost.Priced{
			Name: "jobs", Kind: "queue", Summary: c.summary, Note: "priced on defaults, no usage set",
			Lines: []cost.Line{}, Subtotal: 0,
		}
		if diff := cmp.Diff(want, doc.Items[0]); diff != "" {
			t.Errorf("queue (-want +got):\n%s", diff)
		}
		wantOmitted := []cost.Omission{{Name: "jobs", Kind: "queue", Reason: "requests (3 per message)"}}
		if diff := cmp.Diff(wantOmitted, doc.NotPriced); diff != "" {
			t.Errorf("not priced (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff([]string{c.euw2}, usageSKUs(t, jobs, "eu-west-2")); diff != "" {
			t.Errorf("fifo=%v eu-west-2 meter (-want +got):\n%s", c.fifo, diff)
		}
		if diff := cmp.Diff([]string{c.use1}, usageSKUs(t, jobs, "us-east-1")); diff != "" {
			t.Errorf("fifo=%v us-east-1 meter (-want +got):\n%s", c.fifo, diff)
		}
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

func TestServiceCostPricesTheTasksAndThePublicLoadBalancer(t *testing.T) {
	doc := estimate(t, "eu-west-2", []ir.Node{serviceNode(t, "s1", "web", ir.ServiceProps{Image: "nginx:1.27", Public: true})})
	want := cost.Priced{
		Name: "web", Kind: "service", Summary: "0.25 vCPU, 0.5 GB, 1 task, public", Note: "priced on defaults, no usage set",
		Lines: []cost.Line{
			{Label: "vcpu", Quantity: 182.5, Unit: "vCPU-h", UnitPrice: 0.04656, Amount: 8.50, SKU: "fargate-vcpu-euw2"},
			{Label: "memory", Quantity: 365, Unit: "GB-h", UnitPrice: 0.00511, Amount: 1.87, SKU: "fargate-gb-euw2"},
			{Label: "load balancer", Quantity: 730, Unit: "h", UnitPrice: 0.02646, Amount: 19.32, SKU: "alb-euw2"},
		},
		Subtotal: 29.69,
	}
	if diff := cmp.Diff(want, doc.Items[0]); diff != "" {
		t.Errorf("service (-want +got):\n%s", diff)
	}
	wantOmitted := []cost.Omission{
		{Name: "web", Kind: "service", Reason: "egress"},
		{Name: "web", Kind: "service", Reason: "capacity units"},
		{Name: "network", Kind: "implicit VPC", Reason: "nat gateway data"},
	}
	if diff := cmp.Diff(wantOmitted, doc.NotPriced); diff != "" {
		t.Errorf("not priced (-want +got):\n%s", diff)
	}
	if doc.Total != 66.19 {
		t.Errorf("total = %v", doc.Total)
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
		{ir.SizeSmall, 3, "0.25 vCPU, 0.5 GB, 3 tasks, private", 547.5, 1095, 31.09},
		{ir.SizeMedium, 2, "1 vCPU, 2 GB, 2 tasks, private", 1460, 2920, 82.90},
		{ir.SizeLarge, 1, "2 vCPU, 4 GB, 1 task, private", 1460, 2920, 82.90},
		{ir.SizeMedium, 0, "1 vCPU, 2 GB, 0 tasks, private", 0, 0, 0},
	} {
		t.Run(c.summary, func(t *testing.T) {
			node := serviceNode(t, "s1", "web", ir.ServiceProps{Image: "nginx:1.27", Size: c.size, MinReplicas: ir.Ptr(c.replicas)})
			doc := estimate(t, "eu-west-2", []ir.Node{node})
			svc := doc.Items[0]
			if svc.Summary != c.summary || len(svc.Lines) != 2 || svc.Subtotal != c.subtotal {
				t.Errorf("service = %+v", svc)
			}
			if svc.Lines[0].Quantity != c.vcpu || svc.Lines[1].Quantity != c.memory {
				t.Errorf("quantities = %v vCPU-h, %v GB-h, want %v, %v", svc.Lines[0].Quantity, svc.Lines[1].Quantity, c.vcpu, c.memory)
			}
			if len(doc.NotPriced) != 2 || doc.NotPriced[0].Reason != "egress" || doc.NotPriced[1].Name != "network" {
				t.Errorf("not priced = %+v", doc.NotPriced)
			}
		})
	}
}

func TestPrivateServiceGetsALoadBalancerOnlyWhenAGatewayRoutesToIt(t *testing.T) {
	api := ir.Node{ID: "g1", Type: ir.NodeGateway, Name: "api"}
	admin := serviceNode(t, "s2", "admin", ir.ServiceProps{Image: "ghcr.io/example/admin:1"})
	route := func(id, from string) ir.Edge {
		return ir.Edge{ID: id, From: from, To: "s2", Relation: ir.RelRoutes, Properties: ir.EdgeProperties{Path: "/admin"}}
	}

	doc := estimateGraph(t, "eu-west-2", []ir.Node{api, admin}, nil)
	if len(doc.Items) != 3 || doc.Items[1].Name != "admin" || doc.Items[2].Name != "network" || doc.Total != 46.87 {
		t.Errorf("items = %+v, total = %v", doc.Items, doc.Total)
	}

	doc = estimateGraph(t, "eu-west-2", []ir.Node{api, admin}, []ir.Edge{route("e1", "g1")})
	want := cost.Priced{
		Name: "admin", Kind: "implicit ALB", Summary: "internal load balancer, routed from api", Note: "priced on defaults, no usage set",
		Lines:    []cost.Line{{Label: "load balancer", Quantity: 730, Unit: "h", UnitPrice: 0.02646, Amount: 19.32, SKU: "alb-euw2"}},
		Subtotal: 19.32,
	}
	if len(doc.Items) != 4 || doc.Items[3].Name != "network" {
		t.Fatalf("items = %+v", doc.Items)
	}
	if diff := cmp.Diff(want, doc.Items[2]); diff != "" {
		t.Errorf("load balancer (-want +got):\n%s", diff)
	}
	wantOmitted := []cost.Omission{
		{Name: "api", Kind: "gateway", Reason: "requests"},
		{Name: "api", Kind: "gateway", Reason: "data transfer"},
		{Name: "admin", Kind: "service", Reason: "egress"},
		{Name: "admin", Kind: "implicit ALB", Reason: "capacity units"},
		{Name: "network", Kind: "implicit VPC", Reason: "nat gateway data"},
	}
	if diff := cmp.Diff(wantOmitted, doc.NotPriced); diff != "" {
		t.Errorf("not priced (-want +got):\n%s", diff)
	}
	if doc.Total != 66.19 {
		t.Errorf("total = %v", doc.Total)
	}

	// One load balancer however many gateways route to the service.
	admin2 := ir.Node{ID: "g2", Type: ir.NodeGateway, Name: "admin-api"}
	doc = estimateGraph(t, "eu-west-2", []ir.Node{api, admin2, admin}, []ir.Edge{route("e1", "g1"), route("e2", "g2")})
	if len(doc.Items) != 5 || doc.Total != 66.19 {
		t.Errorf("items = %+v, total = %v", doc.Items, doc.Total)
	}
}

func TestLoadBalancerCostAgreesWithTheResolver(t *testing.T) {
	api := ir.Node{ID: "g1", Type: ir.NodeGateway, Name: "api"}
	web := serviceNode(t, "s1", "web", ir.ServiceProps{Image: "nginx:1.27", Public: true})
	admin := serviceNode(t, "s2", "admin", ir.ServiceProps{Image: "ghcr.io/example/admin:1"})
	routes := func(id, to string) ir.Edge {
		return ir.Edge{ID: id, From: "g1", To: to, Relation: ir.RelRoutes, Properties: ir.EdgeProperties{Path: "/" + to}}
	}
	for _, c := range []struct {
		name  string
		nodes []ir.Node
		edges []ir.Edge
	}{
		{"public service", []ir.Node{web}, nil},
		{"private service", []ir.Node{admin}, nil},
		{"routed public service", []ir.Node{api, web}, []ir.Edge{routes("e1", "s1")}},
		{"routed private service", []ir.Node{api, admin}, []ir.Edge{routes("e1", "s2")}},
		{"both routed", []ir.Node{api, web, admin}, []ir.Edge{routes("e1", "s1"), routes("e2", "s2")}},
		{"private service called by another", []ir.Node{web, admin}, []ir.Edge{{ID: "e1", From: "s1", To: "s2", Relation: ir.RelCalls}}},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := newProject(t, c.nodes, c.edges)
			graph, err := resolve.Run(p, New())
			if err != nil {
				t.Fatal(err)
			}
			resolved := map[string]bool{}
			for _, r := range graph.Resources {
				if r.Type == "aws_lb" {
					internal, _ := r.Args.Get("internal")
					resolved[r.SourceLabel] = internal == ir.Bool(true)
				}
			}
			priced := map[string]bool{}
			for _, item := range estimateGraph(t, "eu-west-2", c.nodes, c.edges).Items {
				for _, l := range item.Lines {
					if l.Label == "load balancer" {
						priced[item.Name] = item.Kind == "implicit ALB"
					}
				}
			}
			if diff := cmp.Diff(resolved, priced); diff != "" {
				t.Errorf("load balancers by service, internal or not (-resolver +matcher):\n%s", diff)
			}
		})
	}
}

func TestCacheCostPricesTheNodeHours(t *testing.T) {
	for _, c := range []struct {
		size    ir.Size
		summary string
		sku     string
		price   float64
		amount  float64
	}{
		{ir.SizeSmall, "cache.t4g.micro, redis 7.1, 1 node", "cache-micro-redis-euw2", 0.018, 13.14},
		{ir.SizeMedium, "cache.t4g.medium, redis 7.1, 1 node", "cache-medium-redis-euw2", 0.072, 52.56},
		{ir.SizeLarge, "cache.r7g.large, redis 7.1, 1 node", "cache-large-redis-euw2", 0.256, 186.88},
	} {
		t.Run(string(c.size), func(t *testing.T) {
			node := ir.Node{ID: "c1", Type: ir.NodeCache, Name: "sessions", Properties: props(t, ir.CacheProps{Size: c.size})}
			doc := estimate(t, "eu-west-2", []ir.Node{node})
			want := cost.Priced{
				Name: "sessions", Kind: "cache", Summary: c.summary,
				Lines:    []cost.Line{{Label: "node", Quantity: 730, Unit: "h", UnitPrice: c.price, Amount: c.amount, SKU: c.sku}},
				Subtotal: c.amount,
			}
			if diff := cmp.Diff(want, doc.Items[0]); diff != "" {
				t.Errorf("cache (-want +got):\n%s", diff)
			}
			if len(doc.NotPriced) != 1 || doc.NotPriced[0].Name != "network" {
				t.Errorf("not priced = %+v", doc.NotPriced)
			}
		})
	}
}

func TestBucketCostIsPricedOnDefaultsOnTheStandardMeters(t *testing.T) {
	uploads := ir.Node{ID: "b1", Type: ir.NodeBucket, Name: "uploads"}
	doc := estimate(t, "eu-west-2", []ir.Node{
		uploads,
		{ID: "b2", Type: ir.NodeBucket, Name: "assets", Properties: json.RawMessage(`{"versioning":false,"public":true}`)},
	})
	want := []cost.Priced{
		{Name: "uploads", Kind: "bucket", Summary: "standard storage, versioned, private", Note: "priced on defaults, no usage set", Lines: []cost.Line{}},
		{Name: "assets", Kind: "bucket", Summary: "standard storage, unversioned, public", Note: "priced on defaults, no usage set", Lines: []cost.Line{}},
	}
	if diff := cmp.Diff(want, doc.Items); diff != "" {
		t.Errorf("buckets (-want +got):\n%s", diff)
	}
	wantOmitted := []cost.Omission{
		{Name: "uploads", Kind: "bucket", Reason: "storage"},
		{Name: "uploads", Kind: "bucket", Reason: "put requests (1 in 10)"},
		{Name: "uploads", Kind: "bucket", Reason: "get requests (9 in 10)"},
		{Name: "uploads", Kind: "bucket", Reason: "egress"},
		{Name: "assets", Kind: "bucket", Reason: "storage"},
		{Name: "assets", Kind: "bucket", Reason: "put requests (1 in 10)"},
		{Name: "assets", Kind: "bucket", Reason: "get requests (9 in 10)"},
		{Name: "assets", Kind: "bucket", Reason: "egress"},
	}
	if diff := cmp.Diff(wantOmitted, doc.NotPriced); diff != "" {
		t.Errorf("not priced (-want +got):\n%s", diff)
	}
	if doc.Total != 0 {
		t.Errorf("total = %v", doc.Total)
	}

	if diff := cmp.Diff([]string{"s3-standard-euw2", "s3-put-euw2", "s3-get-euw2", "egress-euw2"}, usageSKUs(t, uploads, "eu-west-2")); diff != "" {
		t.Errorf("eu-west-2 meters (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"s3-standard-use1", "s3-put-use1", "s3-get-use1", "egress-use1"}, usageSKUs(t, uploads, "us-east-1")); diff != "" {
		t.Errorf("us-east-1 meters (-want +got):\n%s", diff)
	}
}

func TestServiceAndCacheCostFollowTheProjectRegion(t *testing.T) {
	doc := estimate(t, "us-east-1", []ir.Node{
		serviceNode(t, "s1", "web", ir.ServiceProps{Image: "nginx:1.27", Public: true}),
		{ID: "c1", Type: ir.NodeCache, Name: "sessions"},
	})
	web := doc.Items[0]
	if web.Lines[0].SKU != "fargate-vcpu-use1" || web.Lines[0].Amount != 7.39 {
		t.Errorf("vcpu = %+v", web.Lines[0])
	}
	if web.Lines[1].SKU != "fargate-gb-use1" || web.Lines[1].Amount != 1.62 {
		t.Errorf("memory = %+v", web.Lines[1])
	}
	if web.Lines[2].SKU != "alb-use1" || web.Lines[2].Amount != 16.43 {
		t.Errorf("load balancer = %+v", web.Lines[2])
	}
	if got := doc.Items[1].Lines[0]; got.SKU != "cache-micro-redis-use1" || got.Amount != 11.68 {
		t.Errorf("cache = %+v", got)
	}
}

func TestRegionalUsageTypesNeedARegionShapedPrefix(t *testing.T) {
	matches := cost.Lookup{Filters: []cost.Filter{regional("LoadBalancerUsage")}}.Predicate()
	for usage, want := range map[string]bool{
		"EUW2-LoadBalancerUsage":          true,
		"USE1-LoadBalancerUsage":          true,
		"EU-LoadBalancerUsage":            true,
		"LoadBalancerUsage":               true,
		"TS-LoadBalancerUsage":            false,
		"EUW2-TS-LoadBalancerUsage":       false,
		"Outposts-LoadBalancerUsage":      false,
		"EUW2-Outposts-LoadBalancerUsage": false,
	} {
		if got := matches(map[string]string{"usagetype": usage}); got != want {
			t.Errorf("%s matches = %v, want %v", usage, got, want)
		}
	}
}

func TestCacheNodeTypeFollowsTheRegionWithTheResolver(t *testing.T) {
	for region, want := range map[string]string{"eu-west-2": "cache.t4g.micro", "me-south-1": "cache.t3.micro", "me-central-1": "cache.t3.micro"} {
		t.Run(region, func(t *testing.T) {
			p := newProject(t, []ir.Node{{ID: "c1", Type: ir.NodeCache, Name: "sessions"}}, nil)
			p.Region = region
			graph, err := resolve.Run(p, New())
			if err != nil {
				t.Fatal(err)
			}
			resolved := ""
			for _, r := range graph.Resources {
				if r.Type == "aws_elasticache_replication_group" {
					nodeType, _ := r.Args.Get("node_type")
					resolved = string(nodeType.(ir.String))
				}
			}
			item, _, err := Cost().Node(p.Nodes[0], region, cost.NodeUsage{})
			if err != nil {
				t.Fatal(err)
			}
			priced := ""
			for _, f := range item.Lookups[0].Filters {
				if f.Attribute == "instanceType" {
					priced = f.Value
				}
			}
			if resolved != want || priced != want {
				t.Errorf("resolver = %q, matcher = %q, want %q", resolved, priced, want)
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
	unsized := map[string]bool{}
	for _, l := range lookups {
		distinct[l.Describe()] = true
		if l.Quantity == 0 {
			unsized[l.Describe()] = true
		}
	}
	// 3 sizes x 2 engines x 2 deployments of instance, 2 x 2 of storage, the nat gateway,
	// Fargate vcpu and memory, the load balancer, 3 cache node types, and the usage meters: the
	// function's requests and duration, the gateway's requests, standard and FIFO requests, the
	// bucket's storage, put and get requests, egress (one meter for buckets and services), the
	// load balancer's capacity units and the nat gateway's data.
	if len(distinct) != 12+4+1+2+1+3+2+1+2+3+1+1+1 {
		t.Errorf("distinct lookups = %d, want 34", len(distinct))
	}
	if len(unsized) != 2+1+2+3+1+1+1 {
		t.Errorf("usage meters with no quantity = %d, want 11", len(unsized))
	}
}

// The bundled snapshot is only as good as the last refresh, so every meter the matchers can
// name has to be there for every region the studio offers.
func TestBundledSnapshotHoldsEveryMeterTheCatalogueNames(t *testing.T) {
	snapshot, err := cost.Bundled(ir.ProviderAWS)
	if err != nil {
		t.Fatal(err)
	}
	for _, region := range snapshot.Regions() {
		lookups, err := Cost().Catalogue(region)
		if err != nil {
			t.Fatal(err)
		}
		for _, l := range lookups {
			if _, err := snapshot.Find(l); err != nil {
				t.Error(err)
			}
		}
	}
}

// Every quantity here is worked by hand from the usage figures: a rate times its period's
// share of a 730 hour month, the size's memory for GB-seconds, three requests a message, a
// tenth of a bucket's requests as writes, and egress standing in for capacity units.
func TestUsageLinesPerNodeType(t *testing.T) {
	api := ir.Node{ID: "g1", Type: ir.NodeGateway, Name: "api"}
	handler := functionNode(t, ir.FunctionProps{})
	jobs := ir.Node{ID: "q1", Type: ir.NodeQueue, Name: "jobs", Properties: props(t, ir.QueueProps{FIFO: true})}
	uploads := ir.Node{ID: "b1", Type: ir.NodeBucket, Name: "uploads"}
	web := serviceNode(t, "s1", "web", ir.ServiceProps{Image: "nginx:1.27", Public: true})
	usage := cost.Usage{
		"api":     {Requests: "500/min"},
		"handler": {Invocations: "2M/month", DurationMs: 300},
		"jobs":    {Messages: "1M/month"},
		"uploads": {StorageGb: 200, Requests: "1M/month", EgressGb: 40},
		"web":     {EgressGb: 100},
		"network": {NatGb: 50},
	}
	doc := estimateUsage(t, "eu-west-2", []ir.Node{api, handler, jobs, uploads, web}, nil, usage)
	want := []cost.Priced{
		{
			Name: "api", Kind: "gateway", Summary: "HTTP API",
			Lines:    []cost.Line{{Label: "requests", Quantity: 21900000, Unit: "requests", UnitPrice: 0.00000116, Amount: 25.40, SKU: "apigw-http-euw2"}},
			Subtotal: 25.40,
		},
		{
			Name: "handler", Kind: "function", Summary: "node, 512 MB, x86_64",
			Lines: []cost.Line{
				{Label: "requests", Quantity: 2000000, Unit: "requests", UnitPrice: 0.0000002, Amount: 0.40, SKU: "lambda-request-euw2"},
				{Label: "duration", Quantity: 300000, Unit: "GB-s", UnitPrice: 0.0000166667, Amount: 5.00, SKU: "lambda-duration-euw2"},
			},
			Subtotal: 5.40,
		},
		{
			Name: "jobs", Kind: "queue", Summary: "FIFO",
			Lines:    []cost.Line{{Label: "requests (3 per message)", Quantity: 3000000, Unit: "requests", UnitPrice: 0.0000005, Amount: 1.50, SKU: "sqs-fifo-euw2"}},
			Subtotal: 1.50,
		},
		{
			Name: "uploads", Kind: "bucket", Summary: "standard storage, versioned, private",
			Lines: []cost.Line{
				{Label: "storage", Quantity: 200, Unit: "GB", UnitPrice: 0.024, Amount: 4.80, SKU: "s3-standard-euw2"},
				{Label: "put requests (1 in 10)", Quantity: 100000, Unit: "requests", UnitPrice: 0.0000053, Amount: 0.53, SKU: "s3-put-euw2"},
				{Label: "get requests (9 in 10)", Quantity: 900000, Unit: "requests", UnitPrice: 0.00000042, Amount: 0.38, SKU: "s3-get-euw2"},
				{Label: "egress", Quantity: 40, Unit: "GB", UnitPrice: 0.09, Amount: 3.60, SKU: "egress-euw2"},
			},
			Subtotal: 9.31,
		},
		{
			Name: "web", Kind: "service", Summary: "0.25 vCPU, 0.5 GB, 1 task, public",
			Lines: []cost.Line{
				{Label: "vcpu", Quantity: 182.5, Unit: "vCPU-h", UnitPrice: 0.04656, Amount: 8.50, SKU: "fargate-vcpu-euw2"},
				{Label: "memory", Quantity: 365, Unit: "GB-h", UnitPrice: 0.00511, Amount: 1.87, SKU: "fargate-gb-euw2"},
				{Label: "load balancer", Quantity: 730, Unit: "h", UnitPrice: 0.02646, Amount: 19.32, SKU: "alb-euw2"},
				{Label: "egress", Quantity: 100, Unit: "GB", UnitPrice: 0.09, Amount: 9.00, SKU: "egress-euw2"},
				{Label: "capacity units", Quantity: 100, Unit: "LCU-h", UnitPrice: 0.0084, Amount: 0.84, SKU: "alb-lcu-euw2"},
			},
			Subtotal: 39.53,
		},
		{
			Name: "network", Kind: "implicit VPC",
			Lines: []cost.Line{
				{Label: "nat gateway", Quantity: 730, Unit: "h", UnitPrice: 0.05, Amount: 36.50, SKU: "nat-euw2"},
				{Label: "nat gateway data", Quantity: 50, Unit: "GB", UnitPrice: 0.05, Amount: 2.50, SKU: "nat-bytes-euw2"},
			},
			Subtotal: 39.00,
		},
	}
	if diff := cmp.Diff(want, doc.Items); diff != "" {
		t.Errorf("items (-want +got):\n%s", diff)
	}
	wantOmitted := []cost.Omission{{Name: "api", Kind: "gateway", Reason: "data transfer"}}
	if diff := cmp.Diff(wantOmitted, doc.NotPriced); diff != "" {
		t.Errorf("not priced (-want +got):\n%s", diff)
	}
	if doc.Total != 120.14 {
		t.Errorf("total = %v", doc.Total)
	}
}

func TestUsageLinesFollowTheProjectRegion(t *testing.T) {
	uploads := ir.Node{ID: "b1", Type: ir.NodeBucket, Name: "uploads"}
	web := serviceNode(t, "s1", "web", ir.ServiceProps{Image: "nginx:1.27", Public: true})
	usage := cost.Usage{"uploads": {EgressGb: 10}, "web": {EgressGb: 10}, "network": {NatGb: 10}}
	doc := estimateUsage(t, "us-east-1", []ir.Node{uploads, web}, nil, usage)
	got := map[string]string{}
	for _, item := range doc.Items {
		for _, l := range item.Lines {
			got[item.Name+" "+l.Label] = l.SKU
		}
	}
	want := map[string]string{
		"uploads egress":           "egress-use1",
		"web vcpu":                 "fargate-vcpu-use1",
		"web memory":               "fargate-gb-use1",
		"web load balancer":        "alb-use1",
		"web egress":               "egress-use1",
		"web capacity units":       "alb-lcu-use1",
		"network nat gateway":      "nat-use1",
		"network nat gateway data": "nat-bytes-use1",
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("skus (-want +got):\n%s", diff)
	}
}

// The duration meter needs the invocations to be there; on its own the figure prices nothing.
func TestFunctionDurationNeedsInvocations(t *testing.T) {
	doc := estimateUsage(t, "eu-west-2", []ir.Node{functionNode(t, ir.FunctionProps{Size: ir.SizeLarge})}, nil, cost.Usage{"handler": {DurationMs: 300}})
	if got := doc.Items[0]; got.Note != "" || len(got.Lines) != 0 {
		t.Errorf("function = %+v", got)
	}
	wantOmitted := []cost.Omission{
		{Name: "handler", Kind: "function", Reason: "requests"},
		{Name: "handler", Kind: "function", Reason: "duration"},
	}
	if diff := cmp.Diff(wantOmitted, doc.NotPriced); diff != "" {
		t.Errorf("not priced (-want +got):\n%s", diff)
	}

	doc = estimateUsage(t, "eu-west-2", []ir.Node{functionNode(t, ir.FunctionProps{Size: ir.SizeLarge})}, nil, cost.Usage{"handler": {Invocations: "1000/day"}})
	if got := doc.Items[0].Lines; len(got) != 1 || got[0].Label != "requests" || got[0].Quantity != 30417 {
		t.Errorf("lines = %+v", got)
	}
	if len(doc.NotPriced) != 1 || doc.NotPriced[0].Reason != "duration" {
		t.Errorf("not priced = %+v", doc.NotPriced)
	}
}

func TestUsagePastTheFirstTierIsNoted(t *testing.T) {
	api := ir.Node{ID: "g1", Type: ir.NodeGateway, Name: "api"}
	doc := estimateUsage(t, "eu-west-2", []ir.Node{api}, nil, cost.Usage{"api": {Requests: "500M/month"}})
	want := cost.Line{
		Label: "requests", Quantity: 500000000, Unit: "requests", UnitPrice: 0.00000116, Amount: 580, SKU: "apigw-http-euw2",
		Note: "past the first tier of 300,000,000 requests, priced at its rate",
	}
	if diff := cmp.Diff(want, doc.Items[0].Lines[0]); diff != "" {
		t.Errorf("line (-want +got):\n%s", diff)
	}

	handler := functionNode(t, ir.FunctionProps{Size: ir.SizeLarge})
	doc = estimateUsage(t, "eu-west-2", []ir.Node{handler}, nil, cost.Usage{"handler": {Invocations: "5k/min", DurationMs: 30000}})
	duration := doc.Items[0].Lines[1]
	if duration.Quantity != 13140000000 || duration.Note != "past the first tier of 6,000,000,000 GB-s, priced at its rate" {
		t.Errorf("duration = %+v", duration)
	}
}

func TestAnUnreadableRateIsAnError(t *testing.T) {
	api := ir.Node{ID: "g1", Type: ir.NodeGateway, Name: "api"}
	p := newProject(t, []ir.Node{api}, nil)
	_, err := cost.Estimate(p, Cost(), priceFixture(), cost.Usage{"api": {Requests: "lots"}}, time.Now())
	if err == nil || !strings.Contains(err.Error(), "gateway 'api': requests \"lots\" must be a number per period") {
		t.Errorf("error = %v", err)
	}
}
