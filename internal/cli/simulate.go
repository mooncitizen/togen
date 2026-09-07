package cli

import (
	"encoding/json"

	"github.com/mooncitizen/togen/internal/workspace"
)

func Simulate(cwd string, asJSON bool) Result {
	doc, err := workspace.Simulate(cwd)
	if err != nil {
		return failure(err)
	}
	if !asJSON {
		return Result{Code: 0, Lines: doc.Table()}
	}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return failure(err)
	}
	return Result{Code: 0, Lines: []string{string(raw)}}
}
