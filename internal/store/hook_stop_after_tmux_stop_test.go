package store

import (
	"context"
	"path/filepath"
	"testing"
)

// TestHookStopRefinesTmuxSourcedStop (task 026): a SessionEnd hook's
// "stopped" landing after a reconcile pass already recorded the pane's
// disappearance as a tmux-sourced "stopped" must still be applied -- SPEC §7
// precedence is hook > tmux, and the write resurrects nothing (the status is
// "stopped" either way). The hook's verdict and reason replace the bare
// liveness fact, and a later stale tmux "stopped" does not replace it back.
// A hook "running" onto the same row stays refused (no return edge from
// stopped), and so does any hook onto a user-sourced stop.
func TestHookStopRefinesTmuxSourcedStop(t *testing.T) {
	home := t.TempDir()
	store, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	create := func(id, source string) {
		t.Helper()
		if _, err := store.CreateSession(ctx, CreateSessionInput{
			ID: id, Name: id, CWD: "/work", Agent: "claude", CapturedPath: "/bin",
			Status: "running", StatusSource: "hook", StatusAt: 10, CreatedAt: 1,
		}); err != nil {
			t.Fatal(err)
		}
		if err := store.UpdateSessionStatus(ctx, StatusUpdateInput{
			SessionID: id, Status: "stopped", Reason: "tmux session disappeared", Source: source, At: 20,
			EventKind: "tmux.session_gone",
		}); err != nil {
			t.Fatal(err)
		}
	}

	create("tmux-stopped", "tmux")
	if err := store.UpdateSessionStatus(ctx, StatusUpdateInput{
		SessionID: "tmux-stopped", Status: "running", Reason: "late", Source: "hook", At: 25,
		EventKind: "session_start",
	}); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetSession(ctx, "tmux-stopped")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "stopped" || got.StatusSource != "tmux" {
		t.Fatalf("a hook \"running\" resurrected a stopped row: status=%q source=%q", got.Status, got.StatusSource)
	}
	if err := store.UpdateSessionStatus(ctx, StatusUpdateInput{
		SessionID: "tmux-stopped", Status: "stopped", Reason: "logout", Source: "hook", At: 30,
		EventKind: "session_end",
	}); err != nil {
		t.Fatal(err)
	}
	got, err = store.GetSession(ctx, "tmux-stopped")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "stopped" || got.StatusSource != "hook" || got.StatusReason != "logout" || got.StatusAt != 30 {
		t.Fatalf("SessionEnd hook after a tmux-sourced stop: status=%q source=%q reason=%q at=%d, want stopped/hook/logout/30", got.Status, got.StatusSource, got.StatusReason, got.StatusAt)
	}

	// The reverse order: a reconcile pass with a stale read writes tmux
	// "stopped" after the hook's stop landed; the hook's verdict stays.
	if err := store.UpdateSessionStatus(ctx, StatusUpdateInput{
		SessionID: "tmux-stopped", Status: "stopped", Reason: "tmux session disappeared", Source: "tmux", At: 40,
		EventKind: "tmux.session_gone",
	}); err != nil {
		t.Fatal(err)
	}
	got, err = store.GetSession(ctx, "tmux-stopped")
	if err != nil {
		t.Fatal(err)
	}
	if got.StatusSource != "hook" || got.StatusReason != "logout" || got.StatusAt != 30 {
		t.Fatalf("a late tmux stop replaced the hook's stop verdict: source=%q reason=%q at=%d", got.StatusSource, got.StatusReason, got.StatusAt)
	}

	create("user-stopped", "user")
	if err := store.UpdateSessionStatus(ctx, StatusUpdateInput{
		SessionID: "user-stopped", Status: "stopped", Reason: "logout", Source: "hook", At: 30,
		EventKind: "session_end",
	}); err != nil {
		t.Fatal(err)
	}
	got, err = store.GetSession(ctx, "user-stopped")
	if err != nil {
		t.Fatal(err)
	}
	if got.StatusSource != "user" || got.StatusReason != "tmux session disappeared" {
		t.Fatalf("a hook overwrote a user-sourced stop: source=%q reason=%q", got.StatusSource, got.StatusReason)
	}
}
