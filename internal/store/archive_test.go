package store

import (
	"context"
	"testing"
)

// TestArchiveSessionSetsArchivedAtAndHidesFromListSessions covers task
// 111's central store contract (SPEC requirement 27): ArchiveSession sets
// archived_at (a durable, queryable column, never a comment) and records
// an "archived" event, and the row disappears from ListSessions' default
// view even though it still exists (GetSession still finds it by id) and
// its Status is left completely untouched -- archived_at is a flag, not a
// status.
func TestArchiveSessionSetsArchivedAtAndHidesFromListSessions(t *testing.T) {
	st := openTombstoneTestStore(t)
	ctx := context.Background()
	kept := createTombstoneTestSession(t, st, ctx, "kept-visible")
	doomed := createTombstoneTestSession(t, st, ctx, "archived-away")

	if err := st.ArchiveSession(ctx, doomed.ID, 200); err != nil {
		t.Fatalf("archive: %v", err)
	}

	visible, err := st.ListSessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(visible) != 1 || visible[0].ID != kept.ID {
		t.Fatalf("ListSessions after archive = %+v, want only %q", visible, kept.ID)
	}

	got, err := st.GetSession(ctx, doomed.ID)
	if err != nil {
		t.Fatalf("GetSession must still find an archived row by id: %v", err)
	}
	if got.ArchivedAt != 200 {
		t.Fatalf("ArchivedAt = %d, want 200", got.ArchivedAt)
	}
	if got.DeletedAt != 0 {
		t.Fatalf("DeletedAt = %d, want 0 -- archiving must never tombstone", got.DeletedAt)
	}
	if got.Status != "starting" {
		// createTombstoneTestSession's CreateSessionInput never sets a
		// Status, which CreateSession defaults to "starting" -- the point
		// of this assertion is only that ArchiveSession never wrote to it.
		t.Fatalf("Status = %q, want the row's original status untouched by ArchiveSession", got.Status)
	}

	var kind string
	if err := st.DB().QueryRowContext(ctx, `SELECT kind FROM events WHERE session_id = ? AND kind = 'archived'`, doomed.ID).Scan(&kind); err != nil {
		t.Fatalf("expected a durable 'archived' event: %v", err)
	}
}

// TestArchiveSessionLeavesStoppedStatusUnchanged proves requirement 27's
// "an archived row keeps whatever status it had" for the stopped case
// specifically: archiving an already-stopped row leaves status exactly
// "stopped", never rewriting it to some notion of "archived" status (there
// is no such status -- SPEC's seven statuses are unchanged by this task).
func TestArchiveSessionLeavesStoppedStatusUnchanged(t *testing.T) {
	st := openTombstoneTestStore(t)
	ctx := context.Background()
	session := createTombstoneTestSession(t, st, ctx, "stopped-then-archived")
	if err := st.UpdateSessionStatus(ctx, StatusUpdateInput{
		SessionID: session.ID, Status: "stopped", Reason: "killed by user", Source: "user", At: 150, EventKind: "killed",
	}); err != nil {
		t.Fatalf("stop the session first: %v", err)
	}

	if err := st.ArchiveSession(ctx, session.ID, 200); err != nil {
		t.Fatalf("archive: %v", err)
	}

	got, err := st.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "stopped" {
		t.Fatalf("Status after archive = %q, want unchanged %q", got.Status, "stopped")
	}
	if got.ArchivedAt != 200 {
		t.Fatalf("ArchivedAt = %d, want 200", got.ArchivedAt)
	}
}

// TestArchiveSessionRejectsMissingSession covers the not-found path shared
// with every other mutateSessionWithEvent-backed method.
func TestArchiveSessionRejectsMissingSession(t *testing.T) {
	st := openTombstoneTestStore(t)
	ctx := context.Background()
	if err := st.ArchiveSession(ctx, "does-not-exist", 100); err == nil {
		t.Fatal("ArchiveSession on an unknown id must fail, got nil error")
	}
}
