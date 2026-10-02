package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/n-orlov/deck/internal/config"
)

// press feeds keys to a settings model in order.
func settingsPress(m Model, keys ...string) Model {
	for _, k := range keys {
		updated, _ := m.Update(key(k))
		m = updated.(Model)
	}
	return m
}

// TestSettingsEnvValueLeftMovesTheCaretWhenCommitted drives the [env] value
// editor only through keys and its committed effect: left moves the caret in
// the value being typed (it does not switch lists), so a key typed after it
// lands inside the text.
func TestSettingsEnvValueLeftMovesTheCaretWhenCommitted(t *testing.T) {
	m := settingsOpenOnEnvField(t, config.Settings{Env: map[string]string{"A": "old"}})
	m = settingsPress(m, "enter", "enter", "abc", "left", "X", "enter")
	if got := m.settingsEdits.Env["A"]; got != "abXc" {
		t.Fatalf("value committed as %q, want %q: left must move the caret inside the value", got, "abXc")
	}
}

// TestSettingsEnvKeyLeftMovesTheCaretWhenCommitted is the same for the entry's
// key, which a new entry opens on.
func TestSettingsEnvKeyLeftMovesTheCaretWhenCommitted(t *testing.T) {
	m := settingsOpenOnEnvField(t, config.Settings{Env: map[string]string{}})
	m = settingsPress(m, "enter", "enter", "AB", "left", "X", "enter", "v", "enter")
	if got, ok := m.settingsEdits.Env["AXB"]; !ok || got != "v" {
		t.Fatalf("env = %v, want AXB=v: left must move the caret inside the key", m.settingsEdits.Env)
	}
}

// TestSettingsEnvEditingKeepsListsWhileTyping: tab swaps key and value, and
// left/right never switch the takeover's lists while an entry is being typed.
func TestSettingsEnvEditingKeepsListsWhileTyping(t *testing.T) {
	m := settingsOpenOnEnvField(t, config.Settings{Env: map[string]string{"A": "old"}})
	m = settingsPress(m, "enter", "enter")
	focus := m.settingsFocus
	m = settingsPress(m, "left", "right", "left")
	if m.settingsFocus != focus || !m.settingsEnvEditing || m.settingsEnvEditingKeyPart {
		t.Fatalf("left/right while typing: focus %d->%d editing=%v keyPart=%v", focus, m.settingsFocus, m.settingsEnvEditing, m.settingsEnvEditingKeyPart)
	}
	m = settingsPress(m, "tab")
	if !m.settingsEnvEditingKeyPart || m.settingsFocus != focus {
		t.Fatalf("tab while typing must swap key/value only: keyPart=%v focus=%d", m.settingsEnvEditingKeyPart, m.settingsFocus)
	}
}

// TestSettingsEnvValueOpensOfferedAndPrintableKeyReplacesIt: the existing
// value is offered (§11.11), so a printable key replaces it, and a bracketed
// paste replaces it too.
func TestSettingsEnvValueOpensOfferedAndPrintableKeyReplacesIt(t *testing.T) {
	m := settingsOpenOnEnvField(t, config.Settings{Env: map[string]string{"A": "old"}})
	if got := settingsPress(m, "enter", "enter", "x", "enter").settingsEdits.Env["A"]; got != "x" {
		t.Fatalf("value committed as %q, want %q (offered value replaced)", got, "x")
	}
	m = settingsPress(m, "enter", "enter")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("pasted"), Paste: true})
	m = settingsPress(updated.(Model), "enter")
	if got := m.settingsEdits.Env["A"]; got != "pasted" {
		t.Fatalf("value after a paste = %q, want %q", got, "pasted")
	}
}

// TestSettingsEnvValueEditorDrawsOneCaretOnTheFocusedField: the focused field
// draws the reverse-video caret, the other none, and a secret-shaped value
// stays masked.
func TestSettingsEnvValueEditorDrawsOneCaretOnTheFocusedField(t *testing.T) {
	m := settingsOpenOnEnvField(t, config.Settings{Env: map[string]string{"API_TOKEN": "hunter2"}})
	m.width, m.height = 100, 30
	m = settingsPress(m, "enter", "enter", "left")
	view := m.View()
	if strings.Contains(view, "hunter2") {
		t.Fatalf("a secret-shaped value is drawn in the clear:\n%s", view)
	}
	if got := strings.Count(view, "\x1b[7m"); got != 1 {
		t.Fatalf("%d carets drawn, want exactly one (the focused field's)", got)
	}
}

// TestSettingsMainKeysStillSwitchListsWhenNothingIsTyped: outside any text
// field, tab, left and right still switch the two lists.
func TestSettingsMainKeysStillSwitchListsWhenNothingIsTyped(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	m.settingsOpen = true
	start := m.settingsFocus
	for _, k := range []string{"tab", "left", "right"} {
		before := m.settingsFocus
		m = settingsPress(m, k)
		if m.settingsFocus == before {
			t.Fatalf("%s did not switch lists (focus stayed %d, started %d)", k, before, start)
		}
	}
}
