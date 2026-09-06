package workspace

import "path/filepath"

const ConfigName = "togen.yml"

func TogenDir(cwd string) string    { return filepath.Join(cwd, "togen") }
func ProjectPath(cwd string) string { return filepath.Join(TogenDir(cwd), "project.json") }
func LayoutPath(cwd string) string  { return filepath.Join(TogenDir(cwd), "layout.json") }
func ViewsPath(cwd string) string   { return filepath.Join(TogenDir(cwd), "views.json") }
func ConfigPath(cwd string) string  { return filepath.Join(cwd, ConfigName) }

// Where configuration lived before ADR 0007.
func LegacyConfigPath(cwd string) string { return filepath.Join(TogenDir(cwd), "togen.json") }

func shownPath(cwd, path string) string {
	rel, err := filepath.Rel(cwd, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}
