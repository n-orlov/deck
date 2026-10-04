package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var fixedNow = time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)

// scanTree makes a scan target with one file and a go.mod.
func scanTree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, f := range []string{"go.mod", "main.go"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// fakeTrivy writes a stand-in trivy that records its argv and exits with
// code, returning its path and the argv log.
func fakeTrivy(t *testing.T, code int) (bin, argvLog string) {
	t.Helper()
	dir := t.TempDir()
	argvLog = filepath.Join(dir, "argv")
	bin = filepath.Join(dir, "trivy")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > '" + argvLog + "'\necho fake trivy report\nexit " +
		string(rune('0'+code)) + "\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, argvLog
}

func opts(t *testing.T, bin, target string) trivyOptions {
	return trivyOptions{
		Binary:     bin,
		Target:     target,
		CacheDir:   filepath.Join(t.TempDir(), "cache"),
		IgnoreFile: filepath.Join(t.TempDir(), ".trivyignore"),
		Severity:   "HIGH,CRITICAL",
		Now:        fixedNow,
	}
}

func writeIgnore(t *testing.T, o trivyOptions, content string) {
	t.Helper()
	if err := os.WriteFile(o.IgnoreFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestTrivyGate_RunsTrivyFsWithRequiredFlags proves the command line the
// criterion names: fs, the three scanners, HIGH,CRITICAL, --ignore-unfixed,
// and the cache dir the caller chose.
func TestTrivyGate_RunsTrivyFsWithRequiredFlags(t *testing.T) {
	bin, argvLog := fakeTrivy(t, 0)
	o := opts(t, bin, scanTree(t))
	ok, out, err := runTrivyGate(o)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v out=%s", ok, err, out)
	}
	raw, _ := os.ReadFile(argvLog)
	argv := strings.Split(strings.TrimSpace(string(raw)), "\n")
	joined := strings.Join(argv, " ")
	for _, want := range []string{
		"fs", "--scanners vuln,secret,misconfig", "--severity HIGH,CRITICAL", "--ignore-unfixed",
		"--exit-code 1", "--cache-dir " + o.CacheDir,
		"--skip-files docs/reports/phase3g-812-stability10/summary.log",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("argv %q lacks %q", joined, want)
		}
	}
	if argv[len(argv)-1] != o.Target {
		t.Errorf("last arg = %q, want the target %q", argv[len(argv)-1], o.Target)
	}
}

// TestTrivyGate_FindingFails: trivy exiting 1 (a seeded finding) fails
// the gate with trivy's report in the output.
func TestTrivyGate_FindingFails(t *testing.T) {
	bin, _ := fakeTrivy(t, 1)
	ok, out, err := runTrivyGate(opts(t, bin, scanTree(t)))
	if err != nil || ok {
		t.Fatalf("ok=%v err=%v, want a failing gate", ok, err)
	}
	if !strings.Contains(out, "fake trivy report") || !strings.Contains(out, "what to do") {
		t.Errorf("output lacks report or guidance:\n%s", out)
	}
}

// TestTrivyGate_ToolFailureIsError: trivy exiting with anything other
// than 0/1, or not existing, is an input error, never a pass.
func TestTrivyGate_ToolFailureIsError(t *testing.T) {
	bin, _ := fakeTrivy(t, 2)
	if ok, _, err := runTrivyGate(opts(t, bin, scanTree(t))); err == nil || ok {
		t.Errorf("exit 2: ok=%v err=%v, want an error", ok, err)
	}
	if ok, _, err := runTrivyGate(opts(t, "/does/not/exist/trivy", scanTree(t))); err == nil || ok {
		t.Errorf("missing binary: ok=%v err=%v, want an error", ok, err)
	}
}

// TestTrivyGate_EmptyScanTargetFails: an empty directory, a missing one,
// and a tree without go.mod must fail, and trivy must not even run (it
// exits 0 on an empty directory).
func TestTrivyGate_EmptyScanTargetFails(t *testing.T) {
	noGoMod := t.TempDir()
	if err := os.WriteFile(filepath.Join(noGoMod, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, target := range map[string]string{
		"empty dir":  t.TempDir(),
		"missing":    filepath.Join(t.TempDir(), "nope"),
		"no go.mod":  noGoMod,
		"empty path": "",
	} {
		bin, argvLog := fakeTrivy(t, 0) // would pass if it were run
		ok, out, err := runTrivyGate(opts(t, bin, target))
		if err != nil || ok {
			t.Errorf("%s: ok=%v err=%v, want a failing gate", name, ok, err)
		}
		if !strings.Contains(out, "scan target") {
			t.Errorf("%s: output does not name the scan target:\n%s", name, out)
		}
		if _, statErr := os.Stat(argvLog); statErr == nil {
			t.Errorf("%s: trivy ran against an unusable target", name)
		}
	}
}

// TestTrivyGate_ExpiredIgnoreEntryFails: a seeded .trivyignore entry
// whose review-by date has passed fails the gate before trivy runs.
func TestTrivyGate_ExpiredIgnoreEntryFails(t *testing.T) {
	bin, argvLog := fakeTrivy(t, 0)
	o := opts(t, bin, scanTree(t))
	writeIgnore(t, o, "CVE-2026-0001 review-by:2026-10-03 # no call path\n")
	ok, out, err := runTrivyGate(o)
	if err != nil || ok {
		t.Fatalf("ok=%v err=%v, want a failing gate", ok, err)
	}
	if !strings.Contains(out, "expired on 2026-10-03") {
		t.Errorf("output does not name the expired entry:\n%s", out)
	}
	if _, statErr := os.Stat(argvLog); statErr == nil {
		t.Error("trivy ran despite an expired exception")
	}
}

// TestTrivyGate_IgnoreEntryDateBoundary: the review-by day itself is
// still valid; the day after is expired.
func TestTrivyGate_IgnoreEntryDateBoundary(t *testing.T) {
	o := opts(t, "", scanTree(t))
	writeIgnore(t, o, "CVE-2026-0001 review-by:2026-10-04 # last good day\n")
	if p, err := checkTrivyIgnore(o.IgnoreFile, fixedNow); err != nil || len(p) != 0 {
		t.Errorf("on the date: problems=%v err=%v, want none", p, err)
	}
	if p, _ := checkTrivyIgnore(o.IgnoreFile, fixedNow.AddDate(0, 0, 1)); len(p) != 1 {
		t.Errorf("a day later: problems=%v, want one", p)
	}
}

// TestTrivyGate_MalformedIgnoreEntriesFail: no reason, no date, an
// impossible date are each a problem; comments and blank lines are not.
func TestTrivyGate_MalformedIgnoreEntriesFail(t *testing.T) {
	o := opts(t, "", "")
	writeIgnore(t, o, strings.Join([]string{
		"# a comment line",
		"",
		"CVE-1 review-by:2099-01-01",           // no reason
		"CVE-2 # reason but no date",           // no date
		"CVE-3 review-by:2099-13-45 # bad day", // not a date
		"CVE-4 review-by:2099-01-01 #   ",      // empty reason
		"CVE-5 review-by:2099-01-01 # fine",
	}, "\n"))
	problems, err := checkTrivyIgnore(o.IgnoreFile, fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) != 4 {
		t.Fatalf("problems = %d, want 4:\n%s", len(problems), strings.Join(problems, "\n"))
	}
}

// TestTrivyIgnore_CheckedInFileIsValid: the repository's own
// .trivyignore (absent means no exceptions) must have only reasoned,
// dated, unexpired entries -- this is what fails once a date passes.
func TestTrivyIgnore_CheckedInFileIsValid(t *testing.T) {
	problems, err := checkTrivyIgnore(filepath.Join(repoRoot(t), ".trivyignore"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) > 0 {
		t.Fatalf(".trivyignore:\n%s", strings.Join(problems, "\n"))
	}
}

// TestTrivySeverityMustCoverHighAndCritical: the gate refuses a severity
// list that drops either level or names an unknown one.
func TestTrivySeverityMustCoverHighAndCritical(t *testing.T) {
	for _, bad := range []string{"", "CRITICAL", "HIGH", "HIGH,BOGUS", "MEDIUM,LOW"} {
		if validateTrivySeverity(bad) == nil {
			t.Errorf("severity %q accepted", bad)
		}
	}
	for _, good := range []string{"HIGH,CRITICAL", "MEDIUM,HIGH,CRITICAL"} {
		if err := validateTrivySeverity(good); err != nil {
			t.Errorf("severity %q refused: %v", good, err)
		}
	}
}

// TestRun_TrivyGateOnFailsOnEmptyTarget drives run() with the gate on
// via a config file and an empty target.
func TestRun_TrivyGateOnFailsOnEmptyTarget(t *testing.T) {
	saved := trivyBase
	t.Cleanup(func() { trivyBase = saved })
	bin, _ := fakeTrivy(t, 0)
	trivyBase = opts(t, bin, t.TempDir())
	path := writeTempFile(t, `{"trivy": {"enabled": true, "severity": "HIGH,CRITICAL"}}`)
	report, exitCode, err := run(path, "")
	if err != nil || exitCode != 1 {
		t.Fatalf("exit=%d err=%v, want 1", exitCode, err)
	}
	if !strings.Contains(report, "=== trivy gate ===") {
		t.Errorf("no trivy section:\n%s", report)
	}
}

// TestCheckedInConfigTurnsTrivyOn: the gate is on in ci/quality.json at
// HIGH,CRITICAL.
func TestCheckedInConfigTurnsTrivyOn(t *testing.T) {
	cfg, err := loadConfig(filepath.Join(repoRoot(t), "ci", "quality.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Trivy.Enabled {
		t.Error("trivy.enabled is false")
	}
	if err := validateTrivySeverity(cfg.Trivy.Severity); err != nil {
		t.Error(err)
	}
}

// TestThresholdsNotLoosened_TrivyOffOrSeverityDroppedFails: switching
// the trivy gate off, or dropping a severity, is a loosening.
func TestThresholdsNotLoosened_TrivyOffOrSeverityDroppedFails(t *testing.T) {
	base := baselineConfig()
	off := base
	off.Trivy.Enabled = false
	if len(looserThresholds(base, off)) == 0 {
		t.Error("trivy switched off: not reported as loosened")
	}
	dropped := base
	dropped.Trivy.Severity = "CRITICAL"
	if len(looserThresholds(base, dropped)) == 0 {
		t.Error("severity HIGH dropped: not reported as loosened")
	}
	widened := base
	widened.Trivy.Severity = "MEDIUM,HIGH,CRITICAL"
	if p := looserThresholds(base, widened); len(p) != 0 {
		t.Errorf("widening severity reported as loosened: %v", p)
	}
}
