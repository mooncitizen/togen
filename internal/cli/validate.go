package cli

import "fmt"

func Validate(cwd string) Result {
	project, errs, err := LoadProject(cwd)
	if err != nil {
		return Result{Code: 1, Lines: []string{err.Error()}}
	}
	if len(errs) > 0 {
		return Result{Code: 1, Lines: errorLines(errs)}
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
