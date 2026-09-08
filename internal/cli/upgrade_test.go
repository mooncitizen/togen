package cli

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/mooncitizen/togen/internal/release"
)

type stubClient struct {
	tag   string
	calls int
}

func (s *stubClient) Latest(context.Context) (release.Release, error) {
	s.calls++
	return release.Release{Tag: s.tag, Assets: map[string]string{}}, nil
}

func (s *stubClient) Get(_ context.Context, tag string) (release.Release, error) {
	s.calls++
	return release.Release{Tag: release.Normalise(tag), Assets: map[string]string{}}, nil
}

func (s *stubClient) Download(context.Context, string) (io.ReadCloser, error) {
	return nil, nil
}

func upgradeOutput(result Result) string {
	return strings.Join(result.Lines, "\n")
}

func TestUpgradeRefusesAHomebrewInstall(t *testing.T) {
	client := &stubClient{tag: "v0.3.0"}
	result := Upgrade(context.Background(), client, UpgradeOptions{
		Current: "0.2.0",
		Method:  release.MethodHomebrew,
		Path:    "/opt/homebrew/Cellar/togen/0.2.0/bin/togen",
	})
	if result.Code != 0 {
		t.Errorf("code = %d, want 0", result.Code)
	}
	if !strings.Contains(upgradeOutput(result), "brew upgrade mooncitizen/tap/togen") {
		t.Errorf("output = %q", upgradeOutput(result))
	}
	if client.calls != 0 {
		t.Errorf("client was called %d times, want 0", client.calls)
	}
}

func TestUpgradeRefusesANixInstall(t *testing.T) {
	client := &stubClient{tag: "v0.3.0"}
	result := Upgrade(context.Background(), client, UpgradeOptions{
		Current: "0.2.0",
		Method:  release.MethodNix,
		Path:    "/nix/store/abc-togen-0.2.0/bin/togen",
	})
	if !strings.Contains(upgradeOutput(result), "nix flake update togen") {
		t.Errorf("output = %q", upgradeOutput(result))
	}
	if client.calls != 0 {
		t.Errorf("client was called %d times, want 0", client.calls)
	}
}

func TestUpgradeRefusesAnUnknownInstall(t *testing.T) {
	client := &stubClient{tag: "v0.3.0"}
	result := Upgrade(context.Background(), client, UpgradeOptions{
		Current: "0.2.0",
		Method:  release.MethodUnknown,
		Path:    "/usr/bin/togen",
	})
	if !strings.Contains(upgradeOutput(result), "/usr/bin/togen") {
		t.Errorf("output = %q", upgradeOutput(result))
	}
	if client.calls != 0 {
		t.Errorf("client was called %d times, want 0", client.calls)
	}
}

func TestUpgradeCheckReportsWithoutActing(t *testing.T) {
	client := &stubClient{tag: "v0.3.0"}
	result := Upgrade(context.Background(), client, UpgradeOptions{
		Current: "0.2.0",
		Check:   true,
		Method:  release.MethodManaged,
		Path:    "/home/paul/.local/bin/togen",
	})
	if result.Code != 0 {
		t.Errorf("code = %d", result.Code)
	}
	out := upgradeOutput(result)
	if !strings.Contains(out, "v0.2.0") || !strings.Contains(out, "v0.3.0") {
		t.Errorf("output = %q", out)
	}
	if client.calls != 1 {
		t.Errorf("client was called %d times, want 1", client.calls)
	}
}

func TestUpgradeCheckOnTheLatestSaysSo(t *testing.T) {
	client := &stubClient{tag: "v0.2.0"}
	result := Upgrade(context.Background(), client, UpgradeOptions{
		Current: "0.2.0",
		Check:   true,
		Method:  release.MethodManaged,
		Path:    "/home/paul/.local/bin/togen",
	})
	if !strings.Contains(upgradeOutput(result), "latest") {
		t.Errorf("output = %q", upgradeOutput(result))
	}
}

func TestUpgradeStopsWhenAlreadyOnTheLatest(t *testing.T) {
	client := &stubClient{tag: "v0.2.0"}
	result := Upgrade(context.Background(), client, UpgradeOptions{
		Current: "0.2.0",
		Yes:     true,
		Method:  release.MethodManaged,
		Path:    "/home/paul/.local/bin/togen",
	})
	if result.Code != 0 {
		t.Errorf("code = %d", result.Code)
	}
	if !strings.Contains(upgradeOutput(result), "latest") {
		t.Errorf("output = %q", upgradeOutput(result))
	}
}

func TestUpgradeRefusesWithoutATerminalOrYes(t *testing.T) {
	client := &stubClient{tag: "v0.3.0"}
	result := Upgrade(context.Background(), client, UpgradeOptions{
		Current: "0.2.0",
		Method:  release.MethodManaged,
		Path:    "/home/paul/.local/bin/togen",
	})
	if result.Code == 0 {
		t.Error("want a non-zero code when there is no terminal and no --yes")
	}
	if !strings.Contains(upgradeOutput(result), "--yes") {
		t.Errorf("output = %q", upgradeOutput(result))
	}
}

func TestUpgradeStopsWhenTheConfirmationIsDeclined(t *testing.T) {
	client := &stubClient{tag: "v0.3.0"}
	result := Upgrade(context.Background(), client, UpgradeOptions{
		Current: "0.2.0",
		Method:  release.MethodManaged,
		Path:    "/home/paul/.local/bin/togen",
		Confirm: func(string) bool { return false },
	})
	if result.Code != 0 {
		t.Errorf("code = %d, want 0 for a declined confirmation", result.Code)
	}
	if !strings.Contains(upgradeOutput(result), "nothing changed") {
		t.Errorf("output = %q", upgradeOutput(result))
	}
}

func TestUpgradeOnADevBuildDoesNotClaimToBeLatest(t *testing.T) {
	client := &stubClient{tag: "v0.3.0"}
	result := Upgrade(context.Background(), client, UpgradeOptions{
		Current: "dev",
		Method:  release.MethodManaged,
		Path:    "/home/paul/.local/bin/togen",
		Confirm: func(string) bool { return false },
	})
	if strings.Contains(upgradeOutput(result), "latest") {
		t.Errorf("output = %q, a dev build must not be called the latest", upgradeOutput(result))
	}
}
