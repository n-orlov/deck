// installcheck_test.go: install.sh verifies the downloaded archive against
// checksums.txt and nothing else: releases are built by Jenkins and carry no
// GitHub build attestation, so the script never asks gh to verify one. The
// tests run the real script with a PATH holding only symlinks to the basic
// tools it needs plus fake gh/curl, so there is no network and a real gh on
// the host never leaks in.
package repocheck

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
	dir    string   // DECK_INSTALL_DIR
	gh     []string // gh calls other than `auth status` and `release download`
	log    string
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

// runInstall runs install.sh. withGH puts the fake gh on PATH; badSum publishes
// a checksums.txt that does not match the archive; env adds FAKE_*/DECK_*
// variables.
func runInstall(t *testing.T, withGH, badSum bool, env ...string) installRun {
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
	if badSum {
		sum = sha256.Sum256([]byte("not the archive"))
	}
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
		if strings.HasPrefix(line, "gh ") && !strings.HasPrefix(line, "gh auth status") && !strings.HasPrefix(line, "gh release download") {
			res.gh = append(res.gh, line)
		}
	}
	return res
}

func TestInstallScriptChecksumOnly(t *testing.T) {
	t.Run("an authenticated gh downloads and nothing else", func(t *testing.T) {
		r := runInstall(t, true, false)
		if r.err != nil || !r.binary {
			t.Fatalf("want a successful install, err=%v binary=%v\n%s", r.err, r.binary, r.out)
		}
		if !strings.Contains(r.log, "gh release download") || strings.Contains(r.log, "curl ") {
			t.Errorf("with an authenticated gh the downloads go through gh:\n%s", r.log)
		}
		if len(r.gh) != 0 {
			t.Errorf("want no gh call beyond auth status and release download, got %q", r.gh)
		}
	})
	t.Run("no gh downloads through curl", func(t *testing.T) {
		r := runInstall(t, false, false)
		if r.err != nil || !r.binary {
			t.Fatalf("want a successful install, err=%v binary=%v\n%s", r.err, r.binary, r.out)
		}
		if !strings.Contains(r.log, "curl ") {
			t.Errorf("without gh the downloads go through curl:\n%s", r.log)
		}
	})
	t.Run("an unauthenticated gh counts as no gh", func(t *testing.T) {
		r := runInstall(t, true, false, "FAKE_GH_AUTH=0")
		if r.err != nil || !r.binary || len(r.gh) != 0 || !strings.Contains(r.log, "curl ") {
			t.Fatalf("want a curl install and no other gh call, err=%v binary=%v gh=%q\n%s", r.err, r.binary, r.gh, r.log)
		}
	})
	t.Run("a checksum mismatch exits non-zero and installs nothing", func(t *testing.T) {
		for _, withGH := range []bool{true, false} {
			r := runInstall(t, withGH, true)
			if r.err == nil || r.binary {
				t.Fatalf("gh=%v: want a non-zero exit and no deck installed, err=%v binary=%v\n%s", withGH, r.err, r.binary, r.out)
			}
			if _, err := os.Stat(r.dir); err == nil {
				t.Errorf("gh=%v: install dir %s was created", withGH, r.dir)
			}
		}
	})
	t.Run("the script has no attestation step", func(t *testing.T) {
		root, err := repositoryRoot()
		if err != nil {
			t.Fatal(err)
		}
		body, err := os.ReadFile(filepath.Join(root, "install.sh"))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(strings.ToLower(string(body)), "attest") {
			t.Error("install.sh mentions attestation; releases carry none and the checksum is the only check")
		}
	})
}
