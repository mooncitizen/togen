package workspace

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/mooncitizen/togen/internal/simulate"
)

const (
	simulationSchemaURL = "https://togen.dev/schema/simulation.schema.json"
	simulationShown     = "togen/simulation.json"
)

// Written by just generate: go:embed cannot reach schema/ from here.
//
//go:embed simulation.schema.json
var simulationSchemaJSON []byte

var simulationSchema = &compiledSchema{url: simulationSchemaURL, raw: simulationSchemaJSON}

// A project with no simulation file has no simulation, and everything behaves as it did
// before one existed (ADR 0011).
func LoadSimulation(cwd string) (simulate.Simulation, error) {
	path := SimulationPath(cwd)
	if !Exists(path) {
		return simulate.Empty(), nil
	}
	raw, err := ReadJSONFile(path, cwd)
	if err != nil {
		return simulate.Simulation{}, err
	}
	return ParseSimulation(raw)
}

func ParseSimulation(raw []byte) (simulate.Simulation, error) {
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return simulate.Simulation{}, &Error{simulationShown + " is not valid JSON"}
	}
	if errs := simulationSchema.check(instance, simulationShown); len(errs) > 0 {
		return simulate.Simulation{}, errs
	}
	var sim simulate.Simulation
	if err := json.Unmarshal(raw, &sim); err != nil {
		return simulate.Simulation{}, &Error{fmt.Sprintf("%s: %s", simulationShown, err)}
	}
	if sim.Version > simulate.Version {
		return simulate.Simulation{}, &Error{versionMessage(simulationShown, sim.Version, simulate.Version)}
	}
	return sim, nil
}

func WriteSimulation(cwd string, sim simulate.Simulation) error {
	return WriteJSONFile(SimulationPath(cwd), sim)
}
