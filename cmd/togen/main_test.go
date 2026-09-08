package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mooncitizen/togen/internal/release"
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

type recordingClient struct{ calls int }

func (r *recordingClient) Latest(context.Context) (release.Release, error) {
	r.calls++
	return release.Release{Tag: "v99.0.0"}, nil
}

func (r *recordingClient) Get(context.Context, string) (release.Release, error) {
	r.calls++
	return release.Release{}, nil
}

func (r *recordingClient) Download(context.Context, string) (io.ReadCloser, error) {
	return nil, nil
}

func withUpdateCheck(t *testing.T, stamped string, terminal bool) *recordingClient {
	t.Helper()
	client := &recordingClient{}
	previousVersion, previousTerminal, previousClient, previousCachePath := version, stderrIsTerminal, newReleaseClient, releaseCachePath
	version = stamped
	stderrIsTerminal = func() bool { return terminal }
	newReleaseClient = func() release.Client { return client }
	cachePath := filepath.Join(t.TempDir(), "version-check.json")
	releaseCachePath = func() (string, error) { return cachePath, nil }
	t.Setenv("TOGEN_NO_UPDATE_CHECK", "")
	t.Setenv("CI", "")
	t.Cleanup(func() {
		version, stderrIsTerminal, newReleaseClient, releaseCachePath = previousVersion, previousTerminal, previousClient, previousCachePath
	})
	return client
}

func TestUpdateCheckRunsOnAnOrdinaryCommand(t *testing.T) {
	inTempDir(t)
	client := withUpdateCheck(t, "0.1.0", true)
	execute(t, "init", "--name", "shop")
	if client.calls != 1 {
		t.Errorf("calls = %d, want 1", client.calls)
	}
}

func TestUpdateCheckIsSuppressed(t *testing.T) {
	cases := []struct {
		name      string
		stamped   string
		terminal  bool
		env       map[string]string
		setup     func(t *testing.T)
		args      []string
		wantCalls int
	}{
		{name: "a dev build", stamped: "dev", terminal: true, args: []string{"init", "--name", "shop"}},
		{name: "no terminal", stamped: "0.1.0", terminal: false, args: []string{"init", "--name", "shop"}},
		{name: "opted out", stamped: "0.1.0", terminal: true, env: map[string]string{"TOGEN_NO_UPDATE_CHECK": "1"}, args: []string{"init", "--name", "shop"}},
		{name: "in CI", stamped: "0.1.0", terminal: true, env: map[string]string{"CI": "true"}, args: []string{"init", "--name", "shop"}},
		{name: "json output", stamped: "0.1.0", terminal: true, setup: func(t *testing.T) { execute(t, "init", "--name", "shop") }, args: []string{"cost", "--json"}},
		{name: "the version command", stamped: "0.1.0", terminal: true, args: []string{"version"}},
		{name: "the help command", stamped: "0.1.0", terminal: true, args: []string{"help"}},
		{name: "the completion command", stamped: "0.1.0", terminal: true, args: []string{"completion", "zsh"}},
		{name: "shell completion requests", stamped: "0.1.0", terminal: true, args: []string{"__complete", "init", ""}},
		// upgrade --check calls client.Latest itself to resolve the release to report;
		// that call is expected. What is suppressed is a second call from the daily check.
		{name: "the upgrade command", stamped: "0.1.0", terminal: true, args: []string{"upgrade", "--check"}, wantCalls: 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			inTempDir(t)
			if c.setup != nil {
				c.setup(t)
			}
			client := withUpdateCheck(t, c.stamped, c.terminal)
			for k, v := range c.env {
				t.Setenv(k, v)
			}
			root := newRootCommand()
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			root.SetArgs(c.args)
			_ = root.ExecuteContext(context.Background())
			if client.calls != c.wantCalls {
				t.Errorf("calls = %d, want %d", client.calls, c.wantCalls)
			}
		})
	}
}
