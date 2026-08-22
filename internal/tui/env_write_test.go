package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// TestListShowsEnvDirtyBadgeOnlyWhenSet proves the `env↻` sidebar badge
// (task 021, SPEC §6.1/§6.3) tracks session.EnvDirty exactly: present on a
// dirty row, absent on an otherwise identical clean one.
func TestListShowsEnvDirtyBadgeOnlyWhenSet(t *testing.T) {
	model := New(nil, config.Settings{}, "")
	model.sessions = []store.Session{
		{Name: "edited-session", Agent: "claude", Status: "running", PermissionProfile: "safe", EnvDirty: true},
		{Name: "clean-session", Agent: "claude", Status: "running", PermissionProfile: "safe", EnvDirty: false},
	}
	view := model.View()
	lines := strings.Split(view, "\n")
	// Each session is a two-line row (name/status on the first, the
	// profile/env badges and "created ..." on the second) -- the badge
	// belongs to the row's SECOND line, never the one carrying the name.
	foundDirtyRow, foundCleanRow := false, false
	for i, line := range lines {
		if strings.Contains(line, "edited-session") {
			foundDirtyRow = true
			if i+1 >= len(lines) || !strings.Contains(lines[i+1], "env\u21bb") {
				t.Fatalf("dirty row's second line missing the env-dirty badge:\n%s", view)
			}
		}
		if strings.Contains(line, "clean-session") {
			foundCleanRow = true
			if i+1 < len(lines) && strings.Contains(lines[i+1], "env\u21bb") {
				t.Fatalf("clean row's second line unexpectedly shows the env-dirty badge:\n%s", view)
			}
		}
	}
	if !foundDirtyRow {
		t.Fatalf("dirty row not found on screen at all:\n%s", view)
	}
	if !foundCleanRow {
		t.Fatalf("clean row not found on screen at all:\n%s", view)
	}
}

// TestEnvEditorEditsAKeyThroughSetSessionEnvAndStaysOpen drives the full
// browse -> open -> type -> commit sequence (task 021) directly against
// updateEnvDialog/submitEnvEdit, proving: enter opens the highlighted row
// preloaded with its current value; backspace/typed runes edit a local
// buffer, never the session directly; enter dispatches exactly one call to
// m.setSessionEnv with the session id, key and typed value; and, unlike
// profileSwitched/resumeModeChanged, a successful envEdited leaves the
// dialog open (SPEC §6.1/§6.3's editor lists several keys at once).
func TestEnvEditorEditsAKeyThroughSetSessionEnvAndStaysOpen(t *testing.T) {
	model := New(nil, config.Settings{}, "")
	model.sessions = []store.Session{{ID: "s1", Name: "sess", Agent: "claude", Env: map[string]string{"K": "before"}}}
	model.envEditing = true

	var calledID, calledKey, calledValue string
	var calls int
	model.setSessionEnv = func(_ context.Context, id, key, value string) (store.Session, error) {
		calls++
		calledID, calledKey, calledValue = id, key, value
		return store.Session{ID: id, Name: "sess", Agent: "claude", Env: map[string]string{"K": value}, EnvDirty: true}, nil
	}

	next, _ := model.updateEnvDialog(tea.KeyMsg{Type: tea.KeyEnter})
	m := next.(Model)
	if m.envEditKey != "K" || m.envEditValue != "before" {
		t.Fatalf("enter on the highlighted row = key %q value %q, want K/before", m.envEditKey, m.envEditValue)
	}

	for len(m.envEditValue) > 0 {
		next, _ = m.updateEnvDialog(tea.KeyMsg{Type: tea.KeyBackspace})
		m = next.(Model)
	}
	next, _ = m.updateEnvDialog(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("after")})
	m = next.(Model)
	if m.envEditValue != "after" {
		t.Fatalf("typed buffer = %q, want after", m.envEditValue)
	}

	next, cmd := m.updateEnvDialog(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.envEditKey != "" {
		t.Fatalf("envEditKey not cleared immediately on submit, got %q", m.envEditKey)
	}
	if cmd == nil {
		t.Fatal("submitting an edit produced no command")
	}
	msg := cmd()
	edited, ok := msg.(envEdited)
	if !ok {
		t.Fatalf("submit command produced %T, want envEdited", msg)
	}
	if edited.err != nil {
		t.Fatalf("envEdited err = %v", edited.err)
	}
	if calls != 1 || calledID != "s1" || calledKey != "K" || calledValue != "after" {
		t.Fatalf("setSessionEnv called %d time(s) with (%q,%q,%q), want exactly one call with (s1,K,after)", calls, calledID, calledKey, calledValue)
	}

	updatedModel, _ := m.Update(edited)
	final := updatedModel.(Model)
	if !final.envEditing {
		t.Fatalf("env editor closed after a successful edit; want it to stay open for further edits")
	}
	if final.envNote != "" {
		t.Fatalf("envNote = %q after a successful edit, want empty", final.envNote)
	}
}

// TestEnvEditorEscCancelsEditWithoutClosingThenClosesTheDialog proves the
// two distinct esc scopes: the first esc while typing only abandons that
// one edit (the row's original value is never touched, and the dialog
// stays open), and a second esc -- now that nothing is being edited --
// closes the whole dialog, exactly as SPEC §11.4 states for every dialog.
func TestEnvEditorEscCancelsEditWithoutClosingThenClosesTheDialog(t *testing.T) {
	model := New(nil, config.Settings{}, "")
	model.sessions = []store.Session{{ID: "s1", Name: "sess", Agent: "claude", Env: map[string]string{"K": "before"}}}
	model.envEditing = true
	model.setSessionEnv = func(context.Context, string, string, string) (store.Session, error) {
		t.Fatal("esc must never call setSessionEnv")
		return store.Session{}, nil
	}

	next, _ := model.updateEnvDialog(tea.KeyMsg{Type: tea.KeyEnter})
	m := next.(Model)
	next, _ = m.updateEnvDialog(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m = next.(Model)

	next, _ = m.updateEnvDialog(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if !m.envEditing {
		t.Fatalf("esc while typing closed the whole dialog; want only the edit cancelled")
	}
	if m.envEditKey != "" || m.envEditValue != "" {
		t.Fatalf("esc while typing left edit state behind: key=%q value=%q", m.envEditKey, m.envEditValue)
	}

	next, _ = m.updateEnvDialog(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if m.envEditing {
		t.Fatalf("esc with nothing being edited did not close the dialog")
	}

	got, err := model.sessions[0], error(nil)
	if got.Env["K"] != "before" || err != nil {
		t.Fatalf("session env changed by an abandoned edit: %+v", got.Env)
	}
}

// TestEnvEditorSubmitWithoutSetterStatesUnavailable proves a nil
// m.setSessionEnv (no envSetter wired) refuses a commit with a stated
// reason rather than silently doing nothing (mirroring
// profileSwitchView/pinView's own "unavailable" notes).
func TestEnvEditorSubmitWithoutSetterStatesUnavailable(t *testing.T) {
	model := New(nil, config.Settings{}, "")
	model.sessions = []store.Session{{ID: "s1", Name: "sess", Agent: "claude", Env: map[string]string{"K": "before"}}}
	model.envEditing = true

	next, _ := model.updateEnvDialog(tea.KeyMsg{Type: tea.KeyEnter})
	m := next.(Model)
	next, cmd := m.updateEnvDialog(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if cmd != nil {
		t.Fatalf("submitting with no envSetter wired produced a command; want none")
	}
	if m.envNote == "" || !strings.Contains(m.envNote, "unavailable") {
		t.Fatalf("envNote = %q, want it to state editing is unavailable", m.envNote)
	}
}

// TestEnvEditorFirstTypedRuneReplacesThePrefilledValueWholesale proves the
// cwd-field-style prefill (task 021): the buffer opens holding the row's
// current value, but the very first typed rune replaces it wholesale
// rather than appending to it -- exactly like createView's own cwd field
// -- while a second batch of typed runes appends normally.
func TestEnvEditorFirstTypedRuneReplacesThePrefilledValueWholesale(t *testing.T) {
	model := New(nil, config.Settings{}, "")
	model.sessions = []store.Session{{ID: "s1", Name: "sess", Agent: "claude", Env: map[string]string{"K": "before"}}}
	model.envEditing = true

	next, _ := model.updateEnvDialog(tea.KeyMsg{Type: tea.KeyEnter})
	m := next.(Model)
	if m.envEditValue != "before" || !m.envEditPrefilled {
		t.Fatalf("opening for edit = value %q prefilled %v, want before/true", m.envEditValue, m.envEditPrefilled)
	}

	next, _ = m.updateEnvDialog(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("after")})
	m = next.(Model)
	if m.envEditValue != "after" {
		t.Fatalf("first typed rune run = %q, want it to replace the prefill wholesale (after)", m.envEditValue)
	}
	if m.envEditPrefilled {
		t.Fatalf("envEditPrefilled still true after the first typed rune")
	}

	next, _ = m.updateEnvDialog(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	m = next.(Model)
	if m.envEditValue != "after2" {
		t.Fatalf("second typed rune run = %q, want it appended (after2)", m.envEditValue)
	}
}
