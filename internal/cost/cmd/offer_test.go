package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/mooncitizen/togen/internal/cost"
	"github.com/mooncitizen/togen/internal/resolve/aws"
)

// testdata/AmazonRDS-eu-west-2.csv is a slice of the real offer file from 2026-09-04: its
// metadata lines, the header, the sixteen rows the matchers should pick and a dozen neighbours
// they should not (reserved terms, other engines and sizes, gp3, mirrored and magnetic storage).
func TestScanOfferPicksOneSkuPerDatabaseLookup(t *testing.T) {
	file, err := os.Open(filepath.Join("testdata", "AmazonRDS-eu-west-2.csv"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()

	jobs, err := plan()
	if err != nil {
		t.Fatal(err)
	}
	var lookups []cost.Lookup
	for _, j := range jobs {
		if j.service == "AmazonRDS" && j.region == "eu-west-2" {
			lookups = j.lookups
		}
	}
	if len(lookups) != 16 {
		t.Fatalf("lookups = %d, want 16", len(lookups))
	}

	matched, err := scanOffer(file, lookups)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for i, l := range lookups {
		if len(matched[i]) != 1 {
			t.Errorf("%s (%s) matched %d rows", l.Label, l.Describe(), len(matched[i]))
			continue
		}
		got[l.Describe()] = matched[i][0].ID
	}
	want := map[string]string{
		"AmazonRDS, productFamily=Database Instance, instanceType=db.t4g.micro, databaseEngine=PostgreSQL, deploymentOption=Single-AZ, termType=OnDemand, regionCode=eu-west-2":  "ATYF9R3XTURNBSMZ",
		"AmazonRDS, productFamily=Database Instance, instanceType=db.t4g.micro, databaseEngine=PostgreSQL, deploymentOption=Multi-AZ, termType=OnDemand, regionCode=eu-west-2":   "D4PD3WMETNDAM8KK",
		"AmazonRDS, productFamily=Database Instance, instanceType=db.t4g.micro, databaseEngine=MySQL, deploymentOption=Single-AZ, termType=OnDemand, regionCode=eu-west-2":       "6HF7KFPVFEAGXYR3",
		"AmazonRDS, productFamily=Database Instance, instanceType=db.t4g.micro, databaseEngine=MySQL, deploymentOption=Multi-AZ, termType=OnDemand, regionCode=eu-west-2":        "P4BR9MYSTR2MBPJP",
		"AmazonRDS, productFamily=Database Instance, instanceType=db.t4g.medium, databaseEngine=PostgreSQL, deploymentOption=Single-AZ, termType=OnDemand, regionCode=eu-west-2": "H6U5RNXSENY7QJ2W",
		"AmazonRDS, productFamily=Database Instance, instanceType=db.t4g.medium, databaseEngine=PostgreSQL, deploymentOption=Multi-AZ, termType=OnDemand, regionCode=eu-west-2":  "VKNEYG4S5TJEYQMC",
		"AmazonRDS, productFamily=Database Instance, instanceType=db.t4g.medium, databaseEngine=MySQL, deploymentOption=Single-AZ, termType=OnDemand, regionCode=eu-west-2":      "WHAXSYFP6WBNPE6M",
		"AmazonRDS, productFamily=Database Instance, instanceType=db.t4g.medium, databaseEngine=MySQL, deploymentOption=Multi-AZ, termType=OnDemand, regionCode=eu-west-2":       "XPCX35MJ6RAV3ZXF",
		"AmazonRDS, productFamily=Database Instance, instanceType=db.r6g.large, databaseEngine=PostgreSQL, deploymentOption=Single-AZ, termType=OnDemand, regionCode=eu-west-2":  "ZE24YKEEYDU6R6TG",
		"AmazonRDS, productFamily=Database Instance, instanceType=db.r6g.large, databaseEngine=PostgreSQL, deploymentOption=Multi-AZ, termType=OnDemand, regionCode=eu-west-2":   "PZVP4J3D7PPCQTFM",
		"AmazonRDS, productFamily=Database Instance, instanceType=db.r6g.large, databaseEngine=MySQL, deploymentOption=Single-AZ, termType=OnDemand, regionCode=eu-west-2":       "8K2BMX8QNA6BX4XU",
		"AmazonRDS, productFamily=Database Instance, instanceType=db.r6g.large, databaseEngine=MySQL, deploymentOption=Multi-AZ, termType=OnDemand, regionCode=eu-west-2":        "TUF3H9GGSBC4EE4J",
		"AmazonRDS, productFamily=Database Storage, volumeType=General Purpose, databaseEngine=PostgreSQL, deploymentOption=Single-AZ, termType=OnDemand, regionCode=eu-west-2":  "TZ3NMVQHVWKUR3Y4",
		"AmazonRDS, productFamily=Database Storage, volumeType=General Purpose, databaseEngine=PostgreSQL, deploymentOption=Multi-AZ, termType=OnDemand, regionCode=eu-west-2":   "VUG2HGVQQDQW5MT6",
		"AmazonRDS, productFamily=Database Storage, volumeType=General Purpose, databaseEngine=MySQL, deploymentOption=Single-AZ, termType=OnDemand, regionCode=eu-west-2":       "QKPB395YS55V9XCT",
		"AmazonRDS, productFamily=Database Storage, volumeType=General Purpose, databaseEngine=MySQL, deploymentOption=Multi-AZ, termType=OnDemand, regionCode=eu-west-2":        "N88PGASTNXFMA6W9",
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("skus (-want +got):\n%s", diff)
	}

	micro := matched[0][0]
	wantSKU := cost.SKU{ID: "ATYF9R3XTURNBSMZ", Service: "AmazonRDS", Unit: "Hrs", Price: 0.018, Attributes: map[string]string{
		"productFamily": "Database Instance", "instanceType": "db.t4g.micro", "databaseEngine": "PostgreSQL",
		"deploymentOption": "Single-AZ", "termType": "OnDemand", "regionCode": "eu-west-2",
	}}
	if diff := cmp.Diff(wantSKU, micro); diff != "" {
		t.Errorf("sku (-want +got):\n%s", diff)
	}
}

const ec2Slice = `"FormatVersion","v1.0"
"Disclaimer","This pricing list is for informational purposes only."
"Publication Date","2026-09-04T23:11:17Z"
"Version","20260904231117"
"OfferCode","AmazonEC2"
"SKU","TermType","Unit","PricePerUnit","Currency","Product Family","usageType","Region Code"
"D8BXG95NJW6MV3B3","OnDemand","Hrs","0.0500000000","USD","NAT Gateway","EUW2-NatGateway-Hours","eu-west-2"
"7YHFNHKKMV3Y57SX","OnDemand","Hrs","0.0500000000","USD","NAT Gateway","EUW2-RegionalNatGateway-Hours","eu-west-2"
"ZECE57EY4F96YNAQ","OnDemand","GB","0.0500000000","USD","NAT Gateway","EUW2-NatGateway-Bytes","eu-west-2"
"M2YSHUBETB3JX4M4","OnDemand","Hrs","0.0450000000","USD","NAT Gateway","NatGateway-Hours","us-east-1"
"KVHMMSJX774M3XRN","OnDemand","Hrs","0.0450000000","USD","NAT Gateway","RegionalNatGateway-Hours","us-east-1"
`

func natLookup(t *testing.T, region string) cost.Lookup {
	t.Helper()
	lookups, err := aws.Cost().Catalogue(region)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range lookups {
		if l.Service == "AmazonEC2" {
			return l
		}
	}
	t.Fatal("no nat gateway lookup")
	return cost.Lookup{}
}

func TestScanOfferPicksThePlainNatGatewayHourWithOrWithoutARegionPrefix(t *testing.T) {
	for region, want := range map[string]string{"eu-west-2": "D8BXG95NJW6MV3B3", "us-east-1": "M2YSHUBETB3JX4M4"} {
		matched, err := scanOffer(strings.NewReader(ec2Slice), []cost.Lookup{natLookup(t, region)})
		if err != nil {
			t.Fatal(err)
		}
		if len(matched[0]) != 1 || matched[0][0].ID != want {
			t.Errorf("%s: matched %+v, want %s", region, matched[0], want)
		}
	}
}

func TestScanOfferRefusesAFileWithoutTheColumnsItNeeds(t *testing.T) {
	_, err := scanOffer(strings.NewReader("\"SKU\",\"Unit\",\"PricePerUnit\",\"Currency\"\n"), []cost.Lookup{natLookup(t, "eu-west-2")})
	if err == nil || !strings.Contains(err.Error(), "no Product Family column") {
		t.Errorf("error = %v", err)
	}
	_, err = scanOffer(strings.NewReader("\"FormatVersion\",\"v1.0\"\n"), nil)
	if err == nil || err.Error() != "no header row" {
		t.Errorf("error = %v", err)
	}
}

func TestEncodeWritesOneSkuPerLine(t *testing.T) {
	raw, err := encode(cost.Snapshot{
		Provider: "aws", Source: "test", Date: "2026-09-05", Currency: "USD",
		Versions: map[string]map[string]string{"AmazonEC2": {"eu-west-2": "v1"}},
		SKUs: []cost.SKU{
			{ID: "a", Service: "AmazonEC2", Attributes: map[string]string{"regionCode": "eu-west-2"}, Unit: "Hrs", Price: 0.05},
			{ID: "b", Service: "AmazonEC2", Attributes: map[string]string{"regionCode": "eu-west-2"}, Unit: "Hrs", Price: 0.045},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "provider": "aws",
  "source": "test",
  "date": "2026-09-05",
  "currency": "USD",
  "versions": {
    "AmazonEC2": {
      "eu-west-2": "v1"
    }
  },
  "skus": [
    {"sku":"a","service":"AmazonEC2","attributes":{"regionCode":"eu-west-2"},"unit":"Hrs","price":0.05},
    {"sku":"b","service":"AmazonEC2","attributes":{"regionCode":"eu-west-2"},"unit":"Hrs","price":0.045}
  ]
}
`
	if diff := cmp.Diff(want, string(raw)); diff != "" {
		t.Errorf("snapshot (-want +got):\n%s", diff)
	}
	if _, err := cost.Load(raw); err != nil {
		t.Errorf("the encoded snapshot does not load: %v", err)
	}
}
