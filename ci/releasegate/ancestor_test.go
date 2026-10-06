package main

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// gateArgs are valid flags; the CI half is never reached by the tests below
// that refuse on reachability, so no fake GitHub server is needed for them.
var gateArgs = []string{"-repo", "o/r", "-sha", "deadbeef"}

func TestRunGateRefusesANonAncestor(t *testing.T) {
	err := runGate(gateArgs, noTokenEnv, func(string) (bool, error) { return false, nil })
	if err == nil {
		t.Fatal("runGate: a sha that is not on origin/main must fail the gate")
	}
	if !strings.Contains(err.Error(), "deadbeef") || !strings.Contains(err.Error(), "not an ancestor of origin/main") {
		t.Errorf("error %q must name the sha and the reachability rule", err)
	}
}

func TestRunGateFailsWhenTheAncestorCheckErrors(t *testing.T) {
	boom := errors.New("fatal: bad object")
	err := runGate(gateArgs, noTokenEnv, func(string) (bool, error) { return true, boom })
	if err == nil {
		t.Fatal("runGate: a check error must fail the gate, even if it also said true")
	}
	if !errors.Is(err, boom) {
		t.Errorf("error %q must wrap the check's error", err)
	}
}

func TestRunGateAcceptsAnAncestorOnceCIIsGreen(t *testing.T) {
	withAPIBase(t, suiteServer(t, "success").baseURL)
	var asked string
	err := runGate(gateArgs, noTokenEnv, func(sha string) (bool, error) { asked = sha; return true, nil })
	if err != nil {
		t.Fatalf("runGate: an ancestor with a green suite must pass: %v", err)
	}
	if asked != "deadbeef" {
		t.Errorf("the check was asked about %q, want the tagged sha", asked)
	}
}

func TestRunGateStillRefusesAnAncestorWithRedCI(t *testing.T) {
	withAPIBase(t, suiteServer(t, "failure").baseURL)
	if err := runGate(gateArgs, noTokenEnv, func(string) (bool, error) { return true, nil }); err == nil {
		t.Fatal("runGate: an ancestor whose suite is red must still fail")
	}
}

func TestRunGateRequiresFlags(t *testing.T) {
	if err := runGate(nil, noTokenEnv, func(string) (bool, error) { return true, nil }); err == nil {
		t.Fatal("runGate: missing -repo/-sha must fail")
	}
}

// suiteServer serves one push suite run of ci.yml with the given conclusion.
func suiteServer(t *testing.T, conclusion string) *pageServer {
	t.Helper()
	checks := `{"total_count":1,"check_runs":[{"name":"suite","status":"completed","conclusion":"` + conclusion + `","started_at":"2026-01-01T10:00:00Z","check_suite":{"id":7}}]}`
	runs := `{"workflow_runs":[{"event":"push","check_suite_id":7,"path":".github/workflows/ci.yml"}]}`
	return newPageServer(t, []string{checks}, []string{runs})
}

// gitIn runs git in dir and returns the trimmed stdout.
func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.invalid",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.invalid",
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// TestGitMergeBaseIsAncestorAgainstARealRepository: a commit on main passes,
// a commit on a side branch is not an ancestor, and an unknown sha or ref is
// an error rather than a verdict.
func TestGitMergeBaseIsAncestorAgainstARealRepository(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	gitIn(t, dir, "init", "-q", "-b", "main")
	gitIn(t, dir, "commit", "-q", "--allow-empty", "-m", "base")
	base := gitIn(t, dir, "rev-parse", "HEAD")
	gitIn(t, dir, "commit", "-q", "--allow-empty", "-m", "tip")
	tip := gitIn(t, dir, "rev-parse", "HEAD")
	gitIn(t, dir, "checkout", "-q", "-b", "side", base)
	gitIn(t, dir, "commit", "-q", "--allow-empty", "-m", "side")
	side := gitIn(t, dir, "rev-parse", "HEAD")
	gitIn(t, dir, "update-ref", "refs/remotes/origin/main", tip)

	for _, tc := range []struct {
		name, sha, ref string
		want, wantErr  bool
	}{
		{"tip is reachable", tip, mainRef, true, false},
		{"older commit is reachable", base, mainRef, true, false},
		{"side branch is not reachable", side, mainRef, false, false},
		{"unknown sha errors", strings.Repeat("1", 40), mainRef, false, true},
		{"unknown ref errors", tip, "origin/nope", false, true},
		{"empty sha errors", "", mainRef, false, true},
		{"option-looking sha errors", "--all", mainRef, false, true},
	} {
		got, err := gitMergeBaseIsAncestor(dir, tc.sha, tc.ref)
		if (err != nil) != tc.wantErr || got != tc.want {
			t.Errorf("%s: got (%v, %v), want (%v, err=%v)", tc.name, got, err, tc.want, tc.wantErr)
		}
	}
}

func TestGitIsAncestorUsesTheMainRef(t *testing.T) {
	// Outside a repository with origin/main the production check must not
	// answer "reachable": here it either errors or says false.
	t.Chdir(t.TempDir())
	if ok, err := gitIsAncestor("deadbeef"); ok && err == nil {
		t.Fatal("gitIsAncestor reported a bogus sha reachable")
	}
}
