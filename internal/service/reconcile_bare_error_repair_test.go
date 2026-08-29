package service

import (
	"context"
	"testing"

	"github.com/n-orlov/deck/internal/store"
)

// TestReconcileRepairsBareErrorRowWithLivePane is review finding 1: a hook-
// or probe-written `error` row with no pane_exit_status at all (never
// observed a crash, so it carries no verdict for a live pane to contradict)
// paired with a live, non-dead pane is exactly the same SPEC §7 invariant
// violation as a `stopped` row or an `error` row that DOES carry a crash
// verdict -- the pane is the part that is right. Before this fix,
// repairTerminalRowWithLivePane's call sat inside the `terminal` expression,
// which is `stopped || PaneExitStatus != nil || (starting && source==user)`
// and therefore never true for a bare `error` row, so this exact row was
// silently left alone forever. The repair now gets its own
// `Status == "stopped" || Status == "error"` test that does not go through
// `terminal`, so this row reaches it too, while `terminal`'s other job (the
// tmux-session-absent branch a few lines down) keeps its present meaning.
func TestReconcileRepairsBareErrorRowWithLivePane(t *testing.T) {
	svc, db, logger, fake := newFakeTMuxRepairService(t, "repair-bare-error")

	session, err := db.CreateSession(context.Background(), store.CreateSessionInput{
		ID: "00000000-0000-4000-8000-000000000051", Name: "bare error, live pane", CWD: t.TempDir(),
		Agent: "claude", CapturedPath: "/bin", Status: "error", StatusSource: "hook", StatusAt: 1, CreatedAt: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if session.PaneExitStatus != nil {
		t.Fatalf("fixture must start with no pane-exit verdict, got %#v", session.PaneExitStatus)
	}
	fakeTMuxLivePane(t, svc.TMux, session.Slug)

	// (c) the repair fires with no TUI keypress: Reconcile is the only call
	// made here, the same reconciliation pass every hook/probe update flows
	// through, never a user action.
	if err := svc.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	// (a) the row is corrected: agent liveness alone supports only the
	// neutral "starting" a fresh pane begins at, sourced from tmux, with a
	// correction event recorded both in the store and the audit log.
	got, err := db.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "starting" || got.StatusSource != "tmux" {
		t.Fatalf("bare error row under a live pane was not repaired: status=%q source=%q (%#v)", got.Status, got.StatusSource, got)
	}
	if got.PaneExitStatus != nil {
		t.Fatalf("repair must not fabricate a pane-exit verdict: %#v", got)
	}
	assertSingleEvent(t, db, session.ID, "tmux.terminal_pane_alive")
	if !auditContains(t, logger.Path(), session.ID, "tmux.terminal_pane_alive") {
		t.Fatalf("audit lacks the repair transition for %q", session.ID)
	}

	// (b) the pane survives: no kill-session/kill-pane/respawn-*/send-keys
	// argv reached tmux.
	assertNoPaneMutation(t, fake)

	// (d) a second Reconcile is a no-op: the row is no longer terminal, so
	// the repeat pass takes the ordinary path and writes nothing new.
	if err := svc.Reconcile(context.Background()); err != nil {
		t.Fatalf("repeat reconcile: %v", err)
	}
	again, err := db.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if again.Status != got.Status || again.StatusSource != got.StatusSource || again.StatusAt != got.StatusAt {
		t.Fatalf("repeat reconcile changed the repaired row: before=%#v after=%#v", got, again)
	}
	assertSingleEvent(t, db, session.ID, "tmux.terminal_pane_alive")
	assertNoPaneMutation(t, fake)
}
