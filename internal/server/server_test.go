package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/google/go-cmp/cmp"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/workspace"
)

func exampleProject() map[string]any {
	return map[string]any{
		"version":     1,
		"name":        "shop",
		"provider":    "aws",
		"region":      "eu-west-2",
		"environment": "dev",
		"nodes": []any{
			map[string]any{"id": "n1", "type": "gateway", "name": "api"},
			map[string]any{"id": "n2", "type": "function", "name": "handler"},
		},
		"edges": []any{
			map[string]any{"id": "e1", "from": "n1", "to": "n2", "relation": "routes"},
		},
	}
}

func overviewLayout(nodes, viewport map[string]any) map[string]any {
	return map[string]any{
		"version": 2,
		"views":   map[string]any{"overview": map[string]any{"nodes": nodes, "viewport": viewport}},
	}
}

func marshal(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func writeDoc(t *testing.T, path string, value any) {
	t.Helper()
	if err := workspace.WriteRaw(path, marshal(t, value)); err != nil {
		t.Fatal(err)
	}
}

func writeConfig(t *testing.T, dir, text string) {
	t.Helper()
	if err := workspace.WriteRaw(workspace.ConfigPath(dir), []byte(text)); err != nil {
		t.Fatal(err)
	}
}

func harness(t *testing.T) (string, *httptest.Server, *Server) {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(workspace.TogenDir(dir), 0o755); err != nil {
		t.Fatal(err)
	}
	writeDoc(t, workspace.ProjectPath(dir), exampleProject())
	writeDoc(t, workspace.LayoutPath(dir), overviewLayout(map[string]any{}, map[string]any{"x": 0, "y": 0, "zoom": 1}))
	writeConfig(t, dir, "version: 1\ntargets: [hcl]\noutDir: infra\n")

	studio, err := New(Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = studio.Close() })
	front := httptest.NewServer(studio.Handler())
	t.Cleanup(front.Close)
	return dir, front, studio
}

func send(t *testing.T, front *httptest.Server, method, path string, body []byte) (int, []byte) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequest(method, front.URL+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	response, err := front.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("%s %s: Cache-Control = %q", method, path, response.Header.Get("Cache-Control"))
	}
	return response.StatusCode, raw
}

func errorLines(t *testing.T, raw []byte) []string {
	t.Helper()
	var body struct {
		Errors ir.Errors `json:"errors"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("parse %s: %v", raw, err)
	}
	lines := make([]string, len(body.Errors))
	for i, e := range body.Errors {
		lines[i] = e.String()
	}
	return lines
}

func TestGetProjectReturnsTheFile(t *testing.T) {
	dir, front, _ := harness(t)
	code, raw := send(t, front, http.MethodGet, "/api/project", nil)
	if code != http.StatusOK {
		t.Fatalf("code = %d, body = %s", code, raw)
	}
	onDisk, err := os.ReadFile(workspace.ProjectPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, onDisk) {
		t.Errorf("body = %s, want %s", raw, onDisk)
	}
}

func TestGetProjectReportsAMissingFile(t *testing.T) {
	dir, front, _ := harness(t)
	if err := os.Remove(workspace.ProjectPath(dir)); err != nil {
		t.Fatal(err)
	}
	code, raw := send(t, front, http.MethodGet, "/api/project", nil)
	if code != http.StatusNotFound {
		t.Fatalf("code = %d, body = %s", code, raw)
	}
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if want := "togen/project.json not found. Run 'togen init' first."; body.Error != want {
		t.Errorf("error = %q, want %q", body.Error, want)
	}
}

func TestGetProjectReportsInvalidJson(t *testing.T) {
	dir, front, _ := harness(t)
	if err := os.WriteFile(workspace.ProjectPath(dir), []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, raw := send(t, front, http.MethodGet, "/api/project", nil)
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("code = %d, body = %s", code, raw)
	}
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if want := "togen/project.json is not valid JSON"; body.Error != want {
		t.Errorf("error = %q, want %q", body.Error, want)
	}
}

// A rename-based write is never half-written on disk, so a GET running
// concurrently with a stream of PUTs should never see a non-JSON body.
func TestConcurrentPutNeverServesATruncatedProject(t *testing.T) {
	_, front, _ := harness(t)
	body := marshal(t, exampleProject())

	stop := make(chan struct{})
	var failed atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		client := front.Client()
		for {
			select {
			case <-stop:
				return
			default:
			}
			request, err := http.NewRequest(http.MethodPut, front.URL+"/api/project", bytes.NewReader(body))
			if err != nil {
				failed.Store(true)
				return
			}
			response, err := client.Do(request)
			if err != nil {
				failed.Store(true)
				return
			}
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
			if response.StatusCode != http.StatusNoContent {
				failed.Store(true)
			}
		}
	}()

	for range 200 {
		code, raw := send(t, front, http.MethodGet, "/api/project", nil)
		if code != http.StatusOK {
			t.Fatalf("code = %d, body = %s", code, raw)
		}
		if !json.Valid(raw) {
			t.Fatalf("body was not valid json: %q", raw)
		}
	}
	close(stop)
	wg.Wait()
	if failed.Load() {
		t.Error("a concurrent PUT failed")
	}
}

func TestPutProjectRefusesAnInvalidProject(t *testing.T) {
	dir, front, _ := harness(t)
	before, err := os.ReadFile(workspace.ProjectPath(dir))
	if err != nil {
		t.Fatal(err)
	}

	project := exampleProject()
	project["edges"] = []any{map[string]any{"id": "e1", "from": "n1", "to": "zz", "relation": "routes"}}
	code, raw := send(t, front, http.MethodPut, "/api/project", marshal(t, project))
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("code = %d, body = %s", code, raw)
	}
	want := []string{"edges.0 (edge e1): edge refers to missing node 'zz'"}
	if diff := cmp.Diff(want, errorLines(t, raw)); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}

	after, err := os.ReadFile(workspace.ProjectPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Error("the project file was written despite the refusal")
	}
}

func TestPutProjectRefusesAResolverError(t *testing.T) {
	_, front, _ := harness(t)
	project := exampleProject()
	project["nodes"] = append(project["nodes"].([]any),
		map[string]any{"id": "n3", "type": "function", "name": "second"})
	project["edges"] = append(project["edges"].([]any),
		map[string]any{"id": "e2", "from": "n1", "to": "n3", "relation": "routes"})

	code, raw := send(t, front, http.MethodPut, "/api/project", marshal(t, project))
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("code = %d, body = %s", code, raw)
	}
	want := []string{"project (edge e2): route 'ANY /' on gateway 'api' is already used by edge 'e1'"}
	if diff := cmp.Diff(want, errorLines(t, raw)); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
}

func TestPutProjectRefusesAMalformedBody(t *testing.T) {
	_, front, _ := harness(t)
	code, raw := send(t, front, http.MethodPut, "/api/project", []byte("{ not json"))
	if code != http.StatusBadRequest {
		t.Fatalf("code = %d, body = %s", code, raw)
	}
}

func TestPutProjectWritesAValidProject(t *testing.T) {
	dir, front, _ := harness(t)
	project := exampleProject()
	project["name"] = "market"
	body := marshal(t, project)

	code, raw := send(t, front, http.MethodPut, "/api/project", body)
	if code != http.StatusNoContent {
		t.Fatalf("code = %d, body = %s", code, raw)
	}
	onDisk, err := os.ReadFile(workspace.ProjectPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(body, onDisk) {
		t.Errorf("file = %s, want %s", onDisk, body)
	}
}

func doRequest(t *testing.T, front *httptest.Server, method, path string, body []byte, headers map[string]string) (int, []byte) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequest(method, front.URL+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := front.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, raw
}

func TestPutRefusesACrossOriginRequest(t *testing.T) {
	dir, front, _ := harness(t)
	before, err := os.ReadFile(workspace.ProjectPath(dir))
	if err != nil {
		t.Fatal(err)
	}

	code, raw := doRequest(t, front, http.MethodPut, "/api/project", marshal(t, exampleProject()),
		map[string]string{"Origin": "http://evil.example"})
	if code != http.StatusForbidden {
		t.Fatalf("code = %d, body = %s", code, raw)
	}
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if want := "cross-origin request refused"; body.Error != want {
		t.Errorf("error = %q, want %q", body.Error, want)
	}

	after, err := os.ReadFile(workspace.ProjectPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Error("the project file was written despite the refusal")
	}
}

func TestPutRefusesACrossSiteFetch(t *testing.T) {
	_, front, _ := harness(t)
	code, raw := doRequest(t, front, http.MethodPut, "/api/project", marshal(t, exampleProject()),
		map[string]string{"Sec-Fetch-Site": "cross-site"})
	if code != http.StatusForbidden {
		t.Fatalf("code = %d, body = %s", code, raw)
	}
}

func TestGenerateRefusesACrossOriginRequest(t *testing.T) {
	_, front, _ := harness(t)
	code, raw := doRequest(t, front, http.MethodPost, "/api/generate", nil,
		map[string]string{"Origin": "http://evil.example"})
	if code != http.StatusForbidden {
		t.Fatalf("code = %d, body = %s", code, raw)
	}
}

func TestPutAllowsASameOriginRequest(t *testing.T) {
	dir, front, _ := harness(t)
	project := exampleProject()
	project["name"] = "market"
	body := marshal(t, project)

	code, raw := doRequest(t, front, http.MethodPut, "/api/project", body,
		map[string]string{"Origin": front.URL, "Sec-Fetch-Site": "same-origin"})
	if code != http.StatusNoContent {
		t.Fatalf("code = %d, body = %s", code, raw)
	}
	onDisk, err := os.ReadFile(workspace.ProjectPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(body, onDisk) {
		t.Errorf("file = %s, want %s", onDisk, body)
	}
}

func TestGetIgnoresOriginHeaders(t *testing.T) {
	_, front, _ := harness(t)
	code, raw := doRequest(t, front, http.MethodGet, "/api/project", nil,
		map[string]string{"Origin": "http://evil.example"})
	if code != http.StatusOK {
		t.Fatalf("code = %d, body = %s", code, raw)
	}
}

func getJSON(t *testing.T, front *httptest.Server, path string, want int) map[string]any {
	t.Helper()
	code, raw := send(t, front, http.MethodGet, path, nil)
	if code != want {
		t.Fatalf("code = %d, want %d, body = %s", code, want, raw)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("parse %s: %v", raw, err)
	}
	return body
}

func TestLayoutRoundTrips(t *testing.T) {
	_, front, _ := harness(t)
	layout := overviewLayout(
		map[string]any{"n1": map[string]any{"x": 40, "y": 80}},
		map[string]any{"x": 0, "y": 0, "zoom": 1.5},
	)
	if code, raw := send(t, front, http.MethodPut, "/api/layout", marshal(t, layout)); code != http.StatusNoContent {
		t.Fatalf("put: code = %d, body = %s", code, raw)
	}
	want := overviewLayout(
		map[string]any{"n1": map[string]any{"x": float64(40), "y": float64(80)}},
		map[string]any{"x": float64(0), "y": float64(0), "zoom": 1.5},
	)
	want["version"] = float64(2)
	if diff := cmp.Diff(want, getJSON(t, front, "/api/layout", http.StatusOK)); diff != "" {
		t.Errorf("layout (-want +got):\n%s", diff)
	}
}

func TestPutLayoutWritesAVersionOneBodyAsVersionTwo(t *testing.T) {
	dir, front, _ := harness(t)
	body := []byte(`{"version":1,"nodes":{"n1":{"x":40,"y":80}},"viewport":{"x":0,"y":0,"zoom":1.5}}`)
	if code, raw := send(t, front, http.MethodPut, "/api/layout", body); code != http.StatusNoContent {
		t.Fatalf("put: code = %d, body = %s", code, raw)
	}
	onDisk, err := os.ReadFile(workspace.LayoutPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(onDisk, &got); err != nil {
		t.Fatal(err)
	}
	want := overviewLayout(
		map[string]any{"n1": map[string]any{"x": float64(40), "y": float64(80)}},
		map[string]any{"x": float64(0), "y": float64(0), "zoom": 1.5},
	)
	want["version"] = float64(2)
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("layout.json (-want +got):\n%s", diff)
	}
}

func TestGetLayoutMigratesAVersionOneFileWithoutRewritingIt(t *testing.T) {
	dir, front, _ := harness(t)
	text := []byte(`{"version":1,"nodes":{"n1":{"x":40,"y":80}},"viewport":{"x":0,"y":0,"zoom":1}}`)
	if err := workspace.WriteRaw(workspace.LayoutPath(dir), text); err != nil {
		t.Fatal(err)
	}
	got := getJSON(t, front, "/api/layout", http.StatusOK)
	if got["version"] != float64(2) {
		t.Errorf("version = %v, want 2", got["version"])
	}
	views, _ := got["views"].(map[string]any)
	overview, _ := views["overview"].(map[string]any)
	if diff := cmp.Diff(map[string]any{"n1": map[string]any{"x": float64(40), "y": float64(80)}}, overview["nodes"]); diff != "" {
		t.Errorf("overview nodes (-want +got):\n%s", diff)
	}
	onDisk, err := os.ReadFile(workspace.LayoutPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(onDisk, text) {
		t.Error("a GET rewrote the layout file")
	}
}

func TestPutLayoutRefusesAnotherVersion(t *testing.T) {
	_, front, _ := harness(t)
	code, raw := send(t, front, http.MethodPut, "/api/layout", []byte(`{"version":3}`))
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("code = %d, body = %s", code, raw)
	}
	want := []string{"project: togen/layout.json is version 3 but this Togen only understands up to 2"}
	if diff := cmp.Diff(want, errorLines(t, raw)); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
}

func TestPutLayoutRefusesTheWrongShapeWithItsPath(t *testing.T) {
	dir, front, _ := harness(t)
	before, err := os.ReadFile(workspace.LayoutPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"version":2,"views":{"overview":{"nodes":{"n1":{"x":"far","y":0}}}}}`)
	code, raw := send(t, front, http.MethodPut, "/api/layout", body)
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("code = %d, body = %s", code, raw)
	}
	want := []string{"views.overview.nodes.n1.x: got string, want number"}
	if diff := cmp.Diff(want, errorLines(t, raw)); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
	after, err := os.ReadFile(workspace.LayoutPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Error("the layout file was written despite the refusal")
	}
}

func exampleViews() map[string]any {
	return map[string]any{
		"version": 1,
		"views": []any{
			map[string]any{"id": "overview", "name": "Overview", "nodes": "*"},
			map[string]any{"id": "orders", "name": "Orders path", "nodes": []any{"n1", "n2"}},
		},
	}
}

func TestGetViewsAnswersWithTheOverviewWhenTheFileIsMissing(t *testing.T) {
	_, front, _ := harness(t)
	want := map[string]any{
		"version": float64(1),
		"views":   []any{map[string]any{"id": "overview", "name": "Overview", "nodes": "*"}},
	}
	if diff := cmp.Diff(want, getJSON(t, front, "/api/views", http.StatusOK)); diff != "" {
		t.Errorf("views (-want +got):\n%s", diff)
	}
}

func TestViewsRoundTrip(t *testing.T) {
	dir, front, _ := harness(t)
	if code, raw := send(t, front, http.MethodPut, "/api/views", marshal(t, exampleViews())); code != http.StatusNoContent {
		t.Fatalf("put: code = %d, body = %s", code, raw)
	}
	onDisk, err := os.ReadFile(workspace.ViewsPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(onDisk, []byte("{\n  \"version\": 1,")) || !bytes.HasSuffix(onDisk, []byte("}\n")) {
		t.Errorf("views.json = %s, want it indented like the CLI writes", onDisk)
	}
	want := map[string]any{
		"version": float64(1),
		"views": []any{
			map[string]any{"id": "overview", "name": "Overview", "nodes": "*"},
			map[string]any{"id": "orders", "name": "Orders path", "nodes": []any{"n1", "n2"}},
		},
	}
	if diff := cmp.Diff(want, getJSON(t, front, "/api/views", http.StatusOK)); diff != "" {
		t.Errorf("views (-want +got):\n%s", diff)
	}
}

func TestPutViewsRefusesWhatTheProjectCannotAnswerFor(t *testing.T) {
	for _, c := range []struct {
		name string
		edit func(views map[string]any)
		want string
	}{
		{"an unknown node", func(views map[string]any) {
			views["views"].([]any)[1].(map[string]any)["nodes"] = []any{"n1", "zz"}
		}, "views.1.nodes.1: view 'orders' refers to missing node 'zz'"},
		{"a missing overview", func(views map[string]any) {
			views["views"] = views["views"].([]any)[1:]
		}, "views: every project has an 'overview' view"},
		{"a duplicate id", func(views map[string]any) {
			views["views"].([]any)[1].(map[string]any)["id"] = "overview"
		}, "views.1.id: duplicate view id 'overview'"},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir, front, _ := harness(t)
			views := exampleViews()
			c.edit(views)
			code, raw := send(t, front, http.MethodPut, "/api/views", marshal(t, views))
			if code != http.StatusUnprocessableEntity {
				t.Fatalf("code = %d, body = %s", code, raw)
			}
			if diff := cmp.Diff([]string{c.want}, errorLines(t, raw)); diff != "" {
				t.Errorf("errors (-want +got):\n%s", diff)
			}
			if workspace.Exists(workspace.ViewsPath(dir)) {
				t.Error("views.json was written despite the refusal")
			}
		})
	}
}

func TestPutViewsRefusesABadShapeWithItsPath(t *testing.T) {
	_, front, _ := harness(t)
	views := exampleViews()
	views["views"].([]any)[1].(map[string]any)["nodes"] = "some"
	code, raw := send(t, front, http.MethodPut, "/api/views", marshal(t, views))
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("code = %d, body = %s", code, raw)
	}
	want := []string{"views.1.nodes: value must be '*'"}
	if diff := cmp.Diff(want, errorLines(t, raw)); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
}

func TestPutViewsNeedsAProjectThatLoads(t *testing.T) {
	dir, front, _ := harness(t)
	if err := os.WriteFile(workspace.ProjectPath(dir), []byte(`{"version":1,"name":"shop"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	code, raw := send(t, front, http.MethodPut, "/api/views", marshal(t, exampleViews()))
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("code = %d, body = %s", code, raw)
	}
	want := []string{"project: the views cannot be checked without a valid project"}
	if diff := cmp.Diff(want, errorLines(t, raw)); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
}

func TestGetViewsReportsABrokenFile(t *testing.T) {
	dir, front, _ := harness(t)
	if err := workspace.WriteRaw(workspace.ViewsPath(dir), []byte(`{"version":1,"views":[{"id":"orders","name":"Orders","nodes":[]}]}`)); err != nil {
		t.Fatal(err)
	}
	code, raw := send(t, front, http.MethodGet, "/api/views", nil)
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("code = %d, body = %s", code, raw)
	}
	want := []string{"views: every project has an 'overview' view"}
	if diff := cmp.Diff(want, errorLines(t, raw)); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
}

func getConfig(t *testing.T, front *httptest.Server, want int) map[string]any {
	t.Helper()
	code, raw := send(t, front, http.MethodGet, "/api/config", nil)
	if code != want {
		t.Fatalf("code = %d, want %d, body = %s", code, want, raw)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("parse %s: %v", raw, err)
	}
	return body
}

func TestGetConfigReturnsTheFileWithItsDefaults(t *testing.T) {
	dir, front, _ := harness(t)
	writeConfig(t, dir, "targets: [hcl]\nstyle:\n  kinds:\n    database:\n      color: \"#C925D1\"\n")
	want := map[string]any{
		"version": float64(1),
		"targets": []any{"hcl"},
		"outDir":  "infra",
		"style": map[string]any{
			"theme": "dark",
			"kinds": map[string]any{"database": map[string]any{"color": "#C925D1"}},
		},
	}
	if diff := cmp.Diff(want, getConfig(t, front, http.StatusOK)); diff != "" {
		t.Errorf("config (-want +got):\n%s", diff)
	}
}

func TestGetConfigAnswersWithDefaultsWhenTheFileIsMissing(t *testing.T) {
	dir, front, _ := harness(t)
	if err := os.Remove(workspace.ConfigPath(dir)); err != nil {
		t.Fatal(err)
	}
	got := getConfig(t, front, http.StatusOK)
	style, _ := got["style"].(map[string]any)
	if style["theme"] != workspace.DefaultTheme {
		t.Errorf("config = %v", got)
	}
}

func TestGetConfigReportsAnInvalidFile(t *testing.T) {
	dir, front, _ := harness(t)
	writeConfig(t, dir, "style:\n  theme: neon\n")
	_, raw := send(t, front, http.MethodGet, "/api/config", nil)
	want := []string{"style.theme: value must be one of 'dark', 'light', 'system'"}
	if diff := cmp.Diff(want, errorLines(t, raw)); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
	if code, _ := send(t, front, http.MethodGet, "/api/config", nil); code != http.StatusUnprocessableEntity {
		t.Errorf("code = %d, want 422", code)
	}
}

func TestGetConfigReportsAStyleForANodeThatDoesNotExist(t *testing.T) {
	dir, front, _ := harness(t)
	writeConfig(t, dir, "style:\n  nodes:\n    handler:\n      color: \"#DD344C\"\n    orders-db:\n      shape: cylinder\n")
	code, raw := send(t, front, http.MethodGet, "/api/config", nil)
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("code = %d, body = %s", code, raw)
	}
	want := []string{"style.nodes.orders-db: there is no node named 'orders-db'"}
	if diff := cmp.Diff(want, errorLines(t, raw)); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}

	writeConfig(t, dir, "style:\n  nodes:\n    handler:\n      color: \"#DD344C\"\n")
	got := getConfig(t, front, http.StatusOK)
	style, _ := got["style"].(map[string]any)
	if _, ok := style["nodes"].(map[string]any)["handler"]; !ok {
		t.Errorf("config = %v", got)
	}
}

func TestGetConfigAnswersWithoutAProjectToCheckStylesAgainst(t *testing.T) {
	for _, c := range []struct {
		name  string
		spoil func(t *testing.T, dir string)
	}{
		{"missing", func(t *testing.T, dir string) {
			if err := os.Remove(workspace.ProjectPath(dir)); err != nil {
				t.Fatal(err)
			}
		}},
		{"not json", func(t *testing.T, dir string) {
			if err := os.WriteFile(workspace.ProjectPath(dir), []byte("not json"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{"invalid", func(t *testing.T, dir string) {
			project := exampleProject()
			project["edges"] = []any{map[string]any{"id": "e1", "from": "n1", "to": "zz", "relation": "routes"}}
			writeDoc(t, workspace.ProjectPath(dir), project)
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir, front, _ := harness(t)
			writeConfig(t, dir, "style:\n  nodes:\n    orders-db:\n      shape: cylinder\n")
			c.spoil(t, dir)
			got := getConfig(t, front, http.StatusOK)
			style, _ := got["style"].(map[string]any)
			if _, ok := style["nodes"].(map[string]any)["orders-db"]; !ok {
				t.Errorf("config = %v", got)
			}
		})
	}
}

func TestGetConfigNamesTheLegacyFile(t *testing.T) {
	dir, front, _ := harness(t)
	if err := os.Remove(workspace.ConfigPath(dir)); err != nil {
		t.Fatal(err)
	}
	if err := workspace.WriteRaw(workspace.LegacyConfigPath(dir), []byte(`{"version":1,"targets":["hcl"],"outDir":"infra"}`)); err != nil {
		t.Fatal(err)
	}
	got := getConfig(t, front, http.StatusOK)
	if got["deprecated"] != workspace.LegacyNote {
		t.Errorf("deprecated = %v, want %q", got["deprecated"], workspace.LegacyNote)
	}
}

func generated(t *testing.T, raw []byte) []workspace.Generated {
	t.Helper()
	var body struct {
		Generated []workspace.Generated `json:"generated"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("parse %s: %v", raw, err)
	}
	return body.Generated
}

func TestPostGenerateWritesTheOutput(t *testing.T) {
	dir, front, _ := harness(t)
	code, raw := send(t, front, http.MethodPost, "/api/generate", []byte(`{"target":"hcl"}`))
	if code != http.StatusOK {
		t.Fatalf("code = %d, body = %s", code, raw)
	}
	want := []workspace.Generated{{
		Dir:   "infra/hcl",
		Files: []string{"main.tf", "outputs.tf", "providers.tf", "variables.tf"},
	}}
	if diff := cmp.Diff(want, generated(t, raw)); diff != "" {
		t.Errorf("generated (-want +got):\n%s", diff)
	}
	if _, err := os.Stat(filepath.Join(dir, "infra", "hcl", "main.tf")); err != nil {
		t.Errorf("infra/hcl/main.tf: %v", err)
	}
}

func TestPostGenerateRefusesAStranger(t *testing.T) {
	dir, front, _ := harness(t)
	if code, raw := send(t, front, http.MethodPost, "/api/generate", nil); code != http.StatusOK {
		t.Fatalf("first: code = %d, body = %s", code, raw)
	}
	if err := workspace.WriteRaw(filepath.Join(dir, "infra", "hcl", "extra.tf"), []byte("")); err != nil {
		t.Fatal(err)
	}
	code, raw := send(t, front, http.MethodPost, "/api/generate", nil)
	if code != http.StatusConflict {
		t.Fatalf("code = %d, body = %s", code, raw)
	}
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body.Error, "infra/hcl contains files Togen did not write: extra.tf") {
		t.Errorf("error = %q", body.Error)
	}
}

func TestPostGenerateReportsAnUnsupportedTarget(t *testing.T) {
	_, front, _ := harness(t)
	code, raw := send(t, front, http.MethodPost, "/api/generate", []byte(`{"target":"pulumi"}`))
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("code = %d, body = %s", code, raw)
	}
	want := []string{"project: target 'pulumi' is not supported yet"}
	if diff := cmp.Diff(want, errorLines(t, raw)); diff != "" {
		t.Errorf("errors (-want +got):\n%s", diff)
	}
}

func TestUnknownPathsServeTheApp(t *testing.T) {
	_, front, _ := harness(t)
	code, raw := send(t, front, http.MethodGet, "/foo", nil)
	if code != http.StatusOK {
		t.Fatalf("code = %d, body = %s", code, raw)
	}
	if !bytes.Contains(raw, []byte("<title>Togen studio</title>")) {
		t.Errorf("body = %s, want the index page", raw)
	}
}

func TestUnknownApiPathsReturnJson(t *testing.T) {
	_, front, _ := harness(t)
	code, raw := send(t, front, http.MethodGet, "/api/nope", nil)
	if code != http.StatusNotFound {
		t.Fatalf("code = %d, body = %s", code, raw)
	}
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("parse %s: %v", raw, err)
	}
	if body.Error == "" {
		t.Errorf("body = %s, want an error message", raw)
	}
}

func listen(t *testing.T, front *httptest.Server, studio *Server) <-chan string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(front.URL, "http")+"/api/events", nil)
	cancel()
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })

	events := make(chan string, 4)
	go func() {
		defer close(events)
		for {
			_, raw, err := conn.Read(context.Background())
			if err != nil {
				return
			}
			var message struct {
				Event string `json:"event"`
			}
			if err := json.Unmarshal(raw, &message); err == nil {
				events <- message.Event
			}
		}
	}()

	for range 200 {
		studio.hub.mu.Lock()
		joined := len(studio.hub.clients)
		studio.hub.mu.Unlock()
		if joined > 0 {
			return events
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the websocket client never reached the hub")
	return nil
}

func expectEvent(t *testing.T, events <-chan string, want string) {
	t.Helper()
	select {
	case got, ok := <-events:
		if !ok {
			t.Fatal("the websocket closed")
		}
		if got != want {
			t.Errorf("event = %q, want %q", got, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("no %s event within two seconds", want)
	}
}

func expectNoEvent(t *testing.T, events <-chan string) {
	t.Helper()
	select {
	case got := <-events:
		t.Fatalf("unexpected %s event", got)
	case <-time.After(time.Second):
	}
}

func TestEventsFollowTheFilesButNotTheApi(t *testing.T) {
	dir, front, studio := harness(t)
	events := listen(t, front, studio)

	outside := exampleProject()
	outside["name"] = "outside"
	writeDoc(t, workspace.ProjectPath(dir), outside)
	expectEvent(t, events, "project-changed")

	own := exampleProject()
	own["name"] = "market"
	body := marshal(t, own)
	if code, raw := send(t, front, http.MethodPut, "/api/project", body); code != http.StatusNoContent {
		t.Fatalf("put: code = %d, body = %s", code, raw)
	}
	expectNoEvent(t, events)

	if err := workspace.WriteRaw(workspace.ProjectPath(dir), body); err != nil {
		t.Fatal(err)
	}
	expectNoEvent(t, events)
}

// Editors save by writing a temp file and renaming it over the target;
// the watcher follows the directory rather than the two files precisely
// so this still fires a single event.
func TestRenameOverwriteFiresOneChangeEvent(t *testing.T) {
	dir, front, studio := harness(t)
	events := listen(t, front, studio)

	saved := exampleProject()
	saved["name"] = "saved-by-editor"
	tmp := filepath.Join(workspace.TogenDir(dir), "project.json.tmp")
	if err := os.WriteFile(tmp, marshal(t, saved), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, workspace.ProjectPath(dir)); err != nil {
		t.Fatal(err)
	}

	expectEvent(t, events, "project-changed")
	expectNoEvent(t, events)
}

func TestEventsFollowTheConfig(t *testing.T) {
	dir, front, studio := harness(t)
	events := listen(t, front, studio)

	writeConfig(t, dir, "targets: [hcl]\noutDir: build\n")
	expectEvent(t, events, "config-changed")

	writeConfig(t, dir, "targets: [hcl]\noutDir: build\n")
	expectNoEvent(t, events)
}

func TestEventsFollowTheLayout(t *testing.T) {
	dir, front, studio := harness(t)
	events := listen(t, front, studio)

	writeDoc(t, workspace.LayoutPath(dir), overviewLayout(
		map[string]any{"n1": map[string]any{"x": 10, "y": 20}},
		map[string]any{"x": 0, "y": 0, "zoom": 1},
	))
	expectEvent(t, events, "layout-changed")
}

func TestEventsFollowTheViewsButNotTheApi(t *testing.T) {
	dir, front, studio := harness(t)
	events := listen(t, front, studio)

	writeDoc(t, workspace.ViewsPath(dir), exampleViews())
	expectEvent(t, events, "views-changed")

	own := exampleViews()
	own["views"].([]any)[1].(map[string]any)["name"] = "Orders"
	if code, raw := send(t, front, http.MethodPut, "/api/views", marshal(t, own)); code != http.StatusNoContent {
		t.Fatalf("put: code = %d, body = %s", code, raw)
	}
	expectNoEvent(t, events)
}
