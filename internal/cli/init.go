package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/workspace"
)

var regions = map[ir.CloudProvider]string{
	ir.ProviderAWS:   "eu-west-2",
	ir.ProviderGCP:   "europe-west2",
	ir.ProviderAzure: "uksouth",
}

type layout struct {
	Version  int            `json:"version"`
	Nodes    map[string]any `json:"nodes"`
	Viewport viewport       `json:"viewport"`
}

type viewport struct {
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	Zoom float64 `json:"zoom"`
}

func Init(cwd, provider, name string) Result {
	if provider == "" {
		provider = string(ir.ProviderAWS)
	}
	cloud := ir.CloudProvider(provider)
	if !slices.Contains(ir.Providers, cloud) {
		return Result{Code: 1, Lines: []string{
			fmt.Sprintf("unknown provider '%s'. Use one of %s.", provider, providerList()),
		}}
	}

	dir := workspace.TogenDir(cwd)
	if workspace.Exists(dir) {
		return Result{Code: 1, Lines: []string{fmt.Sprintf("togen/ already exists in %s", cwd)}}
	}

	if name == "" {
		name = defaultName(cwd)
	}
	project := ir.Project{
		Version:     ir.Version,
		Name:        name,
		Provider:    cloud,
		Region:      regions[cloud],
		Environment: "dev",
		Nodes:       []ir.Node{},
		Edges:       []ir.Edge{},
	}
	raw, err := json.Marshal(project)
	if err != nil {
		return Result{Code: 1, Lines: []string{err.Error()}}
	}
	if _, errs := ir.ValidateProject(raw); len(errs) > 0 {
		return Result{Code: 1, Lines: []string{errs[0].String()}}
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Result{Code: 1, Lines: []string{err.Error()}}
	}
	written := []struct {
		path  string
		value any
	}{
		{workspace.ProjectPath(cwd), project},
		{workspace.LayoutPath(cwd), layout{Version: 1, Nodes: map[string]any{}, Viewport: viewport{Zoom: 1}}},
		{workspace.ConfigPath(cwd), workspace.DefaultConfig()},
	}
	for _, w := range written {
		if err := workspace.WriteJSONFile(w.path, w.value); err != nil {
			return Result{Code: 1, Lines: []string{err.Error()}}
		}
	}
	return Result{Code: 0, Lines: []string{
		"created togen/project.json, togen/layout.json and togen/togen.json",
	}}
}

var (
	notNameChar     = regexp.MustCompile(`[^a-z0-9]+`)
	leadingNonAlpha = regexp.MustCompile(`^[^a-z]+`)
	trailingDashes  = regexp.MustCompile(`-+$`)
)

func defaultName(cwd string) string {
	raw := notNameChar.ReplaceAllString(strings.ToLower(filepath.Base(cwd)), "-")
	raw = leadingNonAlpha.ReplaceAllString(raw, "")
	if len(raw) > 32 {
		raw = raw[:32]
	}
	raw = trailingDashes.ReplaceAllString(raw, "")
	if raw == "" {
		return "project"
	}
	return raw
}

func providerList() string {
	names := make([]string, len(ir.Providers))
	for i, p := range ir.Providers {
		names[i] = string(p)
	}
	return strings.Join(names, ", ")
}
