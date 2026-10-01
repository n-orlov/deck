package service

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/store"
)

// TestReconcileNeverOverwritesALaunchFailureErrorWithSessionDisappeared pins
// task 011's M9 mechanism directly at the Reconcile level, rather than
// racing a real background tick against a real `r` keypress: it forces the
// exact DB shape the race produces (an `error` row, source `tmux`, no
// pane_exit_status, no live tmux session for its slug -- precisely what
// launchFailed, resume.go/shell.go's shared "resume failure" writer, leaves
// behind for every one of SPEC §9.3's three named resume failure causes,
// none of which ever create a tmux session for the failed attempt) and
// then runs the EXACT reconcile pass a real background tick would run.
//
// Before the fix, reconcile.go's "terminal" row set exempted only `stopped`,
// a pane_exit_status crash, and a `starting`+`user` row from its final
// "no live session for this row -> mark it stopped" branch. A plain
// `error`/`tmux` row -- the shape launchFailed leaves -- was NOT exempt, so
// the very next reconcile pass (any pass whose own ListSessions read landed
// after the error write) treated the launch failure's own permanent absence
// of a tmux session as a *fresh* disappearance and overwrote the specific,
// SPEC-mandated reason ("no conversation id is assigned to resume ...")
// with the generic "tmux session disappeared" -- exactly resume_failure.
// feature:9's failure mode (resume_failure.feature:9, dialog "an unknown
// conversation id keeps the row as a retained, explained error"). This test
// calls launchFailed directly (same package, same production function) so
// it exercises the real write shape without needing a real tmux process or
// a real timing race, then calls Reconcile and asserts the row and its
// reason are untouched and no "tmux.session_gone" event/audit entry was
// recorded for it.
func TestReconcileNeverOverwritesALaunchFailureErrorWithSessionDisappeared(t *testing.T) {
	cwd := t.TempDir()
	service, db, logger, _ := newAgentTestService(t, nil, "resume-failure-reconcile-regression")

	// A row left "resumable" by a prior clean exit: stopped, no live tmux
	// session for its slug (deliberately never created here -- the point
	// of M9 is that the failed resume attempt below never creates one
	// either).
	session, err := db.CreateSession(context.Background(), store.CreateSessionInput{
		ID: "resume-failure-regression-noid", Name: "Claude: noid", CWD: cwd, Agent: "claude",
		CapturedPath: os.Getenv("PATH"), Status: "stopped", StatusSource: "user", StatusAt: 1, CreatedAt: 1,
	})
	if err != nil {
		t.Fatalf("create durable session directly: %v", err)
	}

	// The exact write resume.go's unknown-conversation-id branch makes
	// (session.go:164 at HEAD): launchFailed with the SPEC §9.3 reason.
	const wantReasonSubstring = "conversation id"
	if _, failErr := service.launchFailed(context.Background(), session, errors.New(
		`resume session "Claude: noid": no conversation id is assigned to resume (unknown/rejected conversation id)`)); failErr == nil {
		t.Fatalf("launchFailed: want it to return the cause it was given, got nil")
	}

	before, err := db.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("get session after launchFailed: %v", err)
	}
	if before.Status != "error" || !strings.Contains(before.StatusReason, wantReasonSubstring) {
		t.Fatalf("row after launchFailed = status %q reason %q, want error/%q", before.Status, before.StatusReason, wantReasonSubstring)
	}

	// The exact pass a real background reconcile tick (or a post-hook
	// liveness pass for an unrelated session) runs next, with no live tmux
	// session ever having existed for this row's slug.
	if err := service.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	after, err := db.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("get session after reconcile: %v", err)
	}
	if after.Status != "error" {
		t.Fatalf("row status after reconcile = %q, want it to stay error (reconcile must not reclassify a resume-failure error row it never gave a tmux session)", after.Status)
	}
	if !strings.Contains(after.StatusReason, wantReasonSubstring) {
		t.Fatalf("row status reason after reconcile = %q, want it still to name %q (the retained, explained error SPEC §9.3 requires)", after.StatusReason, wantReasonSubstring)
	}

	var goneEvents int
	if err := db.DB().QueryRow(`SELECT count(*) FROM events WHERE session_id = ? AND kind = 'tmux.session_gone'`, session.ID).Scan(&goneEvents); err != nil {
		t.Fatalf("count tmux.session_gone events: %v", err)
	}
	if goneEvents != 0 {
		t.Fatalf("tmux.session_gone events for %q = %d, want 0: reconcile must take no verdict from a row whose absent tmux session it never expected present", session.ID, goneEvents)
	}
	if auditContains(t, logger.Path(), session.ID, "tmux.session_gone") {
		t.Fatalf("audit log for %q unexpectedly contains tmux.session_gone", session.ID)
	}
}
