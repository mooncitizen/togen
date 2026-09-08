package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

func sampleVersionInfo() VersionInfo {
	return VersionInfo{
		Version:  "0.2.0",
		Commit:   "a1b2c3d",
		Date:     "2026-09-07T10:04:11Z",
		Go:       "go1.24.4",
		Platform: "darwin/arm64",
		Install:  "homebrew",
		Path:     "/opt/homebrew/Cellar/togen/0.2.0/bin/togen",
	}
}

func TestVersionPrintsEveryField(t *testing.T) {
	result := Version(sampleVersionInfo(), false)
	if result.Code != 0 {
		t.Fatalf("code = %d", result.Code)
	}
	joined := strings.Join(result.Lines, "\n")
	for _, want := range []string{"togen 0.2.0", "a1b2c3d", "2026-09-07T10:04:11Z", "go1.24.4 darwin/arm64", "homebrew", "/opt/homebrew/Cellar/togen/0.2.0/bin/togen"} {
		if !strings.Contains(joined, want) {
			t.Errorf("output does not contain %q:\n%s", want, joined)
		}
	}
}

func TestVersionJSONCarriesTheSameFields(t *testing.T) {
	result := Version(sampleVersionInfo(), true)
	var got map[string]string
	if err := json.Unmarshal([]byte(strings.Join(result.Lines, "\n")), &got); err != nil {
		t.Fatal(err)
	}
	if got["version"] != "0.2.0" || got["install"] != "homebrew" || got["commit"] != "a1b2c3d" {
		t.Errorf("json = %v", got)
	}
}
