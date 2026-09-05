package cli

import (
	"fmt"

	"github.com/mooncitizen/togen/internal/workspace"
)

func Validate(cwd string) Result {
	_, note, err := workspace.LoadConfig(cwd)
	if err != nil {
		return noted(failure(err), note)
	}
	project, errs, err := workspace.Validate(cwd)
	if err != nil {
		return noted(failure(err), note)
	}
	if len(errs) > 0 {
		return Result{Code: 1, Lines: errorLines(errs), Note: note}
	}
	return Result{Code: 0, Note: note, Lines: []string{fmt.Sprintf(
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
