package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestInitCommandWritesTheProjectFile(t *testing.T) {
	dir := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previous); err != nil {
			t.Fatal(err)
		}
	})

	root := newRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"init", "--provider", "gcp", "--name", "shop"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "togen", "project.json"))
	if err != nil {
		t.Fatalf("project.json: %v", err)
	}
	var project map[string]any
	if err := json.Unmarshal(raw, &project); err != nil {
		t.Fatal(err)
	}
	if project["name"] != "shop" || project["provider"] != "gcp" {
		t.Errorf("project = %v", project)
	}
}
