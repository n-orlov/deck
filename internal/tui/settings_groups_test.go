package tui

import (
	"context"
	"errors"
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

// TestSettingsGroupDeleteWithNoGroupsIsANoOp covers half of R131 part 2's
// "default offers no delete at all": default is never a row in
// m.settingsGroups (store.Group's own doc comment), so with no real group
// created yet there is nothing selected and "d" does nothing at all.
func TestSettingsGroupDeleteWithNoGroupsIsANoOp(t *testing.T) {
	db := openStoreForLastCreateGroup(t)
	m := settingsOpenOnGroupsFields(t, db)

	updated, _ := m.Update(key("d"))
	m = updated.(Model)

	if m.settingsGroupDeleteConfirming {
		t.Fatal("d with an empty group list opened the confirm sub-mode")
	}
	if m.settingsGroupNote != "" {
		t.Errorf("settingsGroupNote = %q after d with no group selected, want untouched", m.settingsGroupNote)
	}
}

// TestSettingsGroupDeleteEmptyGroupHasNoPrompt is R131 part 2's own "an
// empty group is deleted with no prompt at all" (SPEC §11.5): "d" on a
// group with no members calls store.DeleteGroup immediately --
// settingsGroupDeleteConfirming is never set.
func TestSettingsGroupDeleteEmptyGroupHasNoPrompt(t *testing.T) {
	db := openStoreForLastCreateGroup(t)
	if _, err := db.CreateGroup(context.Background(), "empty-one"); err != nil {
		t.Fatal(err)
	}
	m := settingsOpenOnGroupsFields(t, db)

	updated, _ := m.Update(key("d"))
	m = updated.(Model)

	if m.settingsGroupDeleteConfirming {
		t.Fatal("d on an empty group opened the confirm sub-mode; SPEC \u00a711.5 says no prompt at all")
	}
	if !strings.Contains(m.settingsGroupNote, "deleted empty group empty-one") {
		t.Errorf("settingsGroupNote = %q, want it to name the deleted empty group", m.settingsGroupNote)
	}
	groups, err := db.ListGroups(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 0 {
		t.Fatalf("ListGroups() = %+v after deleting the only (empty) group; want none", groups)
	}
}

// TestSettingsGroupDeleteNonEmptyOpensTwoBranchPrompt proves the
// non-empty branch of R131 part 2's prompt: "d" opens a confirm naming
// the member count and describing the "m" branch, and esc abandons it
// leaving the group and its members untouched.
func TestSettingsGroupDeleteNonEmptyOpensTwoBranchPrompt(t *testing.T) {
	db := openStoreForLastCreateGroup(t)
	ctx := context.Background()
	g, err := db.CreateGroup(ctx, "tidy-up")
	if err != nil {
		t.Fatal(err)
	}
	m := settingsOpenOnGroupsFields(t, db)
	m.baseSessions = []store.Session{{ID: "s1", Name: "alpha", GroupID: &g.ID}}

	updated, _ := m.Update(key("d"))
	m = updated.(Model)
	if !m.settingsGroupDeleteConfirming {
		t.Fatal("d on a non-empty group did not open the two-branch confirm")
	}
	body := m.settingsGroupsViewLines(settingsCategories(), settingsCategoryWidth(100), 100-settingsCategoryWidth(100), 20, 24)
	if !strings.Contains(body, "1 session(s)") {
		t.Errorf("groups view does not name the member count while the confirm is open:\n%s", body)
	}
	if !strings.Contains(strings.ToLower(body), "moves them to default") {
		t.Errorf("groups view does not describe the m branch:\n%s", body)
	}

	updated, _ = m.Update(key("esc"))
	m = updated.(Model)
	if m.settingsGroupDeleteConfirming {
		t.Fatal("esc did not close the confirm sub-mode")
	}
	groups, err := db.ListGroups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 {
		t.Fatalf("ListGroups() = %+v after esc; want the group untouched", groups)
	}
}

// TestSettingsGroupDeleteMBranchMovesMembersToDefaultAndDropsGroup is
// R131 part 2's non-destructive branch: "m" sets every member's group_id
// to NULL (structural default) and drops the group row -- destroying no
// session.
func TestSettingsGroupDeleteMBranchMovesMembersToDefaultAndDropsGroup(t *testing.T) {
	db := openStoreForLastCreateGroup(t)
	ctx := context.Background()
	g, err := db.CreateGroup(ctx, "tidy-up")
	if err != nil {
		t.Fatal(err)
	}
	a, err := db.CreateSession(ctx, store.CreateSessionInput{
		ID: "00000000-0000-4000-8000-0000000000c1", Name: "alpha", CWD: "/x",
		Agent: "shell", CapturedPath: "/bin", StatusAt: 100, CreatedAt: 100,
		GroupID: &g.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	b, err := db.CreateSession(ctx, store.CreateSessionInput{
		ID: "00000000-0000-4000-8000-0000000000d2", Name: "beta", CWD: "/x",
		Agent: "shell", CapturedPath: "/bin", StatusAt: 101, CreatedAt: 101,
		GroupID: &g.ID,
	})
	if err != nil {
		t.Fatal(err)
	}

	m := settingsOpenOnGroupsFields(t, db)
	m.baseSessions = []store.Session{a, b}

	updated, _ := m.Update(key("d"))
	m = updated.(Model)
	if !m.settingsGroupDeleteConfirming {
		t.Fatal("d on a non-empty group did not open the confirm")
	}

	updated, cmd := m.Update(key("m"))
	m = updated.(Model)
	if m.settingsGroupDeleteConfirming {
		t.Fatal("m did not close the confirm sub-mode")
	}
	if !strings.Contains(m.settingsGroupNote, "moved to default") {
		t.Errorf("settingsGroupNote = %q, want it to say the sessions moved to default", m.settingsGroupNote)
	}
	if cmd == nil {
		t.Fatal("m returned no command; want m.loadSessions so the already-loaded list picks up the new group_id")
	}

	groups, err := db.ListGroups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 0 {
		t.Fatalf("ListGroups() = %+v after the m branch; want the group row dropped", groups)
	}
	for _, sess := range []struct{ id, name string }{{a.ID, "alpha"}, {b.ID, "beta"}} {
		got, err := db.GetSession(ctx, sess.id)
		if err != nil {
			t.Fatalf("GetSession(%s): %v", sess.name, err)
		}
		if got.GroupID != nil {
			t.Errorf("%s.GroupID after the m branch = %v, want nil (structural default)", sess.name, got.GroupID)
		}
		if got.DeletedAt != 0 {
			t.Errorf("%s.DeletedAt after the m branch = %v, want 0 -- the m branch destroys nothing", sess.name, got.DeletedAt)
		}
	}
}

// TestSettingsGroupDeleteDBranchReachesTheSameServiceCallDDDoes is task
// 019's own guard test: the destructive branch never deletes anything
// itself -- it populates m.marked with exactly the group's members and
// sets the same fields the main list's own dd chord sets for a
// non-empty mark set, closing the settings takeover so the ordinary
// top-level `u` can restore the batch afterward. Submitting from there
// must drive deleteSvc (internal/service.Delete's own seam, mark_test.go's
// own precedent) once per marked session, exactly like a plain dd on a
// marked set does -- never a second deletion implementation.
func TestSettingsGroupDeleteDBranchReachesTheSameServiceCallDDDoes(t *testing.T) {
	db := openStoreForLastCreateGroup(t)
	ctx := context.Background()
	g, err := db.CreateGroup(ctx, "batch-group")
	if err != nil {
		t.Fatal(err)
	}

	var invocations []string
	m := settingsOpenOnGroupsFields(t, db)
	m.deleteSvc = func(_ context.Context, s store.Session) error {
		invocations = append(invocations, s.ID)
		return nil
	}
	m.baseSessions = []store.Session{
		{ID: "s1", Name: "alpha", Status: "stopped", GroupID: &g.ID},
		{ID: "s2", Name: "beta", Status: "stopped", GroupID: &g.ID},
	}
	m.sessions = m.baseSessions

	updated, _ := m.Update(key("d"))
	m = updated.(Model)
	if !m.settingsGroupDeleteConfirming {
		t.Fatal("d on a non-empty group did not open the confirm")
	}

	updated, _ = m.Update(key("d"))
	m = updated.(Model)
	if m.settingsOpen {
		t.Fatal("the destructive d branch left the settings takeover open; it must hand off to the main dd confirm")
	}
	if !m.deleteConfirming {
		t.Fatal("the destructive d branch did not open the bulk delete confirm (m.deleteConfirming)")
	}
	if len(m.marked) != 2 || !m.marked["s1"] || !m.marked["s2"] {
		t.Fatalf("m.marked = %#v after the destructive d branch, want exactly the group's two members", m.marked)
	}
	body := m.deleteConfirmBody()
	if !strings.Contains(body, "2 marked sessions") {
		t.Fatalf("the destructive branch did not open the SAME bulk confirm dd itself opens:\n%s", body)
	}

	updated, cmd := m.Update(key("enter"))
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("submit returned no command")
	}
	updated, _ = m.Update(cmd())
	_ = updated.(Model)

	if len(invocations) != 2 {
		t.Fatalf("deleteSvc invoked %d time(s), want exactly 2 (the same per-session call dd's own bulk path makes): %#v", len(invocations), invocations)
	}
}

// routeSettingsGroupDeleteIntoBulkConfirm drives the destructive branch of
// R131 part 2 up to (but not through) the bulk confirm's own submit: it
// creates group name holding the given member sessions, opens the Groups
// category, presses `d` twice (the two-branch confirm, then its
// destructive answer) and hands back the model sitting on the ordinary
// §9.2 bulk delete confirm. Every test below shares it so they all agree
// on exactly which keys reach that state.
func routeSettingsGroupDeleteIntoBulkConfirm(t *testing.T, db *store.Store, name string, members ...store.Session) (Model, store.Group) {
	t.Helper()
	g, err := db.CreateGroup(context.Background(), name)
	if err != nil {
		t.Fatal(err)
	}
	for i := range members {
		id := g.ID
		members[i].GroupID = &id
	}
	m := settingsOpenOnGroupsFields(t, db)
	m.baseSessions = members
	m.sessions = members

	updated, _ := m.Update(key("d"))
	m = updated.(Model)
	if !m.settingsGroupDeleteConfirming {
		t.Fatal("d on a non-empty group did not open the two-branch confirm")
	}
	updated, _ = m.Update(key("d"))
	m = updated.(Model)
	if !m.deleteConfirming {
		t.Fatal("the destructive d branch did not reach the bulk delete confirm")
	}
	return m, g
}

// groupNamesIn is a tiny read helper so the assertions below name what
// they mean ("is the row still there") rather than indexing ListGroups.
func groupNamesIn(t *testing.T, db *store.Store) []string {
	t.Helper()
	groups, err := db.ListGroups(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, g := range groups {
		names = append(names, g.Name)
	}
	return names
}

// TestSettingsGroupDeleteDBranchDropsTheGroupRowOnceTheBatchCommits is
// R131's own "The group row goes once the batch commits": the destructive
// branch deletes nothing itself, but the moment the routed §9.2 batch
// reports back (sessionsBulkDeleted, every member's delete having
// succeeded) the group row is gone -- so the operator who asked for
// "delete all N sessions" is not left with an empty group standing.
func TestSettingsGroupDeleteDBranchDropsTheGroupRowOnceTheBatchCommits(t *testing.T) {
	db := openStoreForLastCreateGroup(t)
	m, g := routeSettingsGroupDeleteIntoBulkConfirm(t, db, "batch-group",
		store.Session{ID: "s1", Name: "alpha", Status: "stopped"},
		store.Session{ID: "s2", Name: "beta", Status: "stopped"},
	)
	m.deleteSvc = func(_ context.Context, _ store.Session) error { return nil }

	if m.bulkDeleteGroupID != g.ID {
		t.Fatalf("bulkDeleteGroupID = %d after routing into the batch, want the routed group %d", m.bulkDeleteGroupID, g.ID)
	}
	if names := groupNamesIn(t, db); len(names) != 1 {
		t.Fatalf("ListGroups() = %v before the batch commits; the row must still stand until then", names)
	}

	updated, cmd := m.Update(key("enter"))
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("submit returned no command")
	}
	updated, _ = m.Update(cmd())
	m = updated.(Model)

	if names := groupNamesIn(t, db); len(names) != 0 {
		t.Fatalf("ListGroups() = %v after the batch committed, want the group row dropped (R131: it goes once the batch commits)", names)
	}
	if m.bulkDeleteGroupID != 0 || m.bulkDeleteGroupName != "" {
		t.Errorf("bulkDeleteGroup{ID,Name} = %d/%q after the batch, want cleared so a later ordinary dd never inherits it", m.bulkDeleteGroupID, m.bulkDeleteGroupName)
	}
	if m.attachError != "" {
		t.Errorf("attachError = %q, want empty on the happy path", m.attachError)
	}
}

// TestSettingsGroupDeleteDBranchKeepsTheGroupWhenTheConfirmIsCancelled is
// the other half of that timing: esc on the routed bulk confirm commits no
// batch at all, so both the members and the group row survive and nothing
// is left parked for the next dd to trip over.
func TestSettingsGroupDeleteDBranchKeepsTheGroupWhenTheConfirmIsCancelled(t *testing.T) {
	db := openStoreForLastCreateGroup(t)
	m, _ := routeSettingsGroupDeleteIntoBulkConfirm(t, db, "kept-group",
		store.Session{ID: "s1", Name: "alpha", Status: "stopped"},
	)
	m.deleteSvc = func(_ context.Context, _ store.Session) error {
		t.Fatal("esc on the bulk confirm still called deleteSvc")
		return nil
	}

	updated, _ := m.Update(key("esc"))
	m = updated.(Model)

	if m.deleteConfirming {
		t.Fatal("esc did not close the bulk delete confirm")
	}
	if names := groupNamesIn(t, db); len(names) != 1 || names[0] != "kept-group" {
		t.Fatalf("ListGroups() = %v after cancelling the batch, want the group row untouched", names)
	}
	if m.bulkDeleteGroupID != 0 || m.bulkDeleteGroupName != "" {
		t.Errorf("bulkDeleteGroup{ID,Name} = %d/%q after a cancelled batch, want cleared", m.bulkDeleteGroupID, m.bulkDeleteGroupName)
	}
}

// TestSettingsGroupDeleteDBranchKeepsTheGroupWhenAMemberFailsToDelete
// pins the partial-failure rule: a group that still holds a live member
// keeps its row, because dropping it would silently move that survivor to
// default -- which is the OTHER branch's behaviour, the one the operator
// did not pick.
func TestSettingsGroupDeleteDBranchKeepsTheGroupWhenAMemberFailsToDelete(t *testing.T) {
	db := openStoreForLastCreateGroup(t)
	m, _ := routeSettingsGroupDeleteIntoBulkConfirm(t, db, "half-group",
		store.Session{ID: "s1", Name: "alpha", Status: "stopped"},
		store.Session{ID: "s2", Name: "beta", Status: "stopped"},
	)
	m.deleteSvc = func(_ context.Context, s store.Session) error {
		if s.ID == "s2" {
			return errors.New("boom")
		}
		return nil
	}

	updated, cmd := m.Update(key("enter"))
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("submit returned no command")
	}
	updated, _ = m.Update(cmd())
	m = updated.(Model)

	if names := groupNamesIn(t, db); len(names) != 1 || names[0] != "half-group" {
		t.Fatalf("ListGroups() = %v after a partially failed batch, want the group row kept (a live member still names it)", names)
	}
	if !strings.Contains(m.attachError, "boom") {
		t.Errorf("attachError = %q, want the batch's own failure surfaced", m.attachError)
	}
	if m.bulkDeleteGroupID != 0 {
		t.Errorf("bulkDeleteGroupID = %d after the batch reported, want cleared either way", m.bulkDeleteGroupID)
	}
}

// TestOrdinaryBulkDeleteNeverDropsAGroupRow is the containment guard for
// the field above: the group-row deletion belongs to R131's settings
// branch alone, so a plain top-level `dd` over a marked set that happens
// to live in one group deletes the sessions and leaves the group standing.
func TestOrdinaryBulkDeleteNeverDropsAGroupRow(t *testing.T) {
	db := openStoreForLastCreateGroup(t)
	ctx := context.Background()
	g, err := db.CreateGroup(ctx, "untouched-group")
	if err != nil {
		t.Fatal(err)
	}
	m := New(db, config.Settings{}, "")
	m.width, m.height = 100, 40
	m.deleteSvc = func(_ context.Context, _ store.Session) error { return nil }
	id := g.ID
	m.baseSessions = []store.Session{{ID: "s1", Name: "alpha", Status: "stopped", GroupID: &id}}
	m.sessions = m.baseSessions
	m.marked = map[string]bool{"s1": true}
	m.deleteConfirming = true

	updated, cmd := m.Update(key("enter"))
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("submit returned no command")
	}
	updated, _ = m.Update(cmd())
	m = updated.(Model)

	if names := groupNamesIn(t, db); len(names) != 1 || names[0] != "untouched-group" {
		t.Fatalf("ListGroups() = %v after an ordinary bulk dd, want the group row untouched", names)
	}
}
