package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// buildFixtureModule lays out, under t.TempDir(), the shared fixture
// module every end-to-end test below exercises:
//
//   - offender/pkg.go: Offender (seeded high cc, only partially tested)
//     and Clean (low cc, fully tested) -- the "seeded high-CRAP fixture
//     function fails" and "the report names it" cases.
//   - emptytypes/onlytypes.go: a package with zero functions -- "a
//     source tree with zero scored functions...fails".
//   - untracked/pkg.go: a function that exists in source but is in a
//     package `go test -coverprofile` was never pointed at, so the
//     resulting profile has no blocks for its file at all -- "a function
//     present in source but absent from the profile is scored at 0%
//     coverage (not skipped)".
//
// It then runs the REAL go toolchain's `go test -coverprofile` against
// ./offender/... only, so the coverprofile fed to crapgate's own scoring
// is exactly what go test itself produces -- never a hand-crafted one
// whose block ranges might silently drift from what the real tool emits.
func buildFixtureModule(t *testing.T) (dir, coverfile string) {
	t.Helper()
	dir = t.TempDir()

	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/fixture\n\ngo 1.21\n")

	writeFile(t, filepath.Join(dir, "offender", "pkg.go"), `package offender

// Offender is seeded as the high-CRAP function: non-trivial cc, and the
// test below only exercises part of it.
func Offender(x int) int {
	if x > 0 {
		if x > 10 {
			return 1
		}
		return 2
	}
	for i := 0; i < x; i++ {
		if i%2 == 0 {
			return i
		}
	}
	return 0
}

// Clean is a well-covered, low-complexity function that must never show
// up as an offender.
func Clean(x int) int {
	return x + 1
}
`)
	writeFile(t, filepath.Join(dir, "offender", "pkg_test.go"), `package offender

import "testing"

func TestOffenderPartial(t *testing.T) {
	Offender(5)
}

func TestClean(t *testing.T) {
	if Clean(1) != 2 {
		t.Fatal("bad")
	}
}
`)

	writeFile(t, filepath.Join(dir, "emptytypes", "onlytypes.go"), `package emptytypes

type T struct {
	Field int
}
`)

	writeFile(t, filepath.Join(dir, "untracked", "pkg.go"), `package untracked

// Untracked is never built by the fixture's own go test -coverprofile
// run (which only names ./offender/...), so no block for this file ever
// lands in the profile at all.
func Untracked(x int) int {
	if x > 0 {
		return 1
	}
	return 0
}
`)

	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skipf("go toolchain not on PATH, cannot generate a real coverprofile: %v", err)
	}
	coverfile = filepath.Join(dir, "cover.out")
	cmd := exec.Command(goBin, "test", "-coverprofile="+coverfile, "./offender/...")
	cmd.Dir = dir
	cmd.Env = goEnvWithoutGOFLAGS()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go test -coverprofile in fixture module: %v\n%s", err, out)
	}
	return dir, coverfile
}

// goEnvWithoutGOFLAGS is os.Environ() as-is: the fixture module's own
// go.mod is self-contained, so nothing here needs scrubbing beyond what
// the surrounding test process already set up (GOCACHE/GOPATH for the
// sandbox's fallback toolchain, if any).
func goEnvWithoutGOFLAGS() []string {
	return os.Environ()
}

// runInDir runs crapgate's run() with the process's working directory
// set to dir for the duration of the call (findModule discovers the
// module root by walking up from os.Getwd()).
func runInDir(t *testing.T, dir, profile string, max float64, filter string) (report string, exitCode int, err error) {
	t.Helper()
	t.Chdir(dir)
	return run(profile, max, filter)
}

// TestRun_SeededHighCRAPFunctionFailsAndIsNamed proves the central
// claim: a seeded high-CRAP function fails the gate, and the report
// names it with its file:line, cc, coverage and CRAP -- while a
// well-covered, low-complexity sibling in the same package never shows
// up as an offender.
func TestRun_SeededHighCRAPFunctionFailsAndIsNamed(t *testing.T) {
	dir, coverfile := buildFixtureModule(t)

	report, exitCode, err := runInDir(t, dir, coverfile, 10, "offender")
	if err != nil {
		t.Fatalf("run: unexpected error: %v", err)
	}
	if exitCode != 1 {
		t.Fatalf("exitCode = %d, want 1 (Offender must be over the ceiling)", exitCode)
	}
	if !strings.Contains(report, "Offender") {
		t.Errorf("report does not name Offender:\n%s", report)
	}
	if !strings.Contains(report, "pkg.go:5") {
		// Offender starts on line 5 of offender/pkg.go (package, blank, two
		// comment lines, then "func Offender...").
		t.Errorf("report does not carry Offender's file:line (pkg.go:5):\n%s", report)
	}
	if !strings.Contains(report, "cc=") {
		t.Errorf("report does not carry a cc= field:\n%s", report)
	}
	if !strings.Contains(report, "cov=") {
		t.Errorf("report does not carry a cov= field:\n%s", report)
	}
	if !strings.Contains(report, "CRAP=") {
		t.Errorf("report does not carry a CRAP= field:\n%s", report)
	}
	if strings.Contains(report, "Clean") {
		t.Errorf("report names Clean as an offender, but Clean is low-cc and fully covered:\n%s", report)
	}
}

// TestRun_EmptyProfileFailsNonZero proves an empty coverprofile makes
// the scorer exit non-zero, distinct from "zero findings".
func TestRun_EmptyProfileFailsNonZero(t *testing.T) {
	dir, _ := buildFixtureModule(t)
	empty := filepath.Join(dir, "empty.out")
	writeFile(t, empty, "")

	_, exitCode, err := runInDir(t, dir, empty, 10, "offender")
	if err == nil {
		t.Fatalf("run with an empty profile: want an error, got nil (exitCode=%d)", exitCode)
	}
	if exitCode == 0 {
		t.Errorf("run with an empty profile: exitCode = 0, want non-zero")
	}
}

// TestRun_ModeOnlyProfileFailsNonZero covers the other "empty" shape: a
// profile that holds a mode line but zero coverage blocks.
func TestRun_ModeOnlyProfileFailsNonZero(t *testing.T) {
	dir, _ := buildFixtureModule(t)
	modeOnly := filepath.Join(dir, "modeonly.out")
	writeFile(t, modeOnly, "mode: set\n")

	_, exitCode, err := runInDir(t, dir, modeOnly, 10, "offender")
	if err == nil {
		t.Fatalf("run with a mode-only profile: want an error, got nil (exitCode=%d)", exitCode)
	}
	if exitCode == 0 {
		t.Errorf("run with a mode-only profile: exitCode = 0, want non-zero")
	}
}

// TestRun_MissingProfileFailsNonZero proves a coverprofile path that
// does not exist on disk at all makes the scorer exit non-zero.
func TestRun_MissingProfileFailsNonZero(t *testing.T) {
	dir, _ := buildFixtureModule(t)
	missing := filepath.Join(dir, "does-not-exist.out")

	_, exitCode, err := runInDir(t, dir, missing, 10, "offender")
	if err == nil {
		t.Fatalf("run with a missing profile: want an error, got nil (exitCode=%d)", exitCode)
	}
	if exitCode == 0 {
		t.Errorf("run with a missing profile: exitCode = 0, want non-zero")
	}
}

// TestRun_ZeroScoredFunctionsFailsNonZero proves that pointing the
// scanner at a source tree with zero functions in it (a typo'd filter
// must never silently read as "a clean bill of health") makes the
// scorer exit non-zero.
func TestRun_ZeroScoredFunctionsFailsNonZero(t *testing.T) {
	dir, coverfile := buildFixtureModule(t)

	_, exitCode, err := runInDir(t, dir, coverfile, 10, "emptytypes")
	if err == nil {
		t.Fatalf("run against a zero-function package: want an error, got nil (exitCode=%d)", exitCode)
	}
	if exitCode == 0 {
		t.Errorf("run against a zero-function package: exitCode = 0, want non-zero")
	}
	if !strings.Contains(err.Error(), "zero scored functions") {
		t.Errorf("error %q does not say why (zero scored functions)", err.Error())
	}
}

// TestRun_FunctionAbsentFromProfileScoresZeroCoverageNotSkipped proves
// the exact wording of PRD R187's rule: a function present in SOURCE but
// absent from the PROFILE (because go test -coverprofile was never
// pointed at its package) is scored at 0% coverage and still appears in
// the report -- never silently skipped as "no data".
func TestRun_FunctionAbsentFromProfileScoresZeroCoverageNotSkipped(t *testing.T) {
	dir, coverfile := buildFixtureModule(t)

	// max=0 forces even a merely-adequate cc into the offender list, so
	// a function with real statements and no coverage data for it shows
	// up unambiguously, by name, at 0% coverage.
	report, exitCode, err := runInDir(t, dir, coverfile, 0, "untracked")
	if err != nil {
		t.Fatalf("run: unexpected error: %v", err)
	}
	if exitCode != 1 {
		t.Fatalf("exitCode = %d, want 1 (Untracked must be reported, not skipped)", exitCode)
	}
	if !strings.Contains(report, "Untracked") {
		t.Fatalf("report does not name Untracked at all -- it was skipped instead of scored:\n%s", report)
	}
	if !strings.Contains(report, "cov=0.0%") {
		t.Errorf("report does not score Untracked at 0%% coverage:\n%s", report)
	}
}
