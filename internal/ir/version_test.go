package ir

import "testing"

func TestVersion(t *testing.T) {
	if Version != 1 {
		t.Fatalf("Version = %d, want 1", Version)
	}
}
