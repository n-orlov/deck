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

// TestListArchivedSessionsIsTheOnlyRouteBackToAnArchivedRow proves task
// 123/I-10's store half: ListSessions excludes an archived row (already
// covered above), and ListArchivedSessions is the dedicated accessor that
// DOES return it -- while still excluding a merely-active row, and while
// still excluding a row that is BOTH archived and tombstoned, since a
// tombstoned row has no filter route back at all (SoftDeleteSession is a
// harder removal than ArchiveSession).
func TestListArchivedSessionsIsTheOnlyRouteBackToAnArchivedRow(t *testing.T) {
	st := openTombstoneTestStore(t)
	ctx := context.Background()
	active := createTombstoneTestSession(t, st, ctx, "still-active")
	archived := createTombstoneTestSession(t, st, ctx, "archived-away")
	archivedAndDeleted := createTombstoneTestSession(t, st, ctx, "archived-then-deleted")

	if err := st.ArchiveSession(ctx, archived.ID, 200); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if err := st.ArchiveSession(ctx, archivedAndDeleted.ID, 200); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if err := st.SoftDeleteSession(ctx, archivedAndDeleted.ID, 300); err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	got, err := st.ListArchivedSessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != archived.ID {
		t.Fatalf("ListArchivedSessions = %+v, want only %q (never the active row, never the archived-and-tombstoned one)", got, archived.ID)
	}

	// And ListSessions' own default view still excludes it, exactly as
	// requirement 27 already requires -- this is the fact requirement 33's
	// filter exists to work around, restated here so a regression in either
	// method is caught by the same test.
	defaultView, err := st.ListSessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(defaultView) != 1 || defaultView[0].ID != active.ID {
		t.Fatalf("ListSessions = %+v, want only the active row %q", defaultView, active.ID)
	}
}

// TestUnarchiveSessionClearsArchivedAtAndReturnsToListSessions is R71's
// store half (SPEC.md:323-332, issue #8): `A` must not be a one-way door.
// UnarchiveSession mirrors RestoreSession exactly -- archived_at back to
// 0, its own durable "unarchived" event recorded in the same transaction,
// the row back in ListSessions' default view and out of
// ListArchivedSessions -- while leaving Status (and deleted_at) completely
// alone, because archived_at is a flag and unarchiving is not a resume.
func TestUnarchiveSessionClearsArchivedAtAndReturnsToListSessions(t *testing.T) {
	st := openTombstoneTestStore(t)
	ctx := context.Background()
	session := createTombstoneTestSession(t, st, ctx, "archived-then-back")
	if err := st.UpdateSessionStatus(ctx, StatusUpdateInput{
		SessionID: session.ID, Status: "stopped", Reason: "killed by user", Source: "user", At: 150, EventKind: "killed",
	}); err != nil {
		t.Fatalf("stop the session first: %v", err)
	}
	if err := st.ArchiveSession(ctx, session.ID, 200); err != nil {
		t.Fatalf("archive: %v", err)
	}

	if err := st.UnarchiveSession(ctx, session.ID, 300); err != nil {
		t.Fatalf("unarchive: %v", err)
	}

	got, err := st.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ArchivedAt != 0 {
		t.Fatalf("ArchivedAt after unarchive = %d, want 0", got.ArchivedAt)
	}
	if got.DeletedAt != 0 {
		t.Fatalf("DeletedAt = %d, want 0 -- unarchiving must never touch the tombstone", got.DeletedAt)
	}
	if got.Status != "stopped" {
		t.Fatalf("Status after unarchive = %q, want unchanged %q -- unarchiving is not a resume", got.Status, "stopped")
	}

	visible, err := st.ListSessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(visible) != 1 || visible[0].ID != session.ID {
		t.Fatalf("ListSessions after unarchive = %+v, want the row back in the default view", visible)
	}
	stillArchived, err := st.ListArchivedSessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(stillArchived) != 0 {
		t.Fatalf("ListArchivedSessions after unarchive = %+v, want empty", stillArchived)
	}

	var at int64
	if err := st.DB().QueryRowContext(ctx, `SELECT at FROM events WHERE session_id = ? AND kind = 'unarchived'`, session.ID).Scan(&at); err != nil {
		t.Fatalf("expected a durable 'unarchived' event recorded exactly as 'restored' is: %v", err)
	}
	if at != 300 {
		t.Fatalf("unarchived event at = %d, want the 300 passed to UnarchiveSession", at)
	}
}

// TestUnarchiveSessionRejectsMissingSession covers the not-found path
// shared with every other mutateSessionWithEvent-backed method, exactly as
// TestArchiveSessionRejectsMissingSession does for the other direction.
func TestUnarchiveSessionRejectsMissingSession(t *testing.T) {
	st := openTombstoneTestStore(t)
	ctx := context.Background()
	if err := st.UnarchiveSession(ctx, "does-not-exist", 100); err == nil {
		t.Fatal("UnarchiveSession on an unknown id must fail, got nil error")
	}
}

// TestListSessionsIncludingArchivedKeepsArchivedAndDropsTombstoned pins the
// accessor R71 leg 3 added for hook resolution: exactly one term of
// ListSessions' WHERE clause is dropped. Archived rows come back (they are
// hidden from the sidebar, not disowned, so SPEC §8.1's hook keys must still
// find them -- issue #8); tombstoned rows do not (a row awaiting the reaper
// is deliberately no longer a hook target).
func TestListSessionsIncludingArchivedKeepsArchivedAndDropsTombstoned(t *testing.T) {
	st := openTombstoneTestStore(t)
	ctx := context.Background()
	plain := createTombstoneTestSession(t, st, ctx, "plain")
	archived := createTombstoneTestSession(t, st, ctx, "archived")
	tombstoned := createTombstoneTestSession(t, st, ctx, "tombstoned")
	both := createTombstoneTestSession(t, st, ctx, "archived-then-deleted")
	if err := st.ArchiveSession(ctx, archived.ID, 200); err != nil {
		t.Fatal(err)
	}
	if err := st.SoftDeleteSession(ctx, tombstoned.ID, 210); err != nil {
		t.Fatal(err)
	}
	if err := st.ArchiveSession(ctx, both.ID, 220); err != nil {
		t.Fatal(err)
	}
	if err := st.SoftDeleteSession(ctx, both.ID, 230); err != nil {
		t.Fatal(err)
	}

	rows, err := st.ListSessionsIncludingArchived(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, row := range rows {
		got = append(got, row.ID)
	}
	// Same created_at on every fixture row, so the tie-break is id order.
	want := []string{archived.ID, plain.ID}
	if len(got) != len(want) {
		t.Fatalf("ListSessionsIncludingArchived = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ListSessionsIncludingArchived = %v, want %v (created_at, id order)", got, want)
		}
	}
	for _, row := range rows {
		if row.ID == archived.ID && row.ArchivedAt == 0 {
			t.Fatal("archived row returned with ArchivedAt cleared; the flag must be reported as-is")
		}
	}
}
