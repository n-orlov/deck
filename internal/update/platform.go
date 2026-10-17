// Package update installs the latest deck release over the running binary.
//
// It resolves the latest stable tag, asks the GitHub release API whether that
// release is immutable (reported, or required with DECK_REQUIRE_IMMUTABLE=1),
// downloads the platform archive and checksums.txt from the same release,
// verifies the archive's sha256,
// extracts the binary and rename(2)s it over the symlink-resolved current
// executable. Every step before the rename works in memory or on a
// temporary file beside the target, so a failure at any step leaves the old
// binary byte-identical and no stray file in the install directory.
package update

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
)

// DefaultBaseURL is the release repository deck updates from.
const DefaultBaseURL = "https://github.com/n-orlov/deck"

// EnvBaseURL overrides DefaultBaseURL. Tests point it at a loopback fake
// release server; SPEC §13.1 documents it.
const EnvBaseURL = "DECK_UPDATE_URL"

// ErrUnsupportedPlatform is wrapped by every platform refusal.
var ErrUnsupportedPlatform = errors.New("unsupported platform")

// AssetName is the release archive for an OS/architecture pair, e.g.
// deck_linux_amd64.tar.gz. Only linux and darwin on amd64 and arm64 are
// published, so any other pair is a clear error rather than a 404 later.
func AssetName(goos, goarch string) (string, error) {
	switch goos {
	case "linux", "darwin":
	default:
		return "", fmt.Errorf("%w: deck update supports linux and darwin, not %q", ErrUnsupportedPlatform, goos)
	}
	switch goarch {
	case "amd64", "arm64":
	default:
		return "", fmt.Errorf("%w: deck update supports amd64 and arm64, not %q", ErrUnsupportedPlatform, goarch)
	}
	return "deck_" + goos + "_" + goarch + ".tar.gz", nil
}

// baseURL is the update base URL: DECK_UPDATE_URL when set, else the
// default, without a trailing slash.
func baseURL(override string) string {
	if override == "" {
		override = os.Getenv(EnvBaseURL)
	}
	if override == "" {
		override = DefaultBaseURL
	}
	return strings.TrimRight(override, "/")
}

// CheckURL refuses a URL deck would fetch over anything but HTTPS. Plain
// HTTP is allowed only for a loopback host, which is what the test override
// uses.
func CheckURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return fmt.Errorf("update URL %q is not a valid absolute URL", RedactURL(raw))
	}
	if u.Scheme == "https" || (u.Scheme == "http" && isLoopback(u.Hostname())) {
		return nil
	}
	return fmt.Errorf("update URL %q refused: only https is allowed (http only for a loopback host)", RedactURL(raw))
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
