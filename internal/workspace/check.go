package workspace

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/mooncitizen/togen/internal/cost"
	"github.com/mooncitizen/togen/internal/ir"
)

// The schema cannot know the project, so what is keyed by node name is checked here: a style
// needs its node, and a usage entry needs its node and only the keys of that node's type.
func CheckConfig(config Config, project *ir.Project) ir.Errors {
	types := make(map[string]ir.NodeType, len(project.Nodes))
	for _, n := range project.Nodes {
		types[n.Name] = n.Type
	}
	var errs ir.Errors
	if config.Style != nil {
		for _, name := range slices.Sorted(maps.Keys(config.Style.Nodes)) {
			if _, ok := types[name]; !ok {
				errs = append(errs, ir.ValidationError{
					Path:    "style.nodes." + name,
					Message: fmt.Sprintf("there is no node named '%s'", name),
				})
			}
		}
	}
	for _, name := range slices.Sorted(maps.Keys(config.Usage)) {
		errs = append(errs, checkUsage(name, config.Usage[name], types)...)
	}
	return errs
}

func checkUsage(name string, u cost.NodeUsage, types map[string]ir.NodeType) ir.Errors {
	t, isNode := types[name]
	if !isNode && name != cost.NetworkUsage {
		return ir.Errors{{Path: "usage." + name, Message: fmt.Sprintf("there is no node named '%s'", name)}}
	}
	var allowed []string
	if isNode {
		allowed = cost.UsageKeys[t]
	}
	if name == cost.NetworkUsage {
		allowed = append(allowed, cost.NetworkKeys...)
	}
	var errs ir.Errors
	for _, key := range u.Keys() {
		if slices.Contains(allowed, key) {
			continue
		}
		errs = append(errs, ir.ValidationError{Path: "usage." + name + "." + key, Message: usageKeyMessage(name, t, isNode, allowed)})
	}
	for _, key := range []string{"requests", "invocations", "messages"} {
		if _, err := u.Rates()[key].PerMonth(); err != nil {
			errs = append(errs, ir.ValidationError{Path: "usage." + name + "." + key, Message: cost.RateHelp})
		}
	}
	return errs
}

func usageKeyMessage(name string, t ir.NodeType, isNode bool, allowed []string) string {
	what := "the network"
	if isNode {
		what = "a " + string(t)
	}
	if len(allowed) == 0 {
		return fmt.Sprintf("%s takes no usage keys", what)
	}
	return fmt.Sprintf("%s takes %s only", what, strings.Join(allowed, ", "))
}
