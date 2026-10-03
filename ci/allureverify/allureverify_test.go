// Package allureverify tests R197 (GH #61): ci/allure-report.sh verifies the
// Allure archive it downloads against a pinned SHA-256, and install.sh only
// ever speaks https. Both are exercised through the real scripts.
package allureverify

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func repositoryRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal(errors.New("no go.mod above the test directory"))
		}
		dir = parent
	}
}

// fakeAllure writes a gzip tarball shaped like the Allure distribution
// (one top-level directory, bin/allure) whose launcher only records that it ran.
func fakeAllure(t *testing.T, dir string) (path, sum string) {
	t.Helper()
	script := "#!/bin/sh\ncase \"$1\" in\n--version) echo fake-allure;;\ngenerate) mkdir -p \"$5\"; echo ok > \"$5/index.html\";;\nesac\n"
	path = filepath.Join(dir, "allure-fake.tgz")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "allure-fake/bin/allure", Mode: 0o755, Size: int64(len(script))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(script)); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(raw)
	return path, hex.EncodeToString(h[:])
}

// runScript runs ci/allure-report.sh with a PATH holding only the tools it
// needs (never an `allure`, so it always takes the download branch).
func runScript(t *testing.T, archive, sha string) (string, error) {
	t.Helper()
	root := repositoryRoot(t)
	bin := t.TempDir()
	for _, tool := range []string{"sh", "curl", "tar", "mktemp", "rm", "mkdir", "cp", "cut", "sha256sum", "shasum", "dirname", "gzip"} {
		if p, err := exec.LookPath(tool); err == nil {
			if err := os.Symlink(p, filepath.Join(bin, tool)); err != nil {
				t.Fatal(err)
			}
		}
	}
	results := t.TempDir()
	cmd := exec.Command("sh", filepath.Join(root, "ci", "allure-report.sh"), results, filepath.Join(t.TempDir(), "report"))
	cmd.Env = []string{"PATH=" + bin, "HOME=" + t.TempDir(), "TMPDIR=" + t.TempDir(), "ALLURE_URL=file://" + archive, "ALLURE_SHA256=" + sha}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestAllureReportAbortsOnChecksumMismatch(t *testing.T) {
	archive, _ := fakeAllure(t, t.TempDir())
	wrong := strings.Repeat("0", 64)
	out, err := runScript(t, archive, wrong)
	if err == nil {
		t.Fatalf("a wrong checksum must abort the script; output:\n%s", out)
	}
	if !strings.Contains(out, "checksum mismatch") {
		t.Fatalf("failure must be the checksum check, got:\n%s", out)
	}
	if strings.Contains(out, "fake-allure") {
		t.Fatalf("the unverified archive was run:\n%s", out)
	}
}

func TestAllureReportAcceptsTheMatchingChecksum(t *testing.T) {
	archive, sum := fakeAllure(t, t.TempDir())
	out, err := runScript(t, archive, sum)
	if err != nil {
		t.Fatalf("matching checksum must pass: %v\n%s", err, out)
	}
	if !strings.Contains(out, "fake-allure") || !strings.Contains(out, "report written") {
		t.Fatalf("verified archive should have been used:\n%s", out)
	}
}

func TestAllureReportPinsAChecksumForItsDefaultVersion(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(repositoryRoot(t), "ci", "allure-report.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`default_allure_sha256=[0-9a-f]{64}\n`).Match(raw) {
		t.Fatal("ci/allure-report.sh must pin a 64-hex SHA-256 for the default Allure version")
	}
}

func TestInstallScriptCurlIsHTTPSOnlyAndStatesItsChecksumLimit(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(repositoryRoot(t), "install.sh"))
	if err != nil {
		t.Fatal(err)
	}
	var curls int
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") || !strings.Contains(trimmed, "curl ") {
			continue
		}
		curls++
		if !strings.Contains(trimmed, "--proto =https") {
			t.Errorf("curl call without --proto =https: %s", trimmed)
		}
	}
	if curls == 0 {
		t.Fatal("install.sh has no curl call to check")
	}
	if !regexp.MustCompile(`(?s)checksums\.txt[^\n]*\n?[^\n]*SAME release.*corrupt.*not tampering`).Match(raw) {
		t.Error("install.sh must state that checksums.txt comes from the same release (catches corruption, not tampering)")
	}
}
