package cli

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"
)

func startStudio(t *testing.T, cwd string) (string, func() error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	urls := make(chan string, 1)
	stopped := make(chan error, 1)
	go func() {
		stopped <- Studio(ctx, cwd, 0, false, func(url string) { urls <- url })
	}()

	var url string
	select {
	case url = <-urls:
	case err := <-stopped:
		cancel()
		t.Fatalf("studio stopped: %v", err)
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("studio never became ready")
	}
	var once sync.Once
	var result error
	stop := func() error {
		once.Do(func() {
			cancel()
			select {
			case result = <-stopped:
			case <-time.After(5 * time.Second):
				t.Fatal("studio did not stop")
			}
		})
		return result
	}
	t.Cleanup(func() { _ = stop() })
	return url, stop
}

func statusOf(t *testing.T, url string) int {
	t.Helper()
	response, err := http.Get(url)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	return response.StatusCode
}

func TestStudioServesTheApiUntilTheContextIsCancelled(t *testing.T) {
	cwd := t.TempDir()
	if result := Init(cwd, "", "shop", false); result.Code != 0 {
		t.Fatalf("init: %v", result.Lines)
	}
	url, stop := startStudio(t, cwd)
	if code := statusOf(t, url+"/api/project"); code != http.StatusOK {
		t.Errorf("code = %d, want 200", code)
	}
	if err := stop(); err != nil {
		t.Errorf("studio: %v", err)
	}
}

func TestStudioServesWithoutAProject(t *testing.T) {
	url, _ := startStudio(t, t.TempDir())
	if code := statusOf(t, url+"/api/project"); code != http.StatusNotFound {
		t.Errorf("code = %d, want 404", code)
	}
	if code := statusOf(t, url+"/api/examples"); code != http.StatusOK {
		t.Errorf("examples: code = %d, want 200", code)
	}
}
