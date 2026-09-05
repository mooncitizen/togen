package cli

import (
	"encoding/json"

	"github.com/mooncitizen/togen/internal/workspace"
)

func Cost(cwd string, asJSON bool) Result {
	doc, note, err := workspace.Cost(cwd)
	if err != nil {
		return noted(failure(err), note)
	}
	if !asJSON {
		return Result{Code: 0, Lines: doc.Table(), Note: note}
	}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return noted(failure(err), note)
	}
	return Result{Code: 0, Lines: []string{string(raw)}, Note: note}
}
