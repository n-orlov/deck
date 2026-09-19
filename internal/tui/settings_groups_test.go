package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// settingsGroupsCategoryIndex is this file's own small helper mirroring
// settingsOnGroupsCategory's live lookup, used only to drive navigation
// from the tests below (never to assert the category's existence itself
// -- TestSettingsGroupsCategoryIsSynthetic does that).
func settingsGroupsCategoryIndex(t *testing.T) int {
	t.Helper()
	for i, cat := range settingsCategories() {
		if cat.Section == "groups" {
			return i
		}
	}
	t.Fatal("settingsCategories() has no groups category")
	return -1
}

// settingsOpenOnGroupsFields opens the `,` takeover on db, navigates the
// category list down to the Groups category (settingsCategories()' own
// live index, never a hard-coded one, so a schema/category reorder cannot
// silently point this at the wrong category) and switches focus to the
// field panel -- the same "tab then move" an operator would perform by
// hand, exactly like settingsOpenOnEnvField's own precedent
// (settings_env_entries_editor_test.go).
func settingsOpenOnGroupsFields(t *testing.T, db *store.Store) Model {
	t.Helper()
	m := New(db, config.Settings{}, "")
	updated, _ := m.Update(key(","))
	m = updated.(Model)
	if !m.settingsOpen {
		t.Fatal(", did not open settings")
	}
	idx := settingsGroupsCategoryIndex(t)
	for i := 0; i < idx; i++ {
		updated, _ = m.Update(key("j"))
		m = updated.(Model)
	}
	if m.settingsCategoryIndex != idx {
		t.Fatalf("navigation landed on category %d, want %d (groups)", m.settingsCategoryIndex, idx)
	}
	updated, _ = m.Update(key("tab"))
	m = updated.(Model)
	if m.settingsFocus != settingsFocusFields {
		t.Fatal("tab did not move focus to the field panel")
	}
	if !m.settingsOnGroupsCategory() {
		t.Fatal("settingsOpenOnGroupsFields landed somewhere other than the groups category")
	}
	return m
}

// typeIntoGroupEditor sends value's runes one key message at a time,
// mirroring how a live PTY would deliver a typed name; task 118's own
// gotcha (key() splits a multi-rune KeyRunes message rune-by-rune inside
// Update, but only Update does that split -- calling key() once per rune
// here sidesteps needing to rely on that) keeps this independent of it.
func typeIntoGroupEditor(t *testing.T, m Model, value string) Model {
	t.Helper()
	for _, r := range value {
		updated, _ := m.Update(key(string(r)))
		m = updated.(Model)
	}
	return m
}

// TestSettingsGroupsCategoryIsSynthetic proves the Groups category (task
// 018, R131 part 1) exists in settingsCategories() and carries no
// config.Field entries of its own -- it is rendered by its own dedicated
// path (settingsGroupsViewLines), never the generic per-Field walk, so a
// non-empty Fields slice here would be silently ignored by every render
// path and would also (wrongly) have to satisfy
// TestSettingsSchemaParity_EveryRenderedFieldIsBackedBySchema.
func TestSettingsGroupsCategoryIsSynthetic(t *testing.T) {
	idx := settingsGroupsCategoryIndex(t)
	cat := settingsCategories()[idx]
	if cat.Name != "Groups" {
		t.Errorf("groups category Name = %q, want %q", cat.Name, "Groups")
	}
	if len(cat.Fields) != 0 {
		t.Errorf("groups category Fields = %+v, want empty (rendered by settingsGroupsViewLines, not the generic Field walk)", cat.Fields)
	}
}

// TestSettingsGroupsCopyStatesEditsApplyImmediately is SPEC §11.5's own
// requirement made concrete: "That difference is stated in the section
// itself rather than left for the user to discover" -- the Groups
// section's own on-screen copy must say its edits apply immediately, and
// must say esc has nothing to discard here, not just behave that way
// silently.
func TestSettingsGroupsCopyStatesEditsApplyImmediately(t *testing.T) {
	lower := strings.ToLower(settingsGroupsCopy)
	if !strings.Contains(lower, "immediately") {
		t.Errorf("settingsGroupsCopy does not say edits apply immediately: %q", settingsGroupsCopy)
	}
	if !strings.Contains(lower, "esc") || !strings.Contains(lower, "discard") {
		t.Errorf("settingsGroupsCopy does not say esc has nothing to discard: %q", settingsGroupsCopy)
	}
	if !strings.Contains(lower, "state.db") {
		t.Errorf("settingsGroupsCopy does not name state.db as where a group lives: %q", settingsGroupsCopy)
	}

	m := settingsOpenOnGroupsFields(t, nil)
	view := m.settingsGroupsViewLines(settingsCategories(), settingsCategoryWidth(100), 100-settingsCategoryWidth(100), 20, 24)
	if !strings.Contains(view, "applies immediately") {
		t.Errorf("Groups section view does not render its own stated-immediately copy:\n%s", view)
	}
}

// TestSettingsGroupsEscOffersNoDiscardPrompt is SPEC §11.5's other half of
// the same requirement: "esc must not offer to discard group edits it
// cannot discard: a prompt that claims to undo something it has already
// committed is worse than no prompt." Pressing esc while the Groups
// category is open (and no group name is being typed) must close the
// takeover outright, exactly like esc with no unsaved config.toml edit
// does everywhere else -- never m.settingsDiscardConfirm.
func TestSettingsGroupsEscOffersNoDiscardPrompt(t *testing.T) {
	db := openStoreForLastCreateGroup(t)
	if _, err := db.CreateGroup(context.Background(), "existing"); err != nil {
		t.Fatal(err)
	}
	m := settingsOpenOnGroupsFields(t, db)

	updated, _ := m.Update(key("esc"))
	m = updated.(Model)
	if m.settingsDiscardConfirm {
		t.Fatal("esc on the Groups category opened the discard-confirm prompt; SPEC §11.5 forbids this")
	}
	if m.settingsOpen {
		t.Fatal("esc on the Groups category (nothing staged) did not close the takeover")
	}
}

// TestSettingsGroupsEscDuringCreateCancelsOnlyTheEditorNotTheTakeover
// covers the typing sub-mode's own esc (mirroring
// updateSettingsStringEditing's identical shape): esc while "n"'s buffer
// is open abandons only the typed name, leaving the takeover itself open
// on the Groups category, never writing anything to the store.
func TestSettingsGroupsEscDuringCreateCancelsOnlyTheEditorNotTheTakeover(t *testing.T) {
	db := openStoreForLastCreateGroup(t)
	m := settingsOpenOnGroupsFields(t, db)

	updated, _ := m.Update(key("n"))
	m = updated.(Model)
	if !m.settingsGroupCreating {
		t.Fatal("n did not open the group-create editor")
	}
	m = typeIntoGroupEditor(t, m, "abandoned")

	updated, _ = m.Update(key("esc"))
	m = updated.(Model)
	if m.settingsGroupCreating {
		t.Fatal("esc did not close the group-create editor")
	}
	if !m.settingsOpen {
		t.Fatal("esc inside the group-create editor closed the whole takeover, not just the editor")
	}

	groups, err := db.ListGroups(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 0 {
		t.Fatalf("ListGroups() = %+v after an abandoned create; want none written", groups)
	}
}

// TestSettingsGroupsNCreatesGroupImmediately is R131 part 1's own create
// path: "n" opens the typing sub-mode, and enter commits straight to
// state.db (store.CreateGroup) the moment it is pressed -- never staged
// into m.settingsEdits, never gated by ctrl+s.
func TestSettingsGroupsNCreatesGroupImmediately(t *testing.T) {
	db := openStoreForLastCreateGroup(t)
	m := settingsOpenOnGroupsFields(t, db)

	updated, _ := m.Update(key("n"))
	m = updated.(Model)
	m = typeIntoGroupEditor(t, m, "sprint work")
	updated, _ = m.Update(key("enter"))
	m = updated.(Model)

	if m.settingsGroupCreating {
		t.Fatal("enter did not close the group-create editor after a successful create")
	}
	if !strings.Contains(m.settingsGroupNote, "sprint work") {
		t.Errorf("settingsGroupNote = %q, want it to name the created group", m.settingsGroupNote)
	}

	groups, err := db.ListGroups(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || groups[0].Name != "sprint work" {
		t.Fatalf("ListGroups() = %+v; want exactly one group named %q", groups, "sprint work")
	}
	found := false
	for _, g := range m.settingsGroups {
		if g.Name == "sprint work" {
			found = true
		}
	}
	if !found {
		t.Errorf("m.settingsGroups = %+v after create, want it to include the just-created group (refreshed snapshot)", m.settingsGroups)
	}
}

// TestSettingsGroupsCreateShowsValidateGroupNameErrorInline is the "showing
// task 009's validation errors inline" success criterion: committing a
// reserved name ("default", task 009's validateGroupName) must surface
// store.CreateGroup's own error text in m.settingsGroupNote and leave the
// editor open with the rejected text still in the buffer, so the operator
// can fix it in place rather than losing what they typed.
func TestSettingsGroupsCreateShowsValidateGroupNameErrorInline(t *testing.T) {
	db := openStoreForLastCreateGroup(t)
	m := settingsOpenOnGroupsFields(t, db)

	updated, _ := m.Update(key("n"))
	m = updated.(Model)
	m = typeIntoGroupEditor(t, m, "default")
	updated, _ = m.Update(key("enter"))
	m = updated.(Model)

	if !m.settingsGroupCreating {
		t.Fatal("a rejected create closed the editor; it must stay open so the operator can correct the name")
	}
	if m.settingsGroupEditValue != "default" {
		t.Errorf("settingsGroupEditValue = %q after a rejected commit, want the typed value preserved (%q)", m.settingsGroupEditValue, "default")
	}
	if !strings.Contains(m.settingsGroupNote, "reserved") {
		t.Errorf("settingsGroupNote = %q, want it to surface validateGroupName's reserved-name error", m.settingsGroupNote)
	}

	groups, err := db.ListGroups(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 0 {
		t.Fatalf("ListGroups() = %+v after a rejected create; want none written", groups)
	}
}

// TestSettingsGroupsRenameLeavesMemberGroupIDsUntouched is R128/R131's own
// "one row update, carries every member for free" story made concrete
// through the settings takeover's own "r" path (rather than calling
// store.RenameGroup directly, which task 009's own
// TestRenameGroupUpdatesOneRowAndCarriesItsTwoMembers already covers): two
// sessions are assigned to a group, the group is renamed through "r", and
// both rows are RE-READ via store.GetSession to assert their GroupID
// column is the exact same pointer value as before -- RenameGroup updates
// only the groups row, never sessions.group_id.
func TestSettingsGroupsRenameLeavesMemberGroupIDsUntouched(t *testing.T) {
	db := openStoreForLastCreateGroup(t)
	ctx := context.Background()

	g, err := db.CreateGroup(ctx, "sprint work")
	if err != nil {
		t.Fatal(err)
	}
	a, err := db.CreateSession(ctx, store.CreateSessionInput{
		ID: "00000000-0000-4000-8000-0000000000a1", Name: "alpha", CWD: "/x",
		Agent: "shell", CapturedPath: "/bin", StatusAt: 100, CreatedAt: 100,
		GroupID: &g.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	b, err := db.CreateSession(ctx, store.CreateSessionInput{
		ID: "00000000-0000-4000-8000-0000000000b2", Name: "beta", CWD: "/x",
		Agent: "shell", CapturedPath: "/bin", StatusAt: 101, CreatedAt: 101,
		GroupID: &g.ID,
	})
	if err != nil {
		t.Fatal(err)
	}

	m := settingsOpenOnGroupsFields(t, db)
	updated, _ := m.Update(key("r"))
	m = updated.(Model)
	if !m.settingsGroupRenaming {
		t.Fatal("r did not open the group-rename editor on the only (and therefore selected) group")
	}
	if m.settingsGroupEditValue != "sprint work" {
		t.Fatalf("group-rename editor prefilled %q, want the current name %q", m.settingsGroupEditValue, "sprint work")
	}
	// Clear the prefilled name and type the new one -- backspace once per
	// rune, mirroring how an operator would edit the field, then type the
	// replacement.
	for range "sprint work" {
		updated, _ = m.Update(key("backspace"))
		m = updated.(Model)
	}
	m = typeIntoGroupEditor(t, m, "sprint delivery")
	updated, _ = m.Update(key("enter"))
	m = updated.(Model)

	if m.settingsGroupRenaming {
		t.Fatal("enter did not close the group-rename editor after a successful rename")
	}

	groups, err := db.ListGroups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || groups[0].ID != g.ID || groups[0].Name != "sprint delivery" {
		t.Fatalf("ListGroups() = %+v; want one row %d=%q", groups, g.ID, "sprint delivery")
	}

	for _, sess := range []struct{ id, name string }{{a.ID, "alpha"}, {b.ID, "beta"}} {
		got, err := db.GetSession(ctx, sess.id)
		if err != nil {
			t.Fatalf("GetSession(%s): %v", sess.name, err)
		}
		if got.GroupID == nil || *got.GroupID != g.ID {
			t.Errorf("%s.GroupID after rename = %v, want unchanged (still %d) -- RenameGroup must never touch sessions.group_id", sess.name, got.GroupID, g.ID)
		}
		if got.GroupName != "sprint delivery" {
			t.Errorf("%s.GroupName after rename = %q, want %q (resolved live off the renamed row)", sess.name, got.GroupName, "sprint delivery")
		}
	}
}
