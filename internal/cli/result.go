package cli

import "github.com/mooncitizen/togen/internal/ir"

type Result struct {
	Code  int
	Lines []string
}

func errorLines(errs ir.Errors) []string {
	lines := make([]string, len(errs))
	for i, e := range errs {
		lines[i] = e.String()
	}
	return lines
}
