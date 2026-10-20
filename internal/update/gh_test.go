package update

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/testnet"
)

// TestMain keeps every update test off the developer's real gh: PATH holds
// only an empty directory unless a test installs a fake gh.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "deck-update-nopath-")
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("PATH", dir)
	// Behind a rejecting proxy: a test that forgot its fake release server
	// fails the package naming the public host it reached.
	code := testnet.Main(m)
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// fakeGhScript records every call, serves `release download` from FAKE_GH_FILES
// and answers `auth status` with the exit code in FAKE_GH_AUTH.
const fakeGhScript = `#!/bin/sh
echo "$*" >> "$FAKE_GH_LOG"
case "$1 $2" in
"auth status") exit "${FAKE_GH_AUTH:-0}" ;;
"release download")
  shift 2
  while [ $# -gt 0 ]; do
    case "$1" in
      -p) asset="$2"; shift ;;
      -D) dir="$2"; shift ;;
    esac
    shift
  done
  /bin/cp "$FAKE_GH_FILES/$asset" "$dir/$asset"
  ;;
esac
`

type fakeGh struct{ log string }

// installFakeGh puts a fake gh alone on PATH, serving rel's files.
func installFakeGh(t *testing.T, rel *release, auth int) *fakeGh {
	t.Helper()
	bin, files := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(fakeGhScript), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{rel.asset: rel.archive, "checksums.txt": []byte(rel.checksums)} {
		if err := os.WriteFile(filepath.Join(files, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	g := &fakeGh{log: filepath.Join(t.TempDir(), "calls.log")}
	t.Setenv("PATH", bin)
	t.Setenv("FAKE_GH_LOG", g.log)
	t.Setenv("FAKE_GH_FILES", files)
	t.Setenv("FAKE_GH_AUTH", strconv.Itoa(auth))
	return g
}

func (g *fakeGh) calls(t *testing.T) []string {
	t.Helper()
	b, err := os.ReadFile(g.log)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(b)), "\n")
}

func (g *fakeGh) callsStarting(t *testing.T, prefix string) []string {
	var out []string
	for _, c := range g.calls(t) {
		if strings.HasPrefix(c, prefix) {
			out = append(out, c)
		}
	}
	return out
}

func TestDownloadPrefersGh(t *testing.T) {
	rows := []struct {
		name       string
		gh         bool
		auth       int
		wantGh     bool
		wantServer bool
	}{
		{"authenticated gh", true, 0, true, false},
		{"gh not authenticated", true, 1, false, true},
		{"gh absent", false, 0, false, true},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			rel := newRelease(t)
			srv := rel.serve(t)
			hits := map[string]int{}
			var g *fakeGh
			if row.gh {
				g = installFakeGh(t, rel, row.auth)
			}
			// Count server downloads by wrapping the transport.
			client := srv.Client()
			client.Transport = countingTransport{base: http.DefaultTransport, hits: hits}
			_, exe := installOld(t)
			res, err := Run(context.Background(), Options{BaseURL: srv.URL, Repo: DefaultRepo, Client: client, Exe: exe, GOOS: "linux", GOARCH: "amd64"})
			if err != nil {
				t.Fatal(err)
			}
			if fileSum(t, exe) != sum([]byte(newBody)) {
				t.Errorf("binary not replaced (%+v)", res)
			}
			serverDl := 0
			for p, n := range hits {
				if strings.Contains(p, "/releases/download/") {
					serverDl += n
				}
			}
			var ghDl []string
			if g != nil {
				ghDl = g.callsStarting(t, "release download")
			}
			if row.wantGh {
				if len(ghDl) != 2 || !strings.Contains(ghDl[0], "release download "+tagV+" -R n-orlov/deck -p "+rel.asset) {
					t.Errorf("gh download calls = %q, want the archive and checksums.txt through gh", ghDl)
				}
				if serverDl != 0 {
					t.Errorf("server served %d downloads, want none", serverDl)
				}
			} else if len(ghDl) != 0 || serverDl != 2 {
				t.Errorf("gh downloads = %q, server downloads = %d; want plain HTTPS from the server", ghDl, serverDl)
			}
		})
	}
}

type countingTransport struct {
	base http.RoundTripper
	hits map[string]int
}

func (c countingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	c.hits[r.URL.Path]++
	return c.base.RoundTrip(r)
}
