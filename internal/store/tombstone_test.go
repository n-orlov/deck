package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

// openTombstoneTestStore mirrors openRecentCwdTestStore's pattern for the
// delete-tombstone primitives (task 104): a fresh private store per test,
// closed on cleanup.
func openTombstoneTestStore(t *testing.T) *Store {
	t.Helper()
	home := filepath.Join(t.TempDir(), "deck")
	st, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func createTombstoneTestSession(t *testing.T, st *Store, ctx context.Context, id string) Session {
	t.Helper()
	session, err := st.CreateSession(ctx, CreateSessionInput{
		ID: id, Name: id, CWD: "/work/" + id,
		Agent: "shell", CapturedPath: "/bin", StatusAt: 100, CreatedAt: 100,
	})
	if err != nil {
		t.Fatalf("create session %q: %v", id, err)
	}
	return session
}

// TestSoftDeleteSessionTombstonesAndHidesFromListSessions covers task 104's
// central contract: SoftDeleteSession sets deleted_at (a durable, queryable
// column, never a comment) and records a "deleted" event, and the row
// disappears from ListSessions' default view even though it still exists
// (GetSession still finds it by id, which the grace-window undo, task 106,
// and the reap it eventually loses to, task 107, both depend on).
func TestSoftDeleteSessionTombstonesAndHidesFromListSessions(t *testing.T) {
	st := openTombstoneTestStore(t)
	ctx := context.Background()
	kept := createTombstoneTestSession(t, st, ctx, "kept")
	doomed := createTombstoneTestSession(t, st, ctx, "doomed")

	if err := st.SoftDeleteSession(ctx, doomed.ID, 200); err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	visible, err := st.ListSessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(visible) != 1 || visible[0].ID != kept.ID {
		t.Fatalf("ListSessions after soft delete = %+v, want only %q", visible, kept.ID)
	}

	got, err := st.GetSession(ctx, doomed.ID)
	if err != nil {
		t.Fatalf("GetSession must still find a tombstoned row by id: %v", err)
	}
	if got.DeletedAt != 200 {
		t.Fatalf("DeletedAt = %d, want 200", got.DeletedAt)
	}

	var kind, reason string
	if err := st.DB().QueryRowContext(ctx, `SELECT kind, reason FROM events WHERE session_id = ? AND kind = 'deleted'`, doomed.ID).Scan(&kind, &reason); err != nil {
		t.Fatalf("expected a durable 'deleted' event: %v", err)
	}

	deleted, err := st.ListDeletedSessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 1 || deleted[0].ID != doomed.ID {
		t.Fatalf("ListDeletedSessions = %+v, want only %q", deleted, doomed.ID)
	}
}

// TestRestoreSessionClearsTombstoneAndReturnsToListSessions covers task
// 106's undo half of the store contract: RestoreSession clears deleted_at
// and records a "restored" event, and the row is back in ListSessions'
// default view exactly as before -- proving the tombstone is undoable
// within the grace window, not merely settable.
func TestRestoreSessionClearsTombstoneAndReturnsToListSessions(t *testing.T) {
	st := openTombstoneTestStore(t)
	ctx := context.Background()
	session := createTombstoneTestSession(t, st, ctx, "restorable")

	if err := st.SoftDeleteSession(ctx, session.ID, 200); err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	if visible, err := st.ListSessions(ctx); err != nil {
		t.Fatal(err)
	} else if len(visible) != 0 {
		t.Fatalf("ListSessions after soft delete = %+v, want none", visible)
	}

	if err := st.RestoreSession(ctx, session.ID, 300); err != nil {
		t.Fatalf("restore: %v", err)
	}

	visible, err := st.ListSessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(visible) != 1 || visible[0].ID != session.ID {
		t.Fatalf("ListSessions after restore = %+v, want only %q", visible, session.ID)
	}
	if visible[0].DeletedAt != 0 {
		t.Fatalf("DeletedAt after restore = %d, want 0", visible[0].DeletedAt)
	}

	deleted, err := st.ListDeletedSessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 0 {
		t.Fatalf("ListDeletedSessions after restore = %+v, want none", deleted)
	}

	var kind string
	if err := st.DB().QueryRowContext(ctx, `SELECT kind FROM events WHERE session_id = ? AND kind = 'restored'`, session.ID).Scan(&kind); err != nil {
		t.Fatalf("expected a durable 'restored' event: %v", err)
	}
}

// TestReapSessionRequiresTombstoneAndCascadesEvents proves the reap half of
// task 104's contract: ReapSession refuses a live (never soft-deleted) row
// outright -- a caller cannot skip the tombstone/undo window by mistake --
// and once a row IS tombstoned, reaping it permanently deletes the sessions
// row AND cascades its events rows away (ON DELETE CASCADE, schemaV1),
// while still leaving a durable "reaped" orphan event (session_id NULL)
// behind describing that it happened, since the row's own events cannot
// survive to record it themselves.
func TestReapSessionRequiresTombstoneAndCascadesEvents(t *testing.T) {
	st := openTombstoneTestStore(t)
	ctx := context.Background()
	session := createTombstoneTestSession(t, st, ctx, "reapable")

	if err := st.ReapSession(ctx, session.ID, 999); err == nil {
		t.Fatal("ReapSession on a live (never tombstoned) row must be refused, got nil error")
	}

	// A few more events, so the cascade assertion below is not fooled by
	// there being only one row to begin with. CreateSession itself records
	// no event, so both come from explicit mutations here.
	if err := st.SetSessionEnvValue(ctx, session.ID, "K", "v", "user", 150); err != nil {
		t.Fatalf("seed an extra event: %v", err)
	}
	if err := st.SetSessionEnvValue(ctx, session.ID, "K2", "v2", "user", 160); err != nil {
		t.Fatalf("seed a second extra event: %v", err)
	}
	var eventsBefore int
	if err := st.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE session_id = ?`, session.ID).Scan(&eventsBefore); err != nil {
		t.Fatal(err)
	}
	if eventsBefore < 2 {
		t.Fatalf("events for %q before reap = %d, want at least 2 to make the cascade assertion meaningful", session.ID, eventsBefore)
	}

	if err := st.SoftDeleteSession(ctx, session.ID, 200); err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	if err := st.ReapSession(ctx, session.ID, 400); err != nil {
		t.Fatalf("reap: %v", err)
	}

	if _, err := st.GetSession(ctx, session.ID); err == nil {
		t.Fatal("GetSession after reap must fail, the row is permanently gone")
	}

	var eventsAfter int
	if err := st.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE session_id = ?`, session.ID).Scan(&eventsAfter); err != nil {
		t.Fatal(err)
	}
	if eventsAfter != 0 {
		t.Fatalf("events for %q after reap = %d, want 0 (ON DELETE CASCADE)", session.ID, eventsAfter)
	}

	var reason string
	if err := st.DB().QueryRowContext(ctx, `SELECT reason FROM events WHERE session_id IS NULL AND kind = 'reaped' AND payload = ?`, session.ID).Scan(&reason); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("expected a surviving orphan 'reaped' event naming %q, found none", session.ID)
		}
		t.Fatal(err)
	}
}

// TestReapSessionRejectsMissingSession proves ReapSession's error path for
// an id that never existed at all, distinct from the live-row refusal case
// above.
func TestReapSessionRejectsMissingSession(t *testing.T) {
	st := openTombstoneTestStore(t)
	ctx := context.Background()
	if err := st.ReapSession(ctx, "does-not-exist", 100); err == nil {
		t.Fatal("ReapSession on an unknown id must fail, got nil error")
	}
}

// TestSoftDeleteAndRestoreRejectMissingSession covers the not-found path
// shared with every other mutateSessionWithEvent-backed method.
func TestSoftDeleteAndRestoreRejectMissingSession(t *testing.T) {
	st := openTombstoneTestStore(t)
	ctx := context.Background()
	if err := st.SoftDeleteSession(ctx, "does-not-exist", 100); err == nil {
		t.Fatal("SoftDeleteSession on an unknown id must fail, got nil error")
	}
	if err := st.RestoreSession(ctx, "does-not-exist", 100); err == nil {
		t.Fatal("RestoreSession on an unknown id must fail, got nil error")
	}
}
