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
	"sync"
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
func runScriptFrom(t *testing.T, url, sha, results string, extraEnv ...string) (string, string, error) {
	t.Helper()
	root := repositoryRoot(t)
	bin := t.TempDir()
	for _, tool := range []string{"sh", "curl", "tar", "mktemp", "rm", "mkdir", "cp", "mv", "cut", "sha256sum", "shasum", "dirname", "gzip"} {
		if p, err := exec.LookPath(tool); err == nil {
			if err := os.Symlink(p, filepath.Join(bin, tool)); err != nil {
				t.Fatal(err)
			}
		}
	}
	report := filepath.Join(t.TempDir(), "report")
	cmd := exec.Command("sh", filepath.Join(root, "ci", "allure-report.sh"), results, report)
	cmd.Env = []string{"PATH=" + bin, "HOME=" + t.TempDir(), "TMPDIR=" + t.TempDir(), "ALLURE_URL=" + url, "ALLURE_SHA256=" + sha}
	cmd.Env = append(cmd.Env, extraEnv...)
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

// countingHost starts a local HTTP host that counts every connection it
// accepts and answers each with handler (nil: drop the connection without a
// response). It returns the archive URL on it and the connection counter.
func countingHost(t *testing.T, handler http.Handler) (string, func() int) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	conns := 0
	counted := &countingListener{Listener: listener, onAccept: func() { mu.Lock(); conns++; mu.Unlock() }}
	done := make(chan struct{})
	go func() {
		defer close(done)
		if handler == nil {
			for {
				c, err := counted.Accept()
				if err != nil {
					return
				}
				_ = c.Close()
			}
		}
		server := &http.Server{ReadHeaderTimeout: 5 * time.Second, Handler: handler}
		_ = server.Serve(counted)
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		<-done
	})
	return "http://" + listener.Addr().String() + "/allure.tgz", func() int { mu.Lock(); defer mu.Unlock(); return conns }
}

type countingListener struct {
	net.Listener
	onAccept func()
}

func (l *countingListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err == nil {
		l.onAccept()
	}
	return c, err
}

func serveFile(path string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, path) })
}

// The Allure download keeps exactly its pre-cure attempt budget: ONE curl
// attempt. A dropped connection, an HTTP error and an unreachable host each
// fail the report after a single attempt -- no curl --retry, no wrapper loop
// and no back-off may be added to turn one failure into several attempts.
func TestAllureReportDownloadIsASingleAttemptWhateverTheFailure(t *testing.T) {
	_, sum := fakeAllure(t, t.TempDir())
	failures := map[string]http.Handler{
		"dropped connection": nil,
		"http 503":           http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "busy", http.StatusServiceUnavailable) }),
		"http 404":           http.NotFoundHandler(),
	}
	for name, handler := range failures {
		t.Run(name, func(t *testing.T) {
			url, connections := countingHost(t, handler)
			start := time.Now()
			out, _, err := runScriptFrom(t, url, sum, t.TempDir())
			if err == nil {
				t.Fatalf("a failed download must fail the report:\n%s", out)
			}
			if n := connections(); n != 1 {
				t.Fatalf("the download made %d connection attempts, want exactly 1 (the pre-cure budget)\n%s", n, out)
			}
			if time.Since(start) > 1500*time.Millisecond {
				t.Fatalf("a failed download took %v: a back-off between attempts crept in", time.Since(start))
			}
		})
	}
}

func TestAllureReportDownloadIsASingleAttemptWhenTheHostIsUnreachable(t *testing.T) {
	_, sum := fakeAllure(t, t.TempDir())
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	url := "http://" + listener.Addr().String() + "/allure.tgz"
	_ = listener.Close() // nothing listens: connection refused
	start := time.Now()
	out, _, err := runScriptFrom(t, url, sum, t.TempDir())
	if err == nil {
		t.Fatalf("an unreachable release host must fail the report:\n%s", out)
	}
	if time.Since(start) > 1500*time.Millisecond {
		t.Fatalf("an unreachable host took %v: a retry back-off crept in", time.Since(start))
	}
}

// The script text itself carries no retry machinery on the download.
func TestAllureReportScriptAddsNoRetryToTheDownload(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(repositoryRoot(t), "ci", "allure-report.sh"))
	if err != nil {
		t.Fatal(err)
	}
	var curls int
	for _, line := range strings.Split(string(raw), "\n") {
		code := strings.TrimSpace(line)
		if strings.HasPrefix(code, "#") {
			continue
		}
		if strings.Contains(code, "curl ") {
			curls++
			for _, flag := range []string{"--retry", "--retry-delay", "--retry-all-errors", "--retry-max-time", "--retry-connrefused"} {
				if strings.Contains(code, flag) {
					t.Errorf("the download carries %s: %q", flag, code)
				}
			}
		}
		for _, loop := range []string{"while ", "until ", "for attempt", "sleep "} {
			if strings.HasPrefix(code, loop) {
				t.Errorf("a retry/back-off construct %q in the download script: %q", loop, code)
			}
		}
	}
	if curls != 1 {
		t.Errorf("ci/allure-report.sh has %d curl invocations, want exactly 1", curls)
	}
}

// Checksum integrity is retained on every path: a served archive that does
// not match the pin aborts, and is never cached.
func TestAllureReportChecksumMismatchAbortsAndIsNotCached(t *testing.T) {
	archive, _ := fakeAllure(t, t.TempDir())
	url, connections := countingHost(t, serveFile(archive))
	cache := t.TempDir()
	out, _, err := runScriptFrom(t, url, strings.Repeat("0", 64), t.TempDir(), "ALLURE_CACHE_DIR="+cache)
	if err == nil || !strings.Contains(out, "checksum mismatch") {
		t.Fatalf("a mismatching archive must abort: err=%v\n%s", err, out)
	}
	if n := connections(); n != 1 {
		t.Fatalf("%d connections, want 1", n)
	}
	if entries, _ := os.ReadDir(cache); len(entries) != 0 {
		t.Fatalf("an unverified archive was cached: %v", entries)
	}
}

// With a cache dir, the first run downloads once and caches the verified
// archive; the second run needs no connection at all.
func TestAllureReportCachedArchiveIsVerifiedAndFetchedOnce(t *testing.T) {
	archive, sum := fakeAllure(t, t.TempDir())
	url, connections := countingHost(t, serveFile(archive))
	cache := t.TempDir()
	for run := 1; run <= 2; run++ {
		out, _, err := runScriptFrom(t, url, sum, t.TempDir(), "ALLURE_CACHE_DIR="+cache)
		if err != nil || !strings.Contains(out, "report written") {
			t.Fatalf("run %d failed: %v\n%s", run, err, out)
		}
	}
	if n := connections(); n != 1 {
		t.Fatalf("two runs made %d connections, want 1 (second served from the verified cache)", n)
	}
}

// A corrupted cache entry is never trusted: it is re-verified, discarded and
// the archive downloaded afresh (still a single attempt), then re-cached.
func TestAllureReportCorruptCacheIsDiscardedNotTrusted(t *testing.T) {
	archive, sum := fakeAllure(t, t.TempDir())
	url, connections := countingHost(t, serveFile(archive))
	cache := t.TempDir()
	poisoned := filepath.Join(cache, "allure-2.34.1-"+sum+".tgz")
	if err := os.WriteFile(poisoned, []byte("not the archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, _, err := runScriptFrom(t, url, sum, t.TempDir(), "ALLURE_CACHE_DIR="+cache)
	if err != nil || !strings.Contains(out, "fake-allure") {
		t.Fatalf("a corrupt cache entry must fall back to a verified download: %v\n%s", err, out)
	}
	if n := connections(); n != 1 {
		t.Fatalf("%d connections, want 1", n)
	}
}

// A cache that cannot be written never fails a report whose download verified.
func TestAllureReportUnwritableCacheDoesNotFailTheReport(t *testing.T) {
	archive, sum := fakeAllure(t, t.TempDir())
	file := filepath.Join(t.TempDir(), "occupied")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	out, _, err := runScriptFrom(t, "file://"+archive, sum, t.TempDir(), "ALLURE_CACHE_DIR="+filepath.Join(file, "sub"))
	if err != nil || !strings.Contains(out, "report written") {
		t.Fatalf("an unwritable cache must not fail the report: %v\n%s", err, out)
	}
}

// A corrupt cache entry is never a way around the integrity check, and it does
// not buy the download a second attempt: with the release host dropping every
// connection the report fails after exactly one connection, and no entry is
// cached from the failed fetch.
func TestAllureReportCorruptCacheWithAFailingHostFailsAfterOneAttempt(t *testing.T) {
	_, sum := fakeAllure(t, t.TempDir())
	url, connections := countingHost(t, nil)
	cache := t.TempDir()
	poisoned := filepath.Join(cache, "allure-2.34.1-"+sum+".tgz")
	if err := os.WriteFile(poisoned, []byte("not the archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, _, err := runScriptFrom(t, url, sum, t.TempDir(), "ALLURE_CACHE_DIR="+cache)
	if err == nil || strings.Contains(out, "report written") {
		t.Fatalf("a corrupt cache and a failing host must fail the report: err=%v\n%s", err, out)
	}
	if n := connections(); n != 1 {
		t.Fatalf("%d connections, want exactly 1\n%s", n, out)
	}
}

// A host that answers 200 with a truncated or empty body is still one attempt,
// still a checksum failure, and still never cached.
func TestAllureReportTruncatedOrEmptyBodyAbortsOnChecksumAfterOneAttempt(t *testing.T) {
	archive, sum := fakeAllure(t, t.TempDir())
	raw, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	bodies := map[string][]byte{"empty": {}, "truncated": raw[:len(raw)/2]}
	for name, body := range bodies {
		t.Run(name, func(t *testing.T) {
			url, connections := countingHost(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(body) }))
			cache := t.TempDir()
			out, _, err := runScriptFrom(t, url, sum, t.TempDir(), "ALLURE_CACHE_DIR="+cache)
			if err == nil || !strings.Contains(out, "checksum mismatch") {
				t.Fatalf("a bad body must abort on the checksum: err=%v\n%s", err, out)
			}
			if n := connections(); n != 1 {
				t.Fatalf("%d connections, want 1", n)
			}
			if entries, _ := os.ReadDir(cache); len(entries) != 0 {
				t.Fatalf("an unverified archive was cached: %v", entries)
			}
		})
	}
}

// A failed download leaves no cache entry, and a valid cached archive needs no
// connection at all, however badly the release host would have behaved.
func TestAllureReportFailedDownloadCachesNothingAndAValidCacheNeedsNoHost(t *testing.T) {
	archive, sum := fakeAllure(t, t.TempDir())
	cache := t.TempDir()
	failing, failedConns := countingHost(t, http.NotFoundHandler())
	if _, _, err := runScriptFrom(t, failing, sum, t.TempDir(), "ALLURE_CACHE_DIR="+cache); err == nil {
		t.Fatal("a 404 download must fail the report")
	}
	if entries, _ := os.ReadDir(cache); len(entries) != 0 {
		t.Fatalf("a failed download left cache entries: %v", entries)
	}
	if n := failedConns(); n != 1 {
		t.Fatalf("%d connections, want 1", n)
	}
	healthy, _ := countingHost(t, serveFile(archive))
	if out, _, err := runScriptFrom(t, healthy, sum, t.TempDir(), "ALLURE_CACHE_DIR="+cache); err != nil {
		t.Fatalf("priming the cache failed: %v\n%s", err, out)
	}
	dropping, droppedConns := countingHost(t, nil)
	out, _, err := runScriptFrom(t, dropping, sum, t.TempDir(), "ALLURE_CACHE_DIR="+cache)
	if err != nil || !strings.Contains(out, "report written") {
		t.Fatalf("a valid cached archive must carry the report: %v\n%s", err, out)
	}
	if n := droppedConns(); n != 0 {
		t.Fatalf("a cache hit opened %d connections, want 0", n)
	}
}
