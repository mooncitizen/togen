package cli

import (
	"errors"
	"fmt"

	"github.com/mooncitizen/togen/internal/resolve"
)

func Validate(cwd string) Result {
	project, errs, err := LoadProject(cwd)
	if err != nil {
		return Result{Code: 1, Lines: []string{err.Error()}}
	}
	if len(errs) > 0 {
		return Result{Code: 1, Lines: errorLines(errs)}
	}
	// The resolver knows things the schema cannot, such as route key clashes
	// and name limits, so a project only counts as valid once it resolves.
	if newProvider, ok := resolvers[project.Provider]; ok {
		if _, err := resolve.Run(project, newProvider()); err != nil {
			var resolveErr *resolve.ResolveError
			if errors.As(err, &resolveErr) {
				return Result{Code: 1, Lines: errorLines(resolveErr.Errors)}
			}
			return Result{Code: 1, Lines: []string{err.Error()}}
		}
	}
	return Result{Code: 0, Lines: []string{fmt.Sprintf(
		"togen/project.json is valid (%s, %s)",
		plural(len(project.Nodes), "node"),
		plural(len(project.Edges), "edge"),
	)}}
}

func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}
