package cli

import (
	"strings"

	"github.com/mooncitizen/togen/internal/ir"
)

type Result struct {
	Code  int
	Lines []string
	Note  string
}

func errorLines(errs ir.Errors) []string {
	lines := make([]string, len(errs))
	for i, e := range errs {
		lines[i] = e.String()
	}
	return lines
}

func failure(err error) Result {
	return Result{Code: 1, Lines: strings.Split(err.Error(), "\n")}
}

func noted(result Result, note string) Result {
	result.Note = note
	return result
}
