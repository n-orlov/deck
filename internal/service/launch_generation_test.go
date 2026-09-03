package service

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/store"
)

// rowLaunchGeneration reads the generation half of the row's own
// launch_lease_owner: the launch deck currently considers the live one
// (issue #11, R74).
func rowLaunchGeneration(t *testing.T, db *store.Store, sessionID string) string {
	t.Helper()
	var owner string
	if err := db.DB().QueryRow(`SELECT COALESCE(launch_lease_owner, '') FROM sessions WHERE id = ?`, sessionID).Scan(&owner); err != nil {
		t.Fatalf("read launch_lease_owner: %v", err)
	}
	_, generation, found := strings.Cut(owner, "#")
	if !found {
		t.Fatalf("stored launch_lease_owner %q carries no generation", owner)
	}
	if generation == "" {
		t.Fatalf("stored launch_lease_owner %q carries an empty generation", owner)
	}
	return generation
}

// assertNoTMuxEnvironment asserts tmux itself does not know key for this
// session. `show-environment -t <session> KEY` exits non-zero with "unknown
// variable" when it is unset, which is the only honest way to distinguish
// "absent" from "present but empty".
func assertNoTMuxEnvironment(t *testing.T, socket, slug, key string) {
	t.Helper()
	out, err := exec.Command("tmux", "-L", socket, "show-environment", "-t", "deck_"+slug, key).CombinedOutput()
	if err == nil {
		t.Fatalf("tmux environment for %s unexpectedly has %s: %s", slug, key, out)
	}
}

// TestResumeExportsCurrentLaunchGenerationToThePane is R74 leg 1's end of the
// chain (issue #11): the discriminator the launch lease minted has to reach
// the agent's environment, and the value the pane carries has to be the one
// the row currently names. Two successive resumes of the same row export
// different tokens, so a hook from the first pane can be told apart from a
// hook from the second even though both name the same session id.
func TestResumeExportsCurrentLaunchGenerationToThePane(t *testing.T) {
	cwd := t.TempDir()
	stubExecutableOnPath(t, "claude")
	service, db, _, socket := newAgentTestService(t, nil, "launch-generation-test")

	created, err := service.CreateAgent(context.Background(), AgentCreateInput{
		Name: "Claude: generation", CWD: cwd, Agent: "claude", PermissionProfile: "safe",
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	// The first launch of a brand-new row takes no launch lease, so it has no
	// generation and must export none at all rather than an empty one.
	assertTMuxEnvironment(t, socket, created.Slug, "DECK_SESSION_ID", created.ID)
	assertNoTMuxEnvironment(t, socket, created.Slug, "DECK_LAUNCH_GENERATION")

	relaunch := func(label string) string {
		t.Helper()
		if err := service.TMux.Kill(context.Background(), created.Slug); err != nil {
			t.Fatalf("%s: kill pane: %v", label, err)
		}
		stopSession(t, db, created.ID)
		// No lease fixture is needed: each completed launch releases its own
		// lease (R75, task 031), so the previous launch is not still holding
		// one even though the test clock has not advanced.
		session, outcome, err := service.Resume(context.Background(), created.ID)
		if err != nil {
			t.Fatalf("%s: resume: %v", label, err)
		}
		if outcome != ResumeStarted {
			t.Fatalf("%s: outcome = %v, want ResumeStarted", label, outcome)
		}
		generation := rowLaunchGeneration(t, db, created.ID)
		// What the pane really carries, asked of tmux rather than of deck's
		// own bookkeeping: this is the environment the agent's hooks inherit.
		assertTMuxEnvironment(t, socket, session.Slug, "DECK_LAUNCH_GENERATION", generation)
		assertTMuxEnvironment(t, socket, session.Slug, "DECK_SESSION_ID", created.ID)
		return generation
	}

	first := relaunch("first resume")
	second := relaunch("second resume")
	if first == second {
		t.Fatalf("both resumes exported generation %q; two launches of one row must be distinguishable", first)
	}
}

// TestResumeShellSessionCarriesNoLaunchGeneration keeps R74 off the shell
// path: a shell has no Claude-specific hook source at all
// (Shell.Instrument returns nothing), so DECK_LAUNCH_GENERATION -- which
// only Claude's own Instrument sets -- never reaches its pane, even though
// SPEC §6.1's deck-owned session context (DECK_SESSION_ID and friends) now
// does, on every adapter alike (R104). Its resume still takes the launch
// lease, so the row does get a generation -- the point is that the pane
// does not carry a launch-generation token, not that the pane carries no
// deck-owned environment at all.
func TestResumeShellSessionCarriesNoLaunchGeneration(t *testing.T) {
	cwd := t.TempDir()
	service, db, logger, socket := newAgentTestService(t, nil, "launch-generation-shell")
	service.Shell = "/bin/sh"

	created, err := service.CreateShell(context.Background(), ShellCreateInput{
		Name: "Shell: generation", CWD: cwd, Env: map[string]string{"FROM_SESSION": "1"},
	})
	if err != nil {
		t.Fatalf("create shell: %v", err)
	}
	if err := service.TMux.Kill(context.Background(), created.Slug); err != nil {
		t.Fatalf("kill pane: %v", err)
	}
	stopSession(t, db, created.ID)

	session, outcome, err := service.Resume(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("resume shell: %v", err)
	}
	if outcome != ResumeStarted {
		t.Fatalf("outcome = %v, want ResumeStarted", outcome)
	}
	if generation := rowLaunchGeneration(t, db, created.ID); generation == "" {
		t.Fatal("resumed shell row carries no launch generation")
	}
	assertNoTMuxEnvironment(t, socket, session.Slug, "DECK_LAUNCH_GENERATION")
	assertTMuxEnvironment(t, socket, session.Slug, "DECK_SESSION_ID", created.ID)

	for _, record := range auditRecords(t, logger.Path()) {
		if record["event"] != "launch" {
			continue
		}
		for _, key := range jsonStrings(record["env_keys"]) {
			if key == "DECK_LAUNCH_GENERATION" {
				t.Fatalf("shell launch environment unexpectedly carries a launch generation: %#v", record["env_keys"])
			}
		}
	}
}
