package service

import (
	"context"
	"testing"

	"github.com/n-orlov/deck/internal/store"
)

// TestReconcileNeverRepairsHookSourcedErrorWithLivePane and
// TestReconcileNeverRepairsProbeSourcedErrorWithLivePane replace
// TestReconcileRepairsBareErrorRowWithLivePane (task 902, per task 901's
// finding F40, docs/reports/phase3g-findings.md, sha c19bdde). That test
// asserted the repair fires for a bare hook/probe `error` row under a live
// pane; F40 shows that assertion contradicted SPEC.md itself: SPEC §7's
// transition table has `running --turn or API failure--> error` with no pane
// death at all, so a hook- or probe-written `error` row with no
// pane_exit_status is the agent's own considered verdict, not a stale liveness
// guess tmux gets to overrule. Only a row carrying a pane-exit verdict, or one
// whose own source was already `tmux`/`user` (never more than a liveness
// guess to begin with), is the invariant violation the repair exists to fix.
// A `stopped` row keeps being repaired unconditionally regardless of source,
// because SPEC.md:568-570 names it the actual unrecoverable wedge (`canResume`
// only accepts `stopped`; `canKill`/`canReachPane` accept every non-`stopped`
// row, including a live-pane `error`), so these two tests deliberately do not
// touch `stopped`.
func TestReconcileNeverRepairsHookSourcedErrorWithLivePane(t *testing.T) {
	assertLiveSourcedErrorRowUntouched(t, "hook", "00000000-0000-4000-8000-000000000061")
}

func TestReconcileNeverRepairsProbeSourcedErrorWithLivePane(t *testing.T) {
	assertLiveSourcedErrorRowUntouched(t, "probe", "00000000-0000-4000-8000-000000000062")
}

// assertLiveSourcedErrorRowUntouched constructs an `error` row sourced from
// source (hook or probe) with no pane-exit verdict, pairs it with a live,
// non-dead pane via the fake tmux client, runs Reconcile, and asserts the row
// is left byte-for-byte alone: Status, StatusSource, Reason and Acknowledged
// unchanged, and zero tmux.terminal_pane_alive events recorded for it.
func assertLiveSourcedErrorRowUntouched(t *testing.T, source, sessionID string) {
	t.Helper()
	svc, db, _, fake := newFakeTMuxRepairService(t, "repair-live-"+source)

	session, err := db.CreateSession(context.Background(), store.CreateSessionInput{
		ID: sessionID, Name: source + "-sourced error, live pane", CWD: t.TempDir(),
		Agent: "claude", CapturedPath: "/bin", Status: "running", StatusSource: "hook", StatusAt: 1, CreatedAt: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	// The hook or probe's own error write: an agent-considered verdict with no
	// pane-exit evidence behind it at all -- exactly SPEC §7's `running
	// --turn or API failure--> error` transition.
	update := store.StatusUpdateInput{
		SessionID: session.ID, Status: "error", Reason: "agent error", Source: source,
		At: 2, EventKind: source + ".error",
	}
	if source == "probe" {
		// StaleAfter is required for every probe verdict write, matching the
		// staleAfter reconcile.go passes into every real probe.<status> update,
		// and a probe write over a hook-sourced row additionally requires the
		// gap to reach staleAfter (store.go's own probe-freshness guard), so
		// At sits far enough past the fixture's StatusAt=1 to clear it.
		update.StaleAfter = 60000
		update.At = 100000
	}
	if err := db.UpdateSessionStatus(context.Background(), update); err != nil {
		t.Fatal(err)
	}
	before, err := db.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if before.Status != "error" || before.StatusSource != source || before.PaneExitStatus != nil {
		t.Fatalf("fixture is not a bare %s-sourced error row: %#v", source, before)
	}
	if before.Acknowledged {
		t.Fatalf("fixture must start unacknowledged (an error row always sets the unseen marker): %#v", before)
	}
	fakeTMuxLivePane(t, svc.TMux, session.Slug)

	if err := svc.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	after, err := db.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Status != before.Status || after.StatusSource != before.StatusSource ||
		after.StatusReason != before.StatusReason || after.Acknowledged != before.Acknowledged {
		t.Fatalf("a live-pane %s-sourced error row must not be touched by the repair: before=%#v after=%#v", source, before, after)
	}

	var repairEvents int
	if err := db.DB().QueryRow(`SELECT count(*) FROM events WHERE session_id = ? AND kind = 'tmux.terminal_pane_alive'`, session.ID).Scan(&repairEvents); err != nil {
		t.Fatal(err)
	}
	if repairEvents != 0 {
		t.Fatalf("a %s-sourced error row with no pane-exit verdict must never be repaired, got %d repair events", source, repairEvents)
	}
	assertNoPaneMutation(t, fake)
}
