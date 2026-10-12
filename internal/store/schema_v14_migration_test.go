package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"reflect"
	"testing"
)

// buildSchemaV13Fixture advances the v12 adoption fixture one rung: migrated
// by schemaV13 and stamped schema version 13, rows kept.
func buildSchemaV13Fixture(t *testing.T, home, path string) [4]string {
	t.Helper()
	ids := buildSchemaV12AdoptionFixture(t, home, path)
	fixture, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer fixture.Close()
	for _, statement := range schemaV13 {
		if _, err := fixture.Exec(statement); err != nil {
			t.Fatalf("build schemaV13 fixture: %v", err)
		}
	}
	if _, err := fixture.Exec(`UPDATE meta SET version = 13 WHERE key = 'schema_version'`); err != nil {
		t.Fatal(err)
	}
	return ids
}

// TestBookmarksMigrationUpgradesAV13Database: a schema 13 database holding
// four sessions gains an empty bookmarks table, keeps every session, and is
// stamped with the current version. The fixture has no bookmarks table at
// all, so the test fails against the v13 schema.
func TestBookmarksMigrationUpgradesAV13Database(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "state.db")
	buildSchemaV13Fixture(t, home, path)
	pre, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pre.Exec(`SELECT slot FROM bookmarks`); err == nil {
		t.Fatal("the v13 fixture already has a bookmarks table")
	}
	_ = pre.Close()

	st, err := OpenPath(home, path)
	if err != nil {
		t.Fatalf("open v13 fixture for migration: %v", err)
	}
	defer st.Close()
	if SchemaVersion != 14 {
		t.Fatalf("SchemaVersion = %d, want 14", SchemaVersion)
	}
	var version, sessions, marks int
	if err := st.DB().QueryRow(`SELECT version FROM meta WHERE key = 'schema_version'`).Scan(&version); err != nil || version != SchemaVersion {
		t.Fatalf("migrated version = %d, %v; want %d", version, err, SchemaVersion)
	}
	if err := st.DB().QueryRow(`SELECT count(*) FROM sessions`).Scan(&sessions); err != nil || sessions != 4 {
		t.Fatalf("sessions after migration = %d, %v; want 4", sessions, err)
	}
	if err := st.DB().QueryRow(`SELECT count(*) FROM bookmarks`).Scan(&marks); err != nil || marks != 0 {
		t.Fatalf("bookmarks after migration = %d, %v; want an empty table", marks, err)
	}
	if err := st.AssignBookmark(context.Background(), 3, mustFirstSessionID(t, st)); err != nil {
		t.Fatalf("assign a bookmark on the migrated database: %v", err)
	}
}

func mustFirstSessionID(t *testing.T, st *Store) string {
	t.Helper()
	sessions, err := st.ListSessions(context.Background())
	if err != nil || len(sessions) == 0 {
		t.Fatalf("list sessions: %d, %v", len(sessions), err)
	}
	return sessions[0].ID
}

func openBookmarkStore(t *testing.T) (*Store, string, string) {
	t.Helper()
	home := t.TempDir()
	path := filepath.Join(home, "state.db")
	st, err := OpenPath(home, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st, home, path
}

func createBookmarkSession(t *testing.T, st *Store, id, name string) {
	t.Helper()
	if _, err := st.CreateSession(context.Background(), CreateSessionInput{
		ID: id, Name: name, CWD: "/x", Agent: "shell", CapturedPath: "/bin", StatusAt: 100, CreatedAt: 100,
	}); err != nil {
		t.Fatal(err)
	}
}

const (
	bookmarkSessionA = "00000000-0000-4000-8000-00000000b001"
	bookmarkSessionB = "00000000-0000-4000-8000-00000000b002"
)

// TestAssignBookmarkMovesATakenSlot: assigning slot 4 to B after A held it
// leaves one row for slot 4, pointing at B, and A without that slot. A holds
// several slots at once.
func TestAssignBookmarkMovesATakenSlot(t *testing.T) {
	ctx := context.Background()
	st, _, _ := openBookmarkStore(t)
	createBookmarkSession(t, st, bookmarkSessionA, "alpha")
	createBookmarkSession(t, st, bookmarkSessionB, "beta")
	for _, slot := range []int{1, 4, 9} {
		if err := st.AssignBookmark(ctx, slot, bookmarkSessionA); err != nil {
			t.Fatal(err)
		}
	}
	if got, _ := st.BookmarkSlotsOf(ctx, bookmarkSessionA); !reflect.DeepEqual(got, []int{1, 4, 9}) {
		t.Fatalf("A slots = %v, want [1 4 9]: a session may hold several slots", got)
	}
	if err := st.AssignBookmark(ctx, 4, bookmarkSessionB); err != nil {
		t.Fatal(err)
	}
	var rows int
	if err := st.DB().QueryRow(`SELECT count(*) FROM bookmarks WHERE slot = 4`).Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("rows for slot 4 = %d, %v; want 1 (moved, not duplicated)", rows, err)
	}
	all, err := st.Bookmarks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := map[int]string{1: bookmarkSessionA, 4: bookmarkSessionB, 9: bookmarkSessionA}
	if !reflect.DeepEqual(all, want) {
		t.Fatalf("bookmarks = %v, want %v", all, want)
	}
	if got, _ := st.BookmarkSlotsOf(ctx, bookmarkSessionA); !reflect.DeepEqual(got, []int{1, 9}) {
		t.Fatalf("A slots after the move = %v, want [1 9]", got)
	}
}

// TestBookmarkSlotOutsideZeroToNineIsRefused: slots 0 and 9 are accepted, -1
// and 10 are not, by the API and by the table itself.
func TestBookmarkSlotOutsideZeroToNineIsRefused(t *testing.T) {
	ctx := context.Background()
	st, _, _ := openBookmarkStore(t)
	createBookmarkSession(t, st, bookmarkSessionA, "alpha")
	for _, slot := range []int{0, 9} {
		if err := st.AssignBookmark(ctx, slot, bookmarkSessionA); err != nil {
			t.Fatalf("slot %d refused: %v", slot, err)
		}
	}
	for _, slot := range []int{-1, 10} {
		if err := st.AssignBookmark(ctx, slot, bookmarkSessionA); err == nil {
			t.Fatalf("slot %d accepted", slot)
		}
		if _, err := st.DB().Exec(`INSERT INTO bookmarks (slot, session_id) VALUES (?, ?)`, slot, bookmarkSessionA); err == nil {
			t.Fatalf("table accepted slot %d", slot)
		}
	}
	if err := st.AssignBookmark(ctx, 2, "no-such-session"); err == nil {
		t.Fatal("a bookmark on a missing session was accepted")
	}
}

// TestDeletingASessionRemovesItsSlots: a tombstone (dd) and a reap (the grace
// window ending) each leave the session holding no slot, and a slot held by
// another session is untouched.
func TestDeletingASessionRemovesItsSlots(t *testing.T) {
	ctx := context.Background()
	st, _, _ := openBookmarkStore(t)
	createBookmarkSession(t, st, bookmarkSessionA, "alpha")
	createBookmarkSession(t, st, bookmarkSessionB, "beta")
	for _, slot := range []int{1, 2} {
		if err := st.AssignBookmark(ctx, slot, bookmarkSessionA); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.AssignBookmark(ctx, 3, bookmarkSessionB); err != nil {
		t.Fatal(err)
	}
	if err := st.SoftDeleteSession(ctx, bookmarkSessionA, 200); err != nil {
		t.Fatal(err)
	}
	all, _ := st.Bookmarks(ctx)
	if !reflect.DeepEqual(all, map[int]string{3: bookmarkSessionB}) {
		t.Fatalf("bookmarks after a tombstone = %v, want only slot 3 -> B", all)
	}
	if err := st.ReapSession(ctx, bookmarkSessionA, 300); err != nil {
		t.Fatal(err)
	}
	if err := st.SoftDeleteSession(ctx, bookmarkSessionB, 400); err != nil {
		t.Fatal(err)
	}
	if err := st.ReapSession(ctx, bookmarkSessionB, 500); err != nil {
		t.Fatal(err)
	}
	var rows int
	if err := st.DB().QueryRow(`SELECT count(*) FROM bookmarks`).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("bookmarks after both reaps = %d, %v; want 0", rows, err)
	}
	// The cascade alone, without a prior tombstone: a raw delete of the row.
	createBookmarkSession(t, st, bookmarkSessionA, "alpha")
	if err := st.AssignBookmark(ctx, 5, bookmarkSessionA); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB().Exec(`DELETE FROM sessions WHERE id = ?`, bookmarkSessionA); err != nil {
		t.Fatal(err)
	}
	if err := st.DB().QueryRow(`SELECT count(*) FROM bookmarks`).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("bookmarks after the row was deleted = %d, %v; want 0 (cascade)", rows, err)
	}
}

// TestBookmarksSurviveReopeningTheStore: slots written, the store closed and
// opened again from the same file, the same slots read back.
func TestBookmarksSurviveReopeningTheStore(t *testing.T) {
	ctx := context.Background()
	st, home, path := openBookmarkStore(t)
	createBookmarkSession(t, st, bookmarkSessionA, "alpha")
	createBookmarkSession(t, st, bookmarkSessionB, "beta")
	for slot, id := range map[int]string{0: bookmarkSessionA, 7: bookmarkSessionB, 8: bookmarkSessionA} {
		if err := st.AssignBookmark(ctx, slot, id); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenPath(home, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err := reopened.Bookmarks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := map[int]string{0: bookmarkSessionA, 7: bookmarkSessionB, 8: bookmarkSessionA}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("bookmarks after reopening = %v, want %v", got, want)
	}
}

// TestClearBookmarkEmptiesOneSlot: clearing frees a slot and clearing an empty
// slot is not an error.
func TestClearBookmarkEmptiesOneSlot(t *testing.T) {
	ctx := context.Background()
	st, _, _ := openBookmarkStore(t)
	createBookmarkSession(t, st, bookmarkSessionA, "alpha")
	if err := st.AssignBookmark(ctx, 1, bookmarkSessionA); err != nil {
		t.Fatal(err)
	}
	if err := st.ClearBookmark(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if err := st.ClearBookmark(ctx, 1); err != nil {
		t.Fatalf("clearing an empty slot: %v", err)
	}
	if err := st.ClearBookmark(ctx, 10); err == nil {
		t.Fatal("slot 10 accepted")
	}
	if all, _ := st.Bookmarks(ctx); len(all) != 0 {
		t.Fatalf("bookmarks = %v, want none", all)
	}
}
