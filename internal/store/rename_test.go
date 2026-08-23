package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// TestRenameSessionChangesNameOnlyEnforcesUniquenessAndRecordsEvent proves
// task 013's storage half (SPEC §11.4, PRD requirement 31, I-8):
// RenameSession changes ONLY the `name` column -- `slug` (deck's tmux
// identity, deck_<slug>) is left exactly as CreateSession set it -- and
// re-validates uniqueness against both columns exactly like CreateSession
// does, refusing a new name that already exists or whose slug collides
// with another session's own (immutable) slug.
func TestRenameSessionChangesNameOnlyEnforcesUniquenessAndRecordsEvent(t *testing.T) {
	home := t.TempDir()
	s, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	a, err := s.CreateSession(ctx, CreateSessionInput{
		ID: "00000000-0000-4000-8000-0000000000a1", Name: "alpha", CWD: "/x",
		Agent: "shell", CapturedPath: "/bin", StatusAt: 100, CreatedAt: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.CreateSession(ctx, CreateSessionInput{
		ID: "00000000-0000-4000-8000-0000000000b2", Name: "beta", CWD: "/x",
		Agent: "shell", CapturedPath: "/bin", StatusAt: 101, CreatedAt: 101,
	})
	if err != nil {
		t.Fatal(err)
	}

	// A rename to a name already in use by ANOTHER session is refused,
	// and touches neither row.
	if err := s.RenameSession(ctx, a.ID, "beta", "user", 200); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("rename to an existing name = %v; want an 'already exists' error", err)
	}
	// A rename whose derived slug collides with another session's own
	// (immutable) slug is refused too, even though the literal name
	// differs.
	if err := s.RenameSession(ctx, a.ID, "Beta", "user", 201); err == nil || !strings.Contains(err.Error(), "collides") {
		t.Fatalf("rename whose slug collides with an existing slug = %v; want a 'collides' error", err)
	}
	reloadedA, err := s.GetSession(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloadedA.Name != "alpha" || reloadedA.Slug != a.Slug {
		t.Fatalf("a rejected rename mutated the row: name=%q slug=%q, want unchanged alpha/%q", reloadedA.Name, reloadedA.Slug, a.Slug)
	}

	// A legitimate rename changes name and leaves slug -- the tmux
	// identity -- completely untouched.
	if err := s.RenameSession(ctx, a.ID, "Alpha Renamed", "user", 202); err != nil {
		t.Fatal(err)
	}
	renamed, err := s.GetSession(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Name != "Alpha Renamed" {
		t.Fatalf("name = %q, want %q", renamed.Name, "Alpha Renamed")
	}
	if renamed.Slug != a.Slug {
		t.Fatalf("slug changed from %q to %q; RenameSession must never touch slug (the tmux session identity)", a.Slug, renamed.Slug)
	}

	// The rename is recorded as an event naming the new value, matching
	// SetPermissionProfile/SetConversationID's own convention.
	var kind, payload string
	if err := s.DB().QueryRowContext(ctx, `SELECT kind, payload FROM events WHERE session_id = ? AND kind = 'renamed' ORDER BY seq DESC LIMIT 1`, a.ID).Scan(&kind, &payload); err != nil {
		t.Fatalf("read rename event: %v", err)
	}
	if payload != "Alpha Renamed" {
		t.Fatalf("rename event payload = %q, want %q", payload, "Alpha Renamed")
	}

	// b (never renamed) is completely unaffected.
	reloadedB, err := s.GetSession(ctx, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloadedB.Name != "beta" || reloadedB.Slug != b.Slug {
		t.Fatalf("renaming a changed b: name=%q slug=%q", reloadedB.Name, reloadedB.Slug)
	}
}

// TestRenameSessionRejectsEmptyInputs proves the guard clauses: an empty
// session id or an empty new name is refused before any query runs.
func TestRenameSessionRejectsEmptyInputs(t *testing.T) {
	home := t.TempDir()
	s, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if err := s.RenameSession(ctx, "", "name", "user", 100); err == nil {
		t.Fatal("empty session id was accepted")
	}
	if err := s.RenameSession(ctx, "some-id", "", "user", 100); err == nil {
		t.Fatal("empty new name was accepted")
	}
}
