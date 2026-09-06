package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/mooncitizen/togen/internal/ir"
)

const DefaultEnvironment = "dev"

type Sketch struct {
	Name        string `json:"name"`
	Provider    string `json:"provider"`
	Region      string `json:"region"`
	Environment string `json:"environment"`
}

type ExistsError struct{ Path, Dir string }

func (e *ExistsError) Error() string { return fmt.Sprintf("%s already exists in %s", e.Path, e.Dir) }

var (
	kebab           = regexp.MustCompile(ir.KebabPattern)
	notNameChar     = regexp.MustCompile(`[^a-z0-9]+`)
	leadingNonAlpha = regexp.MustCompile(`^[^a-z]+`)
	trailingDashes  = regexp.MustCompile(`-+$`)
)

// An empty field takes the default togen init uses: the directory's name, aws,
// the provider's first region and dev.
func SketchFiles(cwd string, sketch Sketch) (map[string][]byte, error) {
	if sketch.Provider == "" {
		sketch.Provider = string(ir.ProviderAWS)
	}
	provider := ir.CloudProvider(sketch.Provider)
	if !slices.Contains(ir.Providers, provider) {
		return nil, ir.Errors{{Path: "provider", Message: fmt.Sprintf("unknown provider '%s', use one of %s", sketch.Provider, providerList())}}
	}
	defaultRegion, _ := ir.DefaultRegion(provider)
	if sketch.Region == "" {
		sketch.Region = defaultRegion
	}
	if sketch.Environment == "" {
		sketch.Environment = DefaultEnvironment
	}
	if sketch.Name == "" {
		sketch.Name = defaultName(cwd)
	}
	project := ir.Project{
		Version:     ir.Version,
		Name:        sketch.Name,
		Provider:    provider,
		Region:      sketch.Region,
		Environment: sketch.Environment,
		Nodes:       []ir.Node{},
		Edges:       []ir.Edge{},
	}
	raw, err := json.Marshal(project)
	if err != nil {
		return nil, err
	}
	_, errs := ir.ValidateProject(raw)
	// The schema only asks for a non-empty region; a form can send anything.
	if sketch.Region != "" && !kebab.MatchString(sketch.Region) {
		errs = append(errs, ir.ValidationError{Path: "region", Message: fmt.Sprintf("'%s' is not a region id such as %s", sketch.Region, defaultRegion)})
	}
	if len(errs) > 0 {
		return nil, errs
	}

	projectRaw, err := Marshal(project)
	if err != nil {
		return nil, err
	}
	layoutRaw, err := Marshal(DefaultLayout())
	if err != nil {
		return nil, err
	}
	defaults := DefaultConfig()
	return map[string][]byte{
		ProjectFiles[0]: projectRaw,
		ProjectFiles[1]: layoutRaw,
		ConfigName:      ConfigFile(defaults.Targets, defaults.OutDir),
	}, nil
}

// Refuses to touch an existing togen/, and leaves an existing togen.yml as it
// is: that file is hand-written (ADR 0007). The names actually written come back.
func CreateProject(cwd string, files map[string][]byte) ([]string, error) {
	if Exists(TogenDir(cwd)) {
		return nil, &ExistsError{Path: "togen/", Dir: cwd}
	}
	for name := range files {
		if !slices.Contains(ProjectFiles, name) {
			return nil, fmt.Errorf("%s is not a project file", name)
		}
	}
	for _, name := range ProjectFiles {
		if _, ok := files[name]; !ok {
			return nil, fmt.Errorf("there is no %s to write", name)
		}
	}
	if err := os.MkdirAll(TogenDir(cwd), 0o755); err != nil {
		return nil, err
	}
	var written []string
	for _, name := range ProjectFiles {
		path := filepath.Join(cwd, filepath.FromSlash(name))
		if name == ConfigName && Exists(path) {
			continue
		}
		if err := WriteRaw(path, files[name]); err != nil {
			return written, err
		}
		written = append(written, name)
	}
	return written, nil
}

// The style and usage blocks are commented out: they are here to be read and
// uncommented, and the studio reads this file but never writes it (ADR 0007).
func ConfigFile(targets []string, outDir string) []byte {
	return fmt.Appendf(nil, `version: %d
targets: [%s]
outDir: %s

# style:
#   theme: dark            # dark, light or system
#   kinds:                 # every node of a type
#     database:
#       color: "#C925D1"
#       icon: aws/rds      # a bundled icon id, or a path relative to this file
#       shape: cylinder
#   nodes:                 # one node, by name
#     orders-db:
#       color: "#DD344C"

# usage:                   # what togen cost prices the pay-per-use nodes on, by name
#   api:                   # gateway: requests
#     requests: 500/min    # a rate is a number per min, hour, day or month, k and M allowed
#   orders:                # function: invocations and durationMs
#     invocations: 2M/month
#     durationMs: 300
#   jobs:                  # queue: messages
#     messages: 100k/day
#   uploads:               # bucket: storageGb, egressGb and requests
#     storageGb: 200
#     egressGb: 40
#     requests: 1M/month
#   web:                   # service: egressGb
#     egressGb: 100
#   network:               # the implicit network: natGb
#     natGb: 50
`, ir.Version, strings.Join(targets, ", "), outDir)
}

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
