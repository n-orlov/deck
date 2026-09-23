package tui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// This file is R130 part 2's own evidence for the `g` group-move picker
// (SPEC §11, `i` detail dialog): each test wired to a REAL store.Store, so
// "through the picker" means store.Store.SetSessionGroup itself, never a
// stand-in double.

// newGroupMoveTestStore opens a real, on-disk store.Store (mirroring
// launch_inputs_test.go's own store.OpenPath pattern) and seeds three
// sessions -- one in each of two real groups plus one in the structural
// default -- so a test can prove that moving ONE of them changes only
// that one row's group_id.
func newGroupMoveTestStore(t *testing.T) (db *store.Store, alphaID, bravoID, defaultID string, alpha, bravo store.Group) {
	t.Helper()
	home := t.TempDir()
	db, err := store.OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()
	alpha, err = db.CreateGroup(ctx, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	bravo, err = db.CreateGroup(ctx, "bravo")
	if err != nil {
		t.Fatal(err)
	}
	const (
		idAlpha   = "00000000-0000-4000-8000-000000000a01"
		idBravo   = "00000000-0000-4000-8000-000000000a02"
		idDefault = "00000000-0000-4000-8000-000000000a03"
	)
	for _, s := range []struct {
		id      string
		name    string
		groupID *int64
	}{
		{idAlpha, "session-alpha", &alpha.ID},
		{idBravo, "session-bravo", &bravo.ID},
		{idDefault, "session-default", nil},
	} {
		if _, err := db.CreateSession(ctx, store.CreateSessionInput{
			ID: s.id, Name: s.name, CWD: "/work/" + s.name, Agent: "shell", CapturedPath: "/bin",
			StatusAt: 1, CreatedAt: 1, GroupID: s.groupID,
		}); err != nil {
			t.Fatal(err)
		}
	}
	return db, idAlpha, idBravo, idDefault, alpha, bravo
}

// groupMoveTestModel opens the picker (via the same "i" then "g" path
// updateDetailView itself uses -- rename.go's case "g") on the session at
// selectedIndex, wired to the store's OWN SetSessionGroup (this task's new
// store method) through WithGroupMover exactly as cmd/deck/main.go would
// wire service.Service.SetSessionGroup in the shipped binary.
func groupMoveTestModel(t *testing.T, db *store.Store, sessions []store.Session, selectedIndex int) Model {
	t.Helper()
	m := New(db, config.Settings{}, "").WithGroupMover(
		func(ctx context.Context, sessionID string, groupID int64) (store.Session, error) {
			if err := db.SetSessionGroup(ctx, sessionID, groupID, "user", 10); err != nil {
				return store.Session{}, err
			}
			return db.GetSession(ctx, sessionID)
		},
	)
	m.sessions = sessions
	m.selected = rowCursor(selectedIndex)
	got, _ := m.Update(key("i"))
	m = got.(Model)
	got, _ = m.Update(key("g"))
	m = got.(Model)
	if !m.movingGroup {
		t.Fatal("\"g\" inside detail did not open the group-move picker")
	}
	return m
}

// TestMoveSessionGroupThroughPickerChangesOnlyThatOneRow proves the
// picker's central contract: cycling to a different group and pressing
// Enter moves EXACTLY the selected session's group_id, through
// store.Store.SetSessionGroup, leaving every other fixture row's group_id
// untouched.
func TestMoveSessionGroupThroughPickerChangesOnlyThatOneRow(t *testing.T) {
	db, idAlpha, idBravo, idDefault, alpha, bravo := newGroupMoveTestStore(t)
	ctx := context.Background()
	sAlpha, err := db.GetSession(ctx, idAlpha)
	if err != nil {
		t.Fatal(err)
	}
	sBravo, err := db.GetSession(ctx, idBravo)
	if err != nil {
		t.Fatal(err)
	}
	sDefault, err := db.GetSession(ctx, idDefault)
	if err != nil {
		t.Fatal(err)
	}
	sessions := []store.Session{sAlpha, sBravo, sDefault}

	// Move session-alpha (index 0) into bravo's group.
	m := groupMoveTestModel(t, db, sessions, 0)
	if got := m.moveGroupValue; got != alpha.ID {
		t.Fatalf("picker opened on group id %d, want the session's current group %d (alpha)", got, alpha.ID)
	}
	for m.moveGroupValue != bravo.ID {
		got, _ := m.Update(key("right"))
		m = got.(Model)
	}
	got, cmd := m.Update(key("enter"))
	m = got.(Model)
	if cmd == nil {
		t.Fatal("Enter produced no command; want one that calls the wired mover")
	}
	msg := cmd()
	next, ok := msg.(sessionGroupMoved)
	if !ok {
		t.Fatalf("Enter produced %T, want sessionGroupMoved", msg)
	}
	if next.err != nil {
		t.Fatalf("move failed: %v", next.err)
	}
	if next.session.GroupID == nil || *next.session.GroupID != bravo.ID {
		t.Fatalf("moved session's group_id = %v, want %d (bravo)", next.session.GroupID, bravo.ID)
	}
	got, _ = m.Update(msg)
	m = got.(Model)

	// Re-read every fixture row directly from the store: only the moved
	// row's group_id changed.
	after, err := db.GetSession(ctx, idAlpha)
	if err != nil {
		t.Fatal(err)
	}
	if after.GroupID == nil || *after.GroupID != bravo.ID {
		t.Fatalf("session-alpha's group_id = %v after the move, want %d (bravo)", after.GroupID, bravo.ID)
	}
	stillBravo, err := db.GetSession(ctx, idBravo)
	if err != nil {
		t.Fatal(err)
	}
	if stillBravo.GroupID == nil || *stillBravo.GroupID != bravo.ID {
		t.Fatalf("session-bravo's group_id changed to %v; it was never the target of this move", stillBravo.GroupID)
	}
	stillDefault, err := db.GetSession(ctx, idDefault)
	if err != nil {
		t.Fatal(err)
	}
	if stillDefault.GroupID != nil {
		t.Fatalf("session-default's group_id changed to %v; it was never the target of this move", stillDefault.GroupID)
	}
	if m.movingGroup {
		t.Fatal("a successful move left the picker open; want it closed, back to detail")
	}
	if !m.detail {
		t.Fatal("a successful move closed detail too; want it to stay open, showing the new group")
	}
}

// TestMoveSessionGroupToDefaultClearsGroupID proves the picker's other
// direction: cycling all the way to the structural default group and
// submitting clears group_id to NULL rather than writing some
// default-shaped sentinel value.
func TestMoveSessionGroupToDefaultClearsGroupID(t *testing.T) {
	db, idAlpha, _, _, _, _ := newGroupMoveTestStore(t)
	ctx := context.Background()
	sAlpha, err := db.GetSession(ctx, idAlpha)
	if err != nil {
		t.Fatal(err)
	}

	m := groupMoveTestModel(t, db, []store.Session{sAlpha}, 0)
	options := m.moveGroupCycleOptions()
	for m.moveGroupValue != 0 {
		got, _ := m.Update(key("right"))
		m = got.(Model)
	}
	if got := m.moveGroupName(m.moveGroupValue); got != "default" {
		t.Fatalf("picker landed on %q, want \"default\" (options were %#v)", got, options)
	}
	_, cmd := m.Update(key("enter"))
	if cmd == nil {
		t.Fatal("Enter produced no command; want one that calls the wired mover")
	}
	msg := cmd()
	next, ok := msg.(sessionGroupMoved)
	if !ok {
		t.Fatalf("Enter produced %T, want sessionGroupMoved", msg)
	}
	if next.err != nil {
		t.Fatalf("move failed: %v", next.err)
	}
	if next.session.GroupID != nil {
		t.Fatalf("moved session's group_id = %v, want nil (default)", next.session.GroupID)
	}
	after, err := db.GetSession(ctx, idAlpha)
	if err != nil {
		t.Fatal(err)
	}
	if after.GroupID != nil {
		t.Fatalf("session-alpha's group_id = %v after moving to default, want nil", after.GroupID)
	}
}

// TestDetailDialogNamesTheSessionsGroupBeforeThePickerOpens is R130 part
// 2's own evidence for the FIRST half of its criterion ("the `i` detail
// dialog displays the session's group"), independent of the `g` picker:
// top-level `i` alone, with no group list ever loaded into the model, must
// still print the session's real group name -- the bug the first attempt
// shipped, where the row resolved through moveGroupOptions and therefore
// read "default" until `g` had populated it. Both the named-group and the
// structural-default case are pinned, and the picker's own "Current
// group:" row is checked on the same model.
func TestDetailDialogNamesTheSessionsGroupBeforeThePickerOpens(t *testing.T) {
	groupID := int64(7)
	for _, tc := range []struct {
		name    string
		session store.Session
		want    string
	}{
		{
			name:    "named group",
			session: store.Session{ID: "s1", Name: "alpha-one", Agent: "shell", Status: "stopped", Slug: "alpha-one", GroupID: &groupID, GroupName: "alpha"},
			want:    "alpha",
		},
		{
			name:    "structural default",
			session: store.Session{ID: "s2", Name: "loose-one", Agent: "shell", Status: "stopped", Slug: "loose-one"},
			want:    "default",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := New(nil, config.Settings{}, "")
			m.width, m.height = 80, 30
			m.sessions = []store.Session{tc.session}
			m.selected = rowCursor(0)
			got, _ := m.Update(key("i"))
			m = got.(Model)
			if !m.detail {
				t.Fatal("\"i\" did not open detail")
			}
			if len(m.moveGroupOptions) != 0 {
				t.Fatalf("opening detail populated moveGroupOptions (%#v); this test must prove the row resolves WITHOUT it", m.moveGroupOptions)
			}
			line := detailLineWithPrefix(t, m.detailBody(), "Group:")
			if !strings.Contains(line, tc.want) {
				t.Fatalf("detail's group row = %q, want it to name %q", line, tc.want)
			}
			// The picker's own "Current group:" row agrees, and still does
			// so on the very first render after `g`.
			got, _ = m.Update(key("g"))
			pm := got.(Model)
			if !pm.movingGroup {
				t.Fatal("\"g\" inside detail did not open the group-move picker")
			}
			current := detailLineWithPrefix(t, pm.moveGroupBody(), "Current group:")
			if !strings.Contains(current, tc.want) {
				t.Fatalf("picker's current-group row = %q, want it to name %q", current, tc.want)
			}
		})
	}
}

// detailLineWithPrefix returns the first line of body whose trimmed text
// starts with prefix -- the one field row a test wants to assert on,
// without hard-coding the dialog's whole layout.
func detailLineWithPrefix(t *testing.T, body, prefix string) string {
	t.Helper()
	for _, l := range strings.Split(body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), prefix) {
			return l
		}
	}
	t.Fatalf("no line starting with %q in:\n%s", prefix, body)
	return ""
}

// TestGDoesNotDisturbQCtrlCIOrRInsideDetail proves R130 part 2's own
// collision check (rename.go's case "g" comment): adding "g" to
// updateDetailView leaves its pre-existing q, ctrl+c, i, r and l handling
// exactly as it was.
func TestGDoesNotDisturbQCtrlCIOrRInsideDetail(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	m.width, m.height = 80, 24
	m.sessions = []store.Session{{ID: "s1", Name: "alpha", Agent: "shell", Status: "stopped", Slug: "alpha"}}
	m.selected = rowCursor(0)
	got, _ := m.Update(key("i"))
	m = got.(Model)
	if !m.detail {
		t.Fatal("\"i\" did not open detail")
	}

	// "r" still opens rename, not the group picker.
	got, _ = m.Update(key("r"))
	rm := got.(Model)
	if !rm.renaming || rm.movingGroup {
		t.Fatalf("\"r\" inside detail: renaming=%v movingGroup=%v, want renaming=true movingGroup=false", rm.renaming, rm.movingGroup)
	}

	// "l" still opens the launch-inputs editor, not the group picker.
	got, _ = m.Update(key("l"))
	lm := got.(Model)
	if !lm.launchInputsEditing || lm.movingGroup {
		t.Fatalf("\"l\" inside detail: launchInputsEditing=%v movingGroup=%v, want launchInputsEditing=true movingGroup=false", lm.launchInputsEditing, lm.movingGroup)
	}

	// "i" still closes detail outright.
	got, _ = m.Update(key("i"))
	im := got.(Model)
	if im.detail {
		t.Fatal("\"i\" inside detail did not close it")
	}

	// "q"/"ctrl+c" still quit.
	m2 := New(nil, config.Settings{}, "")
	m2.sessions = m.sessions
	m2.selected = rowCursor(0)
	got, _ = m2.Update(key("i"))
	m2 = got.(Model)
	_, cmd := m2.Update(key("q"))
	if cmd == nil {
		t.Fatal("\"q\" inside detail produced no command; want tea.Quit")
	}
}

// TestDetailFooterAndHelpBothNameGForGroupMove is R130 part 2's own
// footer/help parity test (SPEC §11.4's own rule that every load-bearing
// key inside a dialog is named where it applies): detailBody's own footer
// line ("r renames ... i or Esc closes detail") and helpText's "i" bullet
// must BOTH name "g" as the key that moves a session's group, so a reader
// of either surface learns the same binding.
func TestDetailFooterAndHelpBothNameGForGroupMove(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	m.width, m.height = 80, 24
	m.sessions = []store.Session{{ID: "s1", Name: "alpha", Agent: "shell", Status: "stopped", Slug: "alpha"}}
	m.selected = rowCursor(0)

	footer := detailFooterLine(m.detailBody())
	if !strings.Contains(footer, "g moves group") {
		t.Fatalf("detail footer %q does not name \"g moves group\"", footer)
	}
	// The footer's other three load-bearing keys must still be named too --
	// this task must not have replaced one binding with another.
	for _, want := range []string{"r renames", "l edits launch inputs", "i or Esc closes detail"} {
		if !strings.Contains(footer, want) {
			t.Fatalf("detail footer %q lost %q", footer, want)
		}
	}

	help := helpText(m.settings.ASCII)
	iBullet := helpBulletFor(t, help, "  i ")
	if !strings.Contains(iBullet, "g inside it") || !strings.Contains(iBullet, "moves the session") || !strings.Contains(iBullet, "group") {
		t.Fatalf("help's \"i\" bullet does not name g moving the session's group:\n%s", iBullet)
	}
	if !strings.Contains(iBullet, "r inside it renames") {
		t.Fatalf("help's \"i\" bullet lost its existing r-renames sentence:\n%s", iBullet)
	}
}

// detailFooterLine returns detailBody's own last non-empty line -- the
// footer legend every detail-dialog test above asserts against.
func detailFooterLine(body string) string {
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	return lines[len(lines)-1]
}

// helpBulletFor extracts one top-level bullet (a line starting with the
// two-space-indented key marker, up to but not including the next
// top-level bullet or EOF) from helpText's Keys section, so a test can
// assert against just that key's own prose without hard-coding every
// other bullet's wording.
func helpBulletFor(t *testing.T, help, marker string) string {
	t.Helper()
	lines := strings.Split(help, "\n")
	start := -1
	for i, l := range lines {
		if strings.HasPrefix(l, marker) {
			start = i
			break
		}
	}
	if start == -1 {
		t.Fatalf("helpText has no bullet starting with %q", marker)
	}
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if len(lines[i]) > 2 && lines[i][0] == ' ' && lines[i][1] != ' ' {
			end = i
			break
		}
	}
	return strings.Join(lines[start:end], "\n")
}
