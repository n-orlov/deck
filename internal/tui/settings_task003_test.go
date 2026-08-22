package tui

import (
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
)

// settingsOpenOnEnvField opens the takeover and moves the field-list focus
// onto config.Schema's [env] field, the same navigation an operator would
// perform by hand (`,` then tab then up/down to the Environment category's
// only field). Every task 003 test below starts from this so it exercises
// the real settingsCategories()/settingsMove() path rather than poking
// settingsCategoryIndex/settingsFieldIndex at values a schema change could
// silently invalidate.
func settingsOpenOnEnvField(t *testing.T, cfg config.Settings) Model {
	t.Helper()
	model := New(nil, cfg, "")
	updated, _ := model.Update(key(","))
	model = updated.(Model)

	categories := settingsCategories()
	catIdx, fieldIdx := -1, -1
	for ci, cat := range categories {
		for fi, f := range cat.Fields {
			if f.FullKey() == "[env]" {
				catIdx, fieldIdx = ci, fi
			}
		}
	}
	if catIdx < 0 {
		t.Fatal("no [env] field found in settingsCategories()")
	}
	model.settingsCategoryIndex = catIdx
	model.settingsFieldIndex = fieldIdx
	model.settingsFocus = settingsFocusFields
	return model
}

// TestSettingsEnvEnterOpensEntriesEditor proves requirement 17's core
// claim: the [env] field is genuinely editable, not display-only. enter on
// the selected [env] field opens the entries list (settingsEnvOpen) rather
// than doing nothing the way it did before task 003 (KindListOfStrings had
// no case in settingsActivateField).
func TestSettingsEnvEnterOpensEntriesEditor(t *testing.T) {
	model := settingsOpenOnEnvField(t, config.Settings{Env: map[string]string{"A": "1"}})

	updated, _ := model.Update(key("enter"))
	m := updated.(Model)
	if !m.settingsEnvOpen {
		t.Fatal("enter on [env] did not open the entries editor")
	}
	if m.settingsEnvEditing {
		t.Fatal("enter on [env] jumped straight into editing an entry; want the entries list first")
	}
}

// TestSettingsEnvAddEntry drives the whole add flow through Model.Update:
// open [env], select the trailing "add entry" row, enter to start typing,
// type a key, tab to the value, type a value, enter to commit. The staged
// settingsEdits.Env must gain the entry and settingsSavedEdits (what ctrl+s
// would have written already) must NOT change — adding stages the edit,
// it does not write config.toml on its own.
func TestSettingsEnvAddEntry(t *testing.T) {
	model := settingsOpenOnEnvField(t, config.Settings{Env: map[string]string{"EXISTING": "x"}})

	updated, _ := model.Update(key("enter")) // open entries list
	m := updated.(Model)

	// Move onto the trailing "add entry" row (index == len(keys)).
	keys := settingsEnvKeys(m.settingsEdits)
	for i := 0; i < len(keys); i++ {
		updated, _ = m.Update(key("down"))
		m = updated.(Model)
	}
	if m.settingsEnvIndex != len(keys) {
		t.Fatalf("settingsEnvIndex = %d, want %d (the add-entry row)", m.settingsEnvIndex, len(keys))
	}

	updated, _ = m.Update(key("enter")) // start typing a new entry
	m = updated.(Model)
	if !m.settingsEnvEditing || !m.settingsEnvEditingKeyPart {
		t.Fatal("enter on the add-entry row did not start typing a new entry's key")
	}

	updated, _ = m.Update(key("NEW_VAR"))
	m = updated.(Model)
	updated, _ = m.Update(key("tab"))
	m = updated.(Model)
	if m.settingsEnvEditingKeyPart {
		t.Fatal("tab did not move focus from key to value")
	}
	updated, _ = m.Update(key("hello"))
	m = updated.(Model)
	updated, _ = m.Update(key("enter")) // commit
	m = updated.(Model)

	if m.settingsEnvEditing {
		t.Fatal("enter on the value did not leave entry-editing mode")
	}
	if !m.settingsEnvOpen {
		t.Fatal("committing an entry should return to the entries list, not close it")
	}
	if got := m.settingsEdits.Env["NEW_VAR"]; got != "hello" {
		t.Fatalf(`settingsEdits.Env["NEW_VAR"] = %q, want "hello"`, got)
	}
	if got := m.settingsEdits.Env["EXISTING"]; got != "x" {
		t.Fatalf("adding a new entry disturbed the existing one: %q", got)
	}
	if _, staged := m.settingsSavedEdits.Env["NEW_VAR"]; staged {
		t.Fatal("adding an entry wrote it to settingsSavedEdits (i.e. as if saved) before ctrl+s")
	}
}

// TestSettingsEnvEditExistingValue proves changing an existing entry's
// value (not just adding a brand new key) works: open [env], enter on the
// one existing entry, tab to (or start on) the value, retype it, commit.
func TestSettingsEnvEditExistingValue(t *testing.T) {
	model := settingsOpenOnEnvField(t, config.Settings{Env: map[string]string{"A": "old"}})

	updated, _ := model.Update(key("enter")) // open entries list, on "A"
	m := updated.(Model)
	if m.settingsEnvIndex != 0 {
		t.Fatalf("settingsEnvIndex = %d, want 0 (the only entry)", m.settingsEnvIndex)
	}

	updated, _ = m.Update(key("enter")) // start editing "A"
	m = updated.(Model)
	if !m.settingsEnvEditing {
		t.Fatal("enter on an existing entry did not start editing it")
	}
	if m.settingsEnvEditingKeyPart {
		t.Fatal("editing an existing entry should start on its value, not its key")
	}
	if m.settingsEnvEditKey != "A" || m.settingsEnvEditValue != "old" {
		t.Fatalf("editing buffers = key %q value %q, want A/old", m.settingsEnvEditKey, m.settingsEnvEditValue)
	}

	// Clear the seeded value before typing the replacement (backspace
	// three times for "old").
	for i := 0; i < 3; i++ {
		updated, _ = m.Update(key("backspace"))
		m = updated.(Model)
	}
	updated, _ = m.Update(key("new"))
	m = updated.(Model)
	updated, _ = m.Update(key("enter")) // commit
	m = updated.(Model)

	if got := m.settingsEdits.Env["A"]; got != "new" {
		t.Fatalf(`settingsEdits.Env["A"] = %q, want "new"`, got)
	}
	if len(m.settingsEdits.Env) != 1 {
		t.Fatalf("editing a value changed the entry count to %d, want 1", len(m.settingsEdits.Env))
	}
}

// TestSettingsEnvRemoveEntry proves "-" removes the selected entry from
// the staged edits, and that the trailing add-entry row is unaffected (you
// cannot remove "add entry" itself).
func TestSettingsEnvRemoveEntry(t *testing.T) {
	model := settingsOpenOnEnvField(t, config.Settings{Env: map[string]string{"A": "1", "B": "2"}})

	updated, _ := model.Update(key("enter")) // open entries list, on "A" (sorted first)
	m := updated.(Model)
	if got := settingsEnvKeys(m.settingsEdits); len(got) != 2 || got[0] != "A" {
		t.Fatalf("settingsEnvKeys = %v, want [A B]", got)
	}

	updated, _ = m.Update(key("-"))
	m = updated.(Model)
	if _, ok := m.settingsEdits.Env["A"]; ok {
		t.Fatal(`"-" did not remove "A"`)
	}
	if got := m.settingsEdits.Env["B"]; got != "2" {
		t.Fatalf(`removing "A" disturbed "B": %q`, got)
	}
	if !m.settingsEnvOpen {
		t.Fatal(`"-" on an entry unexpectedly closed the entries editor`)
	}

	// Removing "B" too must land cleanly on the add-entry row, not panic
	// or go out of range.
	updated, _ = m.Update(key("-"))
	m = updated.(Model)
	if len(m.settingsEdits.Env) != 0 {
		t.Fatalf("settingsEdits.Env = %v, want empty", m.settingsEdits.Env)
	}
	if m.settingsEnvIndex != 0 {
		t.Fatalf("settingsEnvIndex after emptying = %d, want 0 (the sole add-entry row)", m.settingsEnvIndex)
	}
}

// TestSettingsEnvEscCancelsEditWithoutStaging proves esc while typing a
// key/value abandons the buffer: the entry is not staged, and the entries
// list is left exactly as it was.
func TestSettingsEnvEscCancelsEditWithoutStaging(t *testing.T) {
	model := settingsOpenOnEnvField(t, config.Settings{Env: map[string]string{"A": "1"}})

	updated, _ := model.Update(key("enter")) // open entries list, on "A"
	m := updated.(Model)
	updated, _ = m.Update(key("enter")) // start editing "A"'s value
	m = updated.(Model)
	updated, _ = m.Update(key("Z"))
	m = updated.(Model)

	updated, _ = m.Update(key("esc"))
	m = updated.(Model)
	if m.settingsEnvEditing {
		t.Fatal("esc did not leave entry-editing mode")
	}
	if !m.settingsEnvOpen {
		t.Fatal("esc while editing an entry closed the whole entries list, not just the edit")
	}
	if got := m.settingsEdits.Env["A"]; got != "1" {
		t.Fatalf(`esc staged a change anyway: settingsEdits.Env["A"] = %q, want unchanged "1"`, got)
	}
}

// TestSettingsEnvEscFromEntriesListReturnsToFieldList proves esc from the
// entries list itself (not mid-edit) leaves settingsEnvOpen and returns
// focus to the field list, without touching settingsOpen (requirement 17
// must not accidentally let [env] editing bypass the takeover's own
// esc/discard-confirm contract for the takeover as a whole).
func TestSettingsEnvEscFromEntriesListReturnsToFieldList(t *testing.T) {
	model := settingsOpenOnEnvField(t, config.Settings{Env: map[string]string{"A": "1"}})

	updated, _ := model.Update(key("enter")) // open entries list
	m := updated.(Model)

	updated, _ = m.Update(key("esc"))
	m = updated.(Model)
	if m.settingsEnvOpen {
		t.Fatal("esc from the entries list did not close it")
	}
	if !m.settingsOpen {
		t.Fatal("esc from the [env] entries list closed the whole takeover, want just the entries list")
	}
}

// TestTui049UnavailableActionListUntouched is a companion check for task
// 003's own success criteria ("nothing is removed from
// internal/tui/tui_test.go:49's unavailable-action list... no per-session
// env editor is built"): it re-asserts the exact phrase the help view must
// still refuse to advertise, "env editor", so a future edit to that list
// cannot silently drop it without a test noticing. This does not replace
// git-diff review of the list, only backs it up.
func TestTui049UnavailableActionListUntouched(t *testing.T) {
	model := New(nil, config.Settings{Socket: "test-socket"}, "")
	model.width = 100
	model.help = true
	if got := model.View(); strings.Contains(got, "env editor") {
		t.Fatalf("help view advertises the unavailable per-session env editor:\n%s", got)
	}
}
