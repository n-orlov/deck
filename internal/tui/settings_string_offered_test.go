package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/n-orlov/deck/internal/config"
)

// settingsStringOfferedModel opens the pre_launch editor on a staged value,
// which the shared editor holds as an offered value (§11.11).
func settingsStringOfferedModel(t *testing.T, value string) Model {
	t.Helper()
	m := settingsOpenOnStringField(t, config.Settings{File: config.FileConfig{PreLaunch: value}}, "pre_launch")
	m.width, m.height = 100, 30
	updated, _ := m.Update(key("enter"))
	m = updated.(Model)
	if !m.settingsStringEditing {
		t.Fatal("enter did not open the free-text editor")
	}
	return m
}

// TestSettingsStringEditorOpensOnTheValueAsAnOfferedValue: the editor starts
// on the staged value, offered rather than accepted.
func TestSettingsStringEditorOpensOnTheValueAsAnOfferedValue(t *testing.T) {
	m := settingsStringOfferedModel(t, "echo staged")
	if got := m.settingsStringEdit.Value(); got != "echo staged" {
		t.Fatalf("editor opened on %q, want the staged value", got)
	}
	if !m.settingsStringEdit.Offered() {
		t.Fatal("the opening value is not an offered value")
	}
}

// TestSettingsStringEditorPrintableKeyReplacesTheOfferedValue is §11.11's
// offered-value rule for the free-text editor: the first printable key
// replaces the whole offered value instead of being appended to it.
func TestSettingsStringEditorPrintableKeyReplacesTheOfferedValue(t *testing.T) {
	m := settingsStringOfferedModel(t, "echo staged")
	updated, _ := m.Update(key("x"))
	m = updated.(Model)
	if got := m.settingsStringEdit.Value(); got != "x" {
		t.Fatalf("after one printable key the value is %q, want %q (the offered value replaced)", got, "x")
	}
	updated, _ = m.Update(key("enter"))
	if got := updated.(Model).settingsEdits.PreLaunch; got != "x" {
		t.Fatalf("enter staged %q, want %q", got, "x")
	}
}

// TestSettingsStringEditorPasteReplacesTheOfferedValue: a bracketed paste
// replaces an offered value as well.
func TestSettingsStringEditorPasteReplacesTheOfferedValue(t *testing.T) {
	m := settingsStringOfferedModel(t, "echo staged")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("pasted"), Paste: true})
	m = updated.(Model)
	if got := m.settingsStringEdit.Value(); got != "pasted" {
		t.Fatalf("after a paste the value is %q, want %q", got, "pasted")
	}
}

// TestSettingsStringEditorCaretKeysAcceptAndEditInPlace: left accepts the
// offer and moves the caret, so the next key inserts mid-value, and the
// space a hook command needs is text, not an activation.
func TestSettingsStringEditorCaretKeysAcceptAndEditInPlace(t *testing.T) {
	m := settingsStringOfferedModel(t, "echo staged")
	for _, k := range []string{"left", "left", "left", "left", "left", "left"} {
		updated, _ := m.Update(key(k))
		m = updated.(Model)
	}
	if m.settingsStringEdit.Offered() {
		t.Fatal("left did not accept the offered value")
	}
	for _, k := range []string{"X", " "} {
		updated, _ := m.Update(key(k))
		m = updated.(Model)
	}
	if got := m.settingsStringEdit.Value(); got != "echo X staged" {
		t.Fatalf("value = %q, want %q", got, "echo X staged")
	}
}
