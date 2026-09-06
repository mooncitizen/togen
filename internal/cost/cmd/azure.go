package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"os"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mooncitizen/togen/internal/cost"
	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve/azure"
)

const (
	retailPrices      = "https://prices.azure.com/api/retail/prices"
	retailVersion     = "2023-01-01-preview"
	azureSnapshotPath = "internal/cost/prices/azure.json"
	azureSource       = "Azure Retail Prices API trimmed to the meters the matchers name, per-second and per-batch units normalised to hours and single operations, written by just refresh-prices --provider azure"
	azureWorkers      = 2
	retries           = 5
)

// The API prices a meter per batch (10 executions, 1M operations) or per second, and the
// lines read in hours and single operations, so each unit of measure the matchers can meet
// maps to a factor and the unit the snapshot records. An unlisted one fails the refresh.
var azureUnits = map[string]struct {
	factor float64
	unit   string
}{
	"1 Hour":       {1, "hour"},
	"1/Hour":       {1, "hour"},
	"1/Month":      {1, "month"},
	"1 Second":     {3600, "hour"},
	"1 GiB Second": {3600, "GiB-hour"},
	"1 GB Second":  {1, "GB-second"},
	"1 GB/Month":   {1, "GB-month"},
	"1 GiB/Month":  {1, "GiB-month"},
	"1 GB":         {1, "GB"},
	"1":            {1, "each"},
	"10":           {0.1, "each"},
	"10K":          {1e-4, "each"},
	"1M":           {1e-6, "each"},
}

type retailItem struct {
	MeterID          string  `json:"meterId"`
	ServiceName      string  `json:"serviceName"`
	ProductName      string  `json:"productName"`
	SkuName          string  `json:"skuName"`
	MeterName        string  `json:"meterName"`
	ArmRegionName    string  `json:"armRegionName"`
	UnitOfMeasure    string  `json:"unitOfMeasure"`
	RetailPrice      float64 `json:"retailPrice"`
	TierMinimumUnits float64 `json:"tierMinimumUnits"`
	CurrencyCode     string  `json:"currencyCode"`
}

type retailPage struct {
	Items        []retailItem `json:"Items"`
	NextPageLink string       `json:"NextPageLink"`
}

func runAzure(check bool) error {
	jobs, err := planAzure()
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 5 * time.Minute}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	outcomes := make([]outcome, len(jobs))
	var wg sync.WaitGroup
	slots := make(chan struct{}, azureWorkers)
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
			outcomes[i] = fetchRetail(ctx, client, j)
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
	snapshot := cost.Snapshot{
		Provider: ir.ProviderAzure,
		Source:   azureSource,
		Date:     time.Now().UTC().Format(cost.DateLayout),
		Currency: "USD",
		Versions: map[string]map[string]string{},
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
		if snapshot.Versions[j.service] == nil {
			snapshot.Versions[j.service] = map[string]string{}
		}
		snapshot.Versions[j.service][j.region] = retailVersion
		for _, sku := range o.skus {
			if !slices.ContainsFunc(snapshot.SKUs, func(s cost.SKU) bool { return s.Service == sku.Service && s.ID == sku.ID }) {
				snapshot.SKUs = append(snapshot.SKUs, sku)
			}
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("the matchers do not fit the retail prices:\n  %s", strings.Join(problems, "\n  "))
	}
	sort.Slice(snapshot.SKUs, func(a, b int) bool {
		x, y := snapshot.SKUs[a], snapshot.SKUs[b]
		if x.Service != y.Service {
			return x.Service < y.Service
		}
		if x.Attributes[azureRegion] != y.Attributes[azureRegion] {
			return x.Attributes[azureRegion] < y.Attributes[azureRegion]
		}
		return x.ID < y.ID
	})

	raw, err := encode(snapshot)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "downloaded %s, %d skus, snapshot %s\n", size(downloaded), len(snapshot.SKUs), size(int64(len(raw))))
	if check {
		return compare(snapshot, azureSnapshotPath)
	}
	if err := os.WriteFile(azureSnapshotPath, raw, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "wrote %s\n", azureSnapshotPath)
	return nil
}

const azureRegion = "armRegionName"

// One job per service per region, with the lookups the matchers can ask for there.
func planAzure() ([]job, error) {
	var jobs []job
	for _, region := range ir.Regions[ir.ProviderAzure] {
		lookups, err := azure.Cost().Catalogue(region.ID)
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
			jobs = append(jobs, job{service: service, region: region.ID, lookups: byService[service]})
		}
	}
	return jobs, nil
}

// One filter per service and region, narrowed to the product names when every lookup pins
// one literally, since the API pages a hundred rows at a time and rate limits callers.
func retailFilter(j job) string {
	filter := fmt.Sprintf("armRegionName eq '%s' and currencyCode eq 'USD' and priceType eq 'Consumption' and serviceName eq '%s'",
		j.region, strings.ReplaceAll(j.service, "'", "''"))
	var products []string
	for _, l := range j.lookups {
		i := slices.IndexFunc(l.Filters, func(f cost.Filter) bool { return f.Attribute == "productName" && !f.Pattern })
		if i < 0 {
			return filter
		}
		if p := fmt.Sprintf("productName eq '%s'", strings.ReplaceAll(l.Filters[i].Value, "'", "''")); !slices.Contains(products, p) {
			products = append(products, p)
		}
	}
	return filter + " and (" + strings.Join(products, " or ") + ")"
}

func fetchRetail(ctx context.Context, client *http.Client, j job) outcome {
	query := url.Values{"api-version": {retailVersion}, "$filter": {retailFilter(j)}}
	next := retailPrices + "?" + query.Encode()
	var items []retailItem
	var bytes int64
	for next != "" {
		page, n, err := fetchPage(ctx, client, next)
		if err != nil {
			return outcome{err: err}
		}
		bytes += n
		items = append(items, page.Items...)
		next = page.NextPageLink
	}
	matched, err := matchRetail(items, j.lookups)
	if err != nil {
		return outcome{err: err}
	}
	out := outcome{bytes: bytes}
	for i, l := range j.lookups {
		switch len(matched[i]) {
		case 1:
			out.skus = append(out.skus, matched[i][0])
		case 0:
			out.misses = append(out.misses, fmt.Sprintf("%s (%s) matches nothing", l.Label, l.Describe()))
		default:
			ids := make([]string, len(matched[i]))
			for k, sku := range matched[i] {
				ids[k] = sku.ID
			}
			out.misses = append(out.misses, fmt.Sprintf("%s (%s) matches %d meters, %s", l.Label, l.Describe(), len(ids), strings.Join(ids, ", ")))
		}
	}
	return out
}

// The API answers a burst with 429s, so a page is retried with a growing pause.
func fetchPage(ctx context.Context, client *http.Client, pageURL string) (retailPage, int64, error) {
	var page retailPage
	for attempt := 0; ; attempt++ {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
		if err != nil {
			return page, 0, err
		}
		resp, err := client.Do(request)
		if err != nil {
			return page, 0, err
		}
		body := &counting{reader: resp.Body}
		err = json.NewDecoder(body).Decode(&page)
		_ = resp.Body.Close()
		switch {
		case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
			if attempt == retries {
				return page, 0, fmt.Errorf("%s: %s after %d attempts", pageURL, resp.Status, attempt+1)
			}
			select {
			case <-ctx.Done():
				return page, 0, ctx.Err()
			case <-time.After(time.Duration(2<<attempt) * time.Second):
			}
			continue
		case resp.StatusCode != http.StatusOK:
			return page, 0, fmt.Errorf("%s: %s", pageURL, resp.Status)
		case err != nil:
			return page, 0, fmt.Errorf("%s: %w", pageURL, err)
		}
		return page, body.bytes, nil
	}
}

// matchRetail returns, per lookup, the meters its filters pick. A tiered meter is several rows
// sharing a meter id; the first tier that charges is kept, the free grant below it being a
// monthly allowance rather than a rate, and the next tier's start becomes upTo.
func matchRetail(items []retailItem, lookups []cost.Lookup) ([][]cost.SKU, error) {
	predicates := make([]func(map[string]string) bool, len(lookups))
	for i, l := range lookups {
		predicates[i] = l.Predicate()
	}
	matched := make([][]cost.SKU, len(lookups))
	for i, matches := range predicates {
		byMeter := map[string][]retailItem{}
		var order []string
		for _, item := range items {
			if item.ServiceName != lookups[i].Service || !matches(item.attributes()) {
				continue
			}
			if _, seen := byMeter[item.MeterID]; !seen {
				order = append(order, item.MeterID)
			}
			byMeter[item.MeterID] = append(byMeter[item.MeterID], item)
		}
		for _, id := range order {
			sku, err := skuFromTiers(byMeter[id])
			if err != nil {
				return nil, err
			}
			matched[i] = append(matched[i], sku)
		}
	}
	return matched, nil
}

func (item retailItem) attributes() map[string]string {
	return map[string]string{
		"serviceName":   item.ServiceName,
		"productName":   item.ProductName,
		"skuName":       item.SkuName,
		"meterName":     item.MeterName,
		azureRegion:     item.ArmRegionName,
		"unitOfMeasure": item.UnitOfMeasure,
	}
}

func skuFromTiers(tiers []retailItem) (cost.SKU, error) {
	sort.SliceStable(tiers, func(a, b int) bool { return tiers[a].TierMinimumUnits < tiers[b].TierMinimumUnits })
	chosen := 0
	for i, tier := range tiers {
		if tier.RetailPrice > 0 {
			chosen = i
			break
		}
	}
	item := tiers[chosen]
	if item.CurrencyCode != "USD" {
		return cost.SKU{}, fmt.Errorf("meter %s is priced in %s, not USD", item.MeterID, item.CurrencyCode)
	}
	unit, ok := azureUnits[item.UnitOfMeasure]
	if !ok {
		return cost.SKU{}, fmt.Errorf("meter %s (%s, %s) is priced per %q, which the refresh does not convert",
			item.MeterID, item.ProductName, item.MeterName, item.UnitOfMeasure)
	}
	attributes := item.attributes()
	delete(attributes, "serviceName")
	// Some meters carry one id in every region, so the region is part of the sku's.
	sku := cost.SKU{
		ID:         item.ArmRegionName + "/" + item.MeterID,
		Service:    item.ServiceName,
		Attributes: attributes,
		Unit:       unit.unit,
		Price:      tidy(item.RetailPrice * unit.factor),
	}
	if chosen+1 < len(tiers) {
		sku.UpTo = tidy(tiers[chosen+1].TierMinimumUnits / unit.factor)
	}
	return sku, nil
}

// A rate times 3600 or divided by a million picks up binary noise the snapshot need not carry.
func tidy(v float64) float64 {
	return math.Round(v*1e10) / 1e10
}
