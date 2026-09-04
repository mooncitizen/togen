package cli

import "path/filepath"

func togenDir(cwd string) string    { return filepath.Join(cwd, "togen") }
func projectPath(cwd string) string { return filepath.Join(togenDir(cwd), "project.json") }
func layoutPath(cwd string) string  { return filepath.Join(togenDir(cwd), "layout.json") }
func configPath(cwd string) string  { return filepath.Join(togenDir(cwd), "togen.json") }

func shownPath(cwd, path string) string {
	rel, err := filepath.Rel(cwd, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}
