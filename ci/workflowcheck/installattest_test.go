// installattest_test.go: R236. install.sh verifies the downloaded archive with
// `gh attestation verify --repo n-orlov/deck` after the checksum check. The
// tests run the real script with a PATH holding only symlinks to the basic
// tools it needs plus fake gh/curl, so there is no network and a real gh on
// the host never leaks in.
package workflowcheck

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const fakeGH = `#!/bin/sh
echo "gh $*" >> "$FAKE_LOG"
case "$1 $2" in
  "auth status") [ "${FAKE_GH_AUTH:-1}" = 1 ]; exit $? ;;
  "release download")
    while [ $# -gt 0 ]; do
      case "$1" in -p) name=$2; shift ;; -D) dest=$2; shift ;; esac
      shift
    done
    cp "$FAKE_RELEASE/$name" "$dest/$name"; exit $? ;;
  "attestation verify")
    case "${FAKE_ATTEST:-pass}" in
      pass) echo "Loaded digest sha256:abc"; echo "Verification succeeded!"; exit 0 ;;
      none) echo "Error: failed to fetch attestations: no attestations found for subject" >&2; exit 1 ;;
      *) echo "Error: verifying with issuer sigstore.dev: no attestations were verified (signature mismatch)" >&2; exit 1 ;;
    esac ;;
esac
exit 2
`

const fakeCurl = `#!/bin/sh
echo "curl $*" >> "$FAKE_LOG"
while [ $# -gt 0 ]; do
  case "$1" in -o) out=$2; shift ;; https://*) url=$1 ;; esac
  shift
done
cp "$FAKE_RELEASE/${url##*/}" "$out"
`

type installRun struct {
	out    string
	err    error
	dir    string // DECK_INSTALL_DIR
	gh     []string
	log    string
	notes  int
	binary bool
}

func tarball(t *testing.T) []byte {
	t.Helper()
	script := []byte("#!/bin/sh\necho deck vtest\n")
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "deck", Mode: 0o755, Size: int64(len(script))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(script); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// runInstall runs install.sh. withGH puts the fake gh on PATH; env adds
// FAKE_*/DECK_* variables.
func runInstall(t *testing.T, withGH bool, env ...string) installRun {
	t.Helper()
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" || runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		t.Skipf("install.sh does not support %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	root, err := repositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	release, bin, home := filepath.Join(tmp, "release"), filepath.Join(tmp, "bin"), filepath.Join(tmp, "home")
	for _, d := range []string{release, bin, home} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	asset := fmt.Sprintf("deck_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	archive := tarball(t)
	sum := sha256.Sum256(archive)
	for name, data := range map[string][]byte{asset: archive, "checksums.txt": []byte(fmt.Sprintf("%x  %s\n", sum, asset))} {
		if err := os.WriteFile(filepath.Join(release, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, tool := range []string{"sh", "uname", "tr", "mktemp", "rm", "grep", "sha256sum", "shasum", "tar", "mkdir", "cp", "chmod", "mv", "cat", "gzip"} {
		if p, err := exec.LookPath(tool); err == nil {
			if err := os.Symlink(p, filepath.Join(bin, tool)); err != nil {
				t.Fatal(err)
			}
		}
	}
	fakes := map[string]string{"curl": fakeCurl}
	if withGH {
		fakes["gh"] = fakeGH
	}
	for name, body := range fakes {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	logPath := filepath.Join(tmp, "calls.log")
	dir := filepath.Join(tmp, "install")
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(sh, filepath.Join(root, "install.sh"))
	cmd.Env = append([]string{"PATH=" + bin, "HOME=" + home, "DECK_INSTALL_DIR=" + dir, "FAKE_RELEASE=" + release, "FAKE_LOG=" + logPath}, env...)
	out, runErr := cmd.CombinedOutput()
	logged, _ := os.ReadFile(logPath)
	res := installRun{out: string(out), err: runErr, dir: dir, log: string(logged)}
	_, statErr := os.Stat(filepath.Join(dir, "deck"))
	res.binary = statErr == nil
	for _, line := range strings.Split(res.log, "\n") {
		if strings.HasPrefix(line, "gh attestation verify") {
			res.gh = append(res.gh, line)
		}
	}
	res.notes = strings.Count(res.out, "only the checksum was verified")
	return res
}

func TestInstallScriptAttestation(t *testing.T) {
	t.Run("verification pass installs", func(t *testing.T) {
		r := runInstall(t, true)
		if r.err != nil || !r.binary {
			t.Fatalf("want a successful install, err=%v binary=%v\n%s", r.err, r.binary, r.out)
		}
		if len(r.gh) != 1 || !strings.Contains(r.gh[0], "--repo n-orlov/deck") || !strings.Contains(r.gh[0], ".tar.gz") {
			t.Errorf("want one `gh attestation verify <archive> --repo n-orlov/deck`, got %q", r.gh)
		}
		if r.notes != 0 || !strings.Contains(r.out, "attestation verified") {
			t.Errorf("a verified install says so and prints no checksum-only note:\n%s", r.out)
		}
	})
	t.Run("verification failure exits non-zero and installs nothing", func(t *testing.T) {
		for _, require := range []string{"", "DECK_REQUIRE_ATTESTATION=1"} {
			r := runInstall(t, true, "FAKE_ATTEST=fail", require)
			if r.err == nil || r.binary {
				t.Fatalf("require=%q: want a non-zero exit and no deck installed, err=%v binary=%v\n%s", require, r.err, r.binary, r.out)
			}
			if _, err := os.Stat(r.dir); err == nil {
				t.Errorf("require=%q: install dir %s was created", require, r.dir)
			}
			if !strings.Contains(r.out, "FAILED") || r.notes != 0 {
				t.Errorf("require=%q: want the failure reported, not the checksum-only note:\n%s", require, r.out)
			}
		}
	})
	t.Run("no gh prints the one checksum-only note and installs", func(t *testing.T) {
		r := runInstall(t, false)
		if r.err != nil || !r.binary {
			t.Fatalf("want a successful install, err=%v binary=%v\n%s", r.err, r.binary, r.out)
		}
		if r.notes != 1 || len(r.gh) != 0 {
			t.Errorf("want exactly one checksum-only note and no gh call, notes=%d gh=%q\n%s", r.notes, r.gh, r.out)
		}
		if !strings.Contains(r.log, "curl ") {
			t.Errorf("without gh the downloads go through curl:\n%s", r.log)
		}
	})
	t.Run("an unauthenticated gh counts as no gh", func(t *testing.T) {
		r := runInstall(t, true, "FAKE_GH_AUTH=0")
		if r.err != nil || !r.binary || r.notes != 1 || len(r.gh) != 0 {
			t.Fatalf("want a note-and-install without a verify call, err=%v binary=%v notes=%d gh=%q\n%s", r.err, r.binary, r.notes, r.gh, r.out)
		}
	})
	t.Run("no gh with DECK_REQUIRE_ATTESTATION=1 aborts", func(t *testing.T) {
		r := runInstall(t, false, "DECK_REQUIRE_ATTESTATION=1")
		if r.err == nil || r.binary || !strings.Contains(r.out, "DECK_REQUIRE_ATTESTATION=1") {
			t.Fatalf("want an abort naming DECK_REQUIRE_ATTESTATION with nothing installed, err=%v binary=%v\n%s", r.err, r.binary, r.out)
		}
	})
	t.Run("a release with no attestation continues with the note", func(t *testing.T) {
		r := runInstall(t, true, "FAKE_ATTEST=none")
		if r.err != nil || !r.binary {
			t.Fatalf("want a successful install, err=%v binary=%v\n%s", r.err, r.binary, r.out)
		}
		if r.notes != 1 || len(r.gh) != 1 || !strings.Contains(r.out, "no attestation") {
			t.Errorf("want one note naming the missing attestation after one verify call, notes=%d gh=%q\n%s", r.notes, r.gh, r.out)
		}
	})
	t.Run("a release with no attestation aborts under DECK_REQUIRE_ATTESTATION=1", func(t *testing.T) {
		r := runInstall(t, true, "FAKE_ATTEST=none", "DECK_REQUIRE_ATTESTATION=1")
		if r.err == nil || r.binary {
			t.Fatalf("want an abort with nothing installed, err=%v binary=%v\n%s", r.err, r.binary, r.out)
		}
	})
}
