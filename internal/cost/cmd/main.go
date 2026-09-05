// Refreshes internal/cost/prices/aws.json from the AWS Price List bulk offer files.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mooncitizen/togen/internal/cost"
	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve/aws"
)

// The regions the studio offers, so every one of them has a price for every lookup.
var regions = []string{
	"eu-west-2", "eu-west-1", "eu-west-3", "eu-central-1", "eu-central-2", "eu-north-1", "eu-south-1", "eu-south-2",
	"us-east-1", "us-east-2", "us-west-1", "us-west-2", "ca-central-1", "sa-east-1", "af-south-1",
	"me-south-1", "me-central-1",
	"ap-south-1", "ap-southeast-1", "ap-southeast-2", "ap-northeast-1", "ap-northeast-2", "ap-northeast-3",
}

const (
	pricingHost  = "https://pricing.us-east-1.amazonaws.com"
	snapshotPath = "internal/cost/prices/aws.json"
	source       = "AWS Price List bulk offer files trimmed to the skus the matchers name, written by just refresh-prices"
	workers      = 4
)

func main() {
	check := flag.Bool("check", false, "fetch and compare with the committed snapshot instead of writing it")
	flag.Parse()
	if err := run(*check); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type job struct {
	service string
	region  string
	lookups []cost.Lookup
}

type outcome struct {
	skus   []cost.SKU
	bytes  int64
	misses []string
	err    error
}

func run(check bool) error {
	jobs, err := plan()
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 30 * time.Minute}
	versions, err := currentVersions(client, jobs)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	outcomes := make([]outcome, len(jobs))
	var wg sync.WaitGroup
	slots := make(chan struct{}, workers)
	for i, j := range jobs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			slots <- struct{}{}
			defer func() { <-slots }()
			if ctx.Err() != nil {
				outcomes[i].err = ctx.Err()
				return
			}
			url := pricingHost + strings.TrimSuffix(versions[j.service][j.region], ".json") + ".csv"
			outcomes[i] = fetchOffer(ctx, client, url, j.lookups)
			if outcomes[i].err != nil {
				cancel()
				return
			}
			fmt.Fprintf(os.Stderr, "%s %s: %s, %d skus\n", j.service, j.region, size(outcomes[i].bytes), len(outcomes[i].skus))
		}()
	}
	wg.Wait()

	var downloaded int64
	var problems []string
	seen := map[string]bool{}
	snapshot := cost.Snapshot{
		Provider: ir.ProviderAWS,
		Source:   source,
		Date:     time.Now().UTC().Format(cost.DateLayout),
		Currency: "USD",
		Versions: versions,
	}
	for i, j := range jobs {
		o := outcomes[i]
		if o.err != nil {
			return fmt.Errorf("%s %s: %w", j.service, j.region, o.err)
		}
		downloaded += o.bytes
		for _, miss := range o.misses {
			problems = append(problems, fmt.Sprintf("%s %s: %s", j.service, j.region, miss))
		}
		for _, sku := range o.skus {
			if key := sku.Service + " " + sku.ID; !seen[key] {
				seen[key] = true
				snapshot.SKUs = append(snapshot.SKUs, sku)
			}
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("the matchers do not fit the offer files:\n  %s", strings.Join(problems, "\n  "))
	}
	sort.Slice(snapshot.SKUs, func(a, b int) bool {
		x, y := snapshot.SKUs[a], snapshot.SKUs[b]
		if x.Service != y.Service {
			return x.Service < y.Service
		}
		if x.Attributes["regionCode"] != y.Attributes["regionCode"] {
			return x.Attributes["regionCode"] < y.Attributes["regionCode"]
		}
		return x.ID < y.ID
	})

	raw, err := encode(snapshot)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "downloaded %s, %d skus, snapshot %s\n", size(downloaded), len(snapshot.SKUs), size(int64(len(raw))))
	if check {
		return compare(snapshot)
	}
	if err := os.WriteFile(snapshotPath, raw, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "wrote %s\n", snapshotPath)
	return nil
}

// One job per service per region, with the lookups the matchers can ask for there.
func plan() ([]job, error) {
	var jobs []job
	for _, region := range regions {
		lookups, err := aws.Cost().Catalogue(region)
		if err != nil {
			return nil, err
		}
		byService := map[string][]cost.Lookup{}
		described := map[string]bool{}
		for _, l := range lookups {
			if key := l.Describe(); !described[key] {
				described[key] = true
				byService[l.Service] = append(byService[l.Service], l)
			}
		}
		services := make([]string, 0, len(byService))
		for service := range byService {
			services = append(services, service)
		}
		slices.Sort(services)
		for _, service := range services {
			jobs = append(jobs, job{service: service, region: region, lookups: byService[service]})
		}
	}
	return jobs, nil
}

func currentVersions(client *http.Client, jobs []job) (map[string]map[string]string, error) {
	out := map[string]map[string]string{}
	for _, j := range jobs {
		if _, ok := out[j.service]; ok {
			continue
		}
		index, err := regionIndex(client, j.service)
		if err != nil {
			return nil, err
		}
		out[j.service] = map[string]string{}
		for _, region := range regions {
			url, ok := index[region]
			if !ok {
				return nil, fmt.Errorf("%s has no offer file for %s", j.service, region)
			}
			out[j.service][region] = url
		}
	}
	return out, nil
}

func regionIndex(client *http.Client, service string) (map[string]string, error) {
	url := fmt.Sprintf("%s/offers/v1.0/aws/%s/current/region_index.json", pricingHost, service)
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", url, resp.Status)
	}
	var index struct {
		Regions map[string]struct {
			CurrentVersionURL string `json:"currentVersionUrl"`
		} `json:"regions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&index); err != nil {
		return nil, fmt.Errorf("%s: %w", url, err)
	}
	out := make(map[string]string, len(index.Regions))
	for region, entry := range index.Regions {
		out[region] = entry.CurrentVersionURL
	}
	return out, nil
}

// One sku per line, so a refresh that moves a price is a one line diff.
func encode(s cost.Snapshot) ([]byte, error) {
	skus := s.SKUs
	s.SKUs = nil
	head, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return nil, err
	}
	var b strings.Builder
	b.Write(head[:len(head)-len("\n}")])
	b.WriteString(",\n  \"skus\": [\n")
	for i, sku := range skus {
		line, err := json.Marshal(sku)
		if err != nil {
			return nil, err
		}
		b.WriteString("    ")
		b.Write(line)
		if i < len(skus)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString("  ]\n}\n")
	return []byte(b.String()), nil
}

func compare(live cost.Snapshot) error {
	raw, err := os.ReadFile(snapshotPath)
	if err != nil {
		return err
	}
	committed, err := cost.Load(raw)
	if err != nil {
		return err
	}
	byKey := func(skus []cost.SKU) map[string]cost.SKU {
		out := make(map[string]cost.SKU, len(skus))
		for _, sku := range skus {
			out[sku.Service+" "+sku.ID] = sku
		}
		return out
	}
	was, now := byKey(committed.SKUs), byKey(live.SKUs)
	var changes []string
	for _, sku := range live.SKUs {
		old, ok := was[sku.Service+" "+sku.ID]
		switch {
		case !ok:
			changes = append(changes, "new: "+describe(sku))
		case old.Price != sku.Price || old.Unit != sku.Unit:
			changes = append(changes, fmt.Sprintf("changed: %s, was %g %s", describe(sku), old.Price, old.Unit))
		}
	}
	for _, sku := range committed.SKUs {
		if _, ok := now[sku.Service+" "+sku.ID]; !ok {
			changes = append(changes, "gone: "+describe(sku))
		}
	}
	newer := 0
	for service, byRegion := range live.Versions {
		for region, version := range byRegion {
			if committed.Versions[service][region] != version {
				newer++
			}
		}
	}
	if len(changes) > 0 {
		return fmt.Errorf("the committed snapshot from %s differs from the live offer files:\n  %s\nrun just refresh-prices and review the diff",
			committed.Date, strings.Join(changes, "\n  "))
	}
	fmt.Fprintf(os.Stderr, "the committed snapshot from %s matches the live offer files (%d offer files have a newer version, no priced sku moved)\n",
		committed.Date, newer)
	return nil
}

func describe(sku cost.SKU) string {
	var traits []string
	for attribute, value := range sku.Attributes {
		if attribute != "regionCode" && attribute != "termType" {
			traits = append(traits, value)
		}
	}
	slices.Sort(traits)
	return fmt.Sprintf("%s %s in %s (%s) %g %s", sku.Service, sku.ID, sku.Attributes["regionCode"], strings.Join(traits, ", "), sku.Price, sku.Unit)
}

func size(bytes int64) string {
	switch {
	case bytes >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(bytes)/(1<<30))
	case bytes >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(bytes)/(1<<20))
	}
	return fmt.Sprintf("%.1f KB", float64(bytes)/(1<<10))
}
