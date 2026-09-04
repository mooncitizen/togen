package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfigRaw(t *testing.T, cwd, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(cwd, "togen"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath(cwd), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func configError(t *testing.T, text string) string {
	t.Helper()
	cwd := t.TempDir()
	writeConfigRaw(t, cwd, text)
	_, err := LoadConfig(cwd)
	if err == nil {
		t.Fatal("want an error")
	}
	return err.Error()
}

func TestLoadConfigFillsDefaults(t *testing.T) {
	cwd := t.TempDir()
	writeConfigRaw(t, cwd, `{"version":1}`)
	config, err := LoadConfig(cwd)
	if err != nil {
		t.Fatal(err)
	}
	if config.OutDir != "infra" || len(config.Targets) != 1 || config.Targets[0] != "hcl" {
		t.Errorf("config = %+v", config)
	}
}

func TestLoadConfigRejectsAFutureVersion(t *testing.T) {
	got := configError(t, `{"version":2,"targets":["hcl"],"outDir":"infra"}`)
	want := "togen/togen.json is version 2 but this Togen only understands up to 1"
	if got != want {
		t.Errorf("error = %q, want %q", got, want)
	}
}

func TestLoadConfigRejectsEmptyTargets(t *testing.T) {
	got := configError(t, `{"version":1,"targets":[],"outDir":"infra"}`)
	want := "togen/togen.json has no targets"
	if got != want {
		t.Errorf("error = %q, want %q", got, want)
	}
}

func TestLoadConfigReportsATypeMismatchInOurOwnWords(t *testing.T) {
	for _, c := range []struct{ text, want string }{
		{`{"version":1,"targets":"hcl"}`, "togen/togen.json: targets must be a array"},
		{`{"version":1,"outDir":7}`, "togen/togen.json: outDir must be a string"},
		{`{"version":"one"}`, "togen/togen.json: version must be a number"},
	} {
		if got := configError(t, c.text); got != c.want {
			t.Errorf("error = %q, want %q", got, c.want)
		}
	}
}
