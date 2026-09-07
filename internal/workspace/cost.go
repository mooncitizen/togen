package workspace

import (
	"time"

	"github.com/mooncitizen/togen/internal/cost"
	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve/aws"
	"github.com/mooncitizen/togen/internal/resolve/azure"
	"github.com/mooncitizen/togen/internal/resolve/gcp"
	"github.com/mooncitizen/togen/internal/simulate"
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
	usage, simulated, err := usageFor(cwd, project, config)
	if err != nil {
		return cost.Document{}, note, err
	}
	doc, err := cost.Estimate(project, matchers, snapshot, usage, time.Now())
	doc.Simulated = simulated
	return doc, note, err
}

// An absent simulation is not a broken estimate: the hand-written block stands on its own, as it
// did before ADR 0011. A simulation that is present but broken refuses, naming the file, rather
// than pricing a total the user's file does not describe.
func usageFor(cwd string, project *ir.Project, config Config) (cost.Usage, bool, error) {
	sim, err := LoadSimulation(cwd)
	if err != nil {
		return nil, false, err
	}
	if sim.IsEmpty() {
		return config.Usage, false, nil
	}
	monthly, result, err := simulateSweep(sim, project)
	if err != nil {
		return nil, false, err
	}
	return cost.Merge(simulate.Usage(project, sim, monthly, result), config.Usage), true, nil
}

func Simulate(cwd string) (simulate.Document, error) {
	project, errs, err := Validate(cwd)
	if err != nil {
		return simulate.Document{}, err
	}
	if len(errs) > 0 {
		return simulate.Document{}, errs
	}
	sim, err := LoadSimulation(cwd)
	if err != nil {
		return simulate.Document{}, err
	}
	monthly, result, err := simulateSweep(sim, project)
	if err != nil {
		return simulate.Document{}, err
	}
	return simulate.Describe(project, sim, monthly, result), nil
}

func simulateSweep(sim simulate.Simulation, project *ir.Project) (map[string]float64, simulate.Result, error) {
	if errs := simulate.Validate(sim, project); len(errs) > 0 {
		return nil, simulate.Result{}, errs
	}
	monthly, err := simulate.Monthly(sim)
	if err != nil {
		return nil, simulate.Result{}, err
	}
	result, err := simulate.Run(project, sim, monthly)
	if err != nil {
		return nil, simulate.Result{}, err
	}
	return monthly, result, nil
}
