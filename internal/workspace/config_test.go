package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/mooncitizen/togen/internal/ir"
)

func writeConfig(t *testing.T, cwd, text string) {
	t.Helper()
	if err := os.WriteFile(ConfigPath(cwd), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeLegacyConfig(t *testing.T, cwd, text string) {
	t.Helper()
	if err := os.MkdirAll(TogenDir(cwd), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(LegacyConfigPath(cwd), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func loadConfig(t *testing.T, cwd string) Config {
	t.Helper()
	config, _, err := LoadConfig(cwd)
	if err != nil {
		t.Fatal(err)
	}
	return config
}

func configErrors(t *testing.T, text string) []string {
	t.Helper()
	cwd := t.TempDir()
	writeConfig(t, cwd, text)
	_, _, err := LoadConfig(cwd)
	if err == nil {
		t.Fatal("want an error")
	}
	var errs ir.Errors
	if !errors.As(err, &errs) {
		return []string{err.Error()}
	}
	lines := make([]string, len(errs))
	for i, e := range errs {
		lines[i] = e.Path + ": " + e.Message
	}
	return lines
}

func TestLoadConfigReadsAStyleBlock(t *testing.T) {
	cwd := t.TempDir()
	writeConfig(t, cwd, `
version: 1
targets: [hcl]
outDir: build

style:
  theme: light
  kinds:
    database:
      color: "#C925D1"
      icon: aws/rds
      shape: cylinder
  nodes:
    orders-db:
      color: "#DD344C"
`)
	want := Config{
		Version: 1,
		Targets: []string{"hcl"},
		OutDir:  "build",
		Style: &Style{
			Theme: "light",
			Kinds: map[ir.NodeType]NodeStyle{ir.NodeDatabase: {Color: "#C925D1", Icon: "aws/rds", Shape: "cylinder"}},
			Nodes: map[string]NodeStyle{"orders-db": {Color: "#DD344C"}},
		},
	}
	if diff := cmp.Diff(want, loadConfig(t, cwd)); diff != "" {
		t.Errorf("config (-want +got):\n%s", diff)
	}
}

func TestLoadConfigDefaultsEverythingWhenTheFileIsMissing(t *testing.T) {
	if diff := cmp.Diff(DefaultConfig(), loadConfig(t, t.TempDir())); diff != "" {
		t.Errorf("config (-want +got):\n%s", diff)
	}
}

func TestLoadConfigReplacesTheDefaultTargets(t *testing.T) {
	cwd := t.TempDir()
	writeConfig(t, cwd, "targets: [pulumi]\n")
	if got := loadConfig(t, cwd).Targets; len(got) != 1 || got[0] != "pulumi" {
		t.Errorf("targets = %v", got)
	}
}

func TestLoadConfigDefaultsTheThemeWhenTheStyleBlockOmitsIt(t *testing.T) {
	cwd := t.TempDir()
	writeConfig(t, cwd, "style:\n  kinds:\n    cache:\n      shape: circle\n")
	config := loadConfig(t, cwd)
	if config.Style.Theme != DefaultTheme {
		t.Errorf("theme = %q, want %q", config.Style.Theme, DefaultTheme)
	}
	if config.OutDir != "infra" || len(config.Targets) != 1 || config.Targets[0] != "hcl" {
		t.Errorf("config = %+v", config)
	}
}

func TestLoadConfigFallsBackToTheLegacyFile(t *testing.T) {
	cwd := t.TempDir()
	writeLegacyConfig(t, cwd, `{"version":1,"targets":["hcl"],"outDir":"build"}`)
	config, note, err := LoadConfig(cwd)
	if err != nil {
		t.Fatal(err)
	}
	if config.OutDir != "build" {
		t.Errorf("outDir = %q, want build", config.OutDir)
	}
	if note != LegacyNote {
		t.Errorf("note = %q, want %q", note, LegacyNote)
	}
}

func TestLoadConfigPrefersTheYAMLFileAndSaysNothingAboutTheLegacyOne(t *testing.T) {
	cwd := t.TempDir()
	writeLegacyConfig(t, cwd, `{"version":1,"targets":["hcl"],"outDir":"legacy"}`)
	writeConfig(t, cwd, "outDir: current\n")
	config, note, err := LoadConfig(cwd)
	if err != nil {
		t.Fatal(err)
	}
	if config.OutDir != "current" {
		t.Errorf("outDir = %q, want current", config.OutDir)
	}
	if note != "" {
		t.Errorf("note = %q, want none", note)
	}
}

func TestLoadConfigReportsStyleErrorsWithTheirPaths(t *testing.T) {
	for _, c := range []struct{ name, text, want string }{
		{"theme", "style:\n  theme: neon\n", "style.theme"},
		{"colour", "style:\n  kinds:\n    database:\n      color: purple\n", "style.kinds.database.color"},
		{"shape", "style:\n  nodes:\n    orders-db:\n      shape: blob\n", "style.nodes.orders-db.shape"},
		{"unknown kind", "style:\n  kinds:\n    warehouse:\n      shape: card\n", "style.kinds"},
		{"unknown key", "style:\n  colour: red\n", "style"},
		{"unknown top level key", "provider: aws\n", ConfigName},
		{"empty targets", "targets: []\n", "targets"},
	} {
		t.Run(c.name, func(t *testing.T) {
			lines := configErrors(t, c.text)
			if len(lines) != 1 {
				t.Fatalf("errors = %v, want one", lines)
			}
			if got := lines[0]; !strings.HasPrefix(got, c.want+": ") {
				t.Errorf("error = %q, want the path %q", got, c.want)
			}
		})
	}
}

func TestLoadConfigRejectsAFutureVersion(t *testing.T) {
	for _, c := range []struct{ name, text, want string }{
		{"yaml", "version: 2\n", "togen.yml is version 2 but this Togen only understands up to 1"},
		{"legacy", "", "togen/togen.json is version 2 but this Togen only understands up to 1"},
	} {
		t.Run(c.name, func(t *testing.T) {
			cwd := t.TempDir()
			if c.text != "" {
				writeConfig(t, cwd, c.text)
			} else {
				writeLegacyConfig(t, cwd, `{"version":2}`)
			}
			_, _, err := LoadConfig(cwd)
			if err == nil || err.Error() != c.want {
				t.Errorf("error = %v, want %q", err, c.want)
			}
		})
	}
}

func TestLoadConfigReportsBrokenYAML(t *testing.T) {
	lines := configErrors(t, "targets: [hcl\n")
	if len(lines) != 1 || !strings.HasPrefix(lines[0], "togen.yml: not valid YAML") {
		t.Errorf("errors = %v", lines)
	}
}

func TestLoadConfigRejectsADocumentThatIsNotAMapping(t *testing.T) {
	cwd := t.TempDir()
	writeConfig(t, cwd, "- hcl\n")
	_, _, err := LoadConfig(cwd)
	if err == nil || err.Error() != "togen.yml must be a mapping of settings" {
		t.Errorf("error = %v", err)
	}
}

func TestLoadConfigTreatsAnEmptyFileAsDefaults(t *testing.T) {
	cwd := t.TempDir()
	writeConfig(t, cwd, "\n")
	if diff := cmp.Diff(DefaultConfig(), loadConfig(t, cwd)); diff != "" {
		t.Errorf("config (-want +got):\n%s", diff)
	}
}

func TestLoadLegacyConfigKeepsItsOwnMessages(t *testing.T) {
	for _, c := range []struct{ text, want string }{
		{`{"version":1,"targets":"hcl"}`, "togen/togen.json: targets must be a array"},
		{`{"version":1,"outDir":7}`, "togen/togen.json: outDir must be a string"},
		{`{"version":"one"}`, "togen/togen.json: version must be a number"},
		{`{"version":1,"targets":[]}`, "togen/togen.json has no targets"},
		{"{ not json", "togen/togen.json is not valid JSON"},
	} {
		cwd := t.TempDir()
		writeLegacyConfig(t, cwd, c.text)
		_, _, err := LoadConfig(cwd)
		if err == nil || err.Error() != c.want {
			t.Errorf("error = %v, want %q", err, c.want)
		}
	}
}

func TestConfigPathIsAtTheRoot(t *testing.T) {
	cwd := t.TempDir()
	if got, want := ConfigPath(cwd), filepath.Join(cwd, "togen.yml"); got != want {
		t.Errorf("ConfigPath = %q, want %q", got, want)
	}
	if got, want := LegacyConfigPath(cwd), filepath.Join(cwd, "togen", "togen.json"); got != want {
		t.Errorf("LegacyConfigPath = %q, want %q", got, want)
	}
}
