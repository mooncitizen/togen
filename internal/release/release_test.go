package release

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewerComparesRegardlessOfTheLeadingV(t *testing.T) {
	cases := []struct {
		current, latest string
		want            bool
	}{
		{"0.1.0", "v0.2.0", true},
		{"v0.1.0", "0.2.0", true},
		{"0.2.0", "v0.2.0", false},
		{"0.3.0", "v0.2.0", false},
		{"0.2.0", "v0.2.1", true},
		{"1.0.0-rc.1", "v1.0.0", true},
		{"dev", "v0.2.0", false},
		{"0.1.0", "nightly", false},
		{"", "v0.2.0", false},
	}
	for _, c := range cases {
		if got := Newer(c.current, c.latest); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.current, c.latest, got, c.want)
		}
	}
}

func TestComparableRejectsDevelopmentBuilds(t *testing.T) {
	for _, v := range []string{"0.1.0", "v0.1.0", "v1.2.3-rc.1"} {
		if !Comparable(v) {
			t.Errorf("Comparable(%q) = false, want true", v)
		}
	}
	for _, v := range []string{"dev", "", "not-a-version"} {
		if Comparable(v) {
			t.Errorf("Comparable(%q) = true, want false", v)
		}
	}
}

func TestAssetNameMatchesTheGoreleaserTemplate(t *testing.T) {
	if got := AssetName("darwin", "arm64"); got != "togen_darwin_arm64.tar.gz" {
		t.Errorf("AssetName = %q", got)
	}
	if got := AssetName("linux", "amd64"); got != "togen_linux_amd64.tar.gz" {
		t.Errorf("AssetName = %q", got)
	}
}

func stubGitHub(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	body := `{"tag_name":"v0.2.0","assets":[
		{"name":"togen_darwin_arm64.tar.gz","browser_download_url":"https://example.test/a.tar.gz"},
		{"name":"checksums.txt","browser_download_url":"https://example.test/checksums.txt"}]}`
	mux.HandleFunc("/repos/mooncitizen/togen/releases/latest", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, body)
	})
	mux.HandleFunc("/repos/mooncitizen/togen/releases/tags/v0.1.0", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"tag_name":"v0.1.0","assets":[]}`)
	})
	mux.HandleFunc("/download", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "payload")
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func TestLatestReadsTheTagAndItsAssets(t *testing.T) {
	server := stubGitHub(t)
	rel, err := newClientAt(server.URL).Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rel.Tag != "v0.2.0" {
		t.Errorf("tag = %q", rel.Tag)
	}
	if rel.Assets["checksums.txt"] != "https://example.test/checksums.txt" {
		t.Errorf("assets = %v", rel.Assets)
	}
}

func TestGetNormalisesTheTag(t *testing.T) {
	server := stubGitHub(t)
	rel, err := newClientAt(server.URL).Get(context.Background(), "0.1.0")
	if err != nil {
		t.Fatal(err)
	}
	if rel.Tag != "v0.1.0" {
		t.Errorf("tag = %q", rel.Tag)
	}
}

func TestLatestReportsANonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(server.Close)
	if _, err := newClientAt(server.URL).Latest(context.Background()); err == nil {
		t.Fatal("want an error for a 403")
	}
}

func TestDownloadReturnsTheBody(t *testing.T) {
	server := stubGitHub(t)
	body, err := newClientAt(server.URL).Download(context.Background(), server.URL+"/download")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = body.Close() }()
	raw, err := io.ReadAll(body)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "payload" {
		t.Errorf("body = %q", raw)
	}
}
