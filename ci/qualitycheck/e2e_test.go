package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// repoRoot walks up from this test file's own source location (never
// os.Getwd(), which `go test` sets to the package directory) to find
// the module root -- the directory ci/quality.sh and ci/quality.json
// both live under.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	// this file is <root>/ci/qualitycheck/e2e_test.go
	return filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))
}

// TestQualityShSeededFailingGateExitsNonZero drives the real
// ci/quality.sh (criterion 2), not just this package's own run(), with
// a seeded failing CRAP gate: a near-empty merged coverage profile (so
// every scanned function in the whole repo scores 0% coverage) and a
// ceiling of 0 (so even the lowest-cc function, scored at 0% coverage,
// is over it: CRAP = 1^2*(1-0)^3 + 1 = 2 > 0). That seed is guaranteed
// to produce at least one offender without depending on any particular
// function in the tree, and ci/quality.sh must exit non-zero and print
// a crap-gate report section naming offenders.
func TestQualityShSeededFailingGateExitsNonZero(t *testing.T) {
	root := repoRoot(t)
	if _, err := os.Stat(filepath.Join(root, "ci", "quality.sh")); err != nil {
		t.Fatalf("could not find ci/quality.sh under resolved repo root %s: %v", root, err)
	}

	outdir := t.TempDir()
	// A profile with the mandatory "mode:" header and ONE block (any
	// block at all keeps crapgate from treating this as the "empty
	// profile" input-error case; see TestRun_ModeOnlyProfileFailsNonZero
	// in ci/crapgate's own tests) -- every OTHER scanned function in the
	// whole repo is then "present in source, absent from the profile"
	// (ci/crapgate's own 0%-coverage rule), never "zero scored
	// functions" (that is about SOURCE, not the profile). A ceiling of 0
	// is over for every real function (even cc=1, cov=0% scores CRAP=2),
	// so this seed is guaranteed to fail without depending on any
	// particular function in the tree.
	seeded := "mode: set\n" +
		"github.com/n-orlov/deck/ci/qualitycheck/main.go:1.1,1.1 0 0\n"
	if err := os.WriteFile(filepath.Join(outdir, "coverage-merged.out"), []byte(seeded), 0o644); err != nil {
		t.Fatalf("writing seeded profile: %v", err)
	}

	configPath := filepath.Join(t.TempDir(), "quality.json")
	seededConfig := `{
		"coverage": {"enabled": false, "total_floor": 85, "package_floor": 80, "fixture_floor": 50},
		"crap": {"enabled": true, "ceiling": 0, "fixture_ceiling": 0}
	}`
	if err := os.WriteFile(configPath, []byte(seededConfig), 0o644); err != nil {
		t.Fatalf("writing seeded config: %v", err)
	}

	cmd := exec.Command("sh", "ci/quality.sh", outdir, configPath)
	cmd.Dir = root
	cmd.Env = goChildEnv()
	out, err := cmd.CombinedOutput()

	if err == nil {
		t.Fatalf("ci/quality.sh exited 0, want non-zero (seeded ceiling 0 against an empty profile)\noutput:\n%s", out)
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("ci/quality.sh did not run at all: %v\noutput:\n%s", err, out)
	}
	if exitErr.ExitCode() == 0 {
		t.Fatalf("ci/quality.sh ExitCode() = 0, want non-zero")
	}

	report := string(out)
	if !strings.Contains(report, "=== crap gate ===") {
		t.Errorf("report has no crap gate section:\n%s", report)
	}
	if !strings.Contains(report, "over the ceiling") {
		t.Errorf("report does not name any offender:\n%s", report)
	}
	if !strings.Contains(report, "what to do") {
		t.Errorf("report has no \"what to do\" guidance:\n%s", report)
	}
}

// probeScript is a fake "go" put ahead of the real one on PATH: every
// invocation appends any DECK_-prefixed variable it sees to probeLog,
// then execs the real go binary so ci/quality.sh's actual work (go run
// ./ci/qualitycheck, which in turn go runs ./ci/crapgate) still happens.
// probeLog's own path is passed as QUALITYCHECK_PROBE_LOG rather than a
// DECK_*-prefixed name on purpose: a DECK_*-prefixed carrier would be
// scrubbed by ci/quality.sh itself before this wrapper ever had a
// chance to read it back out.
const probeScriptTemplate = `#!/bin/sh
env | grep '^DECK_' >> "$QUALITYCHECK_PROBE_LOG" || true
exec "%s" "$@"
`

// goChildEnv is os.Environ() with every DECK_* this test process itself
// might have inherited removed first, so only the ONE seeded variable
// each test adds on top is ever in play.
func goChildEnv() []string {
	env := os.Environ()
	out := make([]string, 0, len(env))
	for _, kv := range env {
		if strings.HasPrefix(kv, "DECK_") {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// TestQualityShScrubsDeckEnv proves criterion 3: ci/quality.sh is run
// with a seeded DECK_* variable, and the test fails if any child
// process (go run ./ci/qualitycheck, or the ci/crapgate it in turn
// spawns) sees it.
func TestQualityShScrubsDeckEnv(t *testing.T) {
	root := repoRoot(t)

	realGo, err := exec.LookPath("go")
	if err != nil {
		t.Skipf("go toolchain not on PATH: %v", err)
	}

	wrapperDir := t.TempDir()
	wrapperPath := filepath.Join(wrapperDir, "go")
	script := fmt.Sprintf(probeScriptTemplate, realGo)
	if err := os.WriteFile(wrapperPath, []byte(script), 0o755); err != nil {
		t.Fatalf("writing go wrapper: %v", err)
	}

	probeLog := filepath.Join(t.TempDir(), "probe.log")

	outdir := t.TempDir()
	if err := os.WriteFile(filepath.Join(outdir, "coverage-merged.out"), []byte("mode: set\n"), 0o644); err != nil {
		t.Fatalf("writing seeded profile: %v", err)
	}

	// Every gate off: this test is about environment hygiene, not gate
	// outcomes, so it must pass (exit 0) on its own regardless.
	configPath := filepath.Join(t.TempDir(), "quality.json")
	cfg := `{
		"coverage": {"enabled": false, "total_floor": 85, "package_floor": 80, "fixture_floor": 50},
		"crap": {"enabled": false, "ceiling": 30, "fixture_ceiling": 30}
	}`
	if err := os.WriteFile(configPath, []byte(cfg), 0o644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	env := goChildEnv()
	env = append(env,
		"PATH="+wrapperDir+":"+os.Getenv("PATH"),
		"QUALITYCHECK_PROBE_LOG="+probeLog,
		"DECK_TEST_SEED=must-not-reach-any-child-process",
	)

	cmd := exec.Command("sh", "ci/quality.sh", outdir, configPath)
	cmd.Dir = root
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ci/quality.sh with every gate off: want exit 0, got error %v\noutput:\n%s", err, out)
	}

	leaked, readErr := os.ReadFile(probeLog)
	if readErr == nil && len(leaked) > 0 {
		t.Fatalf("a DECK_* variable reached a child process spawned by ci/quality.sh:\n%s", leaked)
	}
}

// runQualitySh runs the real ci/quality.sh against outdir/configJSON and
// returns its stdout, the quality-report.txt it left in outdir (and
// whether that file exists) and its exit code.
func runQualitySh(t *testing.T, outdir, configJSON string) (stdout, report string, reportExists bool, code int) {
	t.Helper()
	root := repoRoot(t)
	configPath := filepath.Join(t.TempDir(), "quality.json")
	if err := os.WriteFile(configPath, []byte(configJSON), 0o644); err != nil {
		t.Fatalf("writing config: %v", err)
	}
	cmd := exec.Command("sh", "ci/quality.sh", outdir, configPath)
	cmd.Dir = root
	cmd.Env = goChildEnv()
	var so strings.Builder
	cmd.Stdout = &so
	err := cmd.Run()
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("ci/quality.sh did not run: %v", err)
		}
		code = exitErr.ExitCode()
	}
	raw, rerr := os.ReadFile(filepath.Join(outdir, "quality-report.txt"))
	return so.String(), string(raw), rerr == nil, code
}

const seededOneBlockProfile = "mode: set\n" +
	"github.com/n-orlov/deck/ci/qualitycheck/main.go:1.1,1.1 0 0\n"

// TestQualityShWritesReportOnSeededFailingGate: the report the job summary
// shows (outdir/quality-report.txt) holds the failing gate's whole section
// -- verdict, offenders, what to do -- identical to stdout, and the exit
// status still fails.
func TestQualityShWritesReportOnSeededFailingGate(t *testing.T) {
	outdir := t.TempDir()
	if err := os.WriteFile(filepath.Join(outdir, "coverage-merged.out"), []byte(seededOneBlockProfile), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, report, exists, code := runQualitySh(t, outdir,
		`{"crap": {"enabled": true, "ceiling": 0, "fixture_ceiling": 0}}`)
	if code == 0 {
		t.Fatalf("seeded failing gate exited 0")
	}
	if !exists {
		t.Fatalf("quality-report.txt was not written on a failing gate")
	}
	for _, want := range []string{"=== crap gate ===", "over the ceiling", "what to do:"} {
		if !strings.Contains(report, want) {
			t.Errorf("quality-report.txt lacks %q:\n%s", want, report)
		}
	}
	if report != stdout {
		t.Errorf("quality-report.txt differs from stdout\nreport:\n%s\nstdout:\n%s", report, stdout)
	}
}

// TestQualityShWritesReportOnPassingGate: a passing run delivers the
// report too (the summary is for every run, not only failures), and a stale
// report from an earlier run is replaced, never kept.
func TestQualityShWritesReportOnPassingGate(t *testing.T) {
	outdir := t.TempDir()
	if err := os.WriteFile(filepath.Join(outdir, "quality-report.txt"), []byte("STALE\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Every gate off: exit 0 and a one-line "nothing enabled" report.
	stdout, report, exists, code := runQualitySh(t, outdir, `{}`)
	if code != 0 {
		t.Fatalf("all-gates-off run exited %d, want 0", code)
	}
	if !exists || strings.Contains(report, "STALE") {
		t.Fatalf("report missing or stale after a passing run: exists=%v %q", exists, report)
	}
	if !strings.Contains(report, "no gates are enabled") || report != stdout {
		t.Errorf("report %q does not match stdout %q", report, stdout)
	}
}

// TestQualityShWritesReportOnConfigError: a run that dies before any gate
// section (unreadable config) still leaves a non-empty report and a
// non-zero exit, never a missing file the summary step would skip.
func TestQualityShWritesReportOnConfigError(t *testing.T) {
	outdir := t.TempDir()
	_, report, exists, code := runQualitySh(t, outdir, `{not json`)
	if code == 0 {
		t.Fatalf("malformed config exited 0")
	}
	if !exists || !strings.Contains(report, "no gate report was produced") {
		t.Errorf("want a one-line failure report, got exists=%v %q", exists, report)
	}
}
