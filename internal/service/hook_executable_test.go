package service

import (
	"context"
	"testing"
)

// TestLaunchesRecordTheHookExecutableTheAgentIsBoundTo is R204c's persistence
// leg: the deck binary a launch's hook command names is stored on the row, by
// the create and again by a restart under a different binary, so the TUI can
// tell a session still bound to an older deck from one that is current.
func TestLaunchesRecordTheHookExecutableTheAgentIsBoundTo(t *testing.T) {
	cwd := t.TempDir()
	stubExecutableOnPath(t, "claude")
	service, db, _, _ := newAgentTestService(t, nil, "hook-executable")

	created, err := service.CreateAgent(context.Background(), AgentCreateInput{Name: "Claude: bound", CWD: cwd, Agent: "claude", PermissionProfile: "safe"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	original := service.DeckExecutable
	if created.HookExecutable != original {
		t.Fatalf("created row HookExecutable = %q, want %q", created.HookExecutable, original)
	}
	row, err := db.GetSession(context.Background(), created.ID)
	if err != nil || row.HookExecutable != original {
		t.Fatalf("stored HookExecutable = %q, %v; want %q", row.HookExecutable, err, original)
	}

	// The same session restarted by a different deck binary: the pane is
	// relaunched with that binary's hook command, and the row says so.
	upgraded := service
	upgraded.DeckExecutable = original + "-v2"
	restarted, outcome, err := upgraded.Restart(context.Background(), created.ID)
	if err != nil || outcome != ResumeStarted {
		t.Fatalf("restart: %v, outcome %v", err, outcome)
	}
	if restarted.HookExecutable != upgraded.DeckExecutable {
		t.Fatalf("restarted row HookExecutable = %q, want %q", restarted.HookExecutable, upgraded.DeckExecutable)
	}
	row, err = db.GetSession(context.Background(), created.ID)
	if err != nil || row.HookExecutable != upgraded.DeckExecutable {
		t.Fatalf("stored HookExecutable after restart = %q, %v; want %q", row.HookExecutable, err, upgraded.DeckExecutable)
	}
	if row.Status == "error" {
		t.Fatalf("restart left the row in %q", row.Status)
	}
}

// TestCodexRecordsAndPiAndShellDoNotRecordAHookExecutable: Codex instruments
// hooks like Claude and so has a binding to go stale; Pi and a shell launch no
// hook command, so they record nothing and can never show the hint.
func TestCodexRecordsAndPiAndShellDoNotRecordAHookExecutable(t *testing.T) {
	cwd := t.TempDir()
	stubExecutableOnPath(t, "codex")
	stubExecutableOnPath(t, "pi")
	service, _, _, _ := newAgentTestService(t, nil, "hook-executable-others")

	codex, err := service.CreateAgent(context.Background(), AgentCreateInput{Name: "Codex: bound", CWD: cwd, Agent: "codex", PermissionProfile: "safe"})
	if err != nil {
		t.Fatalf("create codex: %v", err)
	}
	if codex.HookExecutable != service.DeckExecutable {
		t.Fatalf("codex HookExecutable = %q, want %q", codex.HookExecutable, service.DeckExecutable)
	}
	pi, err := service.CreateAgent(context.Background(), AgentCreateInput{Name: "Pi: unbound", CWD: cwd, Agent: "pi", PermissionProfile: "safe"})
	if err != nil {
		t.Fatalf("create pi: %v", err)
	}
	if pi.HookExecutable != "" {
		t.Fatalf("pi HookExecutable = %q, want none (Pi launches no hook command)", pi.HookExecutable)
	}
}

// TestRestartRebindsCodexAndLeavesPiUnbound is R204c's persistence leg for the
// other two harnesses: a Codex session restarted by a different deck binary
// records that binary (its inline -c hook command is rebuilt from it), while a
// Pi session, which carries no hook command, still records nothing after the
// restart and so can never show the hint.
func TestRestartRebindsCodexAndLeavesPiUnbound(t *testing.T) {
	cwd := t.TempDir()
	stubExecutableOnPath(t, "codex")
	stubExecutableOnPath(t, "pi")
	service, db, _, _ := newAgentTestService(t, nil, "hook-executable-restart")
	upgraded := service
	upgraded.DeckExecutable = service.DeckExecutable + "-v2"

	for _, tc := range []struct {
		kind string
		want string
	}{{"codex", upgraded.DeckExecutable}, {"pi", ""}} {
		created, err := service.CreateAgent(context.Background(), AgentCreateInput{Name: tc.kind + ": restart", CWD: cwd, Agent: tc.kind, PermissionProfile: "safe"})
		if err != nil {
			t.Fatalf("create %s: %v", tc.kind, err)
		}
		// Codex learns its conversation id from its first SessionStart hook; a
		// restart needs it, so stand in for that hook here.
		if created.ConversationID == "" {
			if err := db.SetConversationID(context.Background(), created.ID, "conversation-"+tc.kind, "hook", 1); err != nil {
				t.Fatalf("set conversation id: %v", err)
			}
		}
		restarted, outcome, err := upgraded.Restart(context.Background(), created.ID)
		if err != nil || outcome != ResumeStarted {
			t.Fatalf("restart %s: %v, outcome %v", tc.kind, err, outcome)
		}
		row, err := db.GetSession(context.Background(), created.ID)
		if err != nil || row.HookExecutable != tc.want || restarted.HookExecutable != tc.want {
			t.Fatalf("%s HookExecutable after restart = %q (row %q), %v; want %q", tc.kind, restarted.HookExecutable, row.HookExecutable, err, tc.want)
		}
	}
}
