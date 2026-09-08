package cli

import (
	"fmt"
	"strings"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/workspace"
)

func Generate(cwd, target, out string, force, strict bool) Result {
	written, note, err := workspace.Generate(cwd, target, out, force, strict)
	if err != nil {
		return noted(failure(err), note)
	}
	var lines []string
	for _, g := range written.Targets {
		lines = append(lines, fmt.Sprintf("wrote %d files to %s", len(g.Files), g.Dir))
		for _, name := range g.Files {
			lines = append(lines, "  "+name)
		}
	}
	if line := notGeneratedLine(written.NotGenerated); line != "" {
		lines = append(lines, line)
	}
	return Result{Code: 0, Lines: lines, Note: note}
}

// A draw-only node is named rather than skipped in silence, so a resource that never
// appeared in the output is never a surprise.
func notGeneratedLine(nodes []ir.NotGenerated) string {
	if len(nodes) == 0 {
		return ""
	}
	names := make([]string, len(nodes))
	for i, n := range nodes {
		names[i] = fmt.Sprintf("%s (%s)", n.Name, n.Type)
	}
	return "not generated (draws only): " + strings.Join(names, ", ")
}
