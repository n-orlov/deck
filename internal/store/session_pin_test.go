package store

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
)

// TestSetSessionsPinnedPinsAndUnpins proves task 003's storage half (SPEC
// §11.3's sidebar pin `p`, phase 4f): SetSessionsPinned writes the store
// clock (`at`) into pinned_at for every id passed with pinned=true, leaves
// an untouched session's own pinned_at at its schemaV8 default of 0, then
// unpinning the same ids writes 0 back -- and none of it ever records an
// events row, pin/unpin being UI-only sidebar state rather than an
// auditable session lifecycle transition.
func TestSetSessionsPinnedPinsAndUnpins(t *testing.T) {
	home := t.TempDir()
	s, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	mk := func(id, name string) {
		if _, err := s.CreateSession(ctx, CreateSessionInput{
			ID: id, Name: name, CWD: "/x", Agent: "shell", CapturedPath: "/bin",
			StatusAt: 100, CreatedAt: 100,
		}); err != nil {
			t.Fatal(err)
		}
	}
	mk("00000000-0000-4000-8000-0000000000a1", "alpha")
	mk("00000000-0000-4000-8000-0000000000b2", "beta")
	mk("00000000-0000-4000-8000-0000000000c3", "gamma")

	ids := []string{
		"00000000-0000-4000-8000-0000000000a1",
		"00000000-0000-4000-8000-0000000000b2",
	}
	const untouched = "00000000-0000-4000-8000-0000000000c3"

	var eventsBefore int
	if err := s.DB().QueryRow(`SELECT count(*) FROM events`).Scan(&eventsBefore); err != nil {
		t.Fatal(err)
	}

	if err := s.SetSessionsPinned(ctx, ids, true, 555); err != nil {
		t.Fatalf("pin: %v", err)
	}
	for _, id := range ids {
		got, err := s.GetSession(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if got.PinnedAt != 555 {
			t.Fatalf("session %q pinned_at = %d; want 555", id, got.PinnedAt)
		}
	}
	gotUntouched, err := s.GetSession(ctx, untouched)
	if err != nil {
		t.Fatal(err)
	}
	if gotUntouched.PinnedAt != 0 {
		t.Fatalf("untouched session pinned_at = %d; want 0", gotUntouched.PinnedAt)
	}

	if err := s.SetSessionsPinned(ctx, ids, false, 999); err != nil {
		t.Fatalf("unpin: %v", err)
	}
	for _, id := range ids {
		got, err := s.GetSession(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if got.PinnedAt != 0 {
			t.Fatalf("session %q pinned_at after unpin = %d; want 0", id, got.PinnedAt)
		}
	}

	var eventsAfter int
	if err := s.DB().QueryRow(`SELECT count(*) FROM events`).Scan(&eventsAfter); err != nil {
		t.Fatal(err)
	}
	if eventsAfter != eventsBefore {
		t.Fatalf("events row count changed from %d to %d; SetSessionsPinned must record no event", eventsBefore, eventsAfter)
	}
}

// TestSetSessionsPinnedIsAtomicAcrossTheBatch proves the transaction half of
// task 003: when one id's own UPDATE is forced to fail (a trigger raising
// ABORT the moment pinned_at would actually change on a specific row),
// SetSessionsPinned leaves EVERY row in the batch -- including the ones
// whose own UPDATE would otherwise have succeeded, whether attempted before
// or after the forced failure -- exactly as it found them, and returns an
// error rather than a partial success.
func TestSetSessionsPinnedIsAtomicAcrossTheBatch(t *testing.T) {
	home := t.TempDir()
	s, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	mk := func(id, name string) {
		if _, err := s.CreateSession(ctx, CreateSessionInput{
			ID: id, Name: name, CWD: "/x", Agent: "shell", CapturedPath: "/bin",
			StatusAt: 100, CreatedAt: 100,
		}); err != nil {
			t.Fatal(err)
		}
	}
	const before = "00000000-0000-4000-8000-0000000000d4"
	const poison = "00000000-0000-4000-8000-0000000000e5"
	const after = "00000000-0000-4000-8000-0000000000f6"
	mk(before, "delta")
	mk(poison, "echo")
	mk(after, "foxtrot")

	if _, err := s.DB().Exec(fmt.Sprintf(`CREATE TRIGGER reject_pin_update BEFORE UPDATE ON sessions
		WHEN NEW.id = %q AND NEW.pinned_at != OLD.pinned_at
		BEGIN SELECT RAISE(ABORT, 'reject pin update'); END`, poison)); err != nil {
		t.Fatal(err)
	}

	if err := s.SetSessionsPinned(ctx, []string{before, poison, after}, true, 777); err == nil {
		t.Fatal("SetSessionsPinned unexpectedly succeeded with a forced failure in the batch")
	}

	for _, id := range []string{before, poison, after} {
		got, err := s.GetSession(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if got.PinnedAt != 0 {
			t.Fatalf("session %q pinned_at = %d after a rolled-back batch; want 0 (unchanged)", id, got.PinnedAt)
		}
	}
}
