package update

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
)

// countingServer is a fake release server that counts the requests it gets.
func countingServer(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	inner := newRelease(t).serve(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		inner.Config.Handler.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

// specManagedPrefixes reads the list SPEC.md names for the managed-prefix refusal.
func specManagedPrefixes(t *testing.T) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "SPEC.md"))
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`these managed prefixes: (.*?) \(end of managed prefixes`).FindSubmatch(b)
	if m == nil {
		t.Fatal("SPEC.md does not name the managed prefixes")
	}
	var out []string
	for _, tok := range regexp.MustCompile("`([^`]+)`").FindAllSubmatch(m[1], -1) {
		out = append(out, string(tok[1]))
	}
	return out
}

func rootedExe(t *testing.T, rel string) (root, exe string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	exe = filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte(oldBody), 0o755); err != nil {
		t.Fatal(err)
	}
	return root, exe
}

func requireRefusal(t *testing.T, err error, reason string) string {
	t.Helper()
	var r *Refusal
	if !errors.As(err, &r) {
		t.Fatalf("err = %v, want a %s refusal", err, reason)
	}
	if r.Reason != reason || r.Message == "" {
		t.Fatalf("refusal = %+v, want reason %s with a message", r, reason)
	}
	return r.Message
}

func TestRefuseDevBuild(t *testing.T) {
	srv, hits := countingServer(t)
	dir, exe := installOld(t)
	before := fileSum(t, exe)
	_, err := Run(context.Background(), Options{BaseURL: srv.URL, Current: "dev", Exe: exe, GOOS: "linux", GOARCH: "amd64"})
	msg := requireRefusal(t, err, RefusedDev)
	if !strings.Contains(msg, "dev build") {
		t.Errorf("message %q does not say it is a dev build", msg)
	}
	if fileSum(t, exe) != before {
		t.Error("the binary changed")
	}
	if l := listing(t, dir); len(l) != 1 {
		t.Errorf("install dir = %v, want only deck", l)
	}
	if hits.Load() != 0 {
		t.Errorf("%d requests made by a refused update", hits.Load())
	}
}

func TestSpecNamesHomebrewAndNix(t *testing.T) {
	got := strings.Join(specManagedPrefixes(t), " ")
	for _, want := range []string{"/opt/homebrew", "/usr/local/Cellar", "/home/linuxbrew", "/nix/store"} {
		if !strings.Contains(" "+got+" ", " "+want+" ") {
			t.Errorf("SPEC managed prefixes %q lack %s", got, want)
		}
	}
}

func TestRefuseManagedPrefix(t *testing.T) {
	prefixes := specManagedPrefixes(t)
	if strings.Join(prefixes, "\n") != strings.Join(ManagedPrefixes, "\n") {
		t.Fatalf("SPEC list %v != ManagedPrefixes %v", prefixes, ManagedPrefixes)
	}
	srv, hits := countingServer(t)
	for _, p := range prefixes { // one row per SPEC prefix
		t.Run(strings.TrimPrefix(p, "/"), func(t *testing.T) {
			root, exe := rootedExe(t, filepath.Join(p, "deck"))
			before := fileSum(t, exe)
			_, err := Run(context.Background(), Options{BaseURL: srv.URL, Current: "v1.0.0", Exe: exe, GOOS: "linux", GOARCH: "amd64", root: root})
			msg := requireRefusal(t, err, RefusedManaged)
			if !strings.Contains(msg, p) {
				t.Errorf("message %q does not name %s", msg, p)
			}
			if fileSum(t, exe) != before {
				t.Error("the binary changed")
			}
		})
	}
	if hits.Load() != 0 {
		t.Errorf("%d requests made by a refused update", hits.Load())
	}
}

func TestRefuseManagedPrefixThroughSymlink(t *testing.T) {
	root, target := rootedExe(t, "usr/local/Cellar/deck/1.0/bin/deck")
	link := filepath.Join(root, "usr/local/bin/deck")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	before := fileSum(t, target)
	_, err := Run(context.Background(), Options{BaseURL: "http://127.0.0.1:1", Current: "v1.0.0", Exe: link, GOOS: "linux", GOARCH: "amd64", root: root})
	_ = requireRefusal(t, err, RefusedManaged)
	if fileSum(t, target) != before {
		t.Error("the binary changed")
	}
}

func TestUsrLocalBinIsNotManaged(t *testing.T) {
	if m := managedPrefix("/usr/local/bin/deck", ""); m != "" {
		t.Errorf("/usr/local/bin/deck refused as under %s", m)
	}
}

func TestRefuseUnwritableDir(t *testing.T) {
	srv, hits := countingServer(t)
	dir, exe := installOld(t)
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	if f, err := os.CreateTemp(dir, "probe"); err == nil {
		_ = f.Close()
		t.Skip("this user can write to a mode-0555 directory (root), so it cannot be made unwritable")
	}
	before := fileSum(t, exe)
	_, err := Run(context.Background(), Options{BaseURL: srv.URL, Current: "v1.0.0", Exe: exe, GOOS: "linux", GOARCH: "amd64"})
	msg := requireRefusal(t, err, RefusedUnwritable)
	for _, want := range []string{dir, "sudo", "~/.local/bin", "DECK_INSTALL_DIR"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message %q lacks %q", msg, want)
		}
	}
	if fileSum(t, exe) != before {
		t.Error("the binary changed")
	}
	if hits.Load() != 0 {
		t.Errorf("%d requests made by a refused update", hits.Load())
	}
}

// TestRefusalNeverEscalates puts recording fakes for sudo, doas and pkexec on
// PATH and runs the three refusal cases; none of them may be invoked.
func TestRefusalNeverEscalates(t *testing.T) {
	fakes := t.TempDir()
	record := filepath.Join(t.TempDir(), "calls")
	for _, name := range []string{"sudo", "doas", "pkexec"} {
		script := "#!/bin/sh\necho " + name + " \"$@\" >> '" + record + "'\nexit 0\n"
		if err := os.WriteFile(filepath.Join(fakes, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", fakes)

	srv, _ := countingServer(t)
	run := func(o Options) error {
		o.BaseURL, o.GOOS, o.GOARCH = srv.URL, "linux", "amd64"
		_, err := Run(context.Background(), o)
		return err
	}
	_, devExe := installOld(t)
	_ = requireRefusal(t, run(Options{Current: "dev", Exe: devExe}), RefusedDev)

	root, managedExe := rootedExe(t, "opt/homebrew/bin/deck")
	_ = requireRefusal(t, run(Options{Current: "v1.0.0", Exe: managedExe, root: root}), RefusedManaged)

	dir, roExe := installOld(t)
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	if f, err := os.CreateTemp(dir, "probe"); err == nil {
		_ = f.Close()
		t.Log("root can write to a mode-0555 directory: the unwritable case is not exercised here")
	} else {
		_ = requireRefusal(t, run(Options{Current: "v1.0.0", Exe: roExe}), RefusedUnwritable)
	}

	if b, err := os.ReadFile(record); err == nil {
		t.Errorf("a privilege-escalation tool was invoked:\n%s", b)
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

// TestPreflightNamesTheRefusalsWithoutTheNetwork: the TUI indicator's
// preflight returns the same Refusals Run does, for a dev build and a managed
// binary, and nil for a writable one.
func TestPreflightNamesTheRefusalsWithoutTheNetwork(t *testing.T) {
	root, exe := rootedExe(t, "/usr/bin/deck")
	_ = requireRefusal(t, Preflight(Options{Current: "dev", Exe: exe, root: root}), RefusedDev)
	_ = requireRefusal(t, Preflight(Options{Current: "v1.0.0", Exe: exe, root: root}), RefusedManaged)
	_, writable := rootedExe(t, "/usr/local/bin/deck")
	if err := Preflight(Options{Current: "v1.0.0", Exe: writable}); err != nil {
		t.Fatalf("writable binary: Preflight = %v, want nil", err)
	}
}
