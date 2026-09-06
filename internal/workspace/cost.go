package workspace

import (
	"time"

	"github.com/mooncitizen/togen/internal/cost"
	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve/aws"
	"github.com/mooncitizen/togen/internal/resolve/azure"
)

var costMatchers = map[ir.CloudProvider]cost.Matchers{
	ir.ProviderAWS:   aws.Cost(),
	ir.ProviderAzure: azure.Cost(),
}

// The second return is the deprecation note from the configuration, empty when there is none.
func Cost(cwd string) (cost.Document, string, error) {
	config, note, err := LoadConfig(cwd)
	if err != nil {
		return cost.Document{}, note, err
	}
	project, errs, err := Validate(cwd)
	if err != nil {
		return cost.Document{}, note, err
	}
	if len(errs) == 0 {
		errs = CheckConfig(config, project)
	}
	if len(errs) > 0 {
		return cost.Document{}, note, errs
	}
	snapshot, err := cost.Bundled(project.Provider)
	if err != nil {
		return cost.Document{}, note, err
	}
	doc, err := cost.Estimate(project, costMatchers[project.Provider], snapshot, config.Usage, time.Now())
	return doc, note, err
}
