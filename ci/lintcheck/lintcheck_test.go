// Package lintcheck is the R145 probe for task 013/ci/lint.sh. It EXECUTES
// the script -- a copy of the repository's own ci/lint.sh, run inside a
// throwaway git repository -- with stub `go` and `gofmt` binaries first on
// PATH that record every invocation and can be told to fail or report
// drift. The probe then asserts which stages actually ran, in what order,
// and with what exit status. It therefore fails the moment a stage stops
// being invoked, runs out of order, or a failure/drift no longer stops the
// script -- whatever the script's comments or progress echoes still say.
// Tier 3 probe audit (task 025) added this alongside the ci.yml fork-guard,
// ci/suite.sh retry, and ci/releasegate probes.
package lintcheck

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const goStub = `#!/bin/sh
echo "go $*" >> "$LINTPROBE_LOG"
case "$1" in
vet)
    if [ -n "${LINTPROBE_VET_FAIL:-}" ]; then
        echo "stub go vet: failing on request" >&2
        exit 1
    fi
    ;;
mod)
    if [ "${2:-}" = "tidy" ] && [ -n "${LINTPROBE_TIDY_DRIFT:-}" ]; then
        echo "// drift introduced by stub go mod tidy" >> go.mod
    fi
    ;;
esac
exit 0
`

const gofmtStub = `#!/bin/sh
echo "gofmt $*" >> "$LINTPROBE_LOG"
if [ -n "${LINTPROBE_GOFMT_DRIFT:-}" ]; then
    echo "a.go"
fi
exit 0
`

const originalGoMod = "module example.com/lintprobe\n\ngo 1.25\n"

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

type lintRun struct {
	exitCode int
	calls    []string
	output   string
	goMod    string
}

// runLintScript copies the repository's ci/lint.sh into a fresh scratch git
// repository holding one tracked Go file, puts the recording stubs first on
// PATH, runs the script with the given LINTPROBE_* switches, and returns
// its exit code, the ordered stub invocations, its combined output and the
// scratch go.mod as the script left it.
func runLintScript(t *testing.T, switches ...string) lintRun {
	t.Helper()
	root, err := repositoryRoot()
	if err != nil {
		t.Fatalf("repositoryRoot: %v", err)
	}
	script, err := os.ReadFile(filepath.Join(root, "ci", "lint.sh"))
	if err != nil {
		t.Fatalf("read ci/lint.sh: %v", err)
	}

	scratch := t.TempDir()
	repo := filepath.Join(scratch, "repo")
	stubs := filepath.Join(scratch, "stubs")
	logPath := filepath.Join(scratch, "calls.log")
	for _, dir := range []string{filepath.Join(repo, "ci"), stubs} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]struct {
		content string
		mode    os.FileMode
	}{
		filepath.Join(repo, "ci", "lint.sh"): {string(script), 0o755},
		filepath.Join(repo, "a.go"):          {"package lintprobe\n", 0o644},
		filepath.Join(repo, "go.mod"):        {originalGoMod, 0o644},
		filepath.Join(repo, "go.sum"):        {"", 0o644},
		filepath.Join(stubs, "go"):           {goStub, 0o755},
		filepath.Join(stubs, "gofmt"):        {gofmtStub, 0o755},
		logPath:                              {"", 0o644},
	}
	for path, f := range files {
		if err := os.WriteFile(path, []byte(f.content), f.mode); err != nil {
			t.Fatal(err)
		}
	}

	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Fatalf("git not on PATH: %v", err)
	}
	for _, args := range [][]string{{"init", "-q"}, {"add", "a.go", "go.mod", "go.sum", "ci/lint.sh"}} {
		cmd := exec.Command(gitPath, args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	cmd := exec.Command("sh", filepath.Join(repo, "ci", "lint.sh"))
	cmd.Dir = scratch
	cmd.Env = append(os.Environ(),
		"PATH="+stubs+string(os.PathListSeparator)+os.Getenv("PATH"),
		"LINTPROBE_LOG="+logPath,
	)
	for _, s := range switches {
		cmd.Env = append(cmd.Env, s+"=1")
	}
	out, runErr := cmd.CombinedOutput()
	result := lintRun{output: string(out)}
	if runErr != nil {
		var exitErr *exec.ExitError
		if !errors.As(runErr, &exitErr) {
			t.Fatalf("run ci/lint.sh: %v\n%s", runErr, out)
		}
		result.exitCode = exitErr.ExitCode()
	}
	rawLog, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(rawLog)), "\n") {
		if line != "" {
			result.calls = append(result.calls, line)
		}
	}
	rawMod, err := os.ReadFile(filepath.Join(repo, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	result.goMod = string(rawMod)
	return result
}

// TestLintScriptRunsAllThreeStagesFailFastAndExitsOnDrift is the probe for
// task 013's ci/lint.sh. By executing the script against recording stubs it
// requires that:
//
//   - clean tree: `gofmt -l <tracked .go files>`, then `go vet ./...`, then
//     `go mod tidy` are all actually invoked, in that order, and the script
//     exits 0;
//   - gofmt drift: the script exits non-zero and neither vet nor tidy runs;
//   - vet failure: the script exits non-zero and tidy never runs;
//   - go mod tidy drift: the script exits non-zero and restores go.mod.
//
// Demonstrated failing against scratch mutations of a disposable copy of
// ci/lint.sh -- the executable `go vet ./...` line alone deleted (its
// progress echo left in place), and each drift branch's `exit 1` deleted:
// see /run/ralphd/artifacts/probes/tier3/013-*.log.
func TestLintScriptRunsAllThreeStagesFailFastAndExitsOnDrift(t *testing.T) {
	gofmtCall := "gofmt -l a.go"
	vetCall := "go vet ./..."
	tidyCall := "go mod tidy"

	cases := []struct {
		name      string
		switches  []string
		wantFail  bool
		wantCalls []string
	}{
		{"clean tree runs gofmt then vet then tidy and passes", nil, false, []string{gofmtCall, vetCall, tidyCall}},
		{"gofmt drift fails before vet and tidy", []string{"LINTPROBE_GOFMT_DRIFT"}, true, []string{gofmtCall}},
		{"vet failure fails before tidy", []string{"LINTPROBE_VET_FAIL"}, true, []string{gofmtCall, vetCall}},
		{"go mod tidy drift fails", []string{"LINTPROBE_TIDY_DRIFT"}, true, []string{gofmtCall, vetCall, tidyCall}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := runLintScript(t, tc.switches...)
			if tc.wantFail && got.exitCode == 0 {
				t.Errorf("ci/lint.sh exited 0, want non-zero\noutput:\n%s", got.output)
			}
			if !tc.wantFail && got.exitCode != 0 {
				t.Errorf("ci/lint.sh exited %d, want 0\noutput:\n%s", got.exitCode, got.output)
			}
			if !reflect.DeepEqual(got.calls, tc.wantCalls) {
				t.Errorf("stage invocations = %q, want %q\noutput:\n%s", got.calls, tc.wantCalls, got.output)
			}
			if got.goMod != originalGoMod {
				t.Errorf("ci/lint.sh left go.mod modified:\n%s", got.goMod)
			}
		})
	}
}
