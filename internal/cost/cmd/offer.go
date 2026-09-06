package main

import (
	"bufio"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/mooncitizen/togen/internal/cost"
)

// The matchers name attributes the way the JSON offer files do; the CSV heads its columns
// differently.
var columns = map[string]string{
	"productFamily":    "Product Family",
	"termType":         "TermType",
	"regionCode":       "Region Code",
	"usagetype":        "usageType",
	"instanceType":     "Instance Type",
	"databaseEngine":   "Database Engine",
	"deploymentOption": "Deployment Option",
	"volumeType":       "Volume Type",
	"group":            "Group",
	"queueType":        "Queue Type",
	"cacheEngine":      "Cache Engine",
	"storageClass":     "Storage Class",
}

func fetchOffer(ctx context.Context, client *http.Client, url string, lookups []cost.Lookup) outcome {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return outcome{err: err}
	}
	resp, err := client.Do(request)
	if err != nil {
		return outcome{err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return outcome{err: fmt.Errorf("%s: %s", url, resp.Status)}
	}
	body := &counting{reader: resp.Body}
	matched, err := scanOffer(body, lookups)
	if err != nil {
		return outcome{err: fmt.Errorf("%s: %w", url, err)}
	}
	out := outcome{bytes: body.bytes}
	for i, l := range lookups {
		switch len(matched[i]) {
		case 1:
			out.skus = append(out.skus, matched[i][0])
		case 0:
			out.misses = append(out.misses, fmt.Sprintf("%s (%s) matches nothing", l.Label, l.Describe()))
		default:
			ids := make([]string, len(matched[i]))
			for j, sku := range matched[i] {
				ids[j] = sku.ID
			}
			out.misses = append(out.misses, fmt.Sprintf("%s (%s) matches %d rows, %s", l.Label, l.Describe(), len(ids), strings.Join(ids, ", ")))
		}
	}
	return out
}

// scanOffer streams an offer file and returns, per lookup, the rows its filters pick. The
// file starts with a few metadata lines; the header is the row that begins with SKU.
func scanOffer(r io.Reader, lookups []cost.Lookup) ([][]cost.SKU, error) {
	reader := csv.NewReader(bufio.NewReaderSize(r, 4<<20))
	reader.FieldsPerRecord = -1
	reader.ReuseRecord = true

	var header []string
	for header == nil {
		record, err := reader.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil, errors.New("no header row")
			}
			return nil, err
		}
		if record[0] == "SKU" {
			header = append([]string(nil), record...)
		}
	}
	at := make(map[string]int, len(header))
	for i, name := range header {
		at[name] = i
	}
	fixed := map[string]int{}
	for _, name := range []string{"SKU", "Unit", "PricePerUnit", "Currency"} {
		i, ok := at[name]
		if !ok {
			return nil, fmt.Errorf("the offer file has no %s column", name)
		}
		fixed[name] = i
	}

	attributes := map[string]int{}
	predicates := make([]func(map[string]string) bool, len(lookups))
	for i, l := range lookups {
		for _, f := range l.Filters {
			column, ok := columns[f.Attribute]
			if !ok {
				return nil, fmt.Errorf("the refresh does not know which column holds %s", f.Attribute)
			}
			index, ok := at[column]
			if !ok {
				return nil, fmt.Errorf("the offer file has no %s column (for %s)", column, f.Attribute)
			}
			attributes[f.Attribute] = index
		}
		predicates[i] = l.Predicate()
	}
	// A file with tiers says where each row's tier starts, and the predicates keep the first.
	if i, ok := at["StartingRange"]; ok {
		attributes[cost.StartingRange] = i
	}
	endingRange, tiered := at["EndingRange"]

	matched := make([][]cost.SKU, len(lookups))
	row := make(map[string]string, len(attributes))
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			return matched, nil
		}
		if err != nil {
			return nil, err
		}
		if len(record) < len(header) {
			continue
		}
		for attribute, index := range attributes {
			row[attribute] = record[index]
		}
		for i, matches := range predicates {
			if !matches(row) {
				continue
			}
			sku, err := skuFrom(record, fixed, lookups[i].Service, row)
			if err != nil {
				return nil, err
			}
			if tiered && record[endingRange] != "Inf" {
				sku.UpTo, err = strconv.ParseFloat(record[endingRange], 64)
				if err != nil {
					return nil, fmt.Errorf("sku %s: ending range %q: %w", sku.ID, record[endingRange], err)
				}
			}
			matched[i] = append(matched[i], sku)
		}
	}
}

func skuFrom(record []string, fixed map[string]int, service string, attributes map[string]string) (cost.SKU, error) {
	id := record[fixed["SKU"]]
	if currency := record[fixed["Currency"]]; currency != "USD" {
		return cost.SKU{}, fmt.Errorf("sku %s is priced in %s, not USD", id, currency)
	}
	price, err := strconv.ParseFloat(record[fixed["PricePerUnit"]], 64)
	if err != nil {
		return cost.SKU{}, fmt.Errorf("sku %s: price %q: %w", id, record[fixed["PricePerUnit"]], err)
	}
	kept := make(map[string]string, len(attributes))
	for attribute, value := range attributes {
		if value != "" && attribute != cost.StartingRange {
			kept[attribute] = value
		}
	}
	return cost.SKU{ID: id, Service: service, Attributes: kept, Unit: record[fixed["Unit"]], Price: price}, nil
}

type counting struct {
	reader io.Reader
	bytes  int64
}

func (c *counting) Read(p []byte) (int, error) {
	n, err := c.reader.Read(p)
	c.bytes += int64(n)
	return n, err
}
