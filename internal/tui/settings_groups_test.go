package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	mustCreateSessionInGroup(t, db, "s1", "alpha", g.ID, "")
	m := settingsOpenOnGroupsFields(t, db)

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
	s1 := mustCreateSessionInGroup(t, db, "s1", "alpha", g.ID, "stopped")
	s2 := mustCreateSessionInGroup(t, db, "s2", "beta", g.ID, "stopped")

	var invocations []string
	m := settingsOpenOnGroupsFields(t, db)
	m.deleteSvc = func(_ context.Context, s store.Session) error {
		invocations = append(invocations, s.ID)
		return nil
	}
	m.baseSessions = []store.Session{s1, s2}
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

// mustCreateSessionInGroup persists a real session row belonging to
// groupID (task 019/R131 part 2 cure): the group-delete empty/non-empty
// decision and both its branches now read the group's PERSISTED member
// set (store.ListSessionsIncludingArchived), never m.baseSessions/m.sessions
// set directly on the Model -- a test session that only ever existed as a
// bare struct literal in memory would look, from the store's own point of
// view, like it was never a member at all. status "" defaults to
// "stopped", since none of these tests need a live pane.
func mustCreateSessionInGroup(t *testing.T, db *store.Store, id, name string, groupID int64, status string) store.Session {
	t.Helper()
	if status == "" {
		status = "stopped"
	}
	gid := groupID
	s, err := db.CreateSession(context.Background(), store.CreateSessionInput{
		ID: id, Name: name, CWD: "/x", Agent: "shell", CapturedPath: "/bin",
		Status: status, StatusAt: 100, CreatedAt: 100, GroupID: &gid,
	})
	if err != nil {
		t.Fatalf("CreateSession(%s): %v", name, err)
	}
	return s
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
	persisted := make([]store.Session, 0, len(members))
	for _, mem := range members {
		persisted = append(persisted, mustCreateSessionInGroup(t, db, mem.ID, mem.Name, g.ID, mem.Status))
	}
	m := settingsOpenOnGroupsFields(t, db)
	m.baseSessions = persisted
	m.sessions = persisted

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

// TestSettingsGroupDeleteDBranchIncludesFilteredOutMembers is task
// 019/R131 part 2's own cure regression: the destructive "d" branch must
// hand the group's COMPLETE member set to the shared dd path, never one
// re-filtered through whatever the sidebar's active filter happens to
// show right now. Two members, an active filter (m.filterQuery) that
// narrows m.sessions down to one of them -- the confirm still names both,
// deleteSvc still runs twice, and both rows end up tombstoned.
func TestSettingsGroupDeleteDBranchIncludesFilteredOutMembers(t *testing.T) {
	db := openStoreForLastCreateGroup(t)
	ctx := context.Background()
	g, err := db.CreateGroup(ctx, "filtered-group")
	if err != nil {
		t.Fatal(err)
	}
	a := mustCreateSessionInGroup(t, db, "s1", "alpha", g.ID, "stopped")
	b := mustCreateSessionInGroup(t, db, "s2", "beta", g.ID, "stopped")

	var invocations []string
	m := settingsOpenOnGroupsFields(t, db)
	m.deleteSvc = func(ctx context.Context, s store.Session) error {
		invocations = append(invocations, s.ID)
		return db.SoftDeleteSession(ctx, s.ID, 300)
	}
	m.baseSessions = []store.Session{a, b}
	// An active filter that matches only "alpha" -- m.sessions (the
	// sidebar's current, narrowed view) holds just one of the two members.
	m.filterQuery = "alpha"
	m.sessions = m.filteredSessions()
	if len(m.sessions) != 1 {
		t.Fatalf("test setup: m.sessions = %+v, want exactly the one filtered-in member", m.sessions)
	}

	updated, _ := m.Update(key("d"))
	m = updated.(Model)
	if !m.settingsGroupDeleteConfirming {
		t.Fatal("d on a non-empty group did not open the confirm")
	}
	body := m.settingsGroupsViewLines(settingsCategories(), settingsCategoryWidth(100), 100-settingsCategoryWidth(100), 20, 24)
	if !strings.Contains(body, "2 session(s)") {
		t.Errorf("groups view under an active filter = %q, want it to still name BOTH members, not just the filtered-in one", body)
	}

	updated, _ = m.Update(key("d"))
	m = updated.(Model)
	if !m.deleteConfirming {
		t.Fatal("the destructive d branch did not open the bulk delete confirm")
	}
	body = m.deleteConfirmBody()
	if !strings.Contains(body, "2 marked sessions") {
		t.Fatalf("bulk delete confirm under an active filter = %q, want it to name BOTH members, not just the one the filter shows", body)
	}

	updated, cmd := m.Update(key("enter"))
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("submit returned no command")
	}
	updated, _ = m.Update(cmd())
	_ = updated.(Model)

	if len(invocations) != 2 {
		t.Fatalf("deleteSvc invoked %d time(s), want exactly 2 -- the filtered-out member must not be silently dropped: %#v", len(invocations), invocations)
	}
	for _, sess := range []struct{ id, name string }{{a.ID, "alpha"}, {b.ID, "beta"}} {
		got, err := db.GetSession(ctx, sess.id)
		if err != nil {
			t.Fatalf("GetSession(%s): %v", sess.name, err)
		}
		if got.DeletedAt == 0 {
			t.Errorf("%s.DeletedAt after the destructive branch = 0, want tombstoned -- filtering it out of the sidebar must not exempt it from the batch", sess.name)
		}
	}
}

// TestSettingsGroupDeleteMBranchClearsArchivedMembersToo is task
// 019/R131 part 2's cure regression for the move branch: an archived
// member is invisible to m.baseSessions (SPEC's default, archive-free
// view), so the move loop must read the group's persisted member set
// (including archived rows) rather than that view, or an archived member
// would keep naming a group row that no longer exists.
func TestSettingsGroupDeleteMBranchClearsArchivedMembersToo(t *testing.T) {
	db := openStoreForLastCreateGroup(t)
	ctx := context.Background()
	g, err := db.CreateGroup(ctx, "mixed-group")
	if err != nil {
		t.Fatal(err)
	}
	active := mustCreateSessionInGroup(t, db, "s1", "alpha", g.ID, "stopped")
	archived := mustCreateSessionInGroup(t, db, "s2", "beta", g.ID, "stopped")
	if err := db.ArchiveSession(ctx, archived.ID, 200); err != nil {
		t.Fatal(err)
	}

	m := settingsOpenOnGroupsFields(t, db)
	// m.baseSessions is the SPEC default, archive-free view -- it never
	// holds the archived member, exactly like a live sidebar wouldn't.
	m.baseSessions = []store.Session{active}
	m.sessions = m.baseSessions

	updated, _ := m.Update(key("d"))
	m = updated.(Model)
	if !m.settingsGroupDeleteConfirming {
		t.Fatal("d on a group with a live member (plus an archived one) did not open the confirm")
	}
	body := m.settingsGroupsViewLines(settingsCategories(), settingsCategoryWidth(100), 100-settingsCategoryWidth(100), 20, 24)
	if !strings.Contains(body, "2 session(s)") {
		t.Errorf("groups view = %q, want the archived member counted too", body)
	}

	updated, _ = m.Update(key("m"))
	m = updated.(Model)
	if m.settingsGroupDeleteConfirming {
		t.Fatal("m did not close the confirm sub-mode")
	}

	if names := groupNamesIn(t, db); len(names) != 0 {
		t.Fatalf("ListGroups() = %v after the m branch, want the group row dropped", names)
	}
	for _, sess := range []struct{ id, name string }{{active.ID, "alpha"}, {archived.ID, "beta"}} {
		got, err := db.GetSession(ctx, sess.id)
		if err != nil {
			t.Fatalf("GetSession(%s): %v", sess.name, err)
		}
		if got.GroupID != nil {
			t.Errorf("%s.GroupID after the m branch = %v, want nil (structural default) -- an archived member must be cleared too", sess.name, got.GroupID)
		}
		if got.DeletedAt != 0 {
			t.Errorf("%s.DeletedAt after the m branch = %v, want 0 -- the m branch destroys nothing, archived or not", sess.name, got.DeletedAt)
		}
	}
	if archived.ArchivedAt == 0 {
		fetched, err := db.GetSession(ctx, archived.ID)
		if err != nil {
			t.Fatal(err)
		}
		if fetched.ArchivedAt == 0 {
			t.Fatal("test setup: archived member was never actually archived")
		}
	}
	stillArchived, err := db.GetSession(ctx, archived.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stillArchived.ArchivedAt == 0 {
		t.Error("archived member's ArchivedAt was cleared by the m branch; it must stay archived, only its group_id changes")
	}
}

// TestSettingsGroupDeleteArchivedOnlyGroupStillOpensPrompt is task
// 019/R131 part 2's cure regression for the empty/non-empty decision
// itself: a group whose ONLY member is archived is not empty -- it has
// exactly one member, invisible only to m.baseSessions' archive-free
// default view -- so "d" must still open the two-branch confirm rather
// than silently dropping the group (and orphaning the archived member's
// group_id) with no prompt at all.
func TestSettingsGroupDeleteArchivedOnlyGroupStillOpensPrompt(t *testing.T) {
	db := openStoreForLastCreateGroup(t)
	ctx := context.Background()
	g, err := db.CreateGroup(ctx, "archived-only")
	if err != nil {
		t.Fatal(err)
	}
	archived := mustCreateSessionInGroup(t, db, "s1", "solo", g.ID, "stopped")
	if err := db.ArchiveSession(ctx, archived.ID, 200); err != nil {
		t.Fatal(err)
	}

	m := settingsOpenOnGroupsFields(t, db)
	// The archived member is, correctly, absent from the default view.
	m.baseSessions = nil
	m.sessions = nil

	updated, _ := m.Update(key("d"))
	m = updated.(Model)

	if !m.settingsGroupDeleteConfirming {
		t.Fatal("d on an archived-only group deleted it with no prompt; it has one member (archived) and must open the two-branch confirm")
	}
	if strings.Contains(m.settingsGroupNote, "deleted empty group") {
		t.Errorf("settingsGroupNote = %q, want no \"deleted empty group\" note -- the group is not empty", m.settingsGroupNote)
	}
	body := m.settingsGroupsViewLines(settingsCategories(), settingsCategoryWidth(100), 100-settingsCategoryWidth(100), 20, 24)
	if !strings.Contains(body, "1 session(s)") {
		t.Errorf("groups view = %q, want the archived-only group's one member named in the prompt", body)
	}
	if names := groupNamesIn(t, db); len(names) != 1 {
		t.Fatalf("ListGroups() = %v while the confirm is open, want the group row still standing", names)
	}
}

// TestSettingsGroupDeleteDBranchOneUndoRestoresTheWholeBatch is task
// 019/R131 part 2's cure regression for the undo half of the seam: the
// destructive branch's batch (an active member plus an archived one, so
// the whole point of routing through the ONE shared dd path rather than a
// second implementation is exercised) restores with a SINGLE `u`, exactly
// like any other bulk dd batch -- both rows' tombstones clear together.
func TestSettingsGroupDeleteDBranchOneUndoRestoresTheWholeBatch(t *testing.T) {
	db := openStoreForLastCreateGroup(t)
	ctx := context.Background()
	g, err := db.CreateGroup(ctx, "restore-group")
	if err != nil {
		t.Fatal(err)
	}
	active := mustCreateSessionInGroup(t, db, "s1", "alpha", g.ID, "stopped")
	archived := mustCreateSessionInGroup(t, db, "s2", "beta", g.ID, "stopped")
	if err := db.ArchiveSession(ctx, archived.ID, 200); err != nil {
		t.Fatal(err)
	}

	m := settingsOpenOnGroupsFields(t, db)
	m.settings.DeleteGrace = time.Hour
	m.deleteSvc = func(ctx context.Context, s store.Session) error {
		return db.SoftDeleteSession(ctx, s.ID, 300)
	}
	m.restoreSvc = func(ctx context.Context, id string) (store.Session, error) {
		if err := db.RestoreSession(ctx, id, 400); err != nil {
			return store.Session{}, err
		}
		return db.GetSession(ctx, id)
	}
	// The active member is visible in the default view; the archived one
	// never is -- exactly the shape a live sidebar would hand this in.
	m.baseSessions = []store.Session{active}
	m.sessions = m.baseSessions

	updated, _ := m.Update(key("d"))
	m = updated.(Model)
	updated, _ = m.Update(key("d"))
	m = updated.(Model)
	if !m.deleteConfirming {
		t.Fatal("the destructive d branch did not open the bulk delete confirm")
	}

	updated, cmd := m.Update(key("enter"))
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("submit returned no command")
	}
	updated, _ = m.Update(cmd())
	m = updated.(Model)

	for _, sess := range []struct{ id, name string }{{active.ID, "alpha"}, {archived.ID, "beta"}} {
		got, err := db.GetSession(ctx, sess.id)
		if err != nil {
			t.Fatalf("GetSession(%s): %v", sess.name, err)
		}
		if got.DeletedAt == 0 {
			t.Fatalf("%s.DeletedAt after the batch = 0, want tombstoned", sess.name)
		}
	}
	if len(m.batchDeleteUndoSessionIDs) != 2 {
		t.Fatalf("batchDeleteUndoSessionIDs = %#v, want both members of the batch", m.batchDeleteUndoSessionIDs)
	}

	updated, cmd = m.Update(key("u"))
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("u returned no command")
	}
	updated, _ = m.Update(cmd())
	_ = updated.(Model)

	for _, sess := range []struct{ id, name string }{{active.ID, "alpha"}, {archived.ID, "beta"}} {
		got, err := db.GetSession(ctx, sess.id)
		if err != nil {
			t.Fatalf("GetSession(%s): %v", sess.name, err)
		}
		if got.DeletedAt != 0 {
			t.Errorf("%s.DeletedAt after u = %v, want 0 -- one u must restore the WHOLE batch", sess.name, got.DeletedAt)
		}
	}
	stillArchived, err := db.GetSession(ctx, archived.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stillArchived.ArchivedAt == 0 {
		t.Error("archived member's ArchivedAt was lost across the delete/undo round trip; it must stay archived")
	}
}

// mustCreateClaudeSessionWithTranscript persists a real group member whose
// agent is "claude" and whose declared transcript (internal/agent's
// TranscriptPaths, task 109) actually exists on disk under home -- the
// same fixture shape TestDeleteConfirmPurgeShowsExactPathForClaudeWithATranscript
// (delete_purge_test.go) builds for the single-session confirm, reused
// here so cure-01-05's group-delete regression exercises the SAME
// transcriptPathFor/purgeSvc seam rather than a stand-in. Returns the
// persisted session and the exact absolute transcript path.
func mustCreateClaudeSessionWithTranscript(t *testing.T, db *store.Store, home, id, name string, groupID int64) (store.Session, string) {
	t.Helper()
	cwd := filepath.Join(home, "work-"+id)
	if err := os.MkdirAll(cwd, 0o700); err != nil {
		t.Fatal(err)
	}
	project := strings.ReplaceAll(cwd, string(filepath.Separator), "-")
	transcriptDir := filepath.Join(home, ".claude", "projects", project)
	if err := os.MkdirAll(transcriptDir, 0o700); err != nil {
		t.Fatal(err)
	}
	conversationID := id + "11111111-1111-1111-1111-111111111111"
	if len(conversationID) > 36 {
		conversationID = conversationID[len(conversationID)-36:]
	}
	transcriptPath := filepath.Join(transcriptDir, conversationID+".jsonl")
	if err := os.WriteFile(transcriptPath, []byte(`{"message":"hi"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gid := groupID
	s, err := db.CreateSession(context.Background(), store.CreateSessionInput{
		ID: id, Name: name, CWD: cwd, Agent: "claude", CapturedPath: "/bin",
		Status: "stopped", StatusAt: 100, CreatedAt: 100, GroupID: &gid,
		ConversationID: conversationID,
	})
	if err != nil {
		t.Fatalf("CreateSession(%s): %v", name, err)
	}
	return s, transcriptPath
}

// TestSettingsGroupDeleteDBranchOffersPurgeChoiceAndOneUndoStillRestoresTombstones
// is cure-01-05's own regression (R131/SPEC §11): the destructive "d"
// branch reaches the SAME shared §9.2 bulk dd confirm an ordinary marked-set
// delete does, and that confirm now offers the same non-default purge
// choice the single-session dialog does -- opting out preserves every
// declared transcript, opting in purges only the ELIGIBLE ones (a claude
// member with a real fixture transcript, never the shell member, which has
// none), and either choice still tombstones every member and restores the
// whole batch with one shared u, exactly like the plain-delete regression
// above.
func TestSettingsGroupDeleteDBranchOffersPurgeChoiceAndOneUndoStillRestoresTombstones(t *testing.T) {
	for _, tc := range []struct {
		name           string
		cyclePurge     bool
		wantTranscript bool
	}{
		{name: "keep preserves the eligible transcript", cyclePurge: false, wantTranscript: true},
		{name: "purge removes only the eligible transcript", cyclePurge: true, wantTranscript: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			db, err := store.OpenPath(home, filepath.Join(home, "state.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { db.Close() })
			ctx := context.Background()
			g, err := db.CreateGroup(ctx, "transcript-group")
			if err != nil {
				t.Fatal(err)
			}
			claudeSession, transcriptPath := mustCreateClaudeSessionWithTranscript(t, db, home, "s1", "alpha", g.ID)
			shellSession := mustCreateSessionInGroup(t, db, "s2", "beta", g.ID, "stopped")

			m := settingsOpenOnGroupsFields(t, db)
			m.settings.DeleteGrace = time.Hour
			var purgeCalls []string
			m.purgeSvc = func(_ context.Context, path string) error {
				purgeCalls = append(purgeCalls, path)
				return os.Remove(path)
			}
			m.deleteSvc = func(ctx context.Context, s store.Session) error {
				return db.SoftDeleteSession(ctx, s.ID, 300)
			}
			m.restoreSvc = func(ctx context.Context, id string) (store.Session, error) {
				if err := db.RestoreSession(ctx, id, 400); err != nil {
					return store.Session{}, err
				}
				return db.GetSession(ctx, id)
			}
			m.baseSessions = []store.Session{claudeSession, shellSession}
			m.sessions = m.baseSessions

			updated, _ := m.Update(key("d"))
			m = updated.(Model)
			updated, _ = m.Update(key("d"))
			m = updated.(Model)
			if !m.deleteConfirming {
				t.Fatal("the destructive d branch did not open the bulk delete confirm")
			}
			if m.bulkDeletePurgeValue != "keep" {
				t.Fatalf("bulkDeletePurgeValue = %q on open, want the non-default candidate %q", m.bulkDeletePurgeValue, "keep")
			}
			if tc.cyclePurge {
				updated, _ = m.Update(key("right"))
				m = updated.(Model)
				if m.bulkDeletePurgeValue != "purge" {
					t.Fatalf("right did not cycle bulkDeletePurgeValue to %q: got %q", "purge", m.bulkDeletePurgeValue)
				}
			}
			body := m.deleteConfirmBody()
			if !strings.Contains(body, "2 marked sessions") {
				t.Fatalf("the routed confirm does not name the batch size:\n%s", body)
			}

			updated, cmd := m.Update(key("enter"))
			m = updated.(Model)
			if cmd == nil {
				t.Fatal("submit returned no command")
			}
			updated, _ = m.Update(cmd())
			m = updated.(Model)

			if _, err := os.Stat(transcriptPath); tc.wantTranscript {
				if err != nil {
					t.Fatalf("keep must preserve the eligible transcript %q: %v", transcriptPath, err)
				}
				if len(purgeCalls) != 0 {
					t.Fatalf("keep called the purge service: %#v", purgeCalls)
				}
			} else {
				if err == nil {
					t.Fatalf("purge did not remove the eligible transcript %q", transcriptPath)
				}
				if len(purgeCalls) != 1 || purgeCalls[0] != transcriptPath {
					t.Fatalf("purge calls = %#v, want exactly one call for the eligible transcript %q (never the shell member, which has none)", purgeCalls, transcriptPath)
				}
			}

			for _, sess := range []struct{ id, name string }{{claudeSession.ID, "alpha"}, {shellSession.ID, "beta"}} {
				got, err := db.GetSession(ctx, sess.id)
				if err != nil {
					t.Fatalf("GetSession(%s): %v", sess.name, err)
				}
				if got.DeletedAt == 0 {
					t.Fatalf("%s.DeletedAt after the batch = 0, want tombstoned", sess.name)
				}
			}
			if len(m.batchDeleteUndoSessionIDs) != 2 {
				t.Fatalf("batchDeleteUndoSessionIDs = %#v, want both members of the batch", m.batchDeleteUndoSessionIDs)
			}

			updated, cmd = m.Update(key("u"))
			m = updated.(Model)
			if cmd == nil {
				t.Fatal("u returned no command")
			}
			updated, _ = m.Update(cmd())
			_ = updated.(Model)

			for _, sess := range []struct{ id, name string }{{claudeSession.ID, "alpha"}, {shellSession.ID, "beta"}} {
				got, err := db.GetSession(ctx, sess.id)
				if err != nil {
					t.Fatalf("GetSession(%s): %v", sess.name, err)
				}
				if got.DeletedAt != 0 {
					t.Errorf("%s.DeletedAt after u = %v, want 0 -- the shared one-u restore covers this batch exactly as an ordinary bulk dd does", sess.name, got.DeletedAt)
				}
			}
		})
	}
}

// TestSettingsGroupDeleteDBranchUndoDoesNotRebindOntoAGroupCreatedMeanwhile
// is R128/R130's durable-identity regression for the destructive d branch's
// own undo path: the group row is dropped once the batch commits
// (TestSettingsGroupDeleteDBranchDropsTheGroupRowOnceTheBatchCommits), but
// the tombstoned member rows are left with their old group_id untouched --
// exactly the fixture shape TestSettingsGroupDeleteDBranchOneUndoRestoresTheWholeBatch
// already restores from. This test creates a brand-new, unrelated group in
// the window between the drop and the undo (the deleted group's id was the
// table's only row, the exact shape SQLite's ordinary rowid reuse would
// reissue) and then proves the restored member's GroupName still reads back
// as default, never silently rebound onto the new group just because a
// naive id assignment happened to collide.
func TestSettingsGroupDeleteDBranchUndoDoesNotRebindOntoAGroupCreatedMeanwhile(t *testing.T) {
	db := openStoreForLastCreateGroup(t)
	ctx := context.Background()
	g, err := db.CreateGroup(ctx, "restore-group")
	if err != nil {
		t.Fatal(err)
	}
	member := mustCreateSessionInGroup(t, db, "s1", "alpha", g.ID, "stopped")

	m := settingsOpenOnGroupsFields(t, db)
	m.settings.DeleteGrace = time.Hour
	m.deleteSvc = func(ctx context.Context, s store.Session) error {
		return db.SoftDeleteSession(ctx, s.ID, 300)
	}
	m.restoreSvc = func(ctx context.Context, id string) (store.Session, error) {
		if err := db.RestoreSession(ctx, id, 400); err != nil {
			return store.Session{}, err
		}
		return db.GetSession(ctx, id)
	}
	m.baseSessions = []store.Session{member}
	m.sessions = m.baseSessions

	updated, _ := m.Update(key("d"))
	m = updated.(Model)
	updated, _ = m.Update(key("d"))
	m = updated.(Model)
	if !m.deleteConfirming {
		t.Fatal("the destructive d branch did not open the bulk delete confirm")
	}

	updated, cmd := m.Update(key("enter"))
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("submit returned no command")
	}
	updated, _ = m.Update(cmd())
	m = updated.(Model)

	if names := groupNamesIn(t, db); len(names) != 0 {
		t.Fatalf("groups after the batch commits = %v, want none (the group row drops with the batch)", names)
	}

	// The pressure case: create a brand-new, unrelated group right in the
	// window between the drop and the undo, the deleted group's id having
	// been the only row in the table.
	replacement, err := db.CreateGroup(ctx, "unrelated")
	if err != nil {
		t.Fatal(err)
	}
	if replacement.ID == g.ID {
		t.Fatalf("CreateGroup after the drop reused the deleted group's id: got %d, want anything but %d", replacement.ID, g.ID)
	}

	updated, cmd = m.Update(key("u"))
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("u returned no command")
	}
	updated, _ = m.Update(cmd())
	_ = updated.(Model)

	restored, err := db.GetSession(ctx, member.ID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.DeletedAt != 0 {
		t.Fatalf("%s.DeletedAt after u = %v, want 0 -- one u must restore the batch even with an unrelated group created meanwhile", "alpha", restored.DeletedAt)
	}
	if restored.GroupName != "" {
		t.Fatalf("restored session GroupName = %q, want %q (default) -- it must never rebind onto %q (id %d) just because the deleted group's id happened to collide", restored.GroupName, "", replacement.Name, replacement.ID)
	}
	if restored.GroupID == nil || *restored.GroupID != g.ID {
		t.Fatalf("restored session GroupID = %v, want it to keep pointing at the retired id %d untouched", restored.GroupID, g.ID)
	}
}

// TestSettingsGroupsListLongerThanViewportKeepsSelectionAndEditorVisible
// is cure-01-02-2's own regression proof: settingsGroupsViewLines used to
// build the create/rename input and the delete confirm AFTER every group
// row, then hand the whole slice to fitLines, which just truncates to the
// frame's row budget -- so at 80x24 with enough persisted groups to
// overflow that budget, selecting a group near the end of the list (task
// 118/review2's own reproducer: 24 groups, select group-23) left the
// selected row, and any editor/confirm attached to it, entirely off
// screen while the header/copy above still rendered in full. This test
// drives the real Update/View path (never a bare model-flag assertion)
// over exactly that shape for all three of n/r/d, at the smallest
// supported frame (SPEC requirement 14) plus one taller size, so a
// regression that only shows up at one specific height cannot slip back
// in unnoticed.
func TestSettingsGroupsListLongerThanViewportKeepsSelectionAndEditorVisible(t *testing.T) {
	for _, sz := range []struct{ w, h int }{{80, 24}, {100, 40}} {
		t.Run(fmt.Sprintf("%dx%d", sz.w, sz.h), func(t *testing.T) {
			db := emptyGroupsTestStore(t)
			const total = 24
			var groups []store.Group
			for i := 0; i < total; i++ {
				g, err := db.CreateGroup(context.Background(), fmt.Sprintf("group-%02d", i))
				if err != nil {
					t.Fatal(err)
				}
				groups = append(groups, g)
			}
			last := groups[total-1]

			t.Run("create", func(t *testing.T) {
				m := settingsOpenOnGroupsFields(t, db)
				m.width, m.height = sz.w, sz.h
				for i := 0; i < total-1; i++ {
					updated, _ := m.Update(key("down"))
					m = updated.(Model)
				}
				if g, ok := m.settingsSelectedGroup(); !ok || g.Name != last.Name {
					t.Fatalf("selection after %d downs = %+v (ok=%v), want %q", total-1, g, ok, last.Name)
				}
				updated, _ := m.Update(key("n"))
				m = updated.(Model)
				m = typeIntoGroupEditor(t, m, "new-name")
				view := m.View()
				if !strings.Contains(view, "New group name:") {
					t.Fatalf("create editor label invisible at %dx%d with %d groups selecting the last one:\n%s", sz.w, sz.h, total, view)
				}
				if !strings.Contains(view, "new-name") {
					t.Fatalf("create editor's typed value invisible at %dx%d:\n%s", sz.w, sz.h, view)
				}
			})

			t.Run("rename", func(t *testing.T) {
				m := settingsOpenOnGroupsFields(t, db)
				m.width, m.height = sz.w, sz.h
				for i := 0; i < total-1; i++ {
					updated, _ := m.Update(key("down"))
					m = updated.(Model)
				}
				if g, ok := m.settingsSelectedGroup(); !ok || g.Name != last.Name {
					t.Fatalf("selection after %d downs = %+v (ok=%v), want %q", total-1, g, ok, last.Name)
				}
				updated, _ := m.Update(key("r"))
				m = updated.(Model)
				view := m.View()
				if !strings.Contains(view, "New name:") {
					t.Fatalf("rename editor label invisible at %dx%d with %d groups selecting the last one:\n%s", sz.w, sz.h, total, view)
				}
				if !strings.Contains(view, last.Name) {
					t.Fatalf("rename target/prefill %q invisible at %dx%d:\n%s", last.Name, sz.w, sz.h, view)
				}
			})

			t.Run("delete", func(t *testing.T) {
				m := settingsOpenOnGroupsFields(t, db)
				m.width, m.height = sz.w, sz.h
				for i := 0; i < total-1; i++ {
					updated, _ := m.Update(key("down"))
					m = updated.(Model)
				}
				g, ok := m.settingsSelectedGroup()
				if !ok || g.Name != last.Name {
					t.Fatalf("selection after %d downs = %+v (ok=%v), want %q", total-1, g, ok, last.Name)
				}
				mustCreateSessionInGroup(t, db, "member-of-last", "member-of-last", g.ID, "stopped")
				m.baseSessions = append(m.baseSessions, store.Session{ID: "member-of-last", Name: "member-of-last", GroupID: &g.ID})
				m.sessions = m.baseSessions

				updated, _ := m.Update(key("d"))
				m = updated.(Model)
				if !m.settingsGroupDeleteConfirming {
					t.Fatal("d did not open the delete confirm sub-mode")
				}
				view := m.View()
				want := fmt.Sprintf("Delete group %q", last.Name)
				if !strings.Contains(view, want) {
					t.Fatalf("delete confirm target invisible at %dx%d with %d groups selecting the last one (want %q):\n%s", sz.w, sz.h, total, want, view)
				}
				if !strings.Contains(view, "1 session") {
					t.Fatalf("delete confirm member count invisible at %dx%d:\n%s", sz.w, sz.h, view)
				}
				if !strings.Contains(view, "m moves") || !strings.Contains(view, "d deletes") {
					t.Fatalf("delete confirm did not expose both m/d branches at %dx%d:\n%s", sz.w, sz.h, view)
				}

				// Esc still cancels the whole sub-mode without touching the
				// store (updateSettingsGroupDeleteConfirm's own esc branch).
				updated, _ = m.Update(key("esc"))
				m = updated.(Model)
				if m.settingsGroupDeleteConfirming {
					t.Fatal("esc did not close the delete confirm sub-mode")
				}
				remaining, err := db.ListGroups(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				found := false
				for _, rg := range remaining {
					if rg.ID == g.ID {
						found = true
					}
				}
				if !found {
					t.Fatalf("esc must not have deleted the group %q (id %d), but it is gone from ListGroups", g.Name, g.ID)
				}
			})
		})
	}
}

// TestSettingsGroupsWindowKeepsSelectedBlockWhollyInsideCapacity is
// settingsGroupsWindow's own unit-level proof, independent of any
// particular frame size: for every selected index across a 24-group list
// and every one of the three selectedExtra costs (idle/editing/deleting),
// the window it returns always contains the selected index, stays inside
// [0,total) bounds, and -- whenever the capacity is large enough to hold
// the selected block at all (capacity >= 1+extra; the real Groups panel
// at every supported size, 80x24 included, always has that much room,
// since extra tops out at 3) -- never costs more rows than the capacity
// handed to it. Below that floor there is no window that both includes
// the selection and stays within budget, so settingsGroupsWindow degrades
// to showing just the selected block rather than excluding the selection
// outright; this test does not require the impossible in that region,
// only that the returned range stays well-formed and still contains
// selected.
func TestSettingsGroupsWindowKeepsSelectedBlockWhollyInsideCapacity(t *testing.T) {
	const total = 24
	for _, capacity := range []int{1, 3, 5, 10, 16, total, total + 5} {
		for _, extra := range []int{0, 2, 3} {
			for selected := 0; selected < total; selected++ {
				start, end := settingsGroupsWindow(total, selected, extra, capacity)
				if selected < start || selected >= end {
					t.Fatalf("capacity=%d extra=%d selected=%d: window [%d,%d) excludes the selection", capacity, extra, selected, start, end)
				}
				if start < 0 || end > total || start > end {
					t.Fatalf("capacity=%d extra=%d selected=%d: window [%d,%d) out of [0,%d) bounds", capacity, extra, selected, start, end, total)
				}
				if capacity < 1+extra {
					continue
				}
				cost := (end - start - 1) + (1 + extra)
				if cost > capacity && end-start < total {
					t.Fatalf("capacity=%d extra=%d selected=%d: window [%d,%d) costs %d rows, want <= %d", capacity, extra, selected, start, end, cost, capacity)
				}
			}
		}
	}
}
