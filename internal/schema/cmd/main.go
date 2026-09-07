package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/mooncitizen/togen/internal/schema"
	"github.com/mooncitizen/togen/internal/workspace"
)

// The packages that validate cannot embed from schema/, so they keep a copy.
var embedCopies = map[string]string{
	"project.schema.json":    "internal/ir/project.schema.json",
	"togen.schema.json":      "internal/workspace/togen.schema.json",
	"views.schema.json":      "internal/workspace/views.schema.json",
	"simulation.schema.json": "internal/workspace/simulation.schema.json",
	"layout.schema.json":     "internal/workspace/layout.schema.json",
}

// Same for the examples the studio offers, which go:embed cannot reach in examples/.
const bundledDir = "internal/examples/bundled"

func main() {
	if err := schema.Write("schema"); err != nil {
		log.Fatal(err)
	}
	for name, copied := range embedCopies {
		b, err := os.ReadFile(filepath.Join("schema", name))
		if err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(copied, b, 0o644); err != nil {
			log.Fatal(err)
		}
	}
	if err := copyExamples(); err != nil {
		log.Fatal(err)
	}
}

// Only the project files are copied, never the infra/ a generate left behind.
func copyExamples() error {
	entries, err := os.ReadDir("examples")
	if err != nil {
		return err
	}
	if err := os.RemoveAll(bundledDir); err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		for _, name := range workspace.ProjectFiles {
			b, err := os.ReadFile(filepath.Join("examples", entry.Name(), filepath.FromSlash(name)))
			if err != nil {
				return err
			}
			target := filepath.Join(bundledDir, entry.Name(), filepath.FromSlash(name))
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(target, b, 0o644); err != nil {
				return err
			}
		}
	}
	return nil
}
