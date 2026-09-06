package workspace

import (
	"fmt"
	"maps"
	"slices"

	"github.com/mooncitizen/togen/internal/ir"
)

// The schema cannot know the project, so a style keyed by node name is checked here.
func CheckStyle(config Config, project *ir.Project) ir.Errors {
	if config.Style == nil {
		return nil
	}
	names := make(map[string]bool, len(project.Nodes))
	for _, n := range project.Nodes {
		names[n.Name] = true
	}
	var errs ir.Errors
	for _, name := range slices.Sorted(maps.Keys(config.Style.Nodes)) {
		if !names[name] {
			errs = append(errs, ir.ValidationError{
				Path:    "style.nodes." + name,
				Message: fmt.Sprintf("there is no node named '%s'", name),
			})
		}
	}
	return errs
}
