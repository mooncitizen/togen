package ir

import "testing"

func TestDefaultEngineVersionIsTheFirstOne(t *testing.T) {
	for _, c := range []struct {
		provider CloudProvider
		engine   Engine
		want     string
		ok       bool
	}{
		{ProviderAWS, EnginePostgres, "17", true},
		{ProviderAWS, EngineMySQL, "8.4", true},
		{ProviderAWS, "oracle", "", false},
		{ProviderGCP, EnginePostgres, "", false},
	} {
		got, ok := DefaultEngineVersion(c.provider, c.engine)
		if got != c.want || ok != c.ok {
			t.Errorf("DefaultEngineVersion(%q, %q) = %q, %v, want %q, %v", c.provider, c.engine, got, ok, c.want, c.ok)
		}
	}
}
