package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/mooncitizen/togen/internal/schema"
)

// The packages that validate cannot embed from schema/, so they keep a copy.
var embedCopies = map[string]string{
	"project.schema.json": "internal/ir/project.schema.json",
	"togen.schema.json":   "internal/workspace/togen.schema.json",
	"views.schema.json":   "internal/workspace/views.schema.json",
	"layout.schema.json":  "internal/workspace/layout.schema.json",
}

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
}
