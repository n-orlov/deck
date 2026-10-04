package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// writeCrapConfig writes a ci/quality.json with only the crap gate on, at
// the given ceiling.
func writeCrapConfig(t *testing.T, ceiling string) string {
	t.Helper()
	return writeTempFile(t, `{
		"coverage": {"enabled": false, "total_floor": 85, "package_floor": 80, "fixture_floor": 50},
		"crap": {"enabled": true, "ceiling": `+ceiling+`, "fixture_ceiling": `+ceiling+`}
	}`)
}

func oneBlockProfile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "coverage-merged.out")
	seeded := "mode: set\ngithub.com/n-orlov/deck/ci/qualitycheck/main.go:1.1,1.1 0 0\n"
	if err := os.WriteFile(path, []byte(seeded), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestRun_CrapGateOverACeilingFailsWithWhatToDo seeds an impossible ceiling.
func TestRun_CrapGateOverACeilingFailsWithWhatToDo(t *testing.T) {
	t.Chdir(repoRoot(t))
	report, exitCode, err := run(writeCrapConfig(t, "0"), oneBlockProfile(t))
	if err != nil || exitCode != 1 {
		t.Fatalf("run = (%d, %v), want exit 1:\n%s", exitCode, err, report)
	}
	for _, want := range []string{"=== crap gate ===", "what to do: refactor the named offender"} {
		if !strings.Contains(report, want) {
			t.Errorf("report lacks %q:\n%s", want, report)
		}
	}
	if strings.Contains(report, "exit status") {
		t.Errorf("go run's own 'exit status' noise leaked into the report:\n%s", report)
	}
}

// TestRun_CrapGateUnderACeilingPasses uses a ceiling no function can reach.
func TestRun_CrapGateUnderACeilingPasses(t *testing.T) {
	t.Chdir(repoRoot(t))
	report, exitCode, err := run(writeCrapConfig(t, "1000000000"), oneBlockProfile(t))
	if err != nil || exitCode != 0 {
		t.Fatalf("run = (%d, %v), want exit 0:\n%s", exitCode, err, report)
	}
	if !strings.Contains(report, "what to do: nothing") {
		t.Errorf("report lacks the passing line:\n%s", report)
	}
}

// TestRun_CrapGateBadInputIsUsageError: a mode-only profile and a missing
// profile path both fail rather than pass.
func TestRun_CrapGateBadInputIsUsageError(t *testing.T) {
	t.Chdir(repoRoot(t))
	empty := filepath.Join(t.TempDir(), "coverage-merged.out")
	if err := os.WriteFile(empty, []byte("mode: set\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, exitCode, err := run(writeCrapConfig(t, "30"), empty); err == nil || exitCode != 2 {
		t.Errorf("mode-only profile: run = (%d, %v), want exit 2 and an error", exitCode, err)
	}
	if _, exitCode, err := run(writeCrapConfig(t, "30"), ""); err == nil || exitCode != 2 {
		t.Errorf("no profile: run = (%d, %v), want exit 2 and an error", exitCode, err)
	}
}

// gitRepo creates a repo in a temp dir with the named commits (each one
// writes ci/quality.json with the given content) and returns its path.
func gitRepo(t *testing.T, configs ...string) string {
	t.Helper()
	dir := t.TempDir()
	for k, v := range map[string]string{
		"GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@example.com",
		"GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@example.com",
	} {
		t.Setenv(k, v)
	}
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q")
	for _, content := range configs {
		if err := os.MkdirAll(filepath.Join(dir, "ci"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "ci", "quality.json"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		git("add", "-A")
		git("commit", "-q", "-m", "c")
	}
	return dir
}

// TestLoadBaseConfig_ShallowFallbackReadsTheParentCommit: with no
// origin/main the base is HEAD~1, and its quality.json (not the working
// tree's) is the base side of the loosening comparison.
func TestLoadBaseConfig_ShallowFallbackReadsTheParentCommit(t *testing.T) {
	dir := gitRepo(t,
		`{"coverage": {"enabled": true, "total_floor": 85, "package_floor": 80, "fixture_floor": 50}}`,
		`{"coverage": {"enabled": true, "total_floor": 60}}`)
	cfg, err := loadBaseConfig(dir)
	if err != nil {
		t.Fatalf("loadBaseConfig: %v", err)
	}
	if cfg.Coverage.TotalFloor != 85 || !cfg.Coverage.Enabled {
		t.Errorf("base config = %+v, want the parent commit's total_floor 85", cfg.Coverage)
	}
}

// TestResolveBaseRef_NoBaseAtAllNamesBothFailures: a one-commit repo has
// neither origin/main nor HEAD~1.
func TestResolveBaseRef_NoBaseAtAllNamesBothFailures(t *testing.T) {
	dir := gitRepo(t, `{}`)
	_, err := resolveBaseRef(dir)
	if err == nil {
		t.Fatal("resolveBaseRef: want an error for a repo with one commit and no origin/main")
	}
	for _, want := range []string{"git merge-base HEAD origin/main failed", "git rev-parse HEAD~1 also failed"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error lacks %q: %v", want, err)
		}
	}
	if _, err := loadBaseConfig(dir); err == nil {
		t.Error("loadBaseConfig: want the same error to propagate")
	}
}

// TestLoadBaseConfig_UnreadableBaseConfigIsAnError: a base without
// ci/quality.json, and one with malformed JSON, are errors, not "no gates".
func TestLoadBaseConfig_UnreadableBaseConfigIsAnError(t *testing.T) {
	dir := gitRepo(t, `{}`, `{"a": 1}`)
	cmd := exec.Command("git", "rm", "-q", "ci/quality.json")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git rm: %v\n%s", err, out)
	}
	// HEAD~1 still has it; amend the parent away by making the base the
	// commit that lacks the file: commit the removal, then a second commit.
	commit := func() {
		c := exec.Command("git", "commit", "-q", "--allow-empty", "-m", "x")
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git commit: %v\n%s", err, out)
		}
	}
	commit() // removal commit
	commit() // HEAD~1 is now the removal commit, which has no quality.json
	if _, err := loadBaseConfig(dir); err == nil || !strings.Contains(err.Error(), "reading ci/quality.json at base ref") {
		t.Errorf("missing base config: err = %v, want a 'reading ci/quality.json at base ref' error", err)
	}

	dir = gitRepo(t, `not json`, `{}`)
	if _, err := loadBaseConfig(dir); err == nil || !strings.Contains(err.Error(), "parsing ci/quality.json at base ref") {
		t.Errorf("malformed base config: err = %v, want a 'parsing ci/quality.json at base ref' error", err)
	}
}
