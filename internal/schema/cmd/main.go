package main

import (
	"log"
	"os"
	"path/filepath"

	"togen/internal/schema"
)

const embedCopy = "internal/ir/project.schema.json"

func main() {
	if err := schema.Write("schema"); err != nil {
		log.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join("schema", "project.schema.json"))
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(embedCopy, b, 0o644); err != nil {
		log.Fatal(err)
	}
}
