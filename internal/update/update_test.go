package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const (
	oldBody = "old deck binary\n"
	newBody = "new deck binary\n"
	tagV    = "v1.2.3"
)

func tarGz(t *testing.T, name, body string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
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

func sum(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

// release is one fake GitHub release: what the server hands out per path.
type release struct {
	asset     string
	archive   []byte
	checksums string
	missing   map[string]bool // paths answered with 404
}

func newRelease(t *testing.T) *release {
	t.Helper()
	asset, err := AssetName("linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	archive := tarGz(t, "deck", newBody)
	return &release{asset: asset, archive: archive, checksums: sum(archive) + "  " + asset + "\n" + strings.Repeat("0", 64) + "  other.tar.gz\n", missing: map[string]bool{}}
}

func (r *release) serve(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	handle := func(path string, h http.HandlerFunc) {
		mux.HandleFunc(path, func(w http.ResponseWriter, req *http.Request) {
			if r.missing[path] {
				http.NotFound(w, req)
				return
			}
			h(w, req)
		})
	}
	handle("/releases/latest", func(w http.ResponseWriter, req *http.Request) {
		http.Redirect(w, req, "/releases/tag/"+tagV, http.StatusFound)
	})
	dl := "/releases/download/" + tagV + "/"
	handle(dl+r.asset, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(r.archive) })
	handle(dl+"checksums.txt", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(r.checksums)) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func installOld(t *testing.T) (dir, exe string) {
	t.Helper()
	dir = t.TempDir()
	exe = filepath.Join(dir, "deck")
	if err := os.WriteFile(exe, []byte(oldBody), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir, exe
}

func listing(t *testing.T, dir string) []string {
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

func fileSum(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return sum(b)
}

func TestUpdateReplacesBinary(t *testing.T) {
	rel := newRelease(t)
	srv := rel.serve(t)
	t.Setenv(EnvBaseURL, srv.URL) // Options.BaseURL stays empty: the env override is the path under test
	dir, exe := installOld(t)

	res, err := Run(context.Background(), Options{Exe: exe, GOOS: "linux", GOARCH: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(exe)
	if err != nil || string(got) != newBody {
		t.Fatalf("binary = %q, %v; want the new body", got, err)
	}
	if res.Tag != tagV {
		t.Errorf("tag = %q, want %q", res.Tag, tagV)
	}
	if fi, _ := os.Stat(exe); fi.Mode().Perm() != 0o755 {
		t.Errorf("mode = %v, want 0755", fi.Mode().Perm())
	}
	if l := listing(t, dir); !reflect.DeepEqual(l, []string{"deck"}) {
		t.Errorf("install dir = %v, want only deck", l)
	}
}

func TestUpdatePinnedVersionSkipsLookup(t *testing.T) {
	rel := newRelease(t)
	rel.missing["/releases/latest"] = true
	srv := rel.serve(t)
	_, exe := installOld(t)
	res, err := Run(context.Background(), Options{BaseURL: srv.URL + "/", Version: tagV, Exe: exe, GOOS: "linux", GOARCH: "amd64"})
	if err != nil || res.Tag != tagV {
		t.Fatalf("Run = %+v, %v", res, err)
	}
	if _, err := Run(context.Background(), Options{BaseURL: srv.URL, Version: "1.2", Exe: exe, GOOS: "linux", GOARCH: "amd64"}); err == nil {
		t.Error("a malformed pinned version was accepted")
	}
}

func TestUpdateResolvesSymlinkedExecutable(t *testing.T) {
	rel := newRelease(t)
	srv := rel.serve(t)
	realDir, realExe := installOld(t)
	linkDir := t.TempDir()
	link := filepath.Join(linkDir, "deck")
	if err := os.Symlink(realExe, link); err != nil {
		t.Fatal(err)
	}

	res, err := Run(context.Background(), Options{BaseURL: srv.URL, Exe: link, GOOS: "linux", GOARCH: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	if want, _ := filepath.EvalSymlinks(realExe); res.Path != want {
		t.Errorf("replaced %q, want the symlink target %q", res.Path, want)
	}
	if b, _ := os.ReadFile(realExe); string(b) != newBody {
		t.Errorf("symlink target = %q, want the new body", b)
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("the symlink was replaced: %v, %v", fi, err)
	}
	if l := listing(t, linkDir); !reflect.DeepEqual(l, []string{"deck"}) {
		t.Errorf("symlink dir = %v; nothing may be written beside the link", l)
	}
	if l := listing(t, realDir); !reflect.DeepEqual(l, []string{"deck"}) {
		t.Errorf("real dir = %v, want only deck", l)
	}
}

func TestUpdateFailureLeavesOldBinary(t *testing.T) {
	boom := errors.New("injected failure")
	rows := []struct {
		name    string
		breakIt func(*release)
		inject  func(*Options)
		want    string
	}{
		{"tag lookup", func(r *release) { r.missing["/releases/latest"] = true }, nil, "resolve latest release"},
		{"archive download", func(r *release) { r.missing["/releases/download/"+tagV+"/"+r.asset] = true }, nil, "download deck_linux_amd64.tar.gz"},
		{"checksums download", func(r *release) { r.missing["/releases/download/"+tagV+"/checksums.txt"] = true }, nil, "download checksums.txt"},
		{"checksum mismatch", func(r *release) { r.checksums = strings.Repeat("a", 64) + "  " + r.asset + "\n" }, nil, "checksum mismatch"},
		{"missing checksums entry", func(r *release) { r.checksums = strings.Repeat("a", 64) + "  other.tar.gz\n" }, nil, "no entry for"},
		{"tampered archive", func(r *release) { r.archive = append(append([]byte{}, r.archive...), 'x') }, nil, "checksum mismatch"},
		{"unextractable archive", func(r *release) {
			r.archive = []byte("not a gzip stream")
			r.checksums = sum(r.archive) + "  " + r.asset + "\n"
		}, nil, "unreadable archive"},
		{"write error", nil, func(o *Options) {
			o.writeFile = func(name string, data []byte, perm os.FileMode) error {
				if err := writeExclusive(name, data[:len(data)/2], perm); err != nil { // leave a partial file behind
					return err
				}
				return boom
			}
		}, "write the new binary"},
		{"rename error", nil, func(o *Options) { o.rename = func(string, string) error { return boom } }, "replace "},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			rel := newRelease(t)
			if row.breakIt != nil {
				row.breakIt(rel)
			}
			srv := rel.serve(t)
			dir, exe := installOld(t)
			beforeSum, beforeList := fileSum(t, exe), listing(t, dir)

			o := Options{BaseURL: srv.URL, Exe: exe, GOOS: "linux", GOARCH: "amd64"}
			if row.inject != nil {
				row.inject(&o)
			}
			_, err := Run(context.Background(), o)
			if err == nil || !strings.Contains(err.Error(), row.want) {
				t.Fatalf("err = %v, want it to contain %q", err, row.want)
			}
			if got := fileSum(t, exe); got != beforeSum {
				t.Errorf("old binary sha256 = %s, want %s", got, beforeSum)
			}
			if got := listing(t, dir); !reflect.DeepEqual(got, beforeList) {
				t.Errorf("install dir = %v, want %v", got, beforeList)
			}
		})
	}
}

func TestAssetName(t *testing.T) {
	for _, goos := range []string{"linux", "darwin"} {
		for _, arch := range []string{"amd64", "arm64"} {
			got, err := AssetName(goos, arch)
			if want := fmt.Sprintf("deck_%s_%s.tar.gz", goos, arch); err != nil || got != want {
				t.Errorf("AssetName(%s,%s) = %q, %v; want %q", goos, arch, got, err, want)
			}
		}
	}
}

func TestUnsupportedPlatform(t *testing.T) {
	for _, p := range [][3]string{{"windows", "amd64", "windows"}, {"linux", "386", "386"}, {"freebsd", "arm64", "freebsd"}} {
		_, err := AssetName(p[0], p[1])
		if !errors.Is(err, ErrUnsupportedPlatform) || !strings.Contains(err.Error(), p[2]) {
			t.Errorf("AssetName(%s,%s) err = %v, want an unsupported-platform error naming %q", p[0], p[1], err, p[2])
		}
	}
	_, exe := installOld(t)
	_, err := Run(context.Background(), Options{Exe: exe, GOOS: "windows", GOARCH: "amd64"})
	if !errors.Is(err, ErrUnsupportedPlatform) {
		t.Errorf("Run on windows: err = %v, want ErrUnsupportedPlatform", err)
	}
}

func TestBaseURLRequiresHTTPSExceptLoopback(t *testing.T) {
	rows := []struct {
		url string
		ok  bool
	}{
		{"https://github.com/n-orlov/deck", true},
		{"http://127.0.0.1:8080", true},
		{"http://localhost:9", true},
		{"http://[::1]:9", true},
		{"http://github.com/n-orlov/deck", false},
		{"http://example.com", false},
		{"ftp://127.0.0.1/x", false},
		{"http://10.0.0.5:8080", false},
		{"github.com/n-orlov/deck", false},
	}
	for _, r := range rows {
		if err := CheckURL(r.url); (err == nil) != r.ok {
			t.Errorf("CheckURL(%q) = %v, want ok=%v", r.url, err, r.ok)
		}
	}
	_, exe := installOld(t)
	_, err := Run(context.Background(), Options{BaseURL: "http://example.com/deck", Exe: exe, GOOS: "linux", GOARCH: "amd64"})
	if err == nil || !strings.Contains(err.Error(), "only https") {
		t.Errorf("Run with an http base: err = %v, want an https refusal", err)
	}
}

func TestBaseURLDefaults(t *testing.T) {
	t.Setenv(EnvBaseURL, "")
	if got := baseURL(""); got != DefaultBaseURL {
		t.Errorf("default base = %q, want %q", got, DefaultBaseURL)
	}
	t.Setenv(EnvBaseURL, "http://127.0.0.1:1/x/")
	if got := baseURL(""); got != "http://127.0.0.1:1/x" {
		t.Errorf("env base = %q", got)
	}
	if got := baseURL("http://127.0.0.1:2"); got != "http://127.0.0.1:2" {
		t.Errorf("explicit base = %q", got)
	}
}

func TestRedirectToPlainHTTPIsRefused(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://example.com/x", http.StatusFound)
	}))
	defer bad.Close()
	if _, err := get(context.Background(), newClient(), bad.URL, 10); err == nil || !strings.Contains(err.Error(), "only https") {
		t.Errorf("redirect to http://example.com: err = %v, want refusal", err)
	}
}

func TestLatestTagRejectsBadAnswers(t *testing.T) {
	for name, h := range map[string]http.HandlerFunc{
		"no redirect": func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("hi")) },
		"prerelease": func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/releases/tag/v1.0.0-rc1", http.StatusFound)
		},
		"not a tag": func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/releases", http.StatusFound) },
		"traversal": func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/releases/tag/..", http.StatusFound)
		},
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(h)
			defer srv.Close()
			if tag, err := LatestTag(context.Background(), newClient(), srv.URL); err == nil {
				t.Errorf("LatestTag = %q, want an error", tag)
			}
		})
	}
	if _, err := LatestTag(context.Background(), newClient(), "http://127.0.0.1:1"); err == nil {
		t.Error("an unreachable server resolved a tag")
	}
}

func TestDownloadLimitsAndStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/500" {
			http.Error(w, "x", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte("0123456789"))
	}))
	defer srv.Close()
	ctx := context.Background()
	if b, err := get(ctx, newClient(), srv.URL, 10); err != nil || string(b) != "0123456789" {
		t.Errorf("get at the limit = %q, %v", b, err)
	}
	if _, err := get(ctx, newClient(), srv.URL, 9); err == nil || !strings.Contains(err.Error(), "larger than") {
		t.Errorf("oversize: err = %v", err)
	}
	if _, err := get(ctx, newClient(), srv.URL+"/500", 10); err == nil {
		t.Error("a 500 was accepted")
	}
}

func TestExtractBinaryFindsNestedAndRejectsMissing(t *testing.T) {
	if b, err := extractBinary(tarGz(t, "deck_linux_amd64/deck", newBody)); err != nil || string(b) != newBody {
		t.Errorf("nested = %q, %v", b, err)
	}
	if _, err := extractBinary(tarGz(t, "README", "x")); err == nil || !strings.Contains(err.Error(), "no deck binary") {
		t.Errorf("no binary: err = %v", err)
	}
	if _, err := extractBinary(tarGz(t, "deck", "")); err == nil || !strings.Contains(err.Error(), "implausible") {
		t.Errorf("empty binary: err = %v", err)
	}
	trunc := tarGz(t, "deck", newBody)
	if _, err := extractBinary(trunc[:len(trunc)/2]); err == nil {
		t.Error("a truncated archive was extracted")
	}
}

func TestChecksumLineForms(t *testing.T) {
	h := strings.Repeat("AB", 32)
	got, err := checksumFor([]byte("junk\n"+h+" *a.tar.gz\n"), "a.tar.gz")
	if err != nil || got != strings.ToLower(h) {
		t.Errorf("binary-mode line = %q, %v", got, err)
	}
}

func TestResolveExecutableErrors(t *testing.T) {
	if _, err := resolveExecutable(""); err == nil {
		t.Error("empty exe accepted")
	}
	if _, err := resolveExecutable(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Error("missing exe accepted")
	}
}
