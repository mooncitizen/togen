package cost

import (
	"fmt"
	"regexp"
	"strings"
)

type Filter struct {
	Attribute string
	Value     string
	// Value is a regular expression matched against the whole attribute rather than a literal.
	Pattern bool
}

// Lookup names one price: the offer file attributes that pick a single SKU, and how much of it
// a month uses.
type Lookup struct {
	Label    string
	Service  string
	Filters  []Filter
	Quantity float64
	Unit     string
}

func (f Filter) String() string {
	if f.Pattern {
		return f.Attribute + "~" + f.Value
	}
	return f.Attribute + "=" + f.Value
}

func (l Lookup) Describe() string {
	parts := make([]string, 0, len(l.Filters)+1)
	parts = append(parts, l.Service)
	for _, f := range l.Filters {
		parts = append(parts, f.String())
	}
	return strings.Join(parts, ", ")
}

// Predicate compiles the filters once so a caller can test many attribute sets cheaply.
func (l Lookup) Predicate() func(attributes map[string]string) bool {
	type test struct {
		attribute string
		value     string
		pattern   *regexp.Regexp
	}
	tests := make([]test, len(l.Filters))
	for i, f := range l.Filters {
		tests[i] = test{attribute: f.Attribute, value: f.Value}
		if f.Pattern {
			tests[i].pattern = regexp.MustCompile("^(?:" + f.Value + ")$")
		}
	}
	return func(attributes map[string]string) bool {
		for _, t := range tests {
			value := attributes[t.attribute]
			if t.pattern != nil {
				if !t.pattern.MatchString(value) {
					return false
				}
			} else if value != t.value {
				return false
			}
		}
		return true
	}
}

func (s *Snapshot) Find(l Lookup) (SKU, error) {
	matches := l.Predicate()
	var found []SKU
	for _, sku := range s.SKUs {
		if sku.Service == l.Service && matches(sku.Attributes) {
			found = append(found, sku)
		}
	}
	switch len(found) {
	case 1:
		return found[0], nil
	case 0:
		return SKU{}, fmt.Errorf("no %s sku matches %s (%s)", s.Provider, l.Label, l.Describe())
	}
	ids := make([]string, len(found))
	for i, sku := range found {
		ids[i] = sku.ID
	}
	return SKU{}, fmt.Errorf("%d %s skus match %s (%s): the matcher is too loose, it picked %s",
		len(found), s.Provider, l.Label, l.Describe(), strings.Join(ids, ", "))
}
