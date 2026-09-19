package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// TestCreateGroupRejectsReservedDefaultName proves SPEC §11/R128's reserved
// name: "default" is not a row (it is sessions.group_id IS NULL), so a real
// group may never claim that literal name, in any case.
func TestCreateGroupRejectsReservedDefaultName(t *testing.T) {
	home := t.TempDir()
	s, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	for _, name := range []string{"default", "Default", "DEFAULT", "  default  "} {
		if _, err := s.CreateGroup(ctx, name); err == nil || !strings.Contains(err.Error(), "reserved") {
			t.Fatalf("CreateGroup(%q) = %v; want a reserved-name error", name, err)
		}
	}
}

// TestCreateGroupRejectsCaseInsensitiveDuplicate proves R128's
// case-insensitive uniqueness (groups.name UNIQUE COLLATE NOCASE): a second
// group whose name differs only by case from an existing one is refused.
func TestCreateGroupRejectsCaseInsensitiveDuplicate(t *testing.T) {
	home := t.TempDir()
	s, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	if _, err := s.CreateGroup(ctx, "Sprint Work"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateGroup(ctx, "sprint work"); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("CreateGroup of a case-insensitive duplicate = %v; want an 'already exists' error", err)
	}
}

// TestCreateGroupRejects33CharacterName proves the R128 length cap: one rune
// past GroupNameMaxLength (32) is refused.
func TestCreateGroupRejects33CharacterName(t *testing.T) {
	home := t.TempDir()
	s, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	name := strings.Repeat("a", 33)
	if _, err := s.CreateGroup(ctx, name); err == nil || !strings.Contains(err.Error(), "longer than") {
		t.Fatalf("CreateGroup(33-char name) = %v; want a length-cap error", err)
	}
}

// TestCreateGroupAccepts32CharacterName proves the cap is inclusive: exactly
// GroupNameMaxLength (32) characters is accepted, and stored verbatim.
func TestCreateGroupAccepts32CharacterName(t *testing.T) {
	home := t.TempDir()
	s, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	name := strings.Repeat("a", GroupNameMaxLength)
	g, err := s.CreateGroup(ctx, name)
	if err != nil {
		t.Fatalf("CreateGroup(32-char name) = %v; want success", err)
	}
	if g.Name != name || g.ID == 0 {
		t.Fatalf("CreateGroup(32-char name) = %+v; want Name=%q and a nonzero ID", g, name)
	}
}

// TestRenameGroupUpdatesOneRowAndCarriesItsTwoMembers proves R128's whole
// point of keying membership by id rather than name: renaming a group is
// exactly one groups-row UPDATE, and both of its member sessions resolve to
// the new name on their very next read -- through the very same
// sessionsFromClause LEFT JOIN task 008 added -- with no member row ever
// touched.
func TestRenameGroupUpdatesOneRowAndCarriesItsTwoMembers(t *testing.T) {
	home := t.TempDir()
	s, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	g, err := s.CreateGroup(ctx, "sprint work")
	if err != nil {
		t.Fatal(err)
	}
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
	// CreateSession does not yet expose a GroupID field (that is a later
	// task's create-modal work, R130) -- membership is set directly on the
	// row here, exactly as it will ultimately be set, to isolate this
	// test's own concern: that RenameGroup carries members by id.
	if _, err := s.db.ExecContext(ctx, `UPDATE sessions SET group_id = ? WHERE id IN (?, ?)`, g.ID, a.ID, b.ID); err != nil {
		t.Fatal(err)
	}

	if err := s.RenameGroup(ctx, g.ID, "sprint delivery"); err != nil {
		t.Fatal(err)
	}

	groups, err := s.ListGroups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || groups[0].ID != g.ID || groups[0].Name != "sprint delivery" {
		t.Fatalf("ListGroups() = %+v; want one row %d=%q", groups, g.ID, "sprint delivery")
	}

	sessions, err := s.ListSessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, sess := range sessions {
		got[sess.ID] = sess.GroupName
	}
	if got[a.ID] != "sprint delivery" || got[b.ID] != "sprint delivery" {
		t.Fatalf("member GroupName after rename = %+v; want both = %q", got, "sprint delivery")
	}
}

// TestGroupNameRejectsControlCharactersAnywhere proves the control-character
// rule is positional-blind: a control character is refused whether it sits
// inside the name or leads/trails it. Trimming must never quietly repair
// "\nalpha" into "alpha" -- a group name renders verbatim in the sidebar
// header and the settings editor, so a name carrying a control character is
// rejected outright rather than stored as a different name than the operator
// typed. Both the create and the rename path share validateGroupName, so
// both are checked here.
func TestGroupNameRejectsControlCharactersAnywhere(t *testing.T) {
	home := t.TempDir()
	s, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	existing, err := s.CreateGroup(ctx, "sprint work")
	if err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{
		"\nalpha",      // leading newline (TrimSpace would have eaten it)
		"\tbravo\t",    // leading and trailing tab
		"charlie\n",    // trailing newline
		"del\x7fta",    // interior DEL
		"ech\x00o",     // interior NUL
		" \recho\r\n ", // carriage returns amid real spaces
		"\u0085next",   // NEL, a non-ASCII control rune
	} {
		if _, err := s.CreateGroup(ctx, name); err == nil || !strings.Contains(err.Error(), "control character") {
			t.Fatalf("CreateGroup(%q) = %v; want a control-character error", name, err)
		}
		if err := s.RenameGroup(ctx, existing.ID, name); err == nil || !strings.Contains(err.Error(), "control character") {
			t.Fatalf("RenameGroup(%q) = %v; want a control-character error", name, err)
		}
	}

	// Nothing above was written, and the rename never landed.
	groups, err := s.ListGroups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || groups[0].Name != "sprint work" {
		t.Fatalf("ListGroups() = %+v; want the single untouched row %q", groups, "sprint work")
	}

	// Ordinary surrounding spaces are still trimmed, not rejected.
	g, err := s.CreateGroup(ctx, "  tooling maintenance  ")
	if err != nil {
		t.Fatalf("CreateGroup with surrounding spaces = %v; want success", err)
	}
	if g.Name != "tooling maintenance" {
		t.Fatalf("CreateGroup trimmed name = %q; want %q", g.Name, "tooling maintenance")
	}
}

// TestDeleteGroupThenCreateNewGroupNeverReusesTheDeletedID is R128's own
// durable-identity guarantee (schemaV7's groups.id is now
// INTEGER PRIMARY KEY AUTOINCREMENT, not plain INTEGER PRIMARY KEY): a
// group's numeric identity is never handed to a later, unrelated group,
// even when the deleted group was the table's only row (the exact shape
// where SQLite's ordinary rowid reuse -- max(rowid)+1 among rows CURRENTLY
// present -- would otherwise reissue id 1 to the next INSERT). It then
// proves the read-side consequence that makes the id matter at all: a
// session whose group_id still names the deleted id (an orphan the delete
// flow never touched, or one restored later by an undo) keeps reading back
// under default, via the ordinary LEFT JOIN, even after a brand-new group
// exists -- it must never silently resolve to that new group's name just
// because a naive id assignment happened to collide.
func TestDeleteGroupThenCreateNewGroupNeverReusesTheDeletedID(t *testing.T) {
	home := t.TempDir()
	s, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	deleted, err := s.CreateGroup(ctx, "tooling")
	if err != nil {
		t.Fatal(err)
	}

	orphan, err := s.CreateSession(ctx, CreateSessionInput{
		ID: "00000000-0000-4000-8000-0000000000c3", Name: "orphan", CWD: "/x",
		Agent: "shell", CapturedPath: "/bin", StatusAt: 100, CreatedAt: 100,
		GroupID: &deleted.ID,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := s.DeleteGroup(ctx, deleted.ID); err != nil {
		t.Fatal(err)
	}

	replacement, err := s.CreateGroup(ctx, "sprint work")
	if err != nil {
		t.Fatal(err)
	}
	if replacement.ID == deleted.ID {
		t.Fatalf("CreateGroup after delete reused the deleted id: got %d, want anything but %d", replacement.ID, deleted.ID)
	}

	sessions, err := s.ListSessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var got *Session
	for i := range sessions {
		if sessions[i].ID == orphan.ID {
			got = &sessions[i]
		}
	}
	if got == nil {
		t.Fatalf("ListSessions() lost the orphan session %s entirely", orphan.ID)
	}
	if got.GroupName != "" {
		t.Fatalf("orphan session GroupName = %q after a same-slot group was created, want %q (default) -- it must never rebind onto the new group %q", got.GroupName, "", replacement.Name)
	}
	if got.GroupID == nil || *got.GroupID != deleted.ID {
		t.Fatalf("orphan session GroupID = %v, want it to keep pointing at the retired id %d untouched", got.GroupID, deleted.ID)
	}

	groups, err := s.ListGroups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || groups[0].ID != replacement.ID || groups[0].Name != "sprint work" {
		t.Fatalf("ListGroups() = %+v; want exactly the replacement row %d=%q", groups, replacement.ID, "sprint work")
	}
}
