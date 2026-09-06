package azure

import (
	"regexp"
	"testing"

	"github.com/mooncitizen/togen/internal/resolve"
)

var storageAccountNamePattern = regexp.MustCompile(`^[a-z0-9]{3,24}$`)

func TestStorageAccountNameDropsTheHyphens(t *testing.T) {
	ctx := newContext(t, nil)
	if got := storageAccountName(ctx, "user-uploads"); got != "shopdevuseruploads" {
		t.Errorf("name = %q", got)
	}
}

func TestStorageAccountNameCutsALongProjectNameFirst(t *testing.T) {
	p := newProject(t, nil)
	p.Name = "a-project-name-that-is-long-too"
	got := storageAccountName(resolve.NewContext(p), "uploads")
	if got != "aprojectnamethdevuploads" {
		t.Errorf("name = %q", got)
	}
	if !storageAccountNamePattern.MatchString(got) {
		t.Errorf("name %q is not 3 to 24 lowercase alphanumerics", got)
	}
}

func TestStorageAccountNameCutsTheNodeOnceTheProjectIsGone(t *testing.T) {
	p := newProject(t, nil)
	p.Environment = "production-like"
	got := storageAccountName(resolve.NewContext(p), "a-very-long-node-name-for-uploads")
	if got != "productionlikeaverylongn" {
		t.Errorf("name = %q", got)
	}
	if !storageAccountNamePattern.MatchString(got) {
		t.Errorf("name %q is not 3 to 24 lowercase alphanumerics", got)
	}
}
