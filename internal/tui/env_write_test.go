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
	// profile/env badges and the bare age on the second, task 012/R120) --
	// the badge
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
	if m.envEditKey != "K" || m.envEdit.Value() != "before" {
		t.Fatalf("enter on the highlighted row = key %q value %q, want K/before", m.envEditKey, m.envEdit.Value())
	}

	for len(m.envEdit.Value()) > 0 {
		next, _ = m.updateEnvDialog(tea.KeyMsg{Type: tea.KeyBackspace})
		m = next.(Model)
	}
	next, _ = m.updateEnvDialog(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("after")})
	m = next.(Model)
	if m.envEdit.Value() != "after" {
		t.Fatalf("typed buffer = %q, want after", m.envEdit.Value())
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
	if m.envEditKey != "" || m.envEdit.Value() != "" {
		t.Fatalf("esc while typing left edit state behind: key=%q value=%q", m.envEditKey, m.envEdit.Value())
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

// envEditOpened returns a model with the env editor open on key K (value
// "before") and enter already pressed on that row.
func envEditOpened(t *testing.T) Model {
	t.Helper()
	model := New(nil, config.Settings{}, "")
	model.sessions = []store.Session{{ID: "s1", Name: "sess", Agent: "claude", Env: map[string]string{"K": "before"}}}
	model.envEditing = true
	next, _ := model.updateEnvDialog(tea.KeyMsg{Type: tea.KeyEnter})
	return next.(Model)
}

func envKey(m Model, k tea.KeyMsg) Model {
	next, _ := m.updateEnvDialog(k)
	return next.(Model)
}

// TestEnvEditorOpensOnTheCurrentValueAsAnOfferedValue proves the opening
// value is an offered value (§11.11): the whole text, caret at its end.
func TestEnvEditorOpensOnTheCurrentValueAsAnOfferedValue(t *testing.T) {
	m := envEditOpened(t)
	if m.envEdit.Value() != "before" || !m.envEdit.Offered() {
		t.Fatalf("opening for edit = value %q offered %v, want before/true", m.envEdit.Value(), m.envEdit.Offered())
	}
	if m.envEdit.Caret() != len("before") {
		t.Fatalf("caret = %d, want it at the end (%d)", m.envEdit.Caret(), len("before"))
	}
}

// TestEnvEditorPrintableKeyReplacesTheOfferedValueThenAppends proves a
// printable key replaces the offered value wholesale, and a second batch of
// runes is then ordinary typing at the caret.
func TestEnvEditorPrintableKeyReplacesTheOfferedValueThenAppends(t *testing.T) {
	m := envEditOpened(t)
	m = envKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("after")})
	if m.envEdit.Value() != "after" || m.envEdit.Offered() {
		t.Fatalf("first typed run = %q offered %v, want it to replace the offer (after, not offered)", m.envEdit.Value(), m.envEdit.Offered())
	}
	m = envKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	if m.envEdit.Value() != "after2" {
		t.Fatalf("second typed run = %q, want after2", m.envEdit.Value())
	}
}

// TestEnvEditorBackspaceAcceptsTheOfferedValueAndDeletesOneCharacter proves
// backspace on an offered value accepts it and deletes only its last
// character, rather than clearing it wholesale.
func TestEnvEditorBackspaceAcceptsTheOfferedValueAndDeletesOneCharacter(t *testing.T) {
	m := envEditOpened(t)
	m = envKey(m, tea.KeyMsg{Type: tea.KeyBackspace})
	if m.envEdit.Value() != "befor" || m.envEdit.Offered() {
		t.Fatalf("backspace on the offer = %q offered %v, want befor/not offered", m.envEdit.Value(), m.envEdit.Offered())
	}
}

// TestEnvEditorEditsInTheMiddleOfTheValue proves the field has a real
// caret: left steps into the accepted value and typing inserts there.
func TestEnvEditorEditsInTheMiddleOfTheValue(t *testing.T) {
	m := envEditOpened(t)
	for i := 0; i < 3; i++ {
		m = envKey(m, tea.KeyMsg{Type: tea.KeyLeft})
	}
	m = envKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("-")})
	if m.envEdit.Value() != "bef-ore" {
		t.Fatalf("value = %q, want bef-ore", m.envEdit.Value())
	}
}

// TestEnvEditorDrawsACaretNotAnUnderscore proves the edit prompt carries the
// shared editor's reverse-video caret and no `_` stand-in.
func TestEnvEditorDrawsACaretNotAnUnderscore(t *testing.T) {
	m := envEditOpened(t)
	m = envKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ab")})
	_, value := m.envEditPromptLine()
	if strings.Contains(value, "_") {
		t.Fatalf("edit prompt value %q draws an underscore stand-in", value)
	}
	if !strings.Contains(value, "\x1b[7m") {
		t.Fatalf("edit prompt value %q has no reverse-video caret", value)
	}
}

// TestEnvEditorMaskedSecretStaysMaskedWhileEditing proves a secret-shaped
// key's text is never drawn while editing until reveal is on.
func TestEnvEditorMaskedSecretStaysMaskedWhileEditing(t *testing.T) {
	model := New(nil, config.Settings{}, "")
	model.sessions = []store.Session{{ID: "s1", Name: "sess", Agent: "claude", Env: map[string]string{"API_TOKEN": "hunter2"}}}
	model.envEditing = true
	m := envKey(model, tea.KeyMsg{Type: tea.KeyEnter})
	m = envKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("swordfish")})
	_, value := m.envEditPromptLine()
	if strings.Contains(value, "swordfish") || strings.Contains(value, "hunter2") {
		t.Fatalf("masked edit prompt leaks the value: %q", value)
	}
	m.envReveal = true
	_, value = m.envEditPromptLine()
	if !strings.Contains(value, "swordfish") {
		t.Fatalf("revealed edit prompt %q does not show the typed value", value)
	}
}
