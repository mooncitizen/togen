package workspace

import (
	"errors"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
	"github.com/mooncitizen/togen/internal/resolve/aws"
)

var resolvers = map[ir.CloudProvider]func() resolve.Provider{ir.ProviderAWS: aws.New}

func LoadProject(cwd string) (*ir.Project, ir.Errors, error) {
	raw, err := ReadJSONFile(ProjectPath(cwd), cwd)
	if err != nil {
		return nil, nil, err
	}
	return loadRaw(raw)
}

func Validate(cwd string) (*ir.Project, ir.Errors, error) {
	project, errs, err := LoadProject(cwd)
	if err != nil || len(errs) > 0 {
		return nil, errs, err
	}
	return resolves(project)
}

func ValidateRaw(raw []byte) (*ir.Project, ir.Errors, error) {
	project, errs, err := loadRaw(raw)
	if err != nil || len(errs) > 0 {
		return nil, errs, err
	}
	return resolves(project)
}

func loadRaw(raw []byte) (*ir.Project, ir.Errors, error) {
	migrated, err := ir.Migrate(raw)
	if err != nil {
		return nil, nil, &Error{err.Error()}
	}
	project, errs := ir.ValidateProject(migrated)
	if len(errs) > 0 {
		return nil, errs, nil
	}
	return project, nil, nil
}

// The resolver knows things the schema cannot, such as route key clashes and
// name limits, so a project only counts as valid once it resolves. A provider
// with no resolver yet gets the schema and semantic checks only.
func resolves(project *ir.Project) (*ir.Project, ir.Errors, error) {
	newProvider, ok := resolvers[project.Provider]
	if !ok {
		return project, nil, nil
	}
	if _, err := resolve.Run(project, newProvider()); err != nil {
		var resolveErr *resolve.ResolveError
		if errors.As(err, &resolveErr) {
			return nil, resolveErr.Errors, nil
		}
		return nil, nil, err
	}
	return project, nil, nil
}
