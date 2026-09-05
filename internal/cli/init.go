package cli

import (
	"os"
	"slices"
	"strings"

	"github.com/mooncitizen/togen/internal/workspace"
)

func Init(cwd, provider, name string, migrate bool) Result {
	if migrate {
		return migrateConfig(cwd)
	}
	files, err := workspace.SketchFiles(cwd, workspace.Sketch{Name: name, Provider: provider})
	if err != nil {
		return failure(err)
	}
	written, err := workspace.CreateProject(cwd, files)
	if err != nil {
		return failure(err)
	}
	lines := []string{"created " + listed(written)}
	if !slices.Contains(written, workspace.ConfigName) {
		lines = append(lines, workspace.ConfigName+" already exists and was left as it is")
	}
	return Result{Code: 0, Lines: lines}
}

func migrateConfig(cwd string) Result {
	if workspace.Exists(workspace.ConfigPath(cwd)) {
		return Result{Code: 1, Lines: []string{workspace.ConfigName + " already exists, so there is nothing to migrate"}}
	}
	if !workspace.Exists(workspace.LegacyConfigPath(cwd)) {
		return Result{Code: 1, Lines: []string{"there is no togen/togen.json to migrate"}}
	}
	config, _, err := workspace.LoadConfig(cwd)
	if err != nil {
		return failure(err)
	}
	if err := workspace.WriteRaw(workspace.ConfigPath(cwd), workspace.ConfigFile(config.Targets, config.OutDir)); err != nil {
		return Result{Code: 1, Lines: []string{err.Error()}}
	}
	if err := os.Remove(workspace.LegacyConfigPath(cwd)); err != nil {
		return Result{Code: 1, Lines: []string{err.Error()}}
	}
	return Result{Code: 0, Lines: []string{"moved togen/togen.json to " + workspace.ConfigName}}
}

func listed(names []string) string {
	if len(names) < 2 {
		return strings.Join(names, "")
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}
