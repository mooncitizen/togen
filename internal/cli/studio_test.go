package cli

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestStudioServesTheApiUntilTheContextIsCancelled(t *testing.T) {
	cwd := t.TempDir()
	if result := Init(cwd, "", "shop"); result.Code != 0 {
		t.Fatalf("init: %v", result.Lines)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	urls := make(chan string, 1)
	stopped := make(chan error, 1)
	go func() {
		stopped <- Studio(ctx, cwd, 0, false, func(url string) { urls <- url })
	}()

	var url string
	select {
	case url = <-urls:
	case err := <-stopped:
		t.Fatalf("studio stopped: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("studio never became ready")
	}

	response, err := http.Get(url + "/api/project")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		t.Errorf("code = %d, want 200", response.StatusCode)
	}

	cancel()
	select {
	case err := <-stopped:
		if err != nil {
			t.Errorf("studio: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("studio did not stop")
	}
}

func TestStudioRefusesWithoutAProject(t *testing.T) {
	err := Studio(context.Background(), t.TempDir(), 0, false, nil)
	if err == nil {
		t.Fatal("want an error")
	}
	if want := "togen/project.json not found. Run 'togen init' first."; err.Error() != want {
		t.Errorf("error = %q, want %q", err, want)
	}
}
