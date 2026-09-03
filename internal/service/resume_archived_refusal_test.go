package service

import (
	"context"
	"strings"
	"testing"
)

// TestResumeRefusesAnArchivedRowBeforeAnythingIsCreated proves SPEC.md:718
// (#8), and it is deliberately written as an ABSENCE test: the bug it guards
// is not a wrong message, it is a live agent that gets created and then
// hidden behind the archived filter. So the assertions are "no tmux session
// exists on the real socket", "the row still reads stopped (it never even
// briefly read starting)", "archived_at is byte-identical to what Archive
// wrote", and "the audit log gained no second launch record" -- the message
// naming `U` is checked last and is the least load-bearing part.
//
// Why it fails without the guard: AcquireLaunchLease treats a stopped row as
// leasable whatever archived_at says, so an unguarded Resume takes the lease,
// writes status=starting, and really does `new-session` the pane.
func TestResumeRefusesAnArchivedRowBeforeAnythingIsCreated(t *testing.T) {
	ctx := context.Background()
	stubExecutableOnPath(t, "claude")
	svc, db, logger, _ := newAgentTestService(t, nil, "resume-archived")

	created, err := svc.CreateAgent(ctx, AgentCreateInput{
		Name: "Claude: archived", CWD: t.TempDir(), Agent: "claude", PermissionProfile: "safe",
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	// Archive through the real service action `A` uses, which kills the pane
	// first: the result is exactly the state the operator hit -- stopped,
	// archived, hidden from the default list, and still leasable as far as
	// the store is concerned.
	if _, err := svc.Archive(ctx, created); err != nil {
		t.Fatalf("archive: %v", err)
	}
	archived, err := db.GetSession(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if archived.Status != "stopped" || archived.ArchivedAt == 0 {
		t.Fatalf("fixture row = status %q archived_at %d, want stopped and archived", archived.Status, archived.ArchivedAt)
	}
	if live, err := svc.TMux.List(ctx); err != nil || len(live) != 0 {
		t.Fatalf("fixture live tmux sessions = %#v, %v, want none before the resume attempt", live, err)
	}
	launchesBefore := launchRecordCount(t, logger.Path(), created.ID)

	session, outcome, resumeErr := svc.Resume(ctx, created.ID)
	if resumeErr == nil {
		t.Fatal("resume of an archived row succeeded (SPEC.md:718: an archived session is not startable)")
	}
	if outcome != ResumeNotLeasable {
		t.Fatalf("outcome = %v, want ResumeNotLeasable", outcome)
	}

	// The absence assertions, in the order the bug would break them.
	if live, listErr := svc.TMux.List(ctx); listErr != nil || len(live) != 0 {
		t.Fatalf("live tmux sessions after the refused resume = %#v, %v, want none created", live, listErr)
	}
	row, err := db.GetSession(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if row.Status != "stopped" {
		t.Fatalf("row status after the refused resume = %q, want stopped (an archived row must never even briefly read starting)", row.Status)
	}
	if row.ArchivedAt != archived.ArchivedAt {
		t.Fatalf("archived_at after the refused resume = %d, want unchanged %d", row.ArchivedAt, archived.ArchivedAt)
	}
	if row.StatusReason != archived.StatusReason {
		t.Fatalf("status reason after the refused resume = %q, want unchanged %q (the refusal is a message, not an error row)", row.StatusReason, archived.StatusReason)
	}
	if got := launchRecordCount(t, logger.Path(), created.ID); got != launchesBefore {
		t.Fatalf("launch records for the session = %d, want unchanged %d (nothing was launched)", got, launchesBefore)
	}
	if session.ID != created.ID {
		t.Fatalf("returned session id = %q, want the durable row %q for display", session.ID, created.ID)
	}

	// ...and the message the user actually reads names `U` as the way out,
	// since a refusal with no way forward is the same dead end as the bug.
	if !strings.Contains(resumeErr.Error(), "archived") || !strings.Contains(resumeErr.Error(), "unarchive") || !strings.Contains(resumeErr.Error(), "U") {
		t.Fatalf("refusal message = %q, want it to say the row is archived and name the U unarchive key", resumeErr.Error())
	}
}

// TestRestartInheritsTheArchivedRefusalFromResume proves the "one guard
// covers both" half of SPEC.md:718: `R` has no archived check of its own, it
// gets the refusal by routing through Resume. The fixture is deliberately
// the invariant-violating state (archived row, status not stopped) because
// that is the ONLY state in which Restart reaches Resume at all -- an
// archived row is normally stopped, and Restart refuses a stopped row
// earlier with "resume it instead". Archiving is done at the store layer
// here precisely so the pane is NOT killed first, which service.Archive
// would do.
func TestRestartInheritsTheArchivedRefusalFromResume(t *testing.T) {
	ctx := context.Background()
	stubExecutableOnPath(t, "claude")
	svc, db, _, _ := newAgentTestService(t, nil, "restart-archived")

	created, err := svc.CreateAgent(ctx, AgentCreateInput{
		Name: "Claude: restart-archived", CWD: t.TempDir(), Agent: "claude", PermissionProfile: "safe",
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := db.ArchiveSession(ctx, created.ID, 4242); err != nil {
		t.Fatalf("archive the row without killing its pane: %v", err)
	}
	if err := svc.TMux.Kill(ctx, created.Slug); err != nil {
		t.Fatalf("remove the pane so this test cannot pass by adopting one: %v", err)
	}

	_, outcome, restartErr := svc.Restart(ctx, created.ID)
	if restartErr == nil {
		t.Fatal("restart of an archived row succeeded (SPEC.md:718: R routes through resume, so one guard covers both)")
	}
	if outcome != ResumeNotLeasable {
		t.Fatalf("outcome = %v, want ResumeNotLeasable (inherited from Resume)", outcome)
	}
	if !strings.Contains(restartErr.Error(), "archived") || !strings.Contains(restartErr.Error(), "unarchive") || !strings.Contains(restartErr.Error(), "U") {
		t.Fatalf("restart refusal message = %q, want Resume's archived message naming the U unarchive key", restartErr.Error())
	}
	if live, listErr := svc.TMux.List(ctx); listErr != nil || len(live) != 0 {
		t.Fatalf("live tmux sessions after the refused restart = %#v, %v, want none created", live, listErr)
	}
	row, err := db.GetSession(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if row.ArchivedAt != 4242 {
		t.Fatalf("archived_at after the refused restart = %d, want unchanged 4242", row.ArchivedAt)
	}
	if row.Status == "starting" {
		t.Fatalf("row status after the refused restart = %q: an archived row must never read starting", row.Status)
	}
}

// launchRecordCount counts audit "launch" events for one session id, so a
// test can assert that a refusal launched NOTHING rather than assuming it.
func launchRecordCount(t *testing.T, auditPath, sessionID string) int {
	t.Helper()
	count := 0
	for _, record := range auditRecords(t, auditPath) {
		if record["event"] == "launch" && record["session_id"] == sessionID {
			count++
		}
	}
	return count
}
