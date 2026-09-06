package examples

import (
	"embed"
	"io/fs"
	"path"
	"slices"
	"strings"
)

// Copies of examples/ made by just generate: go:embed cannot reach a parent directory.
//
//go:embed bundled
var bundled embed.FS

type Example struct {
	ID          string `json:"id"`
	Description string `json:"description"`
}

var list = []Example{
	{ID: "aws-basic", Description: "One of every node type, wired up with the defaults"},
	{ID: "aws-full", Description: "Every node type, relation and property this milestone supports"},
	{ID: "azure-basic", Description: "A gateway routing to a function that reads a database, on Azure"},
	{ID: "gcp-basic", Description: "A gateway routing to a function on Google Cloud"},
}

func List() []Example { return slices.Clone(list) }

// Keys are paths relative to the project root, togen/project.json and so on.
func Files(id string) (map[string][]byte, bool) {
	if !slices.ContainsFunc(list, func(e Example) bool { return e.ID == id }) {
		return nil, false
	}
	root := path.Join("bundled", id)
	files := map[string][]byte{}
	err := fs.WalkDir(bundled, root, func(p string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		raw, err := bundled.ReadFile(p)
		if err != nil {
			return err
		}
		files[strings.TrimPrefix(p, root+"/")] = raw
		return nil
	})
	if err != nil {
		return nil, false
	}
	return files, true
}
