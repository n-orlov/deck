package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeGolangciRepo builds a scratch repository whose ci/golangci.sh is a
// stub recording that it ran (and from which directory) and exiting with
// the given code.
func fakeGolangciRepo(t *testing.T, code string) (target, log string) {
	t.Helper()
	target = t.TempDir()
	log = filepath.Join(target, "ran.log")
	if err := os.MkdirAll(filepath.Join(target, "ci"), 0o755); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\npwd > '" + log + "'\necho fake golangci report\nexit " + code + "\n"
	if err := os.WriteFile(filepath.Join(target, "ci", "golangci.sh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return target, log
}

// TestGolangciGate_RunsTheSharedInvocationAndFailsOnFindings: the gate runs
// ci/golangci.sh from the target, passes on exit 0 and fails (with the
// linter's own output and a what-to-do line) on any non-zero exit.
func TestGolangciGate_RunsTheSharedInvocationAndFailsOnFindings(t *testing.T) {
	target, log := fakeGolangciRepo(t, "0")
	ok, out, err := runGolangciGate(golangciOptions{Target: target})
	if err != nil || !ok {
		t.Fatalf("clean run: ok=%v err=%v\n%s", ok, err, out)
	}
	if got, rerr := os.ReadFile(log); rerr != nil || !strings.Contains(string(got), filepath.Base(target)) {
		t.Errorf("the script did not run from the target (log %q, err %v)", got, rerr)
	}

	target, _ = fakeGolangciRepo(t, "1")
	ok, out, err = runGolangciGate(golangciOptions{Target: target})
	if err != nil || ok {
		t.Fatalf("findings run: ok=%v err=%v, want a failing gate\n%s", ok, err, out)
	}
	if !strings.Contains(out, "fake golangci report") || !strings.Contains(out, "what to do") {
		t.Errorf("failure output lacks the linter report or the what-to-do line:\n%s", out)
	}
}

// TestGolangciGate_MissingScriptIsAToolingError: a gate pointed at a tree
// without the shared script can never read as clean.
func TestGolangciGate_MissingScriptIsAToolingError(t *testing.T) {
	ok, _, err := runGolangciGate(golangciOptions{Target: t.TempDir()})
	if err == nil || ok {
		t.Fatalf("ok=%v err=%v, want a tooling error", ok, err)
	}
}

// TestGolangciGate_RunWiresTheGate drives run() with a seeded config: a
// failing linter turns the whole run to exit 1 with its own report section.
func TestGolangciGate_RunWiresTheGate(t *testing.T) {
	saved := golangciBase
	t.Cleanup(func() { golangciBase = saved })
	cfgPath := marshalConfig(t, config{Golangci: golangciConfig{Enabled: true}})
	for code, wantExit := range map[string]int{"0": 0, "1": 1} {
		target, _ := fakeGolangciRepo(t, code)
		golangciBase = golangciOptions{Target: target}
		report, exit, err := run(cfgPath, "")
		if err != nil || exit != wantExit {
			t.Errorf("golangci exit %s: run exit=%d err=%v, want %d", code, exit, err, wantExit)
		}
		if !strings.Contains(report, "=== golangci gate ===") {
			t.Errorf("report lacks the golangci section:\n%s", report)
		}
	}
}

// TestGolangciGate_CheckedInConfigIsOn: the gate is on in ci/quality.json,
// and switching it off is a loosening.
func TestGolangciGate_CheckedInConfigIsOn(t *testing.T) {
	cfg, err := loadConfig(filepath.Join(repoRoot(t), "ci", "quality.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Golangci.Enabled {
		t.Error("golangci.enabled is false in ci/quality.json, want true")
	}
	off := cfg
	off.Golangci.Enabled = false
	if len(looserThresholds(cfg, off)) == 0 {
		t.Error("switching golangci off is not reported as a loosening")
	}
}
