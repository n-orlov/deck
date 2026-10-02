package tui

import (
	"context"
	"testing"

	"github.com/n-orlov/deck/internal/config"
)

// TestSettingsGroupRenamePrintableKeyReplacesOfferedNameWhenCommitted drives
// the rename editor only through keys and its committed effect: "r" opens it
// on the group's name, one printable key replaces that offered name (§11.11),
// and enter renames the group to exactly that key.
func TestSettingsGroupRenamePrintableKeyReplacesOfferedNameWhenCommitted(t *testing.T) {
	db := openStoreForLastCreateGroup(t)
	if _, err := db.CreateGroup(context.Background(), "sprint work"); err != nil {
		t.Fatal(err)
	}
	m := settingsOpenOnGroupsFields(t, db)
	for _, k := range []string{"r", "x", "enter"} {
		updated, _ := m.Update(key(k))
		m = updated.(Model)
	}
	groups, err := db.ListGroups(context.Background())
	if err != nil || len(groups) != 1 {
		t.Fatalf("ListGroups = %+v, %v; want one group", groups, err)
	}
	if groups[0].Name != "x" {
		t.Fatalf("group renamed to %q, want %q: a printable key must replace the offered name, not append to it", groups[0].Name, "x")
	}
}

// TestSettingsStringPrintableKeyReplacesOfferedValueWhenCommitted is the same
// rule for the free-text editor: enter opens pre_launch on its staged value,
// one printable key replaces it, and enter stages exactly that key.
func TestSettingsStringPrintableKeyReplacesOfferedValueWhenCommitted(t *testing.T) {
	m := settingsOpenOnStringField(t, config.Settings{File: config.FileConfig{PreLaunch: "echo staged"}}, "pre_launch")
	for _, k := range []string{"enter", "x", "enter"} {
		updated, _ := m.Update(key(k))
		m = updated.(Model)
	}
	if got := m.settingsEdits.PreLaunch; got != "x" {
		t.Fatalf("pre_launch staged as %q, want %q: a printable key must replace the offered value, not append to it", got, "x")
	}
}
