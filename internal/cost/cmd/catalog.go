package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mooncitizen/togen/internal/cost"
	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve/gcp"
)

// The Cloud Billing Catalog refuses unregistered callers, so the refresh needs an API key
// from a Google Cloud project with the API enabled. It comes from the environment, never
// from the repository.
const (
	catalogHost   = "https://cloudbilling.googleapis.com/v1"
	catalogKeyVar = "GCP_BILLING_API_KEY"
	catalogPath   = "internal/cost/prices/gcp.json"
	catalogSource = "Cloud Billing Catalog API trimmed to the skus the matchers name, written by just refresh-prices --provider gcp"
	catalogPage   = 5000
	globalRegion  = "global"
)

type catalogSKU struct {
	ID          string `json:"skuId"`
	Description string `json:"description"`
	Category    struct {
		ResourceFamily string `json:"resourceFamily"`
		ResourceGroup  string `json:"resourceGroup"`
		UsageType      string `json:"usageType"`
	} `json:"category"`
	ServiceRegions []string `json:"serviceRegions"`
	PricingInfo    []struct {
		EffectiveTime     string `json:"effectiveTime"`
		PricingExpression struct {
			UsageUnit   string `json:"usageUnit"`
			TieredRates []struct {
				StartUsageAmount float64 `json:"startUsageAmount"`
				UnitPrice        struct {
					CurrencyCode string `json:"currencyCode"`
					Units        string `json:"units"`
					Nanos        int64  `json:"nanos"`
				} `json:"unitPrice"`
			} `json:"tieredRates"`
		} `json:"pricingExpression"`
	} `json:"pricingInfo"`
}

type catalogPageResponse struct {
	SKUs          []catalogSKU `json:"skus"`
	NextPageToken string       `json:"nextPageToken"`
}

func runCatalog(check bool) error {
	key := os.Getenv(catalogKeyVar)
	if key == "" {
		return fmt.Errorf("the Cloud Billing Catalog API needs a key: set %s to an API key from a Google Cloud project with the Cloud Billing API enabled", catalogKeyVar)
	}
	lookups, err := catalogLookups()
	if err != nil {
		return err
	}
	services := make([]string, 0, len(lookups))
	for service := range lookups {
		if _, ok := gcp.CatalogServices[service]; !ok {
			return fmt.Errorf("the matchers name %s, which the refresh has no catalog id for", service)
		}
		services = append(services, service)
	}
	slices.Sort(services)

	client := &http.Client{Timeout: 30 * time.Minute}
	snapshot := cost.Snapshot{
		Provider: ir.ProviderGCP,
		Source:   catalogSource,
		Date:     time.Now().UTC().Format(cost.DateLayout),
		Currency: "USD",
		Versions: map[string]map[string]string{},
	}
	var downloaded int64
	var problems []string
	for _, service := range services {
		skus, bytes, err := fetchCatalog(context.Background(), client, key, gcp.CatalogServices[service])
		if err != nil {
			return fmt.Errorf("%s: %w", service, err)
		}
		downloaded += bytes
		kept, versions, misses, err := trimCatalog(service, skus, lookups[service])
		if err != nil {
			return fmt.Errorf("%s: %w", service, err)
		}
		fmt.Fprintf(os.Stderr, "%s: %s, %d of %d skus\n", service, size(bytes), len(kept), len(skus))
		problems = append(problems, misses...)
		snapshot.SKUs = append(snapshot.SKUs, kept...)
		snapshot.Versions[service] = versions
	}
	if len(problems) > 0 {
		return fmt.Errorf("the matchers do not fit the catalog:\n  %s", strings.Join(problems, "\n  "))
	}
	sort.Slice(snapshot.SKUs, func(a, b int) bool {
		x, y := snapshot.SKUs[a], snapshot.SKUs[b]
		if x.Service != y.Service {
			return x.Service < y.Service
		}
		if x.Attributes["region"] != y.Attributes["region"] {
			return x.Attributes["region"] < y.Attributes["region"]
		}
		return x.ID < y.ID
	})

	raw, err := encode(snapshot)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "downloaded %s, %d skus, snapshot %s\n", size(downloaded), len(snapshot.SKUs), size(int64(len(raw))))
	if check {
		if _, err := os.Stat(catalogPath); os.IsNotExist(err) {
			return fmt.Errorf("there is no %s to compare with: run just refresh-prices --provider gcp and commit what it writes", catalogPath)
		}
		return compare(catalogPath, snapshot)
	}
	if err := os.WriteFile(catalogPath, raw, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "wrote %s\n", catalogPath)
	return nil
}

// The lookups the matchers can ask for, by service and then by region, each once.
func catalogLookups() (map[string]map[string][]cost.Lookup, error) {
	out := map[string]map[string][]cost.Lookup{}
	for _, region := range ir.Regions[ir.ProviderGCP] {
		lookups, err := gcp.Cost().Catalogue(region.ID)
		if err != nil {
			return nil, err
		}
		described := map[string]bool{}
		for _, l := range lookups {
			if key := l.Describe(); !described[key] {
				described[key] = true
				if out[l.Service] == nil {
					out[l.Service] = map[string][]cost.Lookup{}
				}
				out[l.Service][region.ID] = append(out[l.Service][region.ID], l)
			}
		}
	}
	return out, nil
}

func fetchCatalog(ctx context.Context, client *http.Client, key, serviceID string) ([]catalogSKU, int64, error) {
	var skus []catalogSKU
	var bytes int64
	token := ""
	for {
		query := url.Values{"key": {key}, "pageSize": {strconv.Itoa(catalogPage)}, "currencyCode": {"USD"}}
		if token != "" {
			query.Set("pageToken", token)
		}
		address := fmt.Sprintf("%s/services/%s/skus?%s", catalogHost, serviceID, query.Encode())
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
		if err != nil {
			return nil, 0, err
		}
		resp, err := client.Do(request)
		if err != nil {
			return nil, 0, err
		}
		body := &counting{reader: resp.Body}
		var page catalogPageResponse
		err = json.NewDecoder(body).Decode(&page)
		_ = resp.Body.Close()
		bytes += body.bytes
		switch {
		case resp.StatusCode != http.StatusOK:
			return nil, 0, fmt.Errorf("services/%s/skus: %s", serviceID, resp.Status)
		case err != nil:
			return nil, 0, fmt.Errorf("services/%s/skus: %w", serviceID, err)
		}
		skus = append(skus, page.SKUs...)
		if page.NextPageToken == "" {
			return skus, bytes, nil
		}
		token = page.NextPageToken
	}
}

// trimCatalog keeps the skus the lookups pick, one entry per sku listing the regions it serves,
// and reports every lookup that matches none or several in a region. The version a region
// gets is the newest effective time among the skus kept for it.
func trimCatalog(service string, skus []catalogSKU, byRegion map[string][]cost.Lookup) ([]cost.SKU, map[string]string, []string, error) {
	regions := make([]string, 0, len(byRegion))
	for region := range byRegion {
		regions = append(regions, region)
	}
	slices.Sort(regions)

	type match struct {
		lookup cost.Lookup
		skus   []cost.SKU
	}
	matches := map[string][]*match{}
	predicates := map[string][]func(map[string]string) bool{}
	for _, region := range regions {
		for _, l := range byRegion[region] {
			matches[region] = append(matches[region], &match{lookup: l})
			predicates[region] = append(predicates[region], l.Predicate())
		}
	}

	var kept []cost.SKU
	versions := map[string]string{}
	seen := map[string]bool{}
	for _, raw := range skus {
		served := servedRegions(raw, regions)
		if len(served) == 0 {
			continue
		}
		attributes := map[string]string{
			"resourceFamily": raw.Category.ResourceFamily,
			"resourceGroup":  raw.Category.ResourceGroup,
			"usageType":      raw.Category.UsageType,
			"description":    raw.Description,
			"region":         strings.Join(served, " "),
		}
		for attribute, value := range attributes {
			if value == "" {
				delete(attributes, attribute)
			}
		}
		var sku *cost.SKU
		for _, region := range regions {
			for i, matched := range predicates[region] {
				if !matched(attributes) {
					continue
				}
				if sku == nil {
					converted, err := catalogEntry(service, raw, attributes)
					if err != nil {
						return nil, nil, nil, err
					}
					sku = &converted
				}
				matches[region][i].skus = append(matches[region][i].skus, *sku)
				if effective := effectiveDate(raw); effective > versions[region] {
					versions[region] = effective
				}
			}
		}
		if sku != nil && !seen[sku.ID] {
			seen[sku.ID] = true
			kept = append(kept, *sku)
		}
	}

	var misses []string
	for _, region := range regions {
		for _, m := range matches[region] {
			switch len(m.skus) {
			case 1:
			case 0:
				misses = append(misses, fmt.Sprintf("%s %s: %s (%s) matches nothing", service, region, m.lookup.Label, m.lookup.Describe()))
			default:
				ids := make([]string, len(m.skus))
				for j, sku := range m.skus {
					ids[j] = sku.ID
				}
				misses = append(misses, fmt.Sprintf("%s %s: %s (%s) matches %d skus, %s", service, region, m.lookup.Label, m.lookup.Describe(), len(ids), strings.Join(ids, ", ")))
			}
		}
	}
	return kept, versions, misses, nil
}

// The regions a sku serves among the ones asked for, or global alone for a meter priced the
// same everywhere.
func servedRegions(sku catalogSKU, regions []string) []string {
	var served []string
	for _, region := range sku.ServiceRegions {
		if region == globalRegion {
			return []string{globalRegion}
		}
		if slices.Contains(regions, region) {
			served = append(served, region)
		}
	}
	slices.Sort(served)
	return served
}

// A catalog meter with a free allowance opens with a tier at nothing; the estimate leaves
// free tiers out, so the rate is the first tier that charges, and upTo is where it ends.
func catalogEntry(service string, raw catalogSKU, attributes map[string]string) (cost.SKU, error) {
	if len(raw.PricingInfo) == 0 {
		return cost.SKU{}, fmt.Errorf("sku %s has no pricing info", raw.ID)
	}
	expression := raw.PricingInfo[0].PricingExpression
	if len(expression.TieredRates) == 0 {
		return cost.SKU{}, fmt.Errorf("sku %s has no rates", raw.ID)
	}
	tier := 0
	for i, rate := range expression.TieredRates {
		if (rate.UnitPrice.Units != "0" && rate.UnitPrice.Units != "") || rate.UnitPrice.Nanos != 0 {
			tier = i
			break
		}
	}
	rate := expression.TieredRates[tier]
	if rate.UnitPrice.CurrencyCode != "USD" {
		return cost.SKU{}, fmt.Errorf("sku %s is priced in %s, not USD", raw.ID, rate.UnitPrice.CurrencyCode)
	}
	units, err := strconv.ParseFloat(rate.UnitPrice.Units, 64)
	if rate.UnitPrice.Units == "" {
		units, err = 0, nil
	}
	if err != nil {
		return cost.SKU{}, fmt.Errorf("sku %s: price units %q: %w", raw.ID, rate.UnitPrice.Units, err)
	}
	sku := cost.SKU{
		ID:         raw.ID,
		Service:    service,
		Attributes: attributes,
		Unit:       expression.UsageUnit,
		Price:      units + float64(rate.UnitPrice.Nanos)/1e9,
	}
	if tier+1 < len(expression.TieredRates) {
		sku.UpTo = expression.TieredRates[tier+1].StartUsageAmount
	}
	return sku, nil
}

func effectiveDate(sku catalogSKU) string {
	if len(sku.PricingInfo) == 0 {
		return ""
	}
	date, _, _ := strings.Cut(sku.PricingInfo[0].EffectiveTime, "T")
	return date
}
