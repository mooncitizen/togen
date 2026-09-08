package release

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type countingClient struct {
	tag   string
	err   error
	calls int
}

func (c *countingClient) Latest(context.Context) (Release, error) {
	c.calls++
	if c.err != nil {
		return Release{}, c.err
	}
	return Release{Tag: c.tag}, nil
}

func (c *countingClient) Get(context.Context, string) (Release, error) { return Release{}, nil }

func (c *countingClient) Download(context.Context, string) (io.ReadCloser, error) {
	return nil, nil
}

func cacheIn(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "togen", "version-check.json")
}

func TestCheckFetchesAndReportsANewerRelease(t *testing.T) {
	client := &countingClient{tag: "v0.3.0"}
	path := cacheIn(t)
	notice := Check(context.Background(), client, path, "0.2.0", time.Now())
	if notice == nil {
		t.Fatal("want a notice")
	}
	if notice.Latest != "v0.3.0" || notice.Current != "0.2.0" {
		t.Errorf("notice = %+v", notice)
	}
	if client.calls != 1 {
		t.Errorf("calls = %d, want 1", client.calls)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("cache was not written: %v", err)
	}
}

func TestCheckIsSilentWhenTheLatestIsNotNewer(t *testing.T) {
	client := &countingClient{tag: "v0.2.0"}
	if notice := Check(context.Background(), client, cacheIn(t), "0.2.0", time.Now()); notice != nil {
		t.Errorf("notice = %+v, want nil", notice)
	}
}

func TestCheckUsesTheCacheWithinADay(t *testing.T) {
	path := cacheIn(t)
	now := time.Now()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(map[string]any{"checkedAt": now.Add(-time.Hour), "latest": "v0.3.0"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	client := &countingClient{tag: "v0.9.0"}
	notice := Check(context.Background(), client, path, "0.2.0", now)
	if notice == nil || notice.Latest != "v0.3.0" {
		t.Fatalf("notice = %+v, want the cached v0.3.0", notice)
	}
	if client.calls != 0 {
		t.Errorf("calls = %d, want 0 for a fresh cache", client.calls)
	}
}

func TestCheckRefetchesAfterADay(t *testing.T) {
	path := cacheIn(t)
	now := time.Now()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(map[string]any{"checkedAt": now.Add(-25 * time.Hour), "latest": "v0.3.0"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	client := &countingClient{tag: "v0.9.0"}
	notice := Check(context.Background(), client, path, "0.2.0", now)
	if notice == nil || notice.Latest != "v0.9.0" {
		t.Fatalf("notice = %+v, want the refetched v0.9.0", notice)
	}
	if client.calls != 1 {
		t.Errorf("calls = %d, want 1", client.calls)
	}
}

func TestCheckRefetchesWhenTheCacheIsStampedInTheFuture(t *testing.T) {
	path := cacheIn(t)
	now := time.Now()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(map[string]any{"checkedAt": now.Add(time.Hour), "latest": "v0.3.0"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	client := &countingClient{tag: "v0.9.0"}
	notice := Check(context.Background(), client, path, "0.2.0", now)
	if notice == nil || notice.Latest != "v0.9.0" {
		t.Fatalf("notice = %+v, want the refetched v0.9.0", notice)
	}
	if client.calls != 1 {
		t.Errorf("calls = %d, want 1 for a cache stamped in the future", client.calls)
	}
}

func TestCheckTreatsAFailedRequestAsNoInformation(t *testing.T) {
	client := &countingClient{err: context.DeadlineExceeded}
	path := cacheIn(t)
	if notice := Check(context.Background(), client, path, "0.2.0", time.Now()); notice != nil {
		t.Errorf("notice = %+v, want nil", notice)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("the cache must still be stamped so the next run does not retry: %v", err)
	}
}

func TestCheckIgnoresAnUnreadableCache(t *testing.T) {
	path := cacheIn(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	client := &countingClient{tag: "v0.3.0"}
	if notice := Check(context.Background(), client, path, "0.2.0", time.Now()); notice == nil {
		t.Fatal("want a notice after a corrupt cache is discarded")
	}
	if client.calls != 1 {
		t.Errorf("calls = %d, want 1", client.calls)
	}
}

func TestNoticeLinesNameTheInstallMethodsCommand(t *testing.T) {
	lines := Notice{Current: "0.2.0", Latest: "v0.3.0", Method: MethodHomebrew}.Lines()
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "v0.2.0 -> v0.3.0") {
		t.Errorf("lines = %q", joined)
	}
	if !strings.Contains(joined, "brew upgrade mooncitizen/tap/togen") {
		t.Errorf("lines = %q", joined)
	}
	if !strings.Contains(joined, "TOGEN_NO_UPDATE_CHECK") {
		t.Errorf("lines = %q", joined)
	}
}
