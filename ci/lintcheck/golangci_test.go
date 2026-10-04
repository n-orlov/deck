package lintcheck

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// These tests run the REAL pinned golangci-lint (the deck-ci image installs
// it, see ci/Dockerfile) with the repository's own .golangci.yml against
// small throwaway modules. They prove the config is an honest gate: a seeded
// unchecked error fails, a //nolint with no linter name or no reason fails,
// an empty package list fails, and the _test.go-scoped exclusions do not
// leak into production code.

const fixtureGoMod = "module example.com/lintfixture\n\ngo 1.25\n"

const cleanSource = "// Package fixture is a lint fixture.\npackage fixture\n\n// Answer returns the answer.\nfunc Answer() int { return 42 }\n"

// golangciBinary returns the golangci-lint on PATH; its absence is a broken
// environment, not a skip.
func golangciBinary(t *testing.T) string {
	t.Helper()
	bin, err := exec.LookPath("golangci-lint")
	if err != nil {
		t.Fatalf("golangci-lint is not on PATH (the deck-ci image installs it): %v", err)
	}
	return bin
}

// golangciRun is one golangci-lint invocation: its exit code and output.
type golangciRun struct {
	exitCode int
	output   string
}

// runGolangci writes files into a fresh module and runs golangci-lint with
// the repository's .golangci.yml over the given package patterns.
func runGolangci(t *testing.T, files map[string]string, patterns ...string) golangciRun {
	t.Helper()
	root, err := repositoryRoot()
	if err != nil {
		t.Fatalf("repositoryRoot: %v", err)
	}
	config := filepath.Join(root, ".golangci.yml")
	if _, err := os.Stat(config); err != nil {
		t.Fatalf("the repository's lint config is missing: %v", err)
	}
	dir := t.TempDir()
	files["go.mod"] = fixtureGoMod
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	args := append([]string{"run", "-c", config, "--timeout", "5m"}, patterns...)
	cmd := exec.Command(golangciBinary(t), args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GOTOOLCHAIN=local")
	out, runErr := cmd.CombinedOutput()
	result := golangciRun{output: string(out)}
	var exitErr *exec.ExitError
	switch {
	case runErr == nil:
	case errors.As(runErr, &exitErr):
		result.exitCode = exitErr.ExitCode()
	default:
		t.Fatalf("run golangci-lint: %v\n%s", runErr, out)
	}
	return result
}

func requireClean(t *testing.T, r golangciRun) {
	t.Helper()
	if r.exitCode != 0 {
		t.Fatalf("exit %d, want a clean run\n%s", r.exitCode, r.output)
	}
}

func requireFailure(t *testing.T, r golangciRun, want ...string) {
	t.Helper()
	if r.exitCode == 0 {
		t.Fatalf("exit 0, want a failing run\n%s", r.output)
	}
	for _, w := range want {
		if !strings.Contains(r.output, w) {
			t.Errorf("output lacks %q:\n%s", w, r.output)
		}
	}
}

func TestGolangci_CleanFixturePasses(t *testing.T) {
	requireClean(t, runGolangci(t, map[string]string{"a.go": cleanSource}, "./..."))
}

func TestGolangci_UncheckedErrorFails(t *testing.T) {
	src := "// Package fixture is a lint fixture.\npackage fixture\n\nimport \"os\"\n\n// Drop removes a file and ignores the error.\nfunc Drop() { os.Remove(\"x\") }\n"
	requireFailure(t, runGolangci(t, map[string]string{"a.go": src}, "./..."), "errcheck", "a.go:7")
}

func TestGolangci_NolintWithoutLinterNameFails(t *testing.T) {
	src := "// Package fixture is a lint fixture.\npackage fixture\n\nimport \"os\"\n\n// Drop removes a file and ignores the error.\nfunc Drop() { os.Remove(\"x\") } //nolint // best effort\n"
	requireFailure(t, runGolangci(t, map[string]string{"a.go": src}, "./..."), "nolintlint")
}

func TestGolangci_NolintWithoutReasonFails(t *testing.T) {
	src := "// Package fixture is a lint fixture.\npackage fixture\n\nimport \"os\"\n\n// Drop removes a file and ignores the error.\nfunc Drop() { os.Remove(\"x\") } //nolint:errcheck,gosec\n"
	requireFailure(t, runGolangci(t, map[string]string{"a.go": src}, "./..."), "nolintlint")
}

// TestGolangci_NolintWithLinterAndReasonIsAccepted is the control for the two
// failing //nolint cases above: the same line with a named linter and a
// reason is the one allowed form.
func TestGolangci_NolintWithLinterAndReasonIsAccepted(t *testing.T) {
	src := "// Package fixture is a lint fixture.\npackage fixture\n\nimport \"os\"\n\n// Drop removes a file and ignores the error.\nfunc Drop() { os.Remove(\"x\") } //nolint:errcheck,gosec // best-effort cleanup, nothing to do on failure\n"
	requireClean(t, runGolangci(t, map[string]string{"a.go": src}, "./..."))
}

func TestGolangci_EmptyPackageListFails(t *testing.T) {
	// A module with no Go files at all: nothing was analysed, which must not
	// read as a pass.
	requireFailure(t, runGolangci(t, map[string]string{"README.md": "no go here\n"}, "./..."))
}

func TestGolangci_NonexistentPackagePatternFails(t *testing.T) {
	requireFailure(t, runGolangci(t, map[string]string{"a.go": cleanSource}, "./nosuchdir/..."))
}

// TestGolangci_TestFileScopeDoesNotLeakIntoProduction proves the explicit
// _test.go rule: gosec G304 and an unchecked Close are tolerated in a test
// file, and the very same code in production code fails.
func TestGolangci_TestFileScopeDoesNotLeakIntoProduction(t *testing.T) {
	body := "// Package fixture is a lint fixture.\npackage fixture\n\nimport \"os\"\n\n// Read opens a file by name and closes it.\nfunc Read(name string) {\n\tf, err := os.Open(name)\n\tif err != nil {\n\t\treturn\n\t}\n\tdefer f.Close()\n}\n"
	t.Run("test file is exempt", func(t *testing.T) {
		src := "// Package fixture is a lint fixture.\npackage fixture\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\nfunc TestRead(t *testing.T) {\n\tt.Log(\"read\")\n\tf, err := os.Open(\"x\")\n\tif err != nil {\n\t\treturn\n\t}\n\tdefer f.Close()\n}\n"
		requireClean(t, runGolangci(t, map[string]string{"a.go": cleanSource, "a_test.go": src}, "./..."))
	})
	t.Run("production code is not", func(t *testing.T) {
		requireFailure(t, runGolangci(t, map[string]string{"a.go": body}, "./..."), "errcheck", "G304")
	})
}

// TestGolangci_ConfigEnablesTheRequiredLinters reads the checked-in config
// and holds it to R188: the standard set plus the named extras, nolintlint's
// two strictness switches, and no gocognit.
func TestGolangci_ConfigEnablesTheRequiredLinters(t *testing.T) {
	root, err := repositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, ".golangci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var live []string
	for _, line := range strings.Split(string(raw), "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "#") {
			live = append(live, line)
		}
	}
	text := strings.Join(live, "\n")
	for _, want := range []string{
		`version: "2"`, "default: standard",
		"- gosec", "- revive", "- gocritic", "- errorlint", "- misspell", "- unconvert",
		"- unparam", "- bodyclose", "- copyloopvar", "- nolintlint",
		"require-specific: true", "require-explanation: true",
	} {
		if !strings.Contains(text, want) {
			t.Errorf(".golangci.yml lacks %q", want)
		}
	}
	if strings.Contains(text, "gocognit") {
		t.Error(".golangci.yml enables gocognit; CRAP is the complexity gate")
	}
}
