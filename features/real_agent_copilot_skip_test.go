package features

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cucumber/godog"
)

// writeFakeCopilot installs an executable named copilot, running script, as the
// only entry on PATH. It is a stand-in used by the skip-path tests only.
func writeFakeCopilot(t *testing.T, script string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "copilot"), []byte("#!/bin/sh\n"+script+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+"/usr/bin:/bin")
}

// R223: the scenario must SKIP, never fail, when copilot is not on PATH, so
// CI (which has none) cannot be broken by it. Removing the LookPath check in
// realCopilotSkipReason makes the step proceed and this test fail.
func TestRealCopilotScenarioSkipsWhenCopilotIsNotOnPATH(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	err := (&realCopilotScenario{}).installedAndLoggedInOrSkip(context.Background())
	if !errors.Is(err, godog.ErrSkip) {
		t.Fatalf("with no copilot on PATH the Given step returned %v, want godog.ErrSkip", err)
	}
	reason := realCopilotSkipReason(func(string) (string, error) { return "", errors.New("not found") },
		func(string) error { t.Fatal("the logged-in probe ran without a binary to run"); return nil })
	if !strings.Contains(reason, "no installed copilot CLI on PATH") {
		t.Fatalf("skip reason %q does not say copilot is absent", reason)
	}
}

func TestRealCopilotScenarioSkipsWhenNotLoggedInAndNeverPrintsTheToken(t *testing.T) {
	const sentinel = "deck-sentinel-credential-that-must-never-appear"
	t.Setenv("GITHUB_TOKEN", sentinel)
	t.Setenv("COPILOT_GITHUB_TOKEN", sentinel)
	// A logged-out CLI exits 1; this one also echoes its environment's token to
	// both streams, which must not reach the stated reason.
	writeFakeCopilot(t, `echo "login required $GITHUB_TOKEN"; echo "$COPILOT_GITHUB_TOKEN" >&2; exit 1`)
	path, err := exec.LookPath("copilot")
	if err != nil {
		t.Fatal(err)
	}
	reason := realCopilotSkipReason(func(string) (string, error) { return path, nil }, copilotAnswersProbePrompt)
	if reason == "" {
		t.Fatal("a copilot that cannot answer a prompt did not skip the scenario")
	}
	if !strings.Contains(reason, "not logged in") {
		t.Fatalf("skip reason %q does not say copilot is not logged in", reason)
	}
	if strings.Contains(reason, sentinel) || strings.Contains(reason, "login required") {
		t.Fatalf("skip reason %q leaks the token or the CLI's output", reason)
	}
	if err := (&realCopilotScenario{}).installedAndLoggedInOrSkip(context.Background()); !errors.Is(err, godog.ErrSkip) {
		t.Fatalf("a logged-out copilot made the Given step return %v, want godog.ErrSkip", err)
	}
}

func TestRealCopilotScenarioRunsWhenCopilotAnswersWithATemporaryHome(t *testing.T) {
	record := filepath.Join(t.TempDir(), "home")
	before := probeHomes(t)
	operatorHome := t.TempDir()
	t.Setenv("COPILOT_HOME", operatorHome)
	writeFakeCopilot(t, `printf %s "$COPILOT_HOME" > `+record+`; test "$1" = -p; exit 0`)
	if err := (&realCopilotScenario{}).installedAndLoggedInOrSkip(context.Background()); err != nil {
		t.Fatalf("a copilot that answers its prompt must not skip, got %v", err)
	}
	data, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	probeHome := string(data)
	if probeHome == "" || probeHome == operatorHome {
		t.Fatalf("login probe ran with COPILOT_HOME=%q, want a temporary directory distinct from the operator's %q", probeHome, operatorHome)
	}
	if leftovers := probeHomes(t); len(leftovers) != len(before) {
		t.Fatalf("login probe left its temporary COPILOT_HOME behind: %v", leftovers)
	}
}

// The six fixtures the scenario keys on must still contain the substrings it
// asserts in a live pane, so the scenario and the recorded captures cannot
// drift apart unnoticed.
func TestRealCopilotFixtureKeysAreInTheRecordedFixtures(t *testing.T) {
	if len(realCopilotFixtureKeys) != 6 || len(realCopilotFixtureFiles) != 6 {
		t.Fatalf("want the six probe fixtures, got %d key sets and %d files", len(realCopilotFixtureKeys), len(realCopilotFixtureFiles))
	}
	for fixture, needs := range realCopilotFixtureKeys {
		file, ok := realCopilotFixtureFiles[fixture]
		if !ok {
			t.Fatalf("fixture %q has no recorded file", fixture)
		}
		data, err := os.ReadFile(filepath.Join("..", "internal", "agent", "testdata", "probes", "copilot", file))
		if err != nil {
			t.Fatal(err)
		}
		if !copilotPaneSatisfies(string(data), needs) {
			t.Errorf("recorded fixture %s lacks the key substrings %q the live scenario asserts", file, needs)
		}
	}
}

// CI's default tag filter must keep excluding the copilot scenarios, and every
// scenario in the feature must carry the tag (it sits on the Feature line).
func TestRealCopilotFeatureIsExcludedByTheDefaultTagFilter(t *testing.T) {
	if !strings.Contains(defaultTags, "~@real-agents") {
		t.Fatalf("default tag filter %q no longer excludes @real-agents", defaultTags)
	}
	data, err := os.ReadFile("real_agent_copilot.feature")
	if err != nil {
		t.Fatal(err)
	}
	if first := strings.TrimSpace(strings.SplitN(string(data), "\n", 2)[0]); first != "@real-agents" {
		t.Fatalf("real_agent_copilot.feature starts with %q, want the @real-agents tag", first)
	}
	workflow, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(workflow), "DECK_GODOG_TAGS") {
		t.Fatal("ci.yml sets DECK_GODOG_TAGS, which could opt CI into @real-agents")
	}
}

// probeHomes lists the temporary COPILOT_HOME directories the logged-in probe
// creates, which it must remove before returning.
func probeHomes(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(os.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var found []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "deck-real-copilot-probe-") {
			found = append(found, entry.Name())
		}
	}
	return found
}
