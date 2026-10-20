// toolchain_test.go is task 012's probe (R197, #61): go.mod's `go 1.25.0`
// line only names the language floor, so a `toolchain go1.N.P` line names
// the patched release that `go install` and local builds resolve to. The
// probe fails the moment the toolchain line is removed, names the floor
// release of its line, or is older than the go directive. (CI and release
// builds use the Go in ci/Dockerfile, pinned there.)
package repocheck

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
)

var (
	toolchainLineRE = regexp.MustCompile(`(?m)^toolchain go(1\.\d+(?:\.\d+|rc\d+)?)`)
	goLineRE        = regexp.MustCompile(`(?m)^go (\d+(?:\.\d+)*)`)
	patchVersionRE  = regexp.MustCompile(`^1\.(\d+)\.(\d+)$`)
)

// toolchainVersion extracts the version a go.mod's toolchain line names
// ("" when absent).
func toolchainVersion(gomod string) string {
	if m := toolchainLineRE.FindStringSubmatch(gomod); m != nil {
		return m[1]
	}
	return ""
}

// isFloorRelease reports whether a resolved version is a bare
// 1.25 line (an unpatched "1.25" resolves to the latest, but "1.25.0"
// and a missing patch from the go directive are the floor the task
// exists to avoid).
func atoiOr0(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// goLineVersion returns the version operand of go.mod's `go` directive.
func goLineVersion(gomod string) string {
	if m := goLineRE.FindStringSubmatch(gomod); m != nil {
		return m[1]
	}
	return ""
}

func isFloorRelease(v string) bool {
	m := patchVersionRE.FindStringSubmatch(v)
	if m == nil {
		return false
	}
	patch, _ := strconv.Atoi(m[2])
	return patch == 0
}

func readRepoFile(t *testing.T, rel ...string) string {
	t.Helper()
	root, err := repositoryRoot()
	if err != nil {
		t.Fatalf("repositoryRoot: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(append([]string{root}, rel...)...))
	if err != nil {
		t.Fatalf("read %v: %v", rel, err)
	}
	return string(raw)
}

func TestGoModCarriesPatchedToolchainLine(t *testing.T) {
	gomod := readRepoFile(t, "go.mod")
	v := toolchainVersion(gomod)
	if v == "" {
		t.Fatalf("go.mod has no `toolchain go1.25.N` line: go install falls back to the `go` directive (1.25.0)")
	}
	m := patchVersionRE.FindStringSubmatch(v)
	if m == nil {
		t.Fatalf("go.mod toolchain line names %q, want a 1.N.P patch release", v)
	}
	// The toolchain may run a newer Go line than the language floor (the
	// floor says what the code needs, the toolchain what gets installed),
	// never an older one.
	floor := patchVersionRE.FindStringSubmatch(goLineVersion(gomod))
	if floor == nil {
		t.Fatalf("go.mod has no parsable `go 1.N.P` directive to compare the toolchain line %q with", v)
	}
	if toolMinor, _ := strconv.Atoi(m[1]); toolMinor < atoiOr0(floor[1]) {
		t.Fatalf("go.mod toolchain line names %q, older than the go directive's 1.%s line", v, floor[1])
	}
	if isFloorRelease(v) {
		t.Fatalf("go.mod toolchain line names %q: the floor release misses every later fix of its line", v)
	}
}

// repositoryRoot walks up from the test's directory to the one holding go.mod.
func repositoryRoot() (string, error) {
	directory, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(directory, "go.mod")); err == nil {
			return directory, nil
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return "", errors.New("locate repository root containing go.mod")
		}
		directory = parent
	}
}
