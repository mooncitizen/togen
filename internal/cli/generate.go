package cli

import (
	"fmt"

	"github.com/mooncitizen/togen/internal/workspace"
)

func Generate(cwd, target, out string, force bool) Result {
	generated, note, err := workspace.Generate(cwd, target, out, force)
	if err != nil {
		return noted(failure(err), note)
	}
	var lines []string
	for _, g := range generated {
		lines = append(lines, fmt.Sprintf("wrote %d files to %s", len(g.Files), g.Dir))
		for _, name := range g.Files {
			lines = append(lines, "  "+name)
		}
	}
	return Result{Code: 0, Lines: lines, Note: note}
}
