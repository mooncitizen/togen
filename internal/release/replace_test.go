package release

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeClient struct {
	rel    Release
	bodies map[string]string
	calls  int
}

func (f *fakeClient) Latest(context.Context) (Release, error) {
	f.calls++
	return f.rel, nil
}

func (f *fakeClient) Get(_ context.Context, tag string) (Release, error) {
	f.calls++
	rel := f.rel
	rel.Tag = Normalise(tag)
	return rel, nil
}

func (f *fakeClient) Download(_ context.Context, url string) (io.ReadCloser, error) {
	body, ok := f.bodies[url]
	if !ok {
		return nil, fmt.Errorf("no body for %s", url)
	}
	return io.NopCloser(strings.NewReader(body)), nil
}

func archiveWith(t *testing.T, entries map[string]string) string {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range entries {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(tw, content); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func digest(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func clientFor(t *testing.T, archive, sums string) *fakeClient {
	t.Helper()
	return &fakeClient{
		rel: Release{Tag: "v0.2.0", Assets: map[string]string{
			AssetName("linux", "amd64"): "https://example.test/archive",
			"checksums.txt":             "https://example.test/sums",
		}},
		bodies: map[string]string{
			"https://example.test/archive": archive,
			"https://example.test/sums":    sums,
		},
	}
}

func targetIn(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "togen")
	if err := os.WriteFile(path, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReplaceInstallsTheNewBinary(t *testing.T) {
	archive := archiveWith(t, map[string]string{"togen": "new binary", "LICENSE": "apache"})
	sums := digest(archive) + "  " + AssetName("linux", "amd64") + "\n"
	client := clientFor(t, archive, sums)
	target := targetIn(t)

	if err := Replace(context.Background(), client, client.rel, target, "linux", "amd64"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new binary" {
		t.Errorf("target = %q", got)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("mode = %v, want it executable", info.Mode())
	}
	leftovers(t, filepath.Dir(target))
}

func TestReplaceLeavesTheOriginalWhenTheDigestIsWrong(t *testing.T) {
	archive := archiveWith(t, map[string]string{"togen": "new binary"})
	sums := digest("something else") + "  " + AssetName("linux", "amd64") + "\n"
	client := clientFor(t, archive, sums)
	target := targetIn(t)

	if err := Replace(context.Background(), client, client.rel, target, "linux", "amd64"); err == nil {
		t.Fatal("want an error for a mismatched digest")
	}
	assertUnchanged(t, target)
	leftovers(t, filepath.Dir(target))
}

func TestReplaceLeavesTheOriginalWhenTheChecksumEntryIsMissing(t *testing.T) {
	archive := archiveWith(t, map[string]string{"togen": "new binary"})
	client := clientFor(t, archive, digest(archive)+"  togen_darwin_arm64.tar.gz\n")
	target := targetIn(t)

	if err := Replace(context.Background(), client, client.rel, target, "linux", "amd64"); err == nil {
		t.Fatal("want an error when the asset has no checksum line")
	}
	assertUnchanged(t, target)
}

func TestReplaceRejectsAnArchiveWithoutATogenEntry(t *testing.T) {
	archive := archiveWith(t, map[string]string{"togen-helper": "not it", "LICENSE": "apache"})
	sums := digest(archive) + "  " + AssetName("linux", "amd64") + "\n"
	client := clientFor(t, archive, sums)
	target := targetIn(t)

	if err := Replace(context.Background(), client, client.rel, target, "linux", "amd64"); err == nil {
		t.Fatal("want an error when the archive has no togen entry")
	}
	assertUnchanged(t, target)
}

func TestReplaceReportsAMissingAsset(t *testing.T) {
	client := clientFor(t, "", "")
	target := targetIn(t)
	if err := Replace(context.Background(), client, client.rel, target, "windows", "amd64"); err == nil {
		t.Fatal("want an error when the release has no asset for the platform")
	}
	assertUnchanged(t, target)
}

func assertUnchanged(t *testing.T, target string) {
	t.Helper()
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "old binary" {
		t.Errorf("target = %q, want it untouched", got)
	}
}

func leftovers(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".togen-") {
			t.Errorf("temporary file left behind: %s", entry.Name())
		}
	}
}
