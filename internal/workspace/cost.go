package workspace

import (
	"time"

	"github.com/mooncitizen/togen/internal/cost"
	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve/aws"
	"github.com/mooncitizen/togen/internal/resolve/azure"
	"github.com/mooncitizen/togen/internal/resolve/gcp"
)

var costMatchers = map[ir.CloudProvider]cost.Matchers{
	ir.ProviderAWS:   aws.Cost(),
	ir.ProviderAzure: azure.Cost(),
	ir.ProviderGCP:   gcp.Cost(),
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
	// Matchers without a snapshot to look in say no prices are bundled, the way no matchers do.
	matchers := costMatchers[project.Provider]
	if snapshot == nil {
		matchers = nil
	}
	doc, err := cost.Estimate(project, matchers, snapshot, config.Usage, time.Now())
	return doc, note, err
}
