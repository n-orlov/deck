package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/update"
)

const (
	updNewBody = "the new deck binary\n"
	updLatest  = "v1.2.0"
)

// fakeRelease is a loopback release server: any tag's downloads answer with
// the same archive, `/releases/latest` redirects to updLatest.
type fakeRelease struct {
	archive   []byte
	checksums string
	server    *httptest.Server
}

func updTarGz(t *testing.T, body string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	if err := tw.WriteHeader(&tar.Header{Name: "deck", Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(body)); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func newFakeRelease(t *testing.T, goodChecksum bool) *fakeRelease {
	t.Helper()
	asset, err := update.AssetName(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Skip(err)
	}
	r := &fakeRelease{archive: updTarGz(t, updNewBody)}
	sum := sha256.Sum256(r.archive)
	hexSum := hex.EncodeToString(sum[:])
	if !goodChecksum {
		hexSum = strings.Repeat("0", 64)
	}
	r.checksums = hexSum + "  " + asset + "\n"
	mux := http.NewServeMux()
	mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, req *http.Request) {
		http.Redirect(w, req, "/releases/tag/"+updLatest, http.StatusFound)
	})
	mux.HandleFunc("/releases/download/", func(w http.ResponseWriter, req *http.Request) {
		switch {
		case strings.HasSuffix(req.URL.Path, "/"+asset):
			_, _ = w.Write(r.archive)
		case strings.HasSuffix(req.URL.Path, "/checksums.txt"):
			_, _ = w.Write([]byte(r.checksums))
		default:
			http.NotFound(w, req)
		}
	})
	r.server = httptest.NewServer(mux)
	t.Cleanup(r.server.Close)
	return r
}

func updSum(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func updNames(t *testing.T, dir string) []string {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range ents {
		names = append(names, e.Name())
	}
	return names
}

// withVersion sets the package version for one test, as -ldflags does for a release.
func withVersion(t *testing.T, v string) {
	t.Helper()
	old := version
	version = v
	t.Cleanup(func() { version = old })
}

// TestUpdateVerbBeforeConfigAndTmux: `deck update --check` answers with no
// tmux on PATH, no config file and no state directory, and writes nothing.
func TestUpdateVerbBeforeConfigAndTmux(t *testing.T) {
	rel := newFakeRelease(t, true)
	withVersion(t, "v1.0.0")
	home := t.TempDir()
	t.Setenv("PATH", t.TempDir())
	t.Setenv("HOME", home)
	t.Setenv("DECK_HOME", filepath.Join(home, "no-such-deck-home"))
	t.Setenv("DECK_UPDATE_URL", rel.server.URL)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "cfg"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
	if _, err := exec.LookPath("tmux"); err == nil {
		t.Fatal("tmux is still on PATH")
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"deck", "update", "--check"}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr.String())
	}
	if want := "update available: v1.0.0 -> " + updLatest; !strings.Contains(stdout.String(), want) {
		t.Fatalf("stdout %q lacks %q", stdout.String(), want)
	}
	if names := updNames(t, home); len(names) != 0 {
		t.Fatalf("--check wrote %v under HOME", names)
	}
}

// TestUpdateVerbWinsOverProfileArg: `deck update` runs the verb, not a profile
// named update, even when DECK_PROFILE=update; DECK_PROFILE=update with no
// argument is not the verb.
func TestUpdateVerbWinsOverProfileArg(t *testing.T) {
	withVersion(t, "dev")
	home := t.TempDir()
	t.Setenv("DECK_HOME", filepath.Join(home, "deck"))
	t.Setenv("DECK_PROFILE", "update")
	t.Setenv("DECK_UPDATE_URL", "http://127.0.0.1:1")
	if !isUpdateRequest([]string{"deck", "update"}) || isUpdateRequest([]string{"deck"}) || isUpdateRequest([]string{"deck", "work", "update"}) {
		t.Fatal("isUpdateRequest does not select exactly `deck update ...`")
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"deck", "update"}, strings.NewReader(""), &stdout, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "dev build") {
		t.Fatalf("exit %d, stderr %q: want the verb's dev-build refusal", code, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(home, "deck")); err == nil {
		t.Fatal("deck update created a profile directory")
	}
}

func TestParseUpdateArgs(t *testing.T) {
	good := map[string]updateArgs{
		"":                 {},
		"--check":          {check: true},
		"--version v1.0.0": {version: "v1.0.0"},
		"--version=v1.0.0": {version: "v1.0.0"},
	}
	for in, want := range good {
		got, ok := parseUpdateArgs(strings.Fields(in), &bytes.Buffer{})
		if !ok || got != want {
			t.Errorf("parseUpdateArgs(%q) = %+v ok=%v, want %+v", in, got, ok, want)
		}
	}
	for _, in := range []string{"--bogus", "stray", "--version", "--version v1.0.0 --check"} {
		var stderr bytes.Buffer
		if _, ok := parseUpdateArgs(strings.Fields(in), &stderr); ok || !strings.HasPrefix(stderr.String(), "error: ") {
			t.Errorf("parseUpdateArgs(%q) ok=%v stderr=%q, want an error", in, ok, stderr.String())
		}
	}
}

// updBinaries builds the real deck once per configuration for the rows below.
type updBinaries map[string]string

func (b updBinaries) build(t *testing.T, name, ldflags string, tags ...string) string {
	t.Helper()
	if path, ok := b[name]; ok {
		return path
	}
	path := filepath.Join(t.TempDir(), "deck-"+name)
	args := []string{"build"}
	if len(tags) > 0 {
		args = append(args, "-tags", strings.Join(tags, ","))
	}
	if ldflags != "" {
		args = append(args, "-ldflags", ldflags)
	}
	args = append(args, "-o", path, ".")
	cmd := exec.Command("go", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build deck %s: %v\n%s", name, err, out)
	}
	b[name] = path
	return path
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Clean(dst), data, 0o755); err != nil { //nolint:gosec // a test's own temp directory
		t.Fatal(err)
	}
}

// updRun runs the binary with a hermetic environment and returns stdout,
// stderr and the exit code.
func updRun(t *testing.T, binary, home, path string, env []string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Env = append([]string{"PATH=" + path, "HOME=" + home, "DECK_HOME=" + filepath.Join(home, "deck-home"), "XDG_CONFIG_HOME=" + filepath.Join(home, "cfg"), "XDG_DATA_HOME=" + filepath.Join(home, "data"), "XDG_CACHE_HOME=" + filepath.Join(home, "cache")}, env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			t.Fatalf("run %s %v: %v", binary, args, err)
		}
		code = ee.ExitCode()
	}
	return stdout.String(), stderr.String(), code
}

// TestUpdateVerbAgainstFakeServer drives the real binary, built with a release
// version, against a loopback fake release server.
func TestUpdateVerbAgainstFakeServer(t *testing.T) {
	bins := updBinaries{}
	v100 := func() string { return bins.build(t, "v1.0.0", "-X main.version=v1.0.0") }
	v120 := func() string { return bins.build(t, "v1.2.0", "-X main.version="+updLatest) }
	emptyPath := t.TempDir()

	// place copies the binary into a fresh install directory.
	place := func(src string) (dir, exe string) {
		dir = t.TempDir()
		exe = filepath.Join(dir, "deck")
		copyFile(t, src, exe)
		return dir, exe
	}

	t.Run("success", func(t *testing.T) {
		rel := newFakeRelease(t, true)
		url := "DECK_UPDATE_URL=" + rel.server.URL

		t.Run("install", func(t *testing.T) {
			dir, exe := place(v100())
			home := t.TempDir()
			stdout, stderr, code := updRun(t, exe, home, emptyPath, []string{url}, "update")
			if code != 0 {
				t.Fatalf("exit %d, stderr %q", code, stderr)
			}
			for _, want := range []string{"v1.0.0", updLatest, "restart"} {
				if !strings.Contains(stdout, want) {
					t.Errorf("stdout %q lacks %q", stdout, want)
				}
			}
			if strings.Contains(stdout, "checksum was verified") {
				t.Errorf("stdout %q carries a checksum-only note; the checksum is the only check", stdout)
			}
			if got, want := updSum(t, exe), func() string { s := sha256.Sum256([]byte(updNewBody)); return hex.EncodeToString(s[:]) }(); got != want {
				t.Errorf("binary sha256 = %s, want the release binary %s", got, want)
			}
			if names := updNames(t, dir); len(names) != 1 {
				t.Errorf("install directory holds %v, want only deck", names)
			}
		})

		t.Run("check newer", func(t *testing.T) {
			dir, exe := place(v100())
			home := t.TempDir()
			before := updSum(t, exe)
			stdout, stderr, code := updRun(t, exe, home, emptyPath, []string{url}, "update", "--check")
			if code != 0 || !strings.Contains(stdout, "update available: v1.0.0 -> "+updLatest) {
				t.Fatalf("exit %d stdout %q stderr %q", code, stdout, stderr)
			}
			if updSum(t, exe) != before || len(updNames(t, dir)) != 1 || len(updNames(t, home)) != 0 {
				t.Errorf("--check changed something: dir %v home %v", updNames(t, dir), updNames(t, home))
			}
		})

		t.Run("check current", func(t *testing.T) {
			dir, exe := place(v120())
			home := t.TempDir()
			before := updSum(t, exe)
			stdout, stderr, code := updRun(t, exe, home, emptyPath, []string{url}, "update", "--check")
			if code != 0 || !strings.Contains(stdout, "up to date") {
				t.Fatalf("exit %d stdout %q stderr %q", code, stdout, stderr)
			}
			if updSum(t, exe) != before || len(updNames(t, dir)) != 1 || len(updNames(t, home)) != 0 {
				t.Errorf("--check changed something: dir %v home %v", updNames(t, dir), updNames(t, home))
			}
		})

		t.Run("version downgrade", func(t *testing.T) {
			_, exe := place(v120())
			home := t.TempDir()
			stdout, stderr, code := updRun(t, exe, home, emptyPath, []string{url}, "update", "--version", "v1.0.0")
			if code != 0 {
				t.Fatalf("exit %d, stderr %q", code, stderr)
			}
			for _, want := range []string{updLatest, "v1.0.0", "restart"} {
				if !strings.Contains(stdout, want) {
					t.Errorf("stdout %q lacks %q", stdout, want)
				}
			}
			if updSum(t, exe) == updSum(t, v120()) {
				t.Error("the pinned downgrade left the binary unchanged")
			}
		})

		t.Run("offline check", func(t *testing.T) {
			dead := httptest.NewServer(http.NotFoundHandler())
			deadURL := dead.URL
			dead.Close()
			dir, exe := place(v100())
			home := t.TempDir()
			before := updSum(t, exe)
			_, stderr, code := updRun(t, exe, home, emptyPath, []string{"DECK_UPDATE_URL=" + deadURL}, "update", "--check")
			if code == 0 || !strings.HasPrefix(stderr, "deck update: ") {
				t.Fatalf("exit %d stderr %q, want a clear error and a non-zero exit", code, stderr)
			}
			if updSum(t, exe) != before || len(updNames(t, dir)) != 1 || len(updNames(t, home)) != 0 {
				t.Errorf("offline --check changed something: dir %v home %v", updNames(t, dir), updNames(t, home))
			}
		})
	})

	t.Run("failure", func(t *testing.T) {
		good := newFakeRelease(t, true)
		goodURL := "DECK_UPDATE_URL=" + good.server.URL
		// expect asserts a non-zero exit, a stderr message and an untouched binary.
		expect := func(t *testing.T, stderr string, code int, want string) {
			t.Helper()
			if code == 0 || !strings.HasPrefix(stderr, "deck update: ") || !strings.Contains(stderr, want) {
				t.Fatalf("exit %d stderr %q, want a non-zero exit and %q", code, stderr, want)
			}
		}

		t.Run("checksum mismatch", func(t *testing.T) {
			bad := newFakeRelease(t, false)
			dir, exe := place(v100())
			before := updSum(t, exe)
			_, stderr, code := updRun(t, exe, t.TempDir(), emptyPath, []string{"DECK_UPDATE_URL=" + bad.server.URL}, "update")
			expect(t, stderr, code, "checksum mismatch")
			if updSum(t, exe) != before || len(updNames(t, dir)) != 1 {
				t.Errorf("binary or directory changed: %v", updNames(t, dir))
			}
		})

		t.Run("dev build", func(t *testing.T) {
			dir, exe := place(bins.build(t, "dev", ""))
			before := updSum(t, exe)
			_, stderr, code := updRun(t, exe, t.TempDir(), emptyPath, []string{goodURL}, "update")
			expect(t, stderr, code, "dev build")
			if updSum(t, exe) != before || len(updNames(t, dir)) != 1 {
				t.Errorf("binary or directory changed: %v", updNames(t, dir))
			}
		})

		t.Run("managed prefix", func(t *testing.T) {
			tagged := bins.build(t, "v1.0.0-roottest", "-X main.version=v1.0.0", "deckupdatetest")
			root := t.TempDir()
			dir := filepath.Join(root, "usr", "bin")
			exe := filepath.Join(dir, "deck")
			copyFile(t, tagged, exe)
			before := updSum(t, exe)
			_, stderr, code := updRun(t, exe, t.TempDir(), emptyPath, []string{goodURL, "DECK_UPDATE_TEST_ROOT=" + root}, "update")
			expect(t, stderr, code, "package manager")
			if updSum(t, exe) != before || len(updNames(t, dir)) != 1 {
				t.Errorf("binary or directory changed: %v", updNames(t, dir))
			}
		})

		t.Run("unwritable directory", func(t *testing.T) {
			if os.Geteuid() == 0 {
				t.Skip("root can write to any directory")
			}
			dir, exe := place(v100())
			before := updSum(t, exe)
			if err := os.Chmod(dir, 0o555); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
			_, stderr, code := updRun(t, exe, t.TempDir(), emptyPath, []string{goodURL}, "update")
			expect(t, stderr, code, "cannot write to the install directory")
			if updSum(t, exe) != before || len(updNames(t, dir)) != 1 {
				t.Errorf("binary or directory changed: %v", updNames(t, dir))
			}
		})
	})
}

const updSentinel = "cmd-mirror-password-sentinel"

// TestUpdateVerbRedactsMirrorCredentials drives the real binary against a
// loopback mirror whose DECK_UPDATE_URL carries a password. Every failure
// (HTTP status, truncated body, size limit, unreachable host, refused scheme,
// --check, and passwords holding ' @ ( ) ! $ & * + , ; = ~) keeps every piece
// of the password out of stdout and stderr, still names the host and the
// cause, and leaves the installed binary and directory untouched.
func TestUpdateVerbRedactsMirrorCredentials(t *testing.T) {
	asset, err := update.AssetName(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Skip(err)
	}
	bins := updBinaries{}
	v100 := bins.build(t, "v1.0.0", "-X main.version=v1.0.0")
	emptyPath := t.TempDir()
	mirror := func(h func(w http.ResponseWriter, r *http.Request)) string {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/releases/latest" {
				http.Redirect(w, r, "/releases/tag/"+updLatest, http.StatusFound)
				return
			}
			h(w, r)
		}))
		t.Cleanup(srv.Close)
		return strings.Replace(srv.URL, "http://", "http://mirror-user:"+updSentinel+"@", 1)
	}
	dead := httptest.NewServer(http.NotFoundHandler())
	deadURL := strings.Replace(dead.URL, "http://", "http://mirror-user:"+updSentinel+"@", 1)
	dead.Close()
	type redactRow struct {
		name, url, want string
		args            []string
	}
	rows := []redactRow{
		{"http status", mirror(func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "x", 500) }), "500 Internal Server Error", []string{"update"}},
		{"truncated body", mirror(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Length", "100000")
			_, _ = w.Write([]byte("short"))
		}), "unexpected EOF", []string{"update"}},
		{"size limit", mirror(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/"+asset) {
				_, _ = w.Write(updTarGz(t, updNewBody))
				return
			}
			_, _ = w.Write(bytes.Repeat([]byte("a"), 2<<20))
		}), "larger than", []string{"update"}},
		{"unreachable", deadURL, "127.0.0.1", []string{"update"}},
		{"unreachable check", deadURL, "127.0.0.1", []string{"update", "--check"}},
		{"refused scheme", "ftp://mirror-user:" + updSentinel + "@mirror.example/x", "mirror.example", []string{"update"}},
		{"refused scheme check", "http://mirror-user:" + updSentinel + "@mirror.example/x", "mirror.example", []string{"update", "--check"}},
	}
	// Passwords whose bytes a "safe character" match misses: url.Parse takes
	// the userinfo up to the LAST '@' and accepts ' ( ) ! $ & * + , ; = ~ %.
	// Every piece a partial match could leave behind holds "sentinel".
	unavailable := mirror(func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "down", http.StatusServiceUnavailable) })
	for name, pw := range map[string]string{
		"apostrophe":  "independent'sentinel-password",
		"at sign":     "sentinel-left@sentinel-right",
		"sub-delims":  "sentinel(!$&*+,;=~)sentinel",
		"apostrophe@": "it's-sentinel@'sentinel'",
	} {
		odd := strings.Replace(unavailable, "mirror-user:"+updSentinel, "mirror:"+pw, 1)
		oddDead := strings.Replace(deadURL, "mirror-user:"+updSentinel, "mirror:"+pw, 1)
		rows = append(rows,
			redactRow{name + " pinned 503", odd, "503 Service Unavailable", []string{"update", "--version", "v8.4.1"}},
			redactRow{name + " latest 503", odd, "503 Service Unavailable", []string{"update"}},
			redactRow{name + " unreachable check", oddDead, "127.0.0.1", []string{"update", "--check"}},
		)
	}
	// Secrets holding a byte that ends a free-text match ('"', '\\', '<',
	// whitespace), in the query and the userinfo: an earlier, partial
	// redaction must not leave the piece after that byte behind.
	plain503 := strings.Replace(unavailable, "mirror-user:"+updSentinel+"@", "", 1)
	plainDead := strings.Replace(deadURL, "mirror-user:"+updSentinel+"@", "", 1)
	for name, secret := range map[string]string{
		"query quote":        "?token=sentinel-left\"sentinel-tail",
		"query backslash":    "?token=sentinel-left\\sentinel-tail",
		"query angle":        "?token=sentinel-left<sentinel-tail>",
		"userinfo quote":     "mirror:sentinel-left\"sentinel-tail@",
		"userinfo backslash": "mirror:sentinel-left\\sentinel-tail@",
	} {
		with := func(u string) string {
			if strings.HasPrefix(secret, "?") {
				return u + secret
			}
			return strings.Replace(u, "http://", "http://"+secret, 1)
		}
		rows = append(rows,
			redactRow{name + " pinned 503", with(plain503), "127.0.0.1", []string{"update", "--version", "v8.4.1"}},
			redactRow{name + " unreachable check", with(plainDead), "127.0.0.1", []string{"update", "--check"}},
		)
	}
	// Credentials shorter than any "sentinel" word, holding a byte that ends
	// a free-text match: the one-byte tail "Z" must not survive either.
	for name, secret := range map[string]string{
		"short query angle":     "?t=K>Z",
		"short query space":     "?t=K Z",
		"short query quote":     "?t=K\"Z",
		"short query one byte":  "?t=Z",
		"short query pair":      "?a=1&t=K>Z&b=2",
		"short userinfo space":  "m:K Z@",
		"short userinfo angle":  "m:K>Z@",
		"short userinfo single": "m:Z@",
	} {
		with := func(u string) string {
			if strings.HasPrefix(secret, "?") {
				return u + secret
			}
			return strings.Replace(u, "http://", "http://"+secret, 1)
		}
		rows = append(rows,
			redactRow{name + " pinned 503", with(plain503), "127.0.0.1", []string{"update", "--version", "v8.4.1"}},
			redactRow{name + " latest", with(plain503), "deck update: ", []string{"update"}},
			redactRow{name + " unreachable check", with(plainDead), "127.0.0.1", []string{"update", "--check"}},
		)
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			dir := t.TempDir()
			exe := filepath.Join(dir, "deck")
			copyFile(t, v100, exe)
			before := updSum(t, exe)
			stdout, stderr, code := updRun(t, exe, t.TempDir(), emptyPath, []string{"DECK_UPDATE_URL=" + row.url}, row.args...)
			if code == 0 || !strings.HasPrefix(stderr, "deck update: ") || !strings.Contains(stderr, row.want) {
				t.Fatalf("exit %d stderr %q, want a failure naming %q", code, stderr, row.want)
			}
			if strings.Contains(stdout+stderr, "sentinel") {
				t.Fatalf("the mirror password reached the output: stdout %q stderr %q", stdout, stderr)
			}
			if strings.HasPrefix(row.name, "short ") && strings.ContainsAny(strings.TrimPrefix(stdout+stderr, ""), "ZK") {
				t.Fatalf("a short credential reached the output: stdout %q stderr %q", stdout, stderr)
			}
			if updSum(t, exe) != before || len(updNames(t, dir)) != 1 {
				t.Errorf("binary or directory changed: %v", updNames(t, dir))
			}
		})
	}
}
