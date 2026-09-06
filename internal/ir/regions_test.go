package ir

import (
	"regexp"
	"testing"
)

func TestDefaultRegionIsTheFirstOne(t *testing.T) {
	for _, c := range []struct {
		provider CloudProvider
		want     string
		ok       bool
	}{
		{ProviderAWS, "eu-west-2", true},
		{ProviderGCP, "europe-west2", true},
		{ProviderAzure, "uksouth", true},
		{"oracle", "", false},
	} {
		got, ok := DefaultRegion(c.provider)
		if got != c.want || ok != c.ok {
			t.Errorf("DefaultRegion(%q) = %q, %v, want %q, %v", c.provider, got, ok, c.want, c.ok)
		}
	}
}

func TestEveryProviderListsNamedRegionsWithUniqueIds(t *testing.T) {
	kebab := regexp.MustCompile(KebabPattern)
	for _, p := range Providers {
		regions := Regions[p]
		if len(regions) < 20 {
			t.Errorf("%s has %d regions, want the commonly used ones", p, len(regions))
		}
		seen := map[string]bool{}
		for _, r := range regions {
			if !kebab.MatchString(r.ID) {
				t.Errorf("%s: %q is not a region id", p, r.ID)
			}
			if r.Name == "" {
				t.Errorf("%s: %s has no name", p, r.ID)
			}
			if seen[r.ID] {
				t.Errorf("%s: %s is listed twice", p, r.ID)
			}
			seen[r.ID] = true
		}
	}
}
