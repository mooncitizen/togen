package release

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

const repository = "mooncitizen/togen"

type Release struct {
	Tag    string
	Assets map[string]string
}

type Client interface {
	Latest(ctx context.Context) (Release, error)
	Get(ctx context.Context, tag string) (Release, error)
	Download(ctx context.Context, url string) (io.ReadCloser, error)
}

func Normalise(v string) string {
	if v == "" || strings.HasPrefix(v, "v") {
		return v
	}
	return "v" + v
}

func Comparable(v string) bool {
	return semver.IsValid(Normalise(v))
}

func Newer(current, latest string) bool {
	if !Comparable(current) || !Comparable(latest) {
		return false
	}
	return semver.Compare(Normalise(current), Normalise(latest)) < 0
}

func AssetName(goos, goarch string) string {
	return fmt.Sprintf("togen_%s_%s.tar.gz", goos, goarch)
}

type githubClient struct {
	base string
	http *http.Client
}

func NewClient() Client {
	return newClientAt("https://api.github.com")
}

func newClientAt(base string) *githubClient {
	return &githubClient{base: base, http: &http.Client{}}
}

func (c *githubClient) Latest(ctx context.Context) (Release, error) {
	return c.fetch(ctx, c.base+"/repos/"+repository+"/releases/latest")
}

func (c *githubClient) Get(ctx context.Context, tag string) (Release, error) {
	return c.fetch(ctx, c.base+"/repos/"+repository+"/releases/tags/"+Normalise(tag))
}

func (c *githubClient) fetch(ctx context.Context, url string) (Release, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := c.http.Do(req)
	if err != nil {
		return Release{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("github answered %s for %s", resp.Status, url)
	}
	var body struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return Release{}, err
	}
	rel := Release{Tag: body.TagName, Assets: make(map[string]string, len(body.Assets))}
	for _, asset := range body.Assets {
		rel.Assets[asset.Name] = asset.URL
	}
	return rel, nil
}

// The archive is around 10MB, so the request needs a much longer budget than
// the 3 second metadata calls in fetch. That budget has to cover the body
// read too, which happens after this method returns, so the cancel func is
// not deferred here: it is handed to the returned reader and fires when the
// caller closes it, not when Download itself returns.
const downloadTimeout = 10 * time.Minute

func (c *githubClient) Download(ctx context.Context, url string) (io.ReadCloser, error) {
	ctx, cancel := context.WithTimeout(ctx, downloadTimeout)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		cancel()
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		cancel()
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		cancel()
		return nil, fmt.Errorf("github answered %s for %s", resp.Status, url)
	}
	return &cancelOnClose{ReadCloser: resp.Body, cancel: cancel}, nil
}

type cancelOnClose struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (c *cancelOnClose) Close() error {
	defer c.cancel()
	return c.ReadCloser.Close()
}
