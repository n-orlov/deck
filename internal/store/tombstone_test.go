package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"
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

// TestCreateSessionReapsTombstonedNameHolder covers R77 (SPEC.md §9.2): a
// name held only by a tombstoned row is available again, and CreateSession
// reaps that row -- inside its own transaction, against the SAME *sql.DB
// this test's *Store already opened, never a second connection -- rather
// than reporting a UNIQUE constraint failure or hanging on SQLite's write
// lock. The old row and its events must be gone afterward.
func TestCreateSessionReapsTombstonedNameHolder(t *testing.T) {
	st := openTombstoneTestStore(t)
	ctx := context.Background()
	old := createTombstoneTestSession(t, st, ctx, "old-holder")
	if err := st.SoftDeleteSession(ctx, old.ID, 200); err != nil {
		t.Fatalf("soft delete %q: %v", old.ID, err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := st.CreateSession(ctx, CreateSessionInput{
			ID: "new-holder", Name: old.Name, CWD: "/work/new-holder",
			Agent: "shell", CapturedPath: "/bin", StatusAt: 300, CreatedAt: 300,
		})
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("create session reusing tombstoned name %q: %v", old.Name, err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("create session reusing tombstoned name %q hung (write-lock deadlock)", old.Name)
	}

	var count int
	if err := st.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions WHERE id = ?`, old.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("tombstoned row %q survived the reap-on-create, count = %d", old.ID, count)
	}
	if err := st.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE session_id = ?`, old.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("events for reaped row %q survived, count = %d", old.ID, count)
	}
	if err := st.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE session_id IS NULL AND kind = 'reaped' AND payload = ?`, old.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("orphan 'reaped' event for %q, count = %d, want 1", old.ID, count)
	}
}

// TestCreateSessionReapsTombstonedSlugHolder covers the §3.2 slug half of
// R77: two different Name strings that produce the same slug (Slug lower-
// cases and collapses whitespace to '-'). A tombstoned row holding only
// the slug -- not the exact name -- must still be reaped so the create
// proceeds.
func TestCreateSessionReapsTombstonedSlugHolder(t *testing.T) {
	st := openTombstoneTestStore(t)
	ctx := context.Background()
	old, err := st.CreateSession(ctx, CreateSessionInput{
		ID: "slug-old", Name: "Slug Holder", CWD: "/work/slug-old",
		Agent: "shell", CapturedPath: "/bin", StatusAt: 100, CreatedAt: 100,
	})
	if err != nil {
		t.Fatalf("create slug holder: %v", err)
	}
	if err := st.SoftDeleteSession(ctx, old.ID, 200); err != nil {
		t.Fatalf("soft delete %q: %v", old.ID, err)
	}
	if _, err := st.CreateSession(ctx, CreateSessionInput{
		ID: "slug-new", Name: "slug holder", CWD: "/work/slug-new",
		Agent: "shell", CapturedPath: "/bin", StatusAt: 300, CreatedAt: 300,
	}); err != nil {
		t.Fatalf("create session reusing tombstoned slug: %v", err)
	}
	var count int
	if err := st.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions WHERE id = ?`, old.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("tombstoned slug holder %q survived the reap-on-create, count = %d", old.ID, count)
	}
}

// TestCreateSessionRefusesLiveNameHolder proves a live (never soft-deleted)
// name holder still refuses, unchanged by the tombstone-reap addition.
func TestCreateSessionRefusesLiveNameHolder(t *testing.T) {
	st := openTombstoneTestStore(t)
	ctx := context.Background()
	live := createTombstoneTestSession(t, st, ctx, "live-holder")
	_, err := st.CreateSession(ctx, CreateSessionInput{
		ID: "live-holder-2", Name: live.Name, CWD: "/work/live-holder-2",
		Agent: "shell", CapturedPath: "/bin", StatusAt: 300, CreatedAt: 300,
	})
	if err == nil {
		t.Fatal("create session with a live name holder must be refused, got nil error")
	}
	var count int
	if err := st.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions WHERE id = ?`, live.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("live holder %q must survive a refused create, count = %d", live.ID, count)
	}
}

// TestCreateSessionRefusesArchivedNameHolder proves an archived holder
// (R78: an archived row keeps its name, only `dd` frees it) still refuses,
// distinct from the tombstoned case this task teaches CreateSession to
// reap.
func TestCreateSessionRefusesArchivedNameHolder(t *testing.T) {
	st := openTombstoneTestStore(t)
	ctx := context.Background()
	archived := createTombstoneTestSession(t, st, ctx, "archived-holder")
	if err := st.ArchiveSession(ctx, archived.ID, 200); err != nil {
		t.Fatalf("archive %q: %v", archived.ID, err)
	}
	_, err := st.CreateSession(ctx, CreateSessionInput{
		ID: "archived-holder-2", Name: archived.Name, CWD: "/work/archived-holder-2",
		Agent: "shell", CapturedPath: "/bin", StatusAt: 300, CreatedAt: 300,
	})
	if err == nil {
		t.Fatal("create session with an archived name holder must be refused, got nil error")
	}
	var count int
	if err := st.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions WHERE id = ?`, archived.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("archived holder %q must survive a refused create, count = %d", archived.ID, count)
	}
}

// TestRenameSessionReapsTombstonedNameHolder proves task 004: renaming a
// session onto a name whose only holder is tombstoned succeeds through the
// same reapTombstonedHolderTx helper CreateSession uses (R77, SPEC.md
// §9.2), reaping that holder inside RenameSession's own transaction rather
// than refusing the rename.
func TestRenameSessionReapsTombstonedNameHolder(t *testing.T) {
	st := openTombstoneTestStore(t)
	ctx := context.Background()
	subject := createTombstoneTestSession(t, st, ctx, "rename-subject")
	old := createTombstoneTestSession(t, st, ctx, "rename-old-holder")
	if err := st.SoftDeleteSession(ctx, old.ID, 200); err != nil {
		t.Fatalf("soft delete %q: %v", old.ID, err)
	}

	if err := st.RenameSession(ctx, subject.ID, old.Name, "user", 300); err != nil {
		t.Fatalf("rename onto tombstoned name %q: %v", old.Name, err)
	}

	renamed, err := st.GetSession(ctx, subject.ID)
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Name != old.Name {
		t.Fatalf("name = %q, want %q", renamed.Name, old.Name)
	}
	if renamed.Slug != subject.Slug {
		t.Fatalf("slug changed from %q to %q; a rename must never touch slug", subject.Slug, renamed.Slug)
	}
	var count int
	if err := st.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions WHERE id = ?`, old.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("tombstoned holder %q survived the reap-on-rename, count = %d", old.ID, count)
	}
}

// TestRenameSessionReapsTombstonedSlugHolder covers the §3.2 slug half:
// a tombstoned row holding only the slug -- not the exact name -- must
// still be reaped so the rename proceeds.
func TestRenameSessionReapsTombstonedSlugHolder(t *testing.T) {
	st := openTombstoneTestStore(t)
	ctx := context.Background()
	subject := createTombstoneTestSession(t, st, ctx, "rename-slug-subject")
	old, err := st.CreateSession(ctx, CreateSessionInput{
		ID: "rename-slug-old", Name: "Slug Holder", CWD: "/work/rename-slug-old",
		Agent: "shell", CapturedPath: "/bin", StatusAt: 100, CreatedAt: 100,
	})
	if err != nil {
		t.Fatalf("create slug holder: %v", err)
	}
	if err := st.SoftDeleteSession(ctx, old.ID, 200); err != nil {
		t.Fatalf("soft delete %q: %v", old.ID, err)
	}

	if err := st.RenameSession(ctx, subject.ID, "slug holder", "user", 300); err != nil {
		t.Fatalf("rename reusing tombstoned slug: %v", err)
	}
	var count int
	if err := st.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions WHERE id = ?`, old.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("tombstoned slug holder %q survived the reap-on-rename, count = %d", old.ID, count)
	}
}

// TestRenameSessionRefusesLiveAndArchivedHolders proves a live holder and
// an archived holder (R78: archiving keeps a name reserved) both still
// refuse the rename, unchanged by the tombstone-reap addition, and that a
// refused rename never mutates the subject row.
func TestRenameSessionRefusesLiveAndArchivedHolders(t *testing.T) {
	st := openTombstoneTestStore(t)
	ctx := context.Background()
	subject := createTombstoneTestSession(t, st, ctx, "rename-refuse-subject")
	live := createTombstoneTestSession(t, st, ctx, "rename-refuse-live")
	archived := createTombstoneTestSession(t, st, ctx, "rename-refuse-archived")
	if err := st.ArchiveSession(ctx, archived.ID, 200); err != nil {
		t.Fatalf("archive %q: %v", archived.ID, err)
	}

	if err := st.RenameSession(ctx, subject.ID, live.Name, "user", 300); err == nil {
		t.Fatal("rename onto a live holder's name must be refused, got nil error")
	}
	if err := st.RenameSession(ctx, subject.ID, archived.Name, "user", 301); err == nil {
		t.Fatal("rename onto an archived holder's name must be refused, got nil error")
	}

	reloaded, err := st.GetSession(ctx, subject.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Name != subject.Name {
		t.Fatalf("a refused rename mutated the subject's name to %q, want unchanged %q", reloaded.Name, subject.Name)
	}
	for _, holder := range []Session{live, archived} {
		var count int
		if err := st.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions WHERE id = ?`, holder.ID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("holder %q must survive a refused rename, count = %d", holder.ID, count)
		}
	}
}

// TestTombstonedNameHoldersAndSessionRowExists covers the two reads the
// SERVICE layer needs to finish a name-reuse reap on the filesystem after
// the create's own transaction has committed: which tombstoned rows hold a
// name (or its §3.2 slug), and whether one of them is really gone
// afterwards. Both are plain reads -- neither reaps anything itself, and a
// live or archived holder is deliberately NOT reported (its files stay).
func TestTombstonedNameHoldersAndSessionRowExists(t *testing.T) {
	st := openTombstoneTestStore(t)
	ctx := context.Background()
	live := createTombstoneTestSession(t, st, ctx, "still-live")
	tomb, err := st.CreateSession(ctx, CreateSessionInput{
		ID: "holder-tomb", Name: "Held Name", CWD: "/work/holder-tomb",
		Agent: "shell", CapturedPath: "/bin", StatusAt: 100, CreatedAt: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	archived := createTombstoneTestSession(t, st, ctx, "archived-name")
	if err := st.ArchiveSession(ctx, archived.ID, 150); err != nil {
		t.Fatal(err)
	}

	if ids, err := st.TombstonedNameHolders(ctx, live.Name); err != nil || len(ids) != 0 {
		t.Fatalf("TombstonedNameHolders(live) = %#v, %v, want none", ids, err)
	}
	if ids, err := st.TombstonedNameHolders(ctx, archived.Name); err != nil || len(ids) != 0 {
		t.Fatalf("TombstonedNameHolders(archived) = %#v, %v, want none", ids, err)
	}
	if ids, err := st.TombstonedNameHolders(ctx, tomb.Name); err != nil || len(ids) != 0 {
		t.Fatalf("TombstonedNameHolders(not yet deleted) = %#v, %v, want none", ids, err)
	}
	if err := st.SoftDeleteSession(ctx, tomb.ID, 200); err != nil {
		t.Fatal(err)
	}
	ids, err := st.TombstonedNameHolders(ctx, tomb.Name)
	if err != nil || len(ids) != 1 || ids[0] != tomb.ID {
		t.Fatalf("TombstonedNameHolders(tombstoned name) = %#v, %v, want [%q]", ids, err, tomb.ID)
	}
	// A different Name whose slug matches is reported too (the create would
	// reap that row via the slug pre-check).
	ids, err = st.TombstonedNameHolders(ctx, "held name")
	if err != nil || len(ids) != 1 || ids[0] != tomb.ID {
		t.Fatalf("TombstonedNameHolders(slug-equal name) = %#v, %v, want [%q]", ids, err, tomb.ID)
	}
	if _, err := st.TombstonedNameHolders(ctx, ""); err == nil {
		t.Fatal("TombstonedNameHolders(\"\") returned nil error, want a refusal")
	}

	exists, err := st.SessionRowExists(ctx, tomb.ID)
	if err != nil || !exists {
		t.Fatalf("SessionRowExists(tombstoned) = %v, %v, want true (the row is still there)", exists, err)
	}
	if _, err := st.CreateSession(ctx, CreateSessionInput{
		ID: "holder-new", Name: tomb.Name, CWD: "/work/holder-new",
		Agent: "shell", CapturedPath: "/bin", StatusAt: 300, CreatedAt: 300,
	}); err != nil {
		t.Fatalf("create reusing the tombstoned name: %v", err)
	}
	exists, err = st.SessionRowExists(ctx, tomb.ID)
	if err != nil || exists {
		t.Fatalf("SessionRowExists(reaped) = %v, %v, want false", exists, err)
	}
	if _, err := st.SessionRowExists(ctx, ""); err == nil {
		t.Fatal("SessionRowExists(\"\") returned nil error, want a refusal")
	}
}
