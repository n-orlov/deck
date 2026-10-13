package store

import (
	"context"
	"reflect"
	"testing"
)

func assignSlots(ctx context.Context, t *testing.T, st *Store, sessionID string, slots ...int) {
	t.Helper()
	for _, slot := range slots {
		if err := st.AssignBookmark(ctx, slot, sessionID); err != nil {
			t.Fatalf("assign slot %d to %q: %v", slot, sessionID, err)
		}
	}
}

// TestArchiveClearsBookmarkSlots pins R285: archiving a session empties every
// slot it holds, and another session's slot is left alone.
func TestArchiveClearsBookmarkSlots(t *testing.T) {
	st := openTombstoneTestStore(t)
	ctx := context.Background()
	doomed := createTombstoneTestSession(ctx, t, st, "archived-away")
	kept := createTombstoneTestSession(ctx, t, st, "kept")
	assignSlots(ctx, t, st, doomed.ID, 2, 7)
	assignSlots(ctx, t, st, kept.ID, 4)

	if err := st.ArchiveSession(ctx, doomed.ID, 200); err != nil {
		t.Fatalf("archive: %v", err)
	}
	got, err := st.Bookmarks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if want := map[int]string{4: kept.ID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Bookmarks after archive = %v, want %v", got, want)
	}
}

// TestArchiveFailureKeepsBookmarkSlots pins that the clear rides the archive's
// own transaction: a failed archive (unknown id) leaves seeded slots in place.
func TestArchiveFailureKeepsBookmarkSlots(t *testing.T) {
	st := openTombstoneTestStore(t)
	ctx := context.Background()
	live := createTombstoneTestSession(ctx, t, st, "live")
	assignSlots(ctx, t, st, live.ID, 1, 5)

	if err := st.ArchiveSession(ctx, "does-not-exist", 200); err == nil {
		t.Fatal("archiving an unknown session must fail")
	}
	got, err := st.Bookmarks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if want := map[int]string{1: live.ID, 5: live.ID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Bookmarks after failed archive = %v, want %v", got, want)
	}
}

// TestUnarchiveLeavesBookmarkSlotsEmpty pins that un-archiving does not bring
// the cleared slots back.
func TestUnarchiveLeavesBookmarkSlotsEmpty(t *testing.T) {
	st := openTombstoneTestStore(t)
	ctx := context.Background()
	s := createTombstoneTestSession(ctx, t, st, "round-trip")
	assignSlots(ctx, t, st, s.ID, 3, 9)
	if err := st.ArchiveSession(ctx, s.ID, 200); err != nil {
		t.Fatal(err)
	}
	if err := st.UnarchiveSession(ctx, s.ID, 300); err != nil {
		t.Fatal(err)
	}
	slots, err := st.BookmarkSlotsOf(ctx, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(slots) != 0 {
		t.Fatalf("slots after archive+unarchive = %v, want none", slots)
	}
}

// TestClearBookmarkEmptySlotReturnsNil is a regression pin: clearing an empty
// slot is not an error.
func TestClearBookmarkEmptySlotReturnsNil(t *testing.T) {
	st := openTombstoneTestStore(t)
	if err := st.ClearBookmark(context.Background(), 6); err != nil {
		t.Fatalf("ClearBookmark on an empty slot = %v, want nil", err)
	}
}
