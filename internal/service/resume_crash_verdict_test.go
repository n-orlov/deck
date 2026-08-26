package service

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// TestResumeMakesACrashedRowReconcilableAgain is issue #9's guard-1 leg: the
// reconciler takes no liveness verdict from a row whose pane_exit_status is set
// (`terminal` in reconcile.go), and nothing used to clear that column, so ONE
// crash removed a session from reconciliation for good — three of the
// operator's live rows, one displaying a wrong status for 30+ hours while
// serving its user normally.
//
// The fixture is built entirely through real code paths: a pane really dies
// non-zero, a real reconcile pass collects it (storing pane_exit_status and
// crash_tail and tearing the tmux session down), and the row is really killed
// so it is resumable again — which is the only in-app route out of the wedge,
// since AcquireLaunchLease refuses any status other than stopped.
//
// The observation assertion needs an observable consequence of "this pass
// looked at the row", so the row is a shell: for a live shell pane the pass's
// verdict is the starting → running promotion plus its tmux.shell_live event
// and audit line. Against unfixed code the resumed row still carries the dead
// pane's exit status, the pass skips it at `terminal`, and the promotion never
// happens — a live session frozen at starting exactly as reported.
func TestResumeMakesACrashedRowReconcilableAgain(t *testing.T) {
	cwd := t.TempDir()
	svc, db, logger, _ := newAgentTestService(t, nil, "resume-clears-crash-verdict")
	ctx := context.Background()

	session, err := db.CreateSession(ctx, store.CreateSessionInput{
		ID: "00000000-0000-4000-8000-000000000694", Name: "crashed shell", CWD: cwd,
		Agent: "shell", CapturedPath: os.Getenv("PATH"),
		Status: "running", StatusSource: "tmux", StatusAt: 1, CreatedAt: 1,
	})
	if err != nil {
		t.Fatalf("create fixture row: %v", err)
	}

	// A real crash, collected by a real pass.
	command := `printf 'shell: killed by the OOM reaper\n'; exit 137`
	if _, err := svc.TMux.Create(ctx, tmux.Launch{Slug: session.Slug, CWD: cwd, Command: []string{"/bin/sh", "-c", command}}); err != nil {
		t.Fatalf("create fixture pane: %v", err)
	}
	waitForDeadPane(t, svc.TMux, session.Slug, 137)
	if err := svc.Reconcile(ctx); err != nil {
		t.Fatalf("collect the crashed pane: %v", err)
	}
	crashed, err := db.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("refresh crashed row: %v", err)
	}
	if crashed.Status != "error" || crashed.PaneExitStatus == nil || *crashed.PaneExitStatus != 137 ||
		!strings.Contains(crashed.CrashTail, "OOM reaper") {
		t.Fatalf("fixture is not a stored crash verdict: %#v", crashed)
	}

	// The operator's escape from the wedge: kill the row so it is leasable.
	if err := svc.Kill(ctx, crashed); err != nil {
		t.Fatalf("kill the crashed row: %v", err)
	}
	stopped, err := db.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("refresh killed row: %v", err)
	}
	if stopped.Status != "stopped" || stopped.PaneExitStatus == nil || stopped.CrashTail == "" {
		t.Fatalf("fixture is not discriminating: a stopped row must still carry the previous pane's verdict, got %#v", stopped)
	}

	resumed, outcome, err := svc.Resume(ctx, session.ID)
	if err != nil {
		t.Fatalf("resume a row carrying a stale crash verdict: %v", err)
	}
	if outcome != ResumeStarted {
		t.Fatalf("resume outcome = %v, want ResumeStarted", outcome)
	}
	if resumed.Status != "starting" {
		t.Fatalf("returned session status = %q, want starting", resumed.Status)
	}
	live, err := svc.TMux.HasLivePane(ctx, session.Slug)
	if err != nil {
		t.Fatalf("HasLivePane after resume: %v", err)
	}
	if !live {
		t.Fatal("resume reported ResumeStarted but created no live pane, so this test cannot speak about the new pane's verdict")
	}
	after, err := db.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("refresh resumed row: %v", err)
	}
	if after.Status != "starting" || after.StatusSource != "tmux" {
		t.Fatalf("resumed row = status %q source %q, want the post-launch.ready starting/tmux shape this test reasons about", after.Status, after.StatusSource)
	}

	// The assertion that pins guard 1: the resumed session is observed against
	// tmux on the next pass instead of being skipped as terminal. It comes
	// BEFORE the column assertions below deliberately, so that on unfixed code
	// this test fails on the behaviour the operator saw — a live row frozen at
	// starting — rather than on the column that causes it.
	if err := svc.Reconcile(ctx); err != nil {
		t.Fatalf("reconcile after resume: %v", err)
	}
	observed, err := db.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("refresh reconciled row: %v", err)
	}
	if observed.Status != "running" || observed.StatusSource != "tmux" || observed.StatusReason != "tmux pane is alive" {
		t.Fatalf("the resumed row was skipped by reconciliation rather than observed (#9): %#v", observed)
	}
	var promotions int
	if err := db.DB().QueryRow(`SELECT count(*) FROM events WHERE session_id = ? AND kind = 'tmux.shell_live'`, session.ID).Scan(&promotions); err != nil {
		t.Fatal(err)
	}
	if promotions != 1 {
		t.Fatalf("tmux.shell_live events = %d, want 1: the pass did not observe the resumed row", promotions)
	}
	if !auditContains(t, logger.Path(), session.ID, "tmux.shell_live") {
		t.Fatalf("audit lacks the reconciliation observation of the resumed row %q", session.ID)
	}

	// And the columns the resume replaced along with the pane.
	if observed.PaneExitStatus != nil {
		t.Fatalf("resume created a NEW pane but the previous pane's exit status %d is still attached to the row (#9)", *observed.PaneExitStatus)
	}
	if observed.CrashTail != "" {
		t.Fatalf("resume created a NEW pane but the previous pane's crash tail is still attached to the row (#9): %q", observed.CrashTail)
	}
}
