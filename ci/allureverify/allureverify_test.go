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
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
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
	script := "#!/bin/sh\ncase \"$1\" in\n--version) echo fake-allure;;\ngenerate) mkdir -p \"$5\"; echo ok > \"$5/index.html\"; for f in \"$2\"/*; do echo \"${f##*/}\"; done > \"$5/inputs.txt\";;\nesac\n"
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
	out, _, err := runScriptOver(t, archive, sha, t.TempDir())
	return out, err
}

// runScriptOver is runScript over a given ci/suite.sh results directory; it
// also returns the report directory, where the fake allure leaves inputs.txt,
// the names of the files `allure generate` was handed.
func runScriptOver(t *testing.T, archive, sha, results string) (string, string, error) {
	t.Helper()
	return runScriptFrom(t, "file://"+archive, sha, results)
}

// runScriptFrom is runScriptOver with the archive fetched from url.
func runScriptFrom(t *testing.T, url, sha, results string) (string, string, error) {
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
	report := filepath.Join(t.TempDir(), "report")
	cmd := exec.Command("sh", filepath.Join(root, "ci", "allure-report.sh"), results, report)
	cmd.Env = []string{"PATH=" + bin, "HOME=" + t.TempDir(), "TMPDIR=" + t.TempDir(), "ALLURE_URL=" + url, "ALLURE_SHA256=" + sha}
	out, err := cmd.CombinedOutput()
	return string(out), report, err
}

func generatedInputs(t *testing.T, report string) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(report, "inputs.txt"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.Fields(string(raw))
}

// TestAllureReportBuildsOneReportFromTheNativeResultsDir: with an
// allure-results/ (task 005, R195/R196) the report gets exactly its files --
// unit and features results, environment.properties, executor.json -- and not
// the JUnit that would list the same tests a second time.
func TestAllureReportBuildsOneReportFromTheNativeResultsDir(t *testing.T) {
	archive, sum := fakeAllure(t, t.TempDir())
	results := t.TempDir()
	native := filepath.Join(results, "allure-results")
	merged := filepath.Join(results, "junit-merged")
	for _, d := range []string{native, merged} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		"allure-results/u1-result.json":           `{"labels":[{"name":"parentSuite","value":"unit"}]}`,
		"allure-results/f1-result.json":           `{"labels":[{"name":"parentSuite","value":"features"}]}`,
		"allure-results/f1-attachment.txt":        "frame",
		"allure-results/environment.properties":   "go.version=go1\n",
		"allure-results/executor.json":            "{}",
		"allure-results/godog-allure-summary.txt": "formatter bookkeeping",
		"junit-merged/junit-go.xml":               "<testsuites/>",
		"junit-merged/junit-features.xml":         "<testsuites/>",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(results, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	out, report, err := runScriptOver(t, archive, sum, results)
	if err != nil {
		t.Fatalf("report: %v\n%s", err, out)
	}
	got := strings.Join(generatedInputs(t, report), " ")
	for _, want := range []string{"u1-result.json", "f1-result.json", "f1-attachment.txt", "environment.properties", "executor.json"} {
		if !strings.Contains(got, want) {
			t.Errorf("allure generate was not handed %s; inputs: %s", want, got)
		}
	}
	for _, unwanted := range []string{".xml", "godog-allure-summary.txt"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("allure generate was handed %s, which would duplicate or add noise; inputs: %s", unwanted, got)
		}
	}
}

// TestAllureReportFallsBackToTheMergedJUnitWithoutNativeResults: a results dir
// from before allure-results/ existed still builds a report.
func TestAllureReportFallsBackToTheMergedJUnitWithoutNativeResults(t *testing.T) {
	archive, sum := fakeAllure(t, t.TempDir())
	results := t.TempDir()
	if err := os.MkdirAll(filepath.Join(results, "junit-merged"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(results, "junit-merged", "junit-go.xml"), []byte("<testsuites/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, report, err := runScriptOver(t, archive, sum, results)
	if err != nil {
		t.Fatalf("report: %v\n%s", err, out)
	}
	if got := generatedInputs(t, report); len(got) != 1 || got[0] != "junit-go.xml" {
		t.Fatalf("inputs = %v, want only junit-go.xml", got)
	}
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

// dropFirstConnectionThenServe starts a local HTTP host that closes its first
// connection without a response and serves archive to every later one, and
// returns the archive's URL on it.
func dropFirstConnectionThenServe(t *testing.T, archive string) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	served := make(chan struct{})
	go func() {
		defer close(served)
		first, err := listener.Accept()
		if err != nil {
			return
		}
		_ = first.Close()
		server := &http.Server{
			ReadHeaderTimeout: 5 * time.Second,
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.ServeFile(w, r, archive)
			}),
		}
		_ = server.Serve(listener)
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		<-served
	})
	return "http://" + listener.Addr().String() + "/allure.tgz"
}

// TestAllureReportRetriesADroppedDownloadConnection: the release host
// dropping the first connection (no HTTP response at all, curl exit 52 --
// the same class as the connect failure that once failed a push run) is
// retried, and the archive the retry fetches is still checksum-verified.
func TestAllureReportRetriesADroppedDownloadConnection(t *testing.T) {
	archive, sum := fakeAllure(t, t.TempDir())
	url := dropFirstConnectionThenServe(t, archive)
	out, _, err := runScriptFrom(t, url, sum, t.TempDir())
	if err != nil {
		t.Fatalf("a dropped first connection must be retried, not fail the report: %v\n%s", err, out)
	}
	if !strings.Contains(out, "fake-allure") || !strings.Contains(out, "report written") {
		t.Fatalf("the retried download should have been verified and used:\n%s", out)
	}
}

// TestAllureReportRetryStillAbortsOnChecksumMismatch: retrying never stands
// in for verification -- a mismatching archive served after a dropped
// connection is still refused.
func TestAllureReportRetryStillAbortsOnChecksumMismatch(t *testing.T) {
	archive, _ := fakeAllure(t, t.TempDir())
	url := dropFirstConnectionThenServe(t, archive)
	out, _, err := runScriptFrom(t, url, strings.Repeat("0", 64), t.TempDir())
	if err == nil || !strings.Contains(out, "checksum mismatch") {
		t.Fatalf("a retried download with the wrong checksum must abort: err=%v\n%s", err, out)
	}
}
