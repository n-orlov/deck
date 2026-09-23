package tui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// TestFoldStateSurvivesRestartAndRenameButNotTheCursor is task 015's own
// (D.4) internal/tui proof of the PRD's two header-cursor persistence
// rules together, over a REAL groups row rather than
// TestCollapsedGroupsSurviveModelRebuildFromUIState's arbitrary id 42:
//
//   - "Fold state persists" (ui_state.collapsed_groups, keyed by the
//     durable group id) -- and does so THROUGH a rename, because the key
//     is the id, never the display name a rename changes (group.go:206,
//     SPEC §11: "Keyed by durable groupID, never by name ... so a rename
//     preserves both the fold state and the cursor").
//   - "Do not persist the cursor" -- a header cursor sitting on the very
//     group this test folds and renames must NOT reappear after a
//     restart; the restarted client's cursor is the zero value, exactly
//     as if the group had never been visited.
//
// Sequence: create a real group, fold it by id, park the cursor on its
// header, persist the fold, rename the group (its id is untouched), then
// build a brand-new Model over the same state.db (a "restarted client",
// exactly like TestCollapsedGroupsSurviveModelRebuildFromUIState's own
// shape) and reconstruct its session list under the NEW name. The
// restarted client must see the group still folded under its new name,
// and must NOT see the cursor still parked on that header.
func TestFoldStateSurvivesRestartAndRenameButNotTheCursor(t *testing.T) {
	home := t.TempDir()
	db, err := store.OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	group, err := db.CreateGroup(ctx, "before-rename")
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}

	settings := config.Settings{}
	sessionBefore := store.Session{ID: "s1", Name: "s1", Status: "idle", GroupName: group.Name, GroupID: groupIDPtr(group.ID)}

	m := New(db, settings, "")
	m.sessions = []store.Session{sessionBefore}
	if m.isGroupCollapsed(group.ID) {
		t.Fatalf("fixture sanity: group %d should start expanded", group.ID)
	}

	// Fold the group and park the cursor on its own header -- the same
	// landing spot a session-row `c`/`left` press leaves it on (task 014).
	m.setGroupCollapsed(group.ID, true)
	m.selected = headerCursor(group.ID)

	cmd := m.persistCollapsedGroups()
	if cmd == nil {
		t.Fatal("persistCollapsedGroups returned nil with a store attached")
	}
	if persisted, ok := cmd().(uiStatePersisted); !ok || persisted.err != nil {
		t.Fatalf("persistCollapsedGroups command = %+v, want a successful uiStatePersisted", cmd())
	}

	// Rename the group. Its id (what collapse state and the cursor are
	// keyed by) is untouched; only groups.name changes.
	const newName = "after-rename"
	if err := db.RenameGroup(ctx, group.ID, newName); err != nil {
		t.Fatalf("RenameGroup: %v", err)
	}

	// A restarted client is a fresh New(db, ...) call over the same
	// state.db (TestCollapsedGroupsSurviveModelRebuildFromUIState's own
	// shape), with its session list rebuilt under the group's new name --
	// exactly what a real reconciliation read would hand back after the
	// rename, since sessions.group_id itself never moved.
	restarted := New(db, settings, "")
	restarted.sessions = []store.Session{{ID: "s1", Name: "s1", Status: "idle", GroupName: newName, GroupID: groupIDPtr(group.ID)}}

	if !restarted.isGroupCollapsed(group.ID) {
		t.Fatalf("restarted client lost the fold on group %d across a restart and rename", group.ID)
	}

	// The header text now reads the NEW name -- proof the rename actually
	// took, not just that the id-keyed fold survived independently of it.
	entries := restarted.sidebarEntries(40)
	var headerText string
	for _, e := range entries {
		if e.kind == sidebarLineHeader && e.groupID == group.ID {
			headerText = e.text
			break
		}
	}
	if headerText == "" {
		t.Fatalf("restarted client's sidebar has no header entry for group %d", group.ID)
	}
	if !strings.Contains(headerText, newName) {
		t.Fatalf("restarted client's header for group %d reads %q, want it to name the new name %q", group.ID, headerText, newName)
	}

	// The cursor is the zero value (Row(0)) -- NOT the header the
	// pre-restart client had parked it on. "Do not persist the cursor"
	// (SPEC §11/PRD R137) means a restarted client never resumes on a
	// header just because a previous run's cursor happened to be there
	// when it exited.
	if restarted.selected != rowCursor(0) {
		t.Fatalf("restarted client's cursor = %+v, want the zero value rowCursor(0) -- the cursor must not persist across a restart", restarted.selected)
	}
	if gid, ok := restarted.selected.GroupID(); ok {
		t.Fatalf("restarted client's cursor resolved to header group %d; the cursor must not persist across a restart", gid)
	}
}
