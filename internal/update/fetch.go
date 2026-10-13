package update

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"time"
)

// Size caps: a release archive is a few tens of MB; checksums.txt is tiny.
const (
	maxArchiveBytes   = 256 << 20
	maxChecksumsBytes = 1 << 20
	clientTimeout     = 2 * time.Minute
)

var stableTag = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`)

// newClient is the HTTP client for release downloads: every redirect hop is
// held to CheckURL, so an https release cannot bounce deck onto plain http.
func newClient() *http.Client {
	return &http.Client{Timeout: clientTimeout, CheckRedirect: func(req *http.Request, _ []*http.Request) error {
		return CheckURL(req.URL.String())
	}}
}

// get downloads one URL fully into memory, refusing non-200 and oversize.
func get(ctx context.Context, c *http.Client, raw string, limit int64) ([]byte, error) {
	if err := CheckURL(raw); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", raw, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", raw, err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("GET %s: larger than %d bytes", raw, limit)
	}
	return data, nil
}

// LatestTag resolves the latest stable release tag from base+/releases/latest,
// which answers with a redirect to base+/releases/tag/<tag>.
func LatestTag(ctx context.Context, c *http.Client, base string) (string, error) {
	noFollow := *c
	noFollow.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/releases/latest", nil)
	if err != nil {
		return "", err
	}
	resp, err := noFollow.Do(req)
	if err != nil {
		return "", fmt.Errorf("resolve latest release: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	loc, err := resp.Location()
	if err != nil {
		return "", fmt.Errorf("resolve latest release: %s without a redirect: %w", resp.Status, err)
	}
	return tagFromLocation(loc)
}

func tagFromLocation(loc *url.URL) (string, error) {
	tag := path.Base(loc.Path)
	if path.Base(path.Dir(loc.Path)) != "tag" || !stableTag.MatchString(tag) {
		return "", errors.New("resolve latest release: redirect " + loc.Path + " does not name a stable vX.Y.Z tag")
	}
	return tag, nil
}
