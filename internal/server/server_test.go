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

func harness(t *testing.T) (string, *httptest.Server, *Server) {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(workspace.TogenDir(dir), 0o755); err != nil {
		t.Fatal(err)
	}
	writeDoc(t, workspace.ProjectPath(dir), exampleProject())
	writeDoc(t, workspace.LayoutPath(dir), map[string]any{
		"version":  1,
		"nodes":    map[string]any{},
		"viewport": map[string]any{"x": 0, "y": 0, "zoom": 1},
	})
	if err := workspace.WriteJSONFile(workspace.ConfigPath(dir), workspace.DefaultConfig()); err != nil {
		t.Fatal(err)
	}

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

func TestLayoutRoundTrips(t *testing.T) {
	_, front, _ := harness(t)
	layout := map[string]any{
		"version":  1,
		"nodes":    map[string]any{"n1": map[string]any{"x": 40, "y": 80}},
		"viewport": map[string]any{"x": 0, "y": 0, "zoom": 1.5},
	}
	if code, raw := send(t, front, http.MethodPut, "/api/layout", marshal(t, layout)); code != http.StatusNoContent {
		t.Fatalf("put: code = %d, body = %s", code, raw)
	}
	code, raw := send(t, front, http.MethodGet, "/api/layout", nil)
	if code != http.StatusOK {
		t.Fatalf("get: code = %d, body = %s", code, raw)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(map[string]any{
		"version":  float64(1),
		"nodes":    map[string]any{"n1": map[string]any{"x": float64(40), "y": float64(80)}},
		"viewport": map[string]any{"x": float64(0), "y": float64(0), "zoom": 1.5},
	}, got); diff != "" {
		t.Errorf("layout (-want +got):\n%s", diff)
	}
}

func TestPutLayoutRefusesAnotherVersion(t *testing.T) {
	_, front, _ := harness(t)
	code, raw := send(t, front, http.MethodPut, "/api/layout", []byte(`{"version":2}`))
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("code = %d, body = %s", code, raw)
	}
}

func TestGetConfigReturnsTheTargets(t *testing.T) {
	_, front, _ := harness(t)
	code, raw := send(t, front, http.MethodGet, "/api/config", nil)
	if code != http.StatusOK {
		t.Fatalf("code = %d, body = %s", code, raw)
	}
	var config workspace.Config
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(workspace.DefaultConfig(), config); diff != "" {
		t.Errorf("config (-want +got):\n%s", diff)
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

func TestEventsFollowTheLayout(t *testing.T) {
	dir, front, studio := harness(t)
	events := listen(t, front, studio)

	writeDoc(t, workspace.LayoutPath(dir), map[string]any{
		"version":  1,
		"nodes":    map[string]any{"n1": map[string]any{"x": 10, "y": 20}},
		"viewport": map[string]any{"x": 0, "y": 0, "zoom": 1},
	})
	expectEvent(t, events, "layout-changed")
}
