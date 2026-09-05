package workspace

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mooncitizen/togen/internal/emit/hcl"
	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

var emitters = map[string]func(*ir.Graph) (map[string][]byte, error){"hcl": hcl.Emit}

type Generated struct {
	Dir   string   `json:"dir"`
	Files []string `json:"files"`
}

type StrangerError struct {
	Dir       string
	Strangers []string
}

func (e *StrangerError) Error() string {
	return fmt.Sprintf("%s contains files Togen did not write: %s\nMove them, or run again with --force to replace the whole directory.",
		e.Dir, strings.Join(e.Strangers, ", "))
}

// The second return is the deprecation note from the configuration, empty when there is none.
func Generate(cwd, target, out string, force bool) ([]Generated, string, error) {
	config, note, err := LoadConfig(cwd)
	if err != nil {
		return nil, note, err
	}
	project, errs, err := LoadProject(cwd)
	if err != nil {
		return nil, note, err
	}
	if len(errs) > 0 {
		return nil, note, errs
	}

	newProvider, ok := resolvers[project.Provider]
	if !ok {
		return nil, note, &Error{fmt.Sprintf("provider '%s' is not supported yet", project.Provider)}
	}
	targets := config.Targets
	if target != "" {
		targets = []string{target}
	}
	for _, t := range targets {
		if _, ok := emitters[t]; !ok {
			return nil, note, &Error{fmt.Sprintf("target '%s' is not supported yet", t)}
		}
	}

	graph, err := resolve.Run(project, newProvider())
	if err != nil {
		var resolveErr *resolve.ResolveError
		if errors.As(err, &resolveErr) {
			return nil, note, resolveErr.Errors
		}
		return nil, note, err
	}

	outDir := config.OutDir
	if out != "" {
		outDir = out
	}
	var written []Generated
	for _, t := range targets {
		files, err := emitters[t](graph)
		if err != nil {
			return nil, note, err
		}
		dir := filepath.Join(cwd, outDir, t)
		shown := shownPath(cwd, dir)
		if !force {
			strangers, err := checkOutDir(dir)
			if err != nil {
				return nil, note, err
			}
			if len(strangers) > 0 {
				return nil, note, &StrangerError{Dir: shown, Strangers: strangers}
			}
		}
		if err := writeOutputs(dir, files); err != nil {
			return nil, note, err
		}
		written = append(written, Generated{Dir: shown, Files: sortedKeys(files)})
	}
	return written, note, nil
}
