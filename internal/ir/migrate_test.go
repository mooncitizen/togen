package ir

import (
	"strings"
	"testing"
)

func TestMigrate(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		wantErr string
	}{
		{"a version 1 document passes through", `{"version":1,"name":"x"}`, ""},
		{"no version", `{"name":"x"}`, "project file has no numeric 'version' field"},
		{"a non-numeric version", `{"version":"1"}`, "project file has no numeric 'version' field"},
		{"a version from the future", `{"version":2}`, "project file is version 2 but this Togen only understands up to 1"},
		{"a version with no migration", `{"version":0}`, "no migration from version 0"},
		{"a non-object", `"nope"`, "project file must be a JSON object"},
		{"an array", `[]`, "project file must be a JSON object"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, err := Migrate([]byte(c.raw))
			if c.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
				if string(out) != c.raw {
					t.Fatalf("out = %s, want it unchanged", out)
				}
				return
			}
			if err == nil {
				t.Fatal("want an error")
			}
			if err.Error() != c.wantErr {
				t.Fatalf("error = %q, want %q", err, c.wantErr)
			}
		})
	}
}

func TestMigrateRejectsMalformedJSON(t *testing.T) {
	_, err := Migrate([]byte(`{`))
	if err == nil {
		t.Fatal("want an error")
	}
	if !strings.Contains(err.Error(), "not valid JSON") {
		t.Fatalf("error = %q", err)
	}
}
