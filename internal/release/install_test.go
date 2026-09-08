package release

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClassifyRecognisesNixAndHomebrew(t *testing.T) {
	cases := []struct {
		name, path, cellar string
		want               Method
	}{
		{"nix store", "/nix/store/abc123-togen-0.2.0/bin/togen", "", MethodNix},
		{"homebrew cellar", "/opt/homebrew/Cellar/togen/0.2.0/bin/togen", "", MethodHomebrew},
		{"intel cellar", "/usr/local/Cellar/togen/0.2.0/bin/togen", "", MethodHomebrew},
		{"brew reported prefix", "/custom/brew/togen/0.2.0/bin/togen", "/custom/brew/togen/0.2.0", MethodHomebrew},
		{"cellar for another formula", "/nonexistent-prefix/Cellar/jq/1.7/bin/togen", "", MethodUnknown},
	}
	for _, c := range cases {
		if got := classify(c.path, func() string { return c.cellar }); got != c.want {
			t.Errorf("%s: classify(%q, %q) = %v, want %v", c.name, c.path, c.cellar, got, c.want)
		}
	}
}

func TestClassifyCallsAWritableDirectoryManaged(t *testing.T) {
	dir := t.TempDir()
	if got := classify(filepath.Join(dir, "togen"), func() string { return "" }); got != MethodManaged {
		t.Errorf("classify = %v, want MethodManaged", got)
	}
}

func TestClassifyCallsAnUnwritableDirectoryUnknown(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write anywhere")
	}
	dir := t.TempDir()
	locked := filepath.Join(dir, "locked")
	if err := os.Mkdir(locked, 0o555); err != nil {
		t.Fatal(err)
	}
	if got := classify(filepath.Join(locked, "togen"), func() string { return "" }); got != MethodUnknown {
		t.Errorf("classify = %v, want MethodUnknown", got)
	}
}

func TestUpgradeCommandNamesTheRightManager(t *testing.T) {
	cases := map[Method]string{
		MethodManaged:  "togen upgrade",
		MethodUnknown:  "togen upgrade",
		MethodHomebrew: "brew upgrade mooncitizen/tap/togen",
		MethodNix:      "nix flake update togen",
	}
	for method, want := range cases {
		if got := method.UpgradeCommand(); got != want {
			t.Errorf("%v.UpgradeCommand() = %q, want %q", method, got, want)
		}
	}
}

func TestDetectResolvesTheRunningBinary(t *testing.T) {
	method, path, err := Detect()
	if err != nil {
		t.Fatal(err)
	}
	if path == "" {
		t.Error("path is empty")
	}
	if method == MethodNix && !filepath.IsAbs(path) {
		t.Errorf("path %q is not absolute", path)
	}
}

func TestClassifyDoesNotAskBrewWhenThePathAlreadyDecides(t *testing.T) {
	asked := false
	probe := func() string {
		asked = true
		return ""
	}
	for _, path := range []string{"/nix/store/abc123-togen-0.2.0/bin/togen", "/opt/homebrew/Cellar/togen/0.2.0/bin/togen"} {
		asked = false
		if got := classify(path, probe); got == MethodUnknown {
			t.Errorf("classify(%q) = MethodUnknown", got)
		}
		if asked {
			t.Error("classify shelled out to brew for a path that already decides the method")
		}
	}
}
