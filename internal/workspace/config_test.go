package workspace

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/mooncitizen/togen/internal/cost"
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
		{"short hex", "style:\n  kinds:\n    database:\n      color: \"#C92\"\n", "style.kinds.database.color"},
		{"shape", "style:\n  nodes:\n    orders-db:\n      shape: blob\n", "style.nodes.orders-db.shape"},
		{"unknown icon id", "style:\n  kinds:\n    database:\n      icon: aws/dynamodb\n", "style.kinds.database.icon"},
		{"absolute icon path", "style:\n  nodes:\n    orders-db:\n      icon: /icons/orders.svg\n", "style.nodes.orders-db.icon"},
		{"icon path without a dot", "style:\n  nodes:\n    orders-db:\n      icon: icons/orders.svg\n", "style.nodes.orders-db.icon"},
		{"icon url", "style:\n  nodes:\n    orders-db:\n      icon: https://example.com/orders.svg\n", "style.nodes.orders-db.icon"},
		{"icon that is not a string", "style:\n  nodes:\n    orders-db:\n      icon: 7\n", "style.nodes.orders-db.icon"},
		{"unknown kind", "style:\n  kinds:\n    warehouse:\n      shape: card\n", "style.kinds"},
		{"unknown key", "style:\n  colour: red\n", "style"},
		{"unknown top level key", "provider: aws\n", ConfigName},
		{"empty targets", "targets: []\n", "targets"},
		{"usage key the catalogue has not got", "usage:\n  api:\n    calls: 500/min\n", "usage.api"},
		{"usage node name that is not kebab", "usage:\n  Api:\n    requests: 500/min\n", "usage"},
		{"rate without a period", "usage:\n  api:\n    requests: 500\n", "usage.api.requests"},
		{"rate with a period the grammar lacks", "usage:\n  jobs:\n    messages: 5/week\n", "usage.jobs.messages"},
		{"negative storage", "usage:\n  uploads:\n    storageGb: -1\n", "usage.uploads.storageGb"},
		{"duration that is not a number", "usage:\n  orders:\n    durationMs: fast\n", "usage.orders.durationMs"},
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

func TestLoadConfigSaysWhatARateMayBe(t *testing.T) {
	lines := configErrors(t, "usage:\n  orders:\n    invocations: 2000000\n")
	want := []string{"usage.orders.invocations: " + cost.RateHelp}
	if diff := cmp.Diff(want, lines); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
}

func TestLoadConfigReadsAUsageBlock(t *testing.T) {
	cwd := t.TempDir()
	writeConfig(t, cwd, `
usage:
  api:
    requests: 500/min
  orders:
    invocations: 2M/month
    durationMs: 300
  jobs:
    messages: 100k/day
  uploads:
    storageGb: 200
    egressGb: 40
    requests: 1M/month
  web:
    egressGb: 100
  network:
    natGb: 50
`)
	want := cost.Usage{
		"api":     {Requests: "500/min"},
		"orders":  {Invocations: "2M/month", DurationMs: 300},
		"jobs":    {Messages: "100k/day"},
		"uploads": {StorageGb: 200, EgressGb: 40, Requests: "1M/month"},
		"web":     {EgressGb: 100},
		"network": {NatGb: 50},
	}
	if diff := cmp.Diff(want, loadConfig(t, cwd).Usage); diff != "" {
		t.Errorf("usage (-want +got):\n%s", diff)
	}
}

func TestLoadConfigSaysWhatAnIconMayBe(t *testing.T) {
	lines := configErrors(t, "style:\n  kinds:\n    database:\n      icon: aws/dynamodb\n")
	want := []string{"style.kinds.database.icon: " + iconMessage}
	if diff := cmp.Diff(want, lines); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
}

func TestLoadConfigAcceptsBundledIconsAndRelativePaths(t *testing.T) {
	cwd := t.TempDir()
	writeConfig(t, cwd, `
style:
  kinds:
    database:
      icon: gcp/cloud-sql
    cache:
      icon: ./icons/cache.svg
  nodes:
    orders-db:
      icon: ../shared/icons/orders.svg
`)
	want := &Style{
		Theme: DefaultTheme,
		Kinds: map[ir.NodeType]NodeStyle{
			ir.NodeDatabase: {Icon: "gcp/cloud-sql"},
			ir.NodeCache:    {Icon: "./icons/cache.svg"},
		},
		Nodes: map[string]NodeStyle{"orders-db": {Icon: "../shared/icons/orders.svg"}},
	}
	if diff := cmp.Diff(want, loadConfig(t, cwd).Style); diff != "" {
		t.Errorf("style (-want +got):\n%s", diff)
	}
}

func TestLoadConfigRejectsAnUnknownShape(t *testing.T) {
	lines := configErrors(t, "style:\n  kinds:\n    cache:\n      shape: blob\n")
	want := []string{"style.kinds.cache.shape: value must be one of 'card', 'cylinder', 'hexagon', 'circle'"}
	if diff := cmp.Diff(want, lines); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
}

func styledProject(names ...string) *ir.Project {
	project := &ir.Project{Provider: ir.ProviderAWS}
	for i, name := range names {
		project.Nodes = append(project.Nodes, ir.Node{ID: fmt.Sprintf("n%d", i), Type: ir.NodeService, Name: name})
	}
	return project
}

func typedProject(nodes map[string]ir.NodeType) *ir.Project {
	project := &ir.Project{Provider: ir.ProviderAWS}
	for _, name := range slices.Sorted(maps.Keys(nodes)) {
		project.Nodes = append(project.Nodes, ir.Node{ID: name, Type: nodes[name], Name: name})
	}
	return project
}

func TestCheckConfigPassesEveryUsageKeyOnItsOwnType(t *testing.T) {
	config := DefaultConfig()
	config.Usage = cost.Usage{
		"api":     {Requests: "500/min"},
		"orders":  {Invocations: "2M/month", DurationMs: 300},
		"jobs":    {Messages: "100k/day"},
		"uploads": {StorageGb: 200, EgressGb: 40, Requests: "1M/month"},
		"web":     {EgressGb: 100},
		"network": {NatGb: 50},
	}
	project := typedProject(map[string]ir.NodeType{
		"api": ir.NodeGateway, "orders": ir.NodeFunction, "jobs": ir.NodeQueue, "uploads": ir.NodeBucket, "web": ir.NodeService,
	})
	if errs := CheckConfig(config, project); len(errs) > 0 {
		t.Errorf("errors = %v", errs)
	}
}

func TestCheckConfigRefusesAUsageKeyOnTheWrongType(t *testing.T) {
	config := DefaultConfig()
	config.Usage = cost.Usage{
		"api":       {Invocations: "2M/month", Requests: "500/min"},
		"orders":    {Requests: "500/min", DurationMs: 300},
		"jobs":      {StorageGb: 5, EgressGb: 1},
		"uploads":   {Messages: "1/min"},
		"web":       {NatGb: 5},
		"orders-db": {StorageGb: 50},
		"sessions":  {Requests: "1/min"},
		"network":   {Requests: "1/min"},
	}
	project := typedProject(map[string]ir.NodeType{
		"api": ir.NodeGateway, "orders": ir.NodeFunction, "jobs": ir.NodeQueue, "uploads": ir.NodeBucket, "web": ir.NodeService,
		"orders-db": ir.NodeDatabase, "sessions": ir.NodeCache,
	})
	want := ir.Errors{
		{Path: "usage.api.invocations", Message: "a gateway takes requests only"},
		{Path: "usage.jobs.storageGb", Message: "a queue takes messages only"},
		{Path: "usage.jobs.egressGb", Message: "a queue takes messages only"},
		{Path: "usage.network.requests", Message: "the network takes natGb only"},
		{Path: "usage.orders.requests", Message: "a function takes invocations, durationMs only"},
		{Path: "usage.orders-db.storageGb", Message: "a database takes no usage keys"},
		{Path: "usage.sessions.requests", Message: "a cache takes no usage keys"},
		{Path: "usage.uploads.messages", Message: "a bucket takes storageGb, egressGb, requests only"},
		{Path: "usage.web.natGb", Message: "a service takes egressGb, requests only"},
	}
	if diff := cmp.Diff(want, CheckConfig(config, project)); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
}

func TestCheckConfigNamesTheUsedNodesThatDoNotExist(t *testing.T) {
	config := DefaultConfig()
	config.Usage = cost.Usage{"api": {Requests: "500/min"}, "archive": {StorageGb: 5}, "network": {NatGb: 5}}
	want := ir.Errors{{Path: "usage.archive", Message: "there is no node named 'archive'"}}
	if diff := cmp.Diff(want, CheckConfig(config, typedProject(map[string]ir.NodeType{"api": ir.NodeGateway}))); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
}

// A node called network shares the entry with the implicit network, so both key sets pass.
func TestCheckConfigLetsANodeCalledNetworkKeepTheNetworkKeys(t *testing.T) {
	config := DefaultConfig()
	config.Usage = cost.Usage{"network": {Requests: "500/min", NatGb: 5}}
	if errs := CheckConfig(config, typedProject(map[string]ir.NodeType{"network": ir.NodeGateway})); len(errs) > 0 {
		t.Errorf("errors = %v", errs)
	}
}

// The legacy JSON file is read without the schema, so the rate grammar is checked here too.
func TestCheckConfigRefusesARateTheGrammarCannotRead(t *testing.T) {
	config := DefaultConfig()
	config.Usage = cost.Usage{"api": {Requests: "lots"}}
	want := ir.Errors{{Path: "usage.api.requests", Message: cost.RateHelp}}
	if diff := cmp.Diff(want, CheckConfig(config, typedProject(map[string]ir.NodeType{"api": ir.NodeGateway}))); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
}

func TestCheckConfigNamesTheStyledNodesThatDoNotExist(t *testing.T) {
	config := DefaultConfig()
	config.Style.Nodes = map[string]NodeStyle{
		"web":       {Color: "#DD344C"},
		"orders-db": {Color: "#DD344C"},
		"archive":   {Shape: "cylinder"},
	}
	want := ir.Errors{
		{Path: "style.nodes.archive", Message: "there is no node named 'archive'"},
		{Path: "style.nodes.orders-db", Message: "there is no node named 'orders-db'"},
	}
	if diff := cmp.Diff(want, CheckConfig(config, styledProject("web", "api"))); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
}

func TestCheckConfigPassesKindsAndKnownNodes(t *testing.T) {
	config := DefaultConfig()
	config.Style.Kinds = map[ir.NodeType]NodeStyle{ir.NodeDatabase: {Shape: "cylinder"}}
	config.Style.Nodes = map[string]NodeStyle{"web": {Color: "#DD344C"}}
	if errs := CheckConfig(config, styledProject("web")); len(errs) > 0 {
		t.Errorf("errors = %v", errs)
	}
	if errs := CheckConfig(Config{}, styledProject()); len(errs) > 0 {
		t.Errorf("errors without a style block = %v", errs)
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
