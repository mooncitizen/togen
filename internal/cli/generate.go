package cli

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mooncitizen/togen/internal/emit/hcl"
	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
	"github.com/mooncitizen/togen/internal/resolve/aws"
)

var resolvers = map[ir.CloudProvider]func() resolve.Provider{ir.ProviderAWS: aws.New}

var emitters = map[string]func(*ir.Graph) (map[string][]byte, error){"hcl": hcl.Emit}

func Generate(cwd, target, out string, force bool) Result {
	result, err := generate(cwd, target, out, force)
	if err == nil {
		return result
	}
	var cliErr *CliError
	if errors.As(err, &cliErr) {
		return Result{Code: 1, Lines: []string{cliErr.Message}}
	}
	var resolveErr *resolve.ResolveError
	if errors.As(err, &resolveErr) {
		return Result{Code: 1, Lines: errorLines(resolveErr.Errors)}
	}
	return Result{Code: 1, Lines: []string{err.Error()}}
}

func generate(cwd, target, out string, force bool) (Result, error) {
	config, err := LoadConfig(cwd)
	if err != nil {
		return Result{}, err
	}
	project, errs, err := LoadProject(cwd)
	if err != nil {
		return Result{}, err
	}
	if len(errs) > 0 {
		return Result{Code: 1, Lines: errorLines(errs)}, nil
	}

	newProvider, ok := resolvers[project.Provider]
	if !ok {
		return Result{}, &CliError{fmt.Sprintf("provider '%s' is not supported yet", project.Provider)}
	}
	targets := config.Targets
	if target != "" {
		targets = []string{target}
	}
	for _, t := range targets {
		if _, ok := emitters[t]; !ok {
			return Result{}, &CliError{fmt.Sprintf("target '%s' is not supported yet", t)}
		}
	}

	graph, err := resolve.Run(project, newProvider())
	if err != nil {
		return Result{}, err
	}

	outDir := config.OutDir
	if out != "" {
		outDir = out
	}
	var lines []string
	for _, t := range targets {
		files, err := emitters[t](graph)
		if err != nil {
			return Result{}, err
		}
		dir := filepath.Join(cwd, outDir, t)
		shown := shownPath(cwd, dir)
		if !force {
			strangers, err := checkOutDir(dir)
			if err != nil {
				return Result{}, err
			}
			if len(strangers) > 0 {
				return Result{Code: 1, Lines: []string{
					fmt.Sprintf("%s contains files Togen did not write: %s", shown, strings.Join(strangers, ", ")),
					"Move them, or run again with --force to replace the whole directory.",
				}}, nil
			}
		}
		if err := writeOutputs(dir, files); err != nil {
			return Result{}, err
		}
		names := sortedKeys(files)
		lines = append(lines, fmt.Sprintf("wrote %d files to %s", len(names), shown))
		for _, name := range names {
			lines = append(lines, "  "+name)
		}
	}
	return Result{Code: 0, Lines: lines}, nil
}
