package tui

import (
	"context"
	"testing"

	"github.com/n-orlov/deck/internal/store"
)

// TestUndoArchiveLeavesBookmarksCleared pins R285 through the UI: archiving a
// bookmarked row empties its slots, and `u` brings the row back without them.
func TestUndoArchiveLeavesBookmarksCleared(t *testing.T) {
	m, db := bookmarkKeyModel(t, true)
	ctx := context.Background()
	for slot, id := range map[int]string{1: "s0", 6: "s0", 2: "s1"} {
		if err := db.AssignBookmark(ctx, slot, id); err != nil {
			t.Fatal(err)
		}
	}
	m.archiveSvc = func(ctx context.Context, s store.Session) error {
		return db.ArchiveSession(ctx, s.ID, 200)
	}
	m.kill = func(context.Context, store.Session) error { return nil }
	m.unarchiveSvc = func(ctx context.Context, id string) (store.Session, error) {
		if err := db.UnarchiveSession(ctx, id, 300); err != nil {
			return store.Session{}, err
		}
		return db.GetSession(ctx, id)
	}
	if got := selectedID(t, m); got != "s0" {
		t.Fatalf("selected = %q, want s0", got)
	}

	next, _ := m.Update(key("A"))
	m = next.(Model)
	next, cmd := m.Update(key("enter"))
	m = next.(Model)
	if cmd == nil {
		t.Fatal("confirming the archive issued no command")
	}
	next, _ = m.Update(cmd())
	m = next.(Model)
	if m.archiveUndoSessionID != "s0" {
		t.Fatalf("archiveUndoSessionID = %q, want s0", m.archiveUndoSessionID)
	}
	if slots, _ := db.BookmarkSlotsOf(ctx, "s0"); len(slots) != 0 {
		t.Fatalf("slots after archive = %v, want none", slots)
	}

	next, cmd = m.Update(key("u"))
	m = next.(Model)
	if cmd == nil {
		t.Fatal("u issued no unarchive command")
	}
	next, _ = m.Update(cmd())
	m = next.(Model)

	if archived, err := db.GetSession(ctx, "s0"); err != nil || archived.ArchivedAt != 0 {
		t.Fatalf("after u: err=%v archived_at=%d, want the row unarchived", err, archived.ArchivedAt)
	}
	got, err := db.Bookmarks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[2] != "s1" {
		t.Fatalf("Bookmarks after undo = %v, want only the other session's slot 2", got)
	}
}
