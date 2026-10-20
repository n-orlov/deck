package update

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
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
	_ = os.Unsetenv(EnvRequireAttestation)
	// Behind a rejecting proxy: a test that forgot its fake release server
	// fails the package naming the public host it reached.
	code := testnet.Main(m)
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// fakeGhScript records every call, serves `release download` from FAKE_GH_FILES
// and answers `auth status` / `attestation verify` from FAKE_GH_AUTH,
// FAKE_GH_VERIFY (exit codes) and FAKE_GH_VERIFY_MSG (its output).
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
"attestation verify")
  /bin/cp "$3" "$FAKE_GH_LOG.verified"
  [ -n "$FAKE_GH_VERIFY_MSG" ] && echo "$FAKE_GH_VERIFY_MSG" >&2
  exit "${FAKE_GH_VERIFY:-0}"
  ;;
esac
`

type fakeGh struct{ log string }

// installFakeGh puts a fake gh alone on PATH, serving rel's files.
func installFakeGh(t *testing.T, rel *release, auth, verify int, verifyMsg string) *fakeGh {
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
	t.Setenv("FAKE_GH_AUTH", itoa(auth))
	t.Setenv("FAKE_GH_VERIFY", itoa(verify))
	t.Setenv("FAKE_GH_VERIFY_MSG", verifyMsg)
	return g
}

func itoa(n int) string { return strconv.Itoa(n) }

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

func TestAttestation(t *testing.T) {
	rows := []struct {
		name         string
		gh           bool
		verifyExit   int
		requireEnv   string
		wantReplaced bool
		wantAtt      string
		wantErr      string
	}{
		{"verify passes", true, 0, "", true, AttestationVerified, ""},
		{"verify fails", true, 1, "", false, "", "attestation verification failed"},
		{"gh absent proceeds on the checksum", false, 0, "", true, AttestationSkipped, ""},
		{"gh absent with DECK_REQUIRE_ATTESTATION=1", false, 0, "1", false, "", EnvRequireAttestation + "=1"},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			rel := newRelease(t)
			srv := rel.serve(t)
			var g *fakeGh
			if row.gh {
				g = installFakeGh(t, rel, 0, row.verifyExit, "")
			}
			if row.requireEnv != "" {
				t.Setenv(EnvRequireAttestation, row.requireEnv)
			}
			dir, exe := installOld(t)
			beforeSum, beforeList := fileSum(t, exe), listing(t, dir)

			res, err := Run(context.Background(), Options{BaseURL: srv.URL, Exe: exe, GOOS: "linux", GOARCH: "amd64"})
			if row.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), row.wantErr) {
					t.Fatalf("err = %v, want it to contain %q", err, row.wantErr)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if row.wantReplaced {
				if got := fileSum(t, exe); got != sum([]byte(newBody)) {
					t.Errorf("binary sha256 = %s, want the new binary's", got)
				}
				if res.Attestation != row.wantAtt {
					t.Errorf("attestation = %q, want %q", res.Attestation, row.wantAtt)
				}
			} else if got := fileSum(t, exe); got != beforeSum {
				t.Errorf("old binary sha256 = %s, want %s unchanged", got, beforeSum)
			}
			if got := listing(t, dir); !reflect.DeepEqual(got, beforeList) {
				t.Errorf("install dir = %v, want %v", got, beforeList)
			}
			if g == nil {
				return
			}
			verifies := g.callsStarting(t, "attestation verify")
			if len(verifies) != 1 || !strings.HasSuffix(verifies[0], "/"+rel.asset+" --repo n-orlov/deck") {
				t.Fatalf("verify calls = %q, want one `attestation verify <dir>/%s --repo n-orlov/deck`", verifies, rel.asset)
			}
			got, err := os.ReadFile(g.log + ".verified")
			if err != nil || string(got) != string(rel.archive) {
				t.Errorf("gh verified %d bytes (%v), want the downloaded archive (%d bytes)", len(got), err, len(rel.archive))
			}
		})
	}
}

// TestAttestationFailureWithAuthenticatedGhAlwaysAborts covers R294: with an
// authenticated gh, any nonzero verification result other than "no
// attestations found" aborts the update and leaves the old executable and its
// directory byte-identical, whatever the message says and whether or not
// DECK_REQUIRE_ATTESTATION is set.
func TestAttestationFailureWithAuthenticatedGhAlwaysAborts(t *testing.T) {
	msgs := map[string]string{
		"signature mismatch":          "Error: no attestations were verified (signature mismatch)",
		"silent nonzero exit":         "",
		"mentions no attestation yet": "no attestation was verified",
	}
	for name, msg := range msgs {
		for _, require := range []bool{false, true} {
			t.Run(name+"/require="+strconv.FormatBool(require), func(t *testing.T) {
				rel := newRelease(t)
				srv := rel.serve(t)
				g := installFakeGh(t, rel, 0, 1, msg)
				dir, exe := installOld(t)
				beforeSum, beforeList := fileSum(t, exe), listing(t, dir)
				res, err := Run(context.Background(), Options{BaseURL: srv.URL, Exe: exe, GOOS: "linux", GOARCH: "amd64", RequireAttestation: require})
				if err == nil || !strings.Contains(err.Error(), "attestation verification failed") {
					t.Fatalf("err = %v (res %+v), want an attestation verification failure", err, res)
				}
				if res.Attestation != "" {
					t.Errorf("attestation = %q, want none on an aborted update", res.Attestation)
				}
				if got := fileSum(t, exe); got != beforeSum {
					t.Errorf("old binary sha256 = %s, want %s unchanged", got, beforeSum)
				}
				if got := listing(t, dir); !reflect.DeepEqual(got, beforeList) {
					t.Errorf("install dir = %v, want %v", got, beforeList)
				}
				if n := len(g.callsStarting(t, "attestation verify")); n != 1 {
					t.Errorf("verify calls = %d, want 1", n)
				}
			})
		}
	}
}

// TestAttestationMissingIsCannotVerify: a release with no attestation at all
// (releases built by Jenkins) is "cannot verify", not a failure. It proceeds
// on the checksum with a note, and aborts under DECK_REQUIRE_ATTESTATION=1
// with the old binary and its directory untouched.
func TestAttestationMissingIsCannotVerify(t *testing.T) {
	for _, require := range []bool{false, true} {
		t.Run("require="+strconv.FormatBool(require), func(t *testing.T) {
			rel := newRelease(t)
			srv := rel.serve(t)
			g := installFakeGh(t, rel, 0, 1, "Error: failed to fetch attestations: no attestations found for subject")
			dir, exe := installOld(t)
			beforeSum, beforeList := fileSum(t, exe), listing(t, dir)
			res, err := Run(context.Background(), Options{BaseURL: srv.URL, Exe: exe, GOOS: "linux", GOARCH: "amd64", RequireAttestation: require})
			if n := len(g.callsStarting(t, "attestation verify")); n != 1 {
				t.Errorf("verify calls = %d, want 1", n)
			}
			if require {
				if err == nil || !strings.Contains(err.Error(), "no build attestation") || !strings.Contains(err.Error(), EnvRequireAttestation+"=1") {
					t.Fatalf("err = %v, want the missing attestation and %s=1 named", err, EnvRequireAttestation)
				}
				if got := fileSum(t, exe); got != beforeSum {
					t.Errorf("old binary sha256 = %s, want %s unchanged", got, beforeSum)
				}
				if got := listing(t, dir); !reflect.DeepEqual(got, beforeList) {
					t.Errorf("install dir = %v, want %v", got, beforeList)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if res.Attestation != AttestationSkipped {
				t.Errorf("attestation = %q, want %q", res.Attestation, AttestationSkipped)
			}
			if got := fileSum(t, exe); got != sum([]byte(newBody)) {
				t.Errorf("binary sha256 = %s, want the new binary's", got)
			}
		})
	}
}

// TestAttestationUnauthenticatedGhIsCannotVerify keeps the other
// checksum-only fallback: a gh that is not authenticated is never asked to
// verify, so even a release with no attestation proceeds on the checksum
// (and aborts under DECK_REQUIRE_ATTESTATION=1).
func TestAttestationUnauthenticatedGhIsCannotVerify(t *testing.T) {
	for _, require := range []bool{false, true} {
		rel := newRelease(t)
		srv := rel.serve(t)
		g := installFakeGh(t, rel, 1, 1, "no attestations found for subject")
		_, exe := installOld(t)
		before := fileSum(t, exe)
		res, err := Run(context.Background(), Options{BaseURL: srv.URL, Exe: exe, GOOS: "linux", GOARCH: "amd64", RequireAttestation: require})
		if n := len(g.callsStarting(t, "attestation verify")); n != 0 {
			t.Errorf("require=%v: %d verify calls with an unauthenticated gh, want 0", require, n)
		}
		if require {
			if err == nil || fileSum(t, exe) != before {
				t.Errorf("required: err = %v, binary changed = %v", err, fileSum(t, exe) != before)
			}
			continue
		}
		if err != nil || res.Attestation != AttestationSkipped || fileSum(t, exe) != sum([]byte(newBody)) {
			t.Errorf("not required: res = %+v, err = %v", res, err)
		}
	}
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
				g = installFakeGh(t, rel, row.auth, 0, "")
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
