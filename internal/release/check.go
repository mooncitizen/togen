package release

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const checkInterval = 24 * time.Hour

type Notice struct {
	Current string
	Latest  string
	Method  Method
}

func (n Notice) Lines() []string {
	return []string{
		fmt.Sprintf("A new version of togen is available: %s -> %s", Normalise(n.Current), Normalise(n.Latest)),
		fmt.Sprintf("Run `%s` to update, or set TOGEN_NO_UPDATE_CHECK=1 to stop checking.", n.Method.UpgradeCommand()),
	}
}

type checkState struct {
	CheckedAt time.Time `json:"checkedAt"`
	Latest    string    `json:"latest"`
}

func CachePath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "togen", "version-check.json"), nil
}

func Check(ctx context.Context, client Client, cachePath, current string, method func() Method, now time.Time) *Notice {
	state, fresh := readCheckState(cachePath, now)
	if !fresh {
		rel, err := client.Latest(ctx)
		if err == nil {
			state.Latest = rel.Tag
		}
		state.CheckedAt = now
		writeCheckState(cachePath, state)
	}
	if !Newer(current, state.Latest) {
		return nil
	}
	return &Notice{Current: current, Latest: state.Latest, Method: method()}
}

func readCheckState(path string, now time.Time) (checkState, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return checkState{}, false
	}
	var state checkState
	if err := json.Unmarshal(raw, &state); err != nil {
		return checkState{}, false
	}
	age := now.Sub(state.CheckedAt)
	return state, age >= 0 && age < checkInterval
}

// A failure to write the cache is ignored on purpose: a read-only cache
// directory should cost a user nothing but an extra request a day.
func writeCheckState(path string, state checkState) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return
	}
	_ = os.WriteFile(path, raw, 0o600)
}
