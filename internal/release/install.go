package release

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Method int

const (
	MethodUnknown Method = iota
	MethodManaged
	MethodHomebrew
	MethodNix
)

func (m Method) String() string {
	switch m {
	case MethodManaged:
		return "managed"
	case MethodHomebrew:
		return "homebrew"
	case MethodNix:
		return "nix"
	default:
		return "unknown"
	}
}

func (m Method) UpgradeCommand() string {
	switch m {
	case MethodHomebrew:
		return "brew upgrade mooncitizen/tap/togen"
	case MethodNix:
		return "nix flake update togen"
	default:
		return "togen upgrade"
	}
}

func Detect() (Method, string, error) {
	exe, err := os.Executable()
	if err != nil {
		return MethodUnknown, "", err
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		resolved = exe
	}
	return classify(resolved, brewCellar), resolved, nil
}

func classify(path string, cellar func() string) Method {
	if strings.HasPrefix(path, "/nix/store/") {
		return MethodNix
	}
	if strings.Contains(path, string(os.PathSeparator)+filepath.Join("Cellar", "togen")+string(os.PathSeparator)) {
		return MethodHomebrew
	}
	if prefix := cellar(); prefix != "" && strings.HasPrefix(path, prefix+string(os.PathSeparator)) {
		return MethodHomebrew
	}
	if writable(filepath.Dir(path)) {
		return MethodManaged
	}
	return MethodUnknown
}

// brew --prefix togen, not brew --prefix: the formula's own path never
// collides with an install script's, while the bare prefix is often
// /usr/local and would swallow it.
func brewCellar() string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "brew", "--prefix", "togen").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func writable(dir string) bool {
	probe, err := os.CreateTemp(dir, ".togen-write-check-*")
	if err != nil {
		return false
	}
	name := probe.Name()
	_ = probe.Close()
	_ = os.Remove(name)
	return true
}
