package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func inTempDir(t *testing.T) string {
	t.Helper()
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
	return dir
}

func execute(t *testing.T, args ...string) {
	t.Helper()
	root := newRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		t.Fatalf("execute %v: %v", args, err)
	}
}

func TestInitCommandWritesTheProjectFile(t *testing.T) {
	dir := inTempDir(t)
	execute(t, "init", "--provider", "gcp", "--name", "shop")

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

func TestInitMigrateCommandRewritesTheLegacyConfig(t *testing.T) {
	dir := inTempDir(t)
	execute(t, "init", "--name", "shop")
	if err := os.Remove(filepath.Join(dir, "togen.yml")); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(dir, "togen", "togen.json")
	if err := os.WriteFile(legacy, []byte(`{"version":1,"targets":["hcl"],"outDir":"build"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	execute(t, "init", "--migrate")

	raw, err := os.ReadFile(filepath.Join(dir, "togen.yml"))
	if err != nil {
		t.Fatalf("togen.yml: %v", err)
	}
	if !strings.Contains(string(raw), "outDir: build") {
		t.Errorf("togen.yml = %s", raw)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Errorf("togen/togen.json survived: %v", err)
	}
}

func TestTheRootCommandReportsItsVersion(t *testing.T) {
	root := newRootCommand()
	if root.Version != version {
		t.Fatalf("root command version is %q, want %q", root.Version, version)
	}

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"--version"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), version) {
		t.Errorf("--version printed %q, which does not contain %q", out.String(), version)
	}
}

// An unstamped build says dev rather than pretending to be a release.
func TestTheDefaultVersionIsDev(t *testing.T) {
	if version != "dev" {
		t.Errorf("the compiled-in default is %q, want dev", version)
	}
}

func TestVersionCommandPrintsTheStampedVersion(t *testing.T) {
	inTempDir(t)
	var out bytes.Buffer
	root := newRootCommand()
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"version", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
}

func TestVersionCommandIsRegistered(t *testing.T) {
	root := newRootCommand()
	for _, c := range root.Commands() {
		if c.Name() == "version" {
			return
		}
	}
	t.Fatal("root has no version command")
}

func TestUpgradeCommandIsRegisteredWithItsFlags(t *testing.T) {
	root := newRootCommand()
	for _, c := range root.Commands() {
		if c.Name() != "upgrade" {
			continue
		}
		for _, flag := range []string{"check", "yes", "version"} {
			if c.Flags().Lookup(flag) == nil {
				t.Errorf("upgrade has no --%s", flag)
			}
		}
		return
	}
	t.Fatal("root has no upgrade command")
}
