package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/mooncitizen/togen/internal/examples"
	"github.com/mooncitizen/togen/internal/workspace"
)

func written(t *testing.T, raw []byte) []string {
	t.Helper()
	var body struct {
		Written []string `json:"written"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("parse %s: %v", raw, err)
	}
	return body.Written
}

func errorMessage(t *testing.T, raw []byte) string {
	t.Helper()
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("parse %s: %v", raw, err)
	}
	return body.Error
}

func expectValidProject(t *testing.T, dir string) {
	t.Helper()
	_, errs, err := workspace.Validate(dir)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if len(errs) > 0 {
		t.Errorf("the written project does not validate: %v", errs)
	}
}

func TestGetProjectIsNotFoundWithoutAProject(t *testing.T) {
	_, front, _ := emptyHarness(t)
	code, raw := send(t, front, http.MethodGet, "/api/project", nil)
	if code != http.StatusNotFound {
		t.Fatalf("code = %d, body = %s", code, raw)
	}
	if want := "togen/project.json not found. Run 'togen init' first."; errorMessage(t, raw) != want {
		t.Errorf("error = %q, want %q", errorMessage(t, raw), want)
	}
}

func TestPostInitWritesASketch(t *testing.T) {
	dir, front, _ := emptyHarness(t)
	code, raw := send(t, front, http.MethodPost, "/api/project/init",
		[]byte(`{"name":"shop","provider":"gcp","region":"europe-west2","environment":"staging"}`))
	if code != http.StatusCreated {
		t.Fatalf("code = %d, body = %s", code, raw)
	}
	if diff := cmp.Diff([]string{"togen/project.json", "togen/layout.json", "togen.yml"}, written(t, raw)); diff != "" {
		t.Errorf("written (-want +got):\n%s", diff)
	}

	var project map[string]any
	code, raw = send(t, front, http.MethodGet, "/api/project", nil)
	if code != http.StatusOK {
		t.Fatalf("get: code = %d, body = %s", code, raw)
	}
	if err := json.Unmarshal(raw, &project); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"version":     float64(1),
		"name":        "shop",
		"provider":    "gcp",
		"region":      "europe-west2",
		"environment": "staging",
		"nodes":       []any{},
		"edges":       []any{},
	}
	if diff := cmp.Diff(want, project); diff != "" {
		t.Errorf("project (-want +got):\n%s", diff)
	}
	if code, raw := send(t, front, http.MethodGet, "/api/layout", nil); code != http.StatusOK {
		t.Errorf("layout: code = %d, body = %s", code, raw)
	}
	config, _, err := workspace.LoadConfig(dir)
	if err != nil || config.OutDir != "infra" {
		t.Errorf("togen.yml: config = %+v, err = %v", config, err)
	}
	expectValidProject(t, dir)
}

func TestPostInitCopiesAnExample(t *testing.T) {
	dir, front, _ := emptyHarness(t)
	code, raw := send(t, front, http.MethodPost, "/api/project/init", []byte(`{"example":"aws-full"}`))
	if code != http.StatusCreated {
		t.Fatalf("code = %d, body = %s", code, raw)
	}
	if diff := cmp.Diff([]string{"togen/project.json", "togen/layout.json", "togen.yml"}, written(t, raw)); diff != "" {
		t.Errorf("written (-want +got):\n%s", diff)
	}
	files, _ := examples.Files("aws-full")
	for name, want := range files {
		got, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(want, got) {
			t.Errorf("%s differs from the bundled copy", name)
		}
	}
	expectValidProject(t, dir)
}

func TestPostInitLeavesAnExistingConfigAlone(t *testing.T) {
	dir, front, _ := emptyHarness(t)
	writeConfig(t, dir, "targets: [hcl]\noutDir: build\n")

	code, raw := send(t, front, http.MethodPost, "/api/project/init", []byte(`{"example":"aws-basic"}`))
	if code != http.StatusCreated {
		t.Fatalf("code = %d, body = %s", code, raw)
	}
	if diff := cmp.Diff([]string{"togen/project.json", "togen/layout.json"}, written(t, raw)); diff != "" {
		t.Errorf("written (-want +got):\n%s", diff)
	}
	got, err := os.ReadFile(workspace.ConfigPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "targets: [hcl]\noutDir: build\n" {
		t.Errorf("togen.yml was rewritten:\n%s", got)
	}
}

func TestPostInitRefusesAnUnknownExample(t *testing.T) {
	dir, front, _ := emptyHarness(t)
	code, raw := send(t, front, http.MethodPost, "/api/project/init", []byte(`{"example":"gcp-basic"}`))
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("code = %d, body = %s", code, raw)
	}
	if diff := cmp.Diff([]string{"example: unknown example 'gcp-basic'"}, errorLines(t, raw)); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
	if workspace.Exists(workspace.TogenDir(dir)) {
		t.Error("togen/ was created despite the refusal")
	}
}

func TestPostInitRefusesAnInvalidSketch(t *testing.T) {
	dir, front, _ := emptyHarness(t)
	code, raw := send(t, front, http.MethodPost, "/api/project/init",
		[]byte(`{"name":"Bad Name","provider":"aws","region":"EU West","environment":"Prod"}`))
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("code = %d, body = %s", code, raw)
	}
	var body struct {
		Errors []struct {
			Path    string `json:"path"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, e := range body.Errors {
		paths = append(paths, e.Path)
		if e.Message == "" {
			t.Errorf("%s has no message", e.Path)
		}
	}
	slices.Sort(paths)
	if diff := cmp.Diff([]string{"environment", "name", "region"}, paths); diff != "" {
		t.Errorf("error paths (-want +got):\n%s", diff)
	}

	code, raw = send(t, front, http.MethodPost, "/api/project/init", []byte(`{"provider":"oracle"}`))
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("code = %d, body = %s", code, raw)
	}
	if diff := cmp.Diff([]string{"provider: unknown provider 'oracle', use one of aws, gcp, azure"}, errorLines(t, raw)); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
	if workspace.Exists(workspace.TogenDir(dir)) {
		t.Error("togen/ was created despite the refusal")
	}
}

func TestPostInitRefusesAMalformedBody(t *testing.T) {
	_, front, _ := emptyHarness(t)
	for _, body := range []string{"{ not json", `["shop"]`} {
		if code, raw := send(t, front, http.MethodPost, "/api/project/init", []byte(body)); code != http.StatusBadRequest {
			t.Errorf("%s: code = %d, body = %s", body, code, raw)
		}
	}
}

func TestPostInitRefusesWhenTheProjectExists(t *testing.T) {
	dir, front, _ := harness(t)
	before, err := os.ReadFile(workspace.ProjectPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	code, raw := send(t, front, http.MethodPost, "/api/project/init", []byte(`{"name":"other"}`))
	if code != http.StatusConflict {
		t.Fatalf("code = %d, body = %s", code, raw)
	}
	if want := "togen/ already exists in " + dir; errorMessage(t, raw) != want {
		t.Errorf("error = %q, want %q", errorMessage(t, raw), want)
	}
	after, err := os.ReadFile(workspace.ProjectPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Error("the project file was written despite the refusal")
	}
}

func TestPostInitRefusesACrossOriginRequest(t *testing.T) {
	dir, front, _ := emptyHarness(t)
	code, raw := doRequest(t, front, http.MethodPost, "/api/project/init", []byte(`{"example":"aws-basic"}`),
		map[string]string{"Origin": "http://evil.example"})
	if code != http.StatusForbidden {
		t.Fatalf("code = %d, body = %s", code, raw)
	}
	if workspace.Exists(workspace.TogenDir(dir)) {
		t.Error("togen/ was created despite the refusal")
	}
}

func TestGetExamplesListsTheBundledOnes(t *testing.T) {
	_, front, _ := emptyHarness(t)
	code, raw := send(t, front, http.MethodGet, "/api/examples", nil)
	if code != http.StatusOK {
		t.Fatalf("code = %d, body = %s", code, raw)
	}
	var body struct {
		Examples []examples.Example `json:"examples"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("parse %s: %v", raw, err)
	}
	if diff := cmp.Diff(examples.List(), body.Examples); diff != "" {
		t.Errorf("examples (-want +got):\n%s", diff)
	}
	if len(body.Examples) == 0 || body.Examples[0].ID != "aws-basic" {
		t.Errorf("examples = %v, want aws-basic first", body.Examples)
	}
}

func expectEvents(t *testing.T, events <-chan string, want ...string) {
	t.Helper()
	var got []string
	for range want {
		select {
		case event, ok := <-events:
			if !ok {
				t.Fatal("the websocket closed")
			}
			got = append(got, event)
		case <-time.After(2 * time.Second):
			t.Fatalf("got %v within two seconds, want %v", got, want)
		}
	}
	slices.Sort(got)
	slices.Sort(want)
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("events (-want +got):\n%s", diff)
	}
}

// togen init in another terminal while the studio is up: the new directory is
// watched from then on, and the files it arrived with are announced.
func TestEventsFollowAProjectCreatedOutside(t *testing.T) {
	dir, front, studio := emptyHarness(t)
	events := listen(t, front, studio)

	files, err := workspace.SketchFiles(dir, workspace.Sketch{Name: "shop"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := workspace.CreateProject(dir, files); err != nil {
		t.Fatal(err)
	}
	expectEvents(t, events, "project-changed", "layout-changed", "config-changed")
	expectNoEvent(t, events)

	changed := exampleProject()
	changed["name"] = "edited"
	writeDoc(t, workspace.ProjectPath(dir), changed)
	expectEvent(t, events, "project-changed")
}

func TestEventsFollowTheFilesAfterInitThroughTheApi(t *testing.T) {
	dir, front, studio := emptyHarness(t)
	events := listen(t, front, studio)

	if code, raw := send(t, front, http.MethodPost, "/api/project/init", []byte(`{"name":"shop"}`)); code != http.StatusCreated {
		t.Fatalf("init: code = %d, body = %s", code, raw)
	}
	expectNoEvent(t, events)

	writeDoc(t, workspace.LayoutPath(dir), map[string]any{
		"version":  1,
		"nodes":    map[string]any{"n1": map[string]any{"x": 10, "y": 20}},
		"viewport": map[string]any{"x": 0, "y": 0, "zoom": 1},
	})
	expectEvent(t, events, "layout-changed")
}
