package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/n-orlov/deck/internal/store"
)

// settingsRenameModel opens the settings Groups panel with one group named
// name and presses "r" on it, so the rename editor holds that name as an
// offered value (§11.11).
func settingsRenameModel(t *testing.T, name string) (Model, *store.Store) {
	t.Helper()
	db := openStoreForLastCreateGroup(t)
	if _, err := db.CreateGroup(context.Background(), name); err != nil {
		t.Fatal(err)
	}
	m := settingsOpenOnGroupsFields(t, db)
	m.width, m.height = 100, 30
	updated, _ := m.Update(key("r"))
	m = updated.(Model)
	if !m.settingsGroupRenaming {
		t.Fatal("r did not open the group-rename editor")
	}
	return m, db
}

// TestSettingsGroupRenameOpensOnTheNameAsAnOfferedValue: the rename editor
// starts on the current name, offered, so it is drawn as an unaccepted offer.
func TestSettingsGroupRenameOpensOnTheNameAsAnOfferedValue(t *testing.T) {
	m, _ := settingsRenameModel(t, "sprint work")
	if got := m.settingsGroupEdit.Value(); got != "sprint work" {
		t.Fatalf("editor opened on %q, want the group's name", got)
	}
	if !m.settingsGroupEdit.Offered() {
		t.Fatal("the opening name is not an offered value")
	}
}

// TestSettingsGroupRenamePrintableKeyReplacesTheOfferedName is §11.11's
// offered-value rule for the group-name editor: the first printable key
// replaces the whole offered name instead of being appended to it.
func TestSettingsGroupRenamePrintableKeyReplacesTheOfferedName(t *testing.T) {
	m, _ := settingsRenameModel(t, "sprint work")
	updated, _ := m.Update(key("x"))
	m = updated.(Model)
	if got := m.settingsGroupEdit.Value(); got != "x" {
		t.Fatalf("after one printable key the name is %q, want %q (the offered value replaced)", got, "x")
	}
	if m.settingsGroupEdit.Offered() {
		t.Fatal("the value is still offered after a printable key replaced it")
	}
}

// TestSettingsGroupRenamePasteReplacesTheOfferedName: a bracketed paste
// replaces an offered name as well.
func TestSettingsGroupRenamePasteReplacesTheOfferedName(t *testing.T) {
	m, _ := settingsRenameModel(t, "sprint work")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("pasted"), Paste: true})
	m = updated.(Model)
	if got := m.settingsGroupEdit.Value(); got != "pasted" {
		t.Fatalf("after a paste the name is %q, want %q", got, "pasted")
	}
}

// TestSettingsGroupRenameCaretKeyAcceptsAndEditsInPlace: left accepts the
// offer and moves the caret, so the next key inserts mid-name.
func TestSettingsGroupRenameCaretKeyAcceptsAndEditsInPlace(t *testing.T) {
	m, db := settingsRenameModel(t, "sprint work")
	updated, _ := m.Update(key("left"))
	m = updated.(Model)
	if m.settingsGroupEdit.Offered() {
		t.Fatal("left did not accept the offered name")
	}
	updated, _ = m.Update(key("X"))
	m = updated.(Model)
	if got := m.settingsGroupEdit.Value(); got != "sprint worXk" {
		t.Fatalf("name = %q, want %q (inserted before the last rune)", got, "sprint worXk")
	}
	updated, _ = m.Update(key("backspace"))
	m = updated.(Model)
	if got := m.settingsGroupEdit.Value(); got != "sprint work" {
		t.Fatalf("name = %q after backspace, want %q", got, "sprint work")
	}
	updated, _ = m.Update(key("enter"))
	m = updated.(Model)
	groups, err := db.ListGroups(context.Background())
	if err != nil || len(groups) != 1 || groups[0].Name != "sprint work" {
		t.Fatalf("ListGroups = %+v, %v; want the one group unchanged", groups, err)
	}
}

// TestSettingsGroupCreateOpensEmptyAndEditsInPlace: "n" opens an empty,
// unoffered editor whose caret keys work.
func TestSettingsGroupCreateOpensEmptyAndEditsInPlace(t *testing.T) {
	db := openStoreForLastCreateGroup(t)
	m := settingsOpenOnGroupsFields(t, db)
	updated, _ := m.Update(key("n"))
	m = updated.(Model)
	m = typeIntoGroupEditor(t, m, "ac")
	updated, _ = m.Update(key("left"))
	m = updated.(Model)
	m = typeIntoGroupEditor(t, m, "b")
	if got := m.settingsGroupEdit.Value(); got != "abc" {
		t.Fatalf("name = %q, want %q", got, "abc")
	}
}

// TestSettingsGroupEditDrawsTheCaretNotAnUnderscore: the typed row carries the
// editor's caret (reverse video) and no trailing "_" stand-in.
func TestSettingsGroupEditDrawsTheCaretNotAnUnderscore(t *testing.T) {
	m, _ := settingsRenameModel(t, "sprint")
	updated, _ := m.Update(key("left"))
	m = updated.(Model)
	view := m.View()
	if !strings.Contains(view, "\x1b[7m") {
		t.Fatalf("no reverse-video caret in the rename row:\n%s", view)
	}
	if plain := withoutCaretSGR(view); !strings.Contains(plain, "New name:  sprint") || strings.Contains(plain, "sprint_") {
		t.Fatalf("rename row does not draw the name with the caret and no underscore:\n%s", plain)
	}
}
