package service

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/store"
)

// sessionContextKeys is SPEC section 6.1's nine DECK_SESSION_* variables,
// plus the deck-owned DECK_HOME that sits in the same unoverridable layer
// (session_context.go's sessionContextEnv). Task 001 (R104) merges every
// one of these last, above instrumentation, on every adapter and on both
// launch paths; this file is the unit evidence for that.
var sessionContextKeys = []string{
	"DECK_SESSION_ID", "DECK_SESSION_NAME", "DECK_SESSION_SLUG", "DECK_SESSION_CWD",
	"DECK_SESSION_AGENT", "DECK_SESSION_WORKSPACE", "DECK_SESSION_PROFILE",
	"DECK_SESSION_CONVERSATION_ID", "DECK_SESSION_LAUNCH_KIND", "DECK_HOME",
}

// tmuxShowEnvironment reports one key of a deck_<slug> tmux session's own
// environment table: (value, true) when tmux has it set -- including set to
// an empty value, which tmux still reports as "KEY=" with a zero exit
// status -- or ("", false) when tmux reports "unknown variable" (non-zero
// exit). That is the exact present-vs-absent distinction this file's tests
// turn on, confirmed empirically against the real tmux binary this suite
// runs against (`show-environment -t <session> <name>` on a session
// launched with `-e NAME=` prints "NAME=" and exits 0; on a name never
// passed at all it prints "unknown variable: NAME" and exits 1).
func tmuxShowEnvironment(t *testing.T, socket, slug, key string) (string, bool) {
	t.Helper()
	out, err := exec.Command("tmux", "-L", socket, "show-environment", "-t", "deck_"+slug, key).CombinedOutput()
	if err != nil {
		return "", false
	}
	line := strings.TrimSpace(string(out))
	return strings.TrimPrefix(line, key+"="), true
}

// tmuxSessionContextEnv snapshots every sessionContextKeys entry from a
// deck_<slug> tmux session's own environment table right now. A caller
// keeps the returned map around across a kill+relaunch of the same slug
// (which replaces that session's environment table wholesale) so create's
// and resume's snapshots can both be compared once both exist.
func tmuxSessionContextEnv(t *testing.T, socket, slug string) map[string]string {
	t.Helper()
	got := make(map[string]string, len(sessionContextKeys))
	for _, key := range sessionContextKeys {
		if value, present := tmuxShowEnvironment(t, socket, slug, key); present {
			got[key] = value
		}
	}
	return got
}

// assertTMuxEnvironmentAbsent fails unless key is entirely absent from the
// deck_<slug> session's own environment table -- used for
// DECK_LAUNCH_GENERATION, whose absent-when-no-lease-was-taken semantics
// (Claude.Instrument's own doc comment) are deliberately NOT the
// empty-rather-than-absent rule R104's nine variables follow.
func assertTMuxEnvironmentAbsent(t *testing.T, socket, slug, key string) {
	t.Helper()
	if value, present := tmuxShowEnvironment(t, socket, slug, key); present {
		t.Fatalf("tmux environment %s = %q, want absent (no launch lease taken)", key, value)
	}
}

// wantSessionContext computes the exact SPEC section 6.1 map a launch of
// session under launchKind should have produced. It is deliberately
// re-derived field by field here, rather than calling session_context.go's
// own sessionContextEnv, so this test cannot pass by construction against a
// production bug that changes both sides identically.
func wantSessionContext(session store.Session, deckHome, launchKind string) map[string]string {
	return map[string]string{
		"DECK_SESSION_ID":              session.ID,
		"DECK_SESSION_NAME":            session.Name,
		"DECK_SESSION_SLUG":            session.Slug,
		"DECK_SESSION_CWD":             session.CWD,
		"DECK_SESSION_AGENT":           session.Agent,
		"DECK_SESSION_WORKSPACE":       session.WorkspaceColumn,
		"DECK_SESSION_PROFILE":         session.PermissionProfile,
		"DECK_SESSION_CONVERSATION_ID": session.ConversationID,
		"DECK_SESSION_LAUNCH_KIND":     launchKind,
		"DECK_HOME":                    deckHome,
	}
}

// assertSessionContextEnv snapshots the deck_<slug> session's environment
// table and fails unless every sessionContextKeys entry is PRESENT (never
// merely absent-and-therefore-vacuously-unequal) with exactly the value
// wantSessionContext computes for session under launchKind, then returns
// the snapshot so the caller can compare it against another launch's.
func assertSessionContextEnv(t *testing.T, socket string, session store.Session, deckHome, launchKind string) map[string]string {
	t.Helper()
	got := tmuxSessionContextEnv(t, socket, session.Slug)
	want := wantSessionContext(session, deckHome, launchKind)
	for key, wantValue := range want {
		gotValue, present := got[key]
		if !present {
			t.Fatalf("%s missing from pane environment (launch kind %s), want present with %q", key, launchKind, wantValue)
		}
		if gotValue != wantValue {
			t.Fatalf("%s = %q, want %q (launch kind %s, session %+v)", key, gotValue, wantValue, launchKind, session)
		}
	}
	return got
}

// TestSessionContextEnvAcrossAdaptersAndLaunchPaths is task 002's unit
// evidence for R104: for each of the three adapter kinds cmd/deck/main.go
// registers (agent.NewShell, agent.NewClaude, agent.NewPi), it launches a
// session on the create path and then, after killing and stopping it, on
// the resume path, and asserts every one of the nine DECK_SESSION_*
// variables (plus DECK_HOME) actually lands in the pane's own tmux
// environment table with the row's own value on both.
func TestSessionContextEnvAcrossAdaptersAndLaunchPaths(t *testing.T) {
	for _, kind := range []string{"claude", "pi", "shell"} {
		kind := kind
		t.Run(kind, func(t *testing.T) {
			cwd := t.TempDir()
			if kind != "shell" {
				// claude/pi are never actually installed in CI; stub a real,
				// executable file of that name on PATH so Resume's own
				// lookPathIn check (agent binary must be found on PATH)
				// succeeds and the launch reaches tmux at all.
				stubExecutableOnPath(t, kind)
			}
			service, db, _, socket := newAgentTestService(t, map[string]string{"DECK_SESSION_NAME": "config-lied"}, "session-context-"+kind)

			// Both a session env entry and a config [env] entry claim the
			// same name deck's own layer owns; R104 requires both to lose.
			sessionEnv := map[string]string{"DECK_SESSION_NAME": "session-lied"}

			var created store.Session
			var err error
			if kind == "shell" {
				created, err = service.CreateShell(context.Background(), ShellCreateInput{
					Name: "Context: " + kind, CWD: cwd, Env: sessionEnv,
				})
			} else {
				created, err = service.CreateAgent(context.Background(), AgentCreateInput{
					Name: "Context: " + kind, CWD: cwd, Agent: kind, PermissionProfile: "safe", Env: sessionEnv,
				})
			}
			if err != nil {
				t.Fatalf("create %s session: %v", kind, err)
			}
			// A brand-new row's workspace column is never written by any
			// service call (no INSERT and no UPDATE in internal/store names
			// it), so it is genuinely unset here on both launches of this
			// same row -- the fixture this test uses to demonstrate R104's
			// "empty rather than absent" rule for workspace.
			if created.WorkspaceColumn != "" {
				t.Fatalf("%s: create-returned session workspace column = %q, want unset -- this test's empty-not-absent fixture assumption no longer holds", kind, created.WorkspaceColumn)
			}

			createEnv := assertSessionContextEnv(t, socket, created, service.DeckHome, LaunchKindCreate)
			if createEnv["DECK_SESSION_WORKSPACE"] != "" {
				t.Fatalf("%s: create DECK_SESSION_WORKSPACE = %q, want present with an empty value (unset workspace)", kind, createEnv["DECK_SESSION_WORKSPACE"])
			}
			if createEnv["DECK_SESSION_NAME"] == "session-lied" || createEnv["DECK_SESSION_NAME"] == "config-lied" {
				t.Fatalf("%s: create DECK_SESSION_NAME lost to a user-controlled layer: %q", kind, createEnv["DECK_SESSION_NAME"])
			}
			// No launch lease is ever taken on a brand-new row's first
			// launch (its first resume/restart takes the first one), so
			// DECK_LAUNCH_GENERATION must be entirely absent, not merely
			// empty, on every adapter's create path.
			assertTMuxEnvironmentAbsent(t, socket, created.Slug, "DECK_LAUNCH_GENERATION")

			if kind == "shell" {
				// shell never assigns a conversation id (Capabilities().
				// AssignsConversationID is false): this is the fixture for
				// R104's "empty rather than absent" rule for
				// conversation_id.
				if createEnv["DECK_SESSION_CONVERSATION_ID"] != "" {
					t.Fatalf("shell: create DECK_SESSION_CONVERSATION_ID = %q, want present with an empty value (never assigned)", createEnv["DECK_SESSION_CONVERSATION_ID"])
				}
			} else if created.ConversationID == "" {
				t.Fatalf("%s: create-time conversation id unexpectedly empty", kind)
			}

			// Resume path: kill the create-time pane, mark the row stopped
			// (mirroring what reconcile does once a pane actually goes
			// away), and resume it -- SPEC section 6.1's second launch kind.
			if err := service.TMux.Kill(context.Background(), created.Slug); err != nil {
				t.Fatalf("kill %s pane before resume: %v", kind, err)
			}
			stopSession(t, db, created.ID)
			resumed, outcome, err := service.Resume(context.Background(), created.ID)
			if err != nil {
				t.Fatalf("resume %s session: %v", kind, err)
			}
			if outcome != ResumeStarted {
				t.Fatalf("resume %s outcome = %v, want ResumeStarted", kind, outcome)
			}
			resumeEnv := assertSessionContextEnv(t, socket, resumed, service.DeckHome, LaunchKindResume)
			if resumeEnv["DECK_SESSION_NAME"] == "session-lied" || resumeEnv["DECK_SESSION_NAME"] == "config-lied" {
				t.Fatalf("%s: resume DECK_SESSION_NAME lost to a user-controlled layer: %q", kind, resumeEnv["DECK_SESSION_NAME"])
			}
			if kind == "shell" && resumeEnv["DECK_SESSION_CONVERSATION_ID"] != "" {
				t.Fatalf("shell: resume DECK_SESSION_CONVERSATION_ID = %q, want present with an empty value", resumeEnv["DECK_SESSION_CONVERSATION_ID"])
			}

			// R104: create and resume are the same session's two launches of
			// one untouched row, so they must differ in
			// DECK_SESSION_LAUNCH_KIND and in no other DECK_SESSION_* key (nor
			// in DECK_HOME). This compares every key in sessionContextKeys
			// except the launch kind itself -- no key is excluded -- in both
			// directions, so a key present on only one of the two launches
			// fails here as well.
			for _, key := range sessionContextKeys {
				if key == "DECK_SESSION_LAUNCH_KIND" {
					continue
				}
				createValue, createPresent := createEnv[key]
				resumeValue, resumePresent := resumeEnv[key]
				if createPresent != resumePresent || createValue != resumeValue {
					t.Fatalf("%s: create/resume disagree on %s: %q (present %v) vs %q (present %v), want identical -- only DECK_SESSION_LAUNCH_KIND may differ", kind, key, createValue, createPresent, resumeValue, resumePresent)
				}
			}
			if len(createEnv) != len(resumeEnv) || len(createEnv) != len(sessionContextKeys) {
				t.Fatalf("%s: create/resume key sets = %d/%d keys, want both %d (every sessionContextKeys entry present on both launches)", kind, len(createEnv), len(resumeEnv), len(sessionContextKeys))
			}
			if createEnv["DECK_SESSION_LAUNCH_KIND"] != LaunchKindCreate || resumeEnv["DECK_SESSION_LAUNCH_KIND"] != LaunchKindResume {
				t.Fatalf("%s: launch kinds = %q/%q, want %q/%q", kind, createEnv["DECK_SESSION_LAUNCH_KIND"], resumeEnv["DECK_SESSION_LAUNCH_KIND"], LaunchKindCreate, LaunchKindResume)
			}
			// The unset workspace column is exported empty on the resume path
			// too, not silently replaced by store.GetSession's §11 grouping
			// fallback (the cwd's basename) on the way back out of the row.
			if resumeEnv["DECK_SESSION_WORKSPACE"] != "" {
				t.Fatalf("%s: resume DECK_SESSION_WORKSPACE = %q, want present with an empty value (workspace column still unset)", kind, resumeEnv["DECK_SESSION_WORKSPACE"])
			}
			if resumed.Workspace == "" {
				t.Fatalf("%s: resumed row's §11 grouping label unexpectedly empty -- the DECK_SESSION_WORKSPACE assertion above would then be vacuous", kind)
			}

			_ = db
		})
	}
}
