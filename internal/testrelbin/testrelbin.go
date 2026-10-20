// Package testrelbin is test support: it hands out the bytes of a real,
// tiny Go executable whose embedded build info (runtime/debug.BuildInfo, read
// back by debug/buildinfo) reports a requested module version. Release
// fixtures serve it as the "deck" binary of a fake release, so the code under
// test sees what a genuine release asset carries: a Go binary whose Main.Version
// is the release tag.
//
// Only _test.go files import this package; no production package may.
package testrelbin

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const (
	// modulePath is the module the template binary is built as, so Main.Path
	// matches a real release's.
	modulePath = "github.com/n-orlov/deck"
	// develVersion is the Main.Version a `go build` outside any VCS checkout
	// records.
	develVersion = "(devel)"
	// padMarker introduces the filler run recorded in the build settings (via
	// -ldflags); the tag's extra length is taken out of that run so the build
	// info blob keeps its exact size.
	padMarker = "main.pad="
	padLen    = 256
)

var (
	// goTool and buildEnv are captured at program start: a test that later
	// changes PATH, HOME or the Go environment must not affect the build.
	goTool   = findGo()
	buildEnv = os.Environ()

	templateOnce sync.Once
	template     []byte
	templateErr  error

	mu    sync.Mutex
	cache = map[string][]byte{}
)

func findGo() string {
	if p, err := exec.LookPath("go"); err == nil {
		return p
	}
	return filepath.Join(os.Getenv("GOROOT"), "bin", "go")
}

// Bytes returns the executable bytes of a Go binary whose build info reports
// tag as the main module's version. The returned slice is the caller's own.
func Bytes(tb testing.TB, tag string) []byte {
	tb.Helper()
	b, err := Build(tag)
	if err != nil {
		tb.Fatalf("testrelbin: %v", err)
	}
	return b
}

// Devel returns the executable bytes of a Go binary whose build info is
// readable but carries no release version: Main.Version is "(devel)", as for a
// `go build` outside any tagged checkout. The returned slice is the caller's own.
func Devel(tb testing.TB) []byte {
	tb.Helper()
	mu.Lock()
	defer mu.Unlock()
	templateOnce.Do(func() { template, templateErr = buildTemplate() })
	if templateErr != nil {
		tb.Fatalf("testrelbin: %v", templateErr)
	}
	return append([]byte(nil), template...)
}

// Build is Bytes for callers without a testing.TB.
func Build(tag string) ([]byte, error) {
	if tag == "" || strings.ContainsAny(tag, "\t\n\r ") {
		return nil, fmt.Errorf("invalid version tag %q", tag)
	}
	mu.Lock()
	defer mu.Unlock()
	if b, ok := cache[tag]; ok {
		return append([]byte(nil), b...), nil
	}
	templateOnce.Do(func() { template, templateErr = buildTemplate() })
	if templateErr != nil {
		return nil, templateErr
	}
	b, err := stamp(template, tag)
	if err != nil {
		return nil, err
	}
	cache[tag] = b
	return append([]byte(nil), b...), nil
}

func buildTemplate() ([]byte, error) {
	dir, err := os.MkdirTemp("", "testrelbin-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	files := map[string]string{
		"go.mod":  "module " + modulePath + "\n\ngo 1.21\n",
		"main.go": "package main\n\nfunc main() {}\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			return nil, err
		}
	}
	out := filepath.Join(dir, "deck")
	ldflags := "-s -w -X " + padMarker + strings.Repeat("Z", padLen)
	cmd := exec.Command(goTool, "build", "-buildvcs=false", "-ldflags="+ldflags, "-o", out, ".") //nolint:gosec // G204: the go tool and a fixed argument list; test-only
	cmd.Dir = dir
	cmd.Env = append(append([]string(nil), buildEnv...),
		"CGO_ENABLED=0", "GOFLAGS=", "GOWORK=off", "GOTOOLCHAIN=local", "GO111MODULE=on")
	if msg, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("build template binary with %s: %w\n%s", goTool, err, msg)
	}
	return os.ReadFile(out) //nolint:gosec // G304: a path inside the temp dir created above
}

// stamp rewrites the template's module-version text to tag without changing
// the binary's size or layout. The linker keeps the build-info text in more
// than one place, so every copy is rewritten.
func stamp(tmpl []byte, tag string) ([]byte, error) {
	modLine := []byte("mod\t" + modulePath + "\t" + develVersion)
	marker := []byte(padMarker + strings.Repeat("Z", padLen))
	newLine := []byte("mod\t" + modulePath + "\t" + tag)
	delta := len(newLine) - len(modLine)
	if padLen-delta < 0 {
		return nil, fmt.Errorf("version tag %q is too long", tag)
	}
	out := append([]byte(nil), tmpl...)
	copies := 0
	for from := 0; ; {
		rel := bytes.Index(tmpl[from:], modLine)
		if rel < 0 {
			break
		}
		idx := from + rel
		end := idx + len(modLine)
		rel = bytes.Index(tmpl[end:], marker)
		if rel < 0 {
			return nil, fmt.Errorf("template binary has no filler after its module line at offset %d", idx)
		}
		padStart := end + rel + len(padMarker)
		pos := idx
		pos += copy(out[pos:], newLine)
		pos += copy(out[pos:], tmpl[end:padStart])
		pos += copy(out[pos:], bytes.Repeat([]byte("Z"), padLen-delta))
		if pos != padStart+padLen {
			return nil, fmt.Errorf("internal error: stamped build info changed size (%d != %d)", pos, padStart+padLen)
		}
		copies++
		from = pos
	}
	if bytes.Contains(out, modLine) {
		return nil, fmt.Errorf("a %q build-info copy was left unstamped", modLine)
	}
	if copies == 0 {
		return nil, fmt.Errorf("template binary has no %q build-info line", modLine)
	}
	return out, nil
}
