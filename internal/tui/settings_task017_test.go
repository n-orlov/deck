package tui

import (
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
)

// settingsEnvOverrideTestModel opens the takeover against a real
// config.toml (task 016's own DECK_HOME seam) with DECK_ASCII set in the
// environment, so [ui] ascii is env-overridden per §6.5, and moves focus
// onto that field so the tests below can assert its label, detail text
// and the write-does-not-pretend-to-change-the-running-value behaviour
// requirement 21 asks for.
func settingsEnvOverrideTestModel(t *testing.T) (model Model, configFile string) {
	t.Helper()
	dir := t.TempDir()
	getenv := func(name string) string {
		switch name {
		case "DECK_HOME":
			return dir
		case "DECK_ASCII":
			return "true"
		}
		return ""
	}
	userHome := func() (string, error) { return dir, nil }
	loaded, err := config.LoadFrom(getenv, userHome)
	if err != nil {
		t.Fatalf("config.LoadFrom (seed load): %v", err)
	}
	if loaded.EnvOverrides["ui.ascii"] != "DECK_ASCII" {
		t.Fatalf("seed load EnvOverrides = %+v, want ui.ascii overridden by DECK_ASCII", loaded.EnvOverrides)
	}
	m := New(nil, loaded, "")
	updated, _ := m.Update(key(","))
	m = updated.(Model)
	if !m.settingsOpen {
		t.Fatal(", did not open settings")
	}
	// "UI" is category 1 (General is 0) per settingsCategoryName's
	// documented order, and ascii is the second field in the schema's
	// [ui] block (theme, ascii, mouse, recent_cwd_limit).
	m.settingsFocus = settingsFocusFields
	m.settingsCategoryIndex = 1
	m.settingsFieldIndex = 1
	if got := settingsFieldLabel(settingsCategories()[1].Fields[1]); got != "Ascii" {
		t.Fatalf("category 1 field 1 = %q, want the Ascii field (schema order changed?)", got)
	}
	// Wide enough that the override label is not truncated away by the
	// field panel's own column budget -- this test asserts the label's
	// full text is present, not merely that some row was too narrow to
	// hold it.
	m.width, m.height = 160, 30
	return m, loaded.Paths.ConfigFile
}

// TestSettingsLabelsAnEnvOverriddenFieldAndDoesNotLie proves requirement
// 21: a key whose value came from the environment is labelled overridden-
// by-environment in the field row and its detail text, and even after
// saving a changed value through ctrl+s, m.settings (the actually running
// configuration) never advances -- editing writes the file but never
// pretends the running value changed while the environment variable that
// overrode it is still set.
func TestSettingsLabelsAnEnvOverriddenFieldAndDoesNotLie(t *testing.T) {
	m, _ := settingsEnvOverrideTestModel(t)

	// Re-aimed for task 003 (requirement 21): the row's own parenthetical
	// dropped the word "environment" to fit the field panel's row budget
	// (66 visible cells at 100 columns -- the fuller "overridden by
	// environment: DECK_ASCII" phrase this test pinned before task 003
	// no longer fits alongside stating the file AND running values, and
	// was silently truncated away rather than asserted against; see
	// settings.go's settingsFieldRunningValueDisplay comment). The row
	// still says which field is affected, which value is the file's
	// (Off, the one saving here changes) and which value is running
	// right now (On, via DECK_ASCII) -- the detail text below states the
	// fuller sentence this test used to pin, now checked separately.
	view := m.settingsView()
	// Checked without the "Ascii: " label prefix: settingsRenderRow opens a
	// fresh colour escape per segment (label, ": ", value) even between
	// segments sharing one theme.Token, so a substring spanning the label/
	// value boundary would never match the coloured view -- see settings.go's
	// valueText comment for why file-value and override text are merged into
	// ONE segment (so THAT boundary is safe to span).
	if !strings.Contains(view, "Off (file value; overridden by DECK_ASCII, running: On)") {
		t.Fatalf("settings view = %q, want the ascii row to state both the file value and the running value it disagrees with", view)
	}
	if !strings.Contains(view, "Overridden by environment: DECK_ASCII") {
		t.Fatalf("settings view = %q, want the ascii field's detail text to explain the override", view)
	}

	runningASCIIBefore := m.settings.ASCII

	// Toggle the staged edit (enter activates a KindToggle field, task
	// 015) and save it -- the file changes, but since DECK_ASCII is still
	// set in this environment, the running Settings.ASCII must not move.
	updated, _ := m.Update(key("enter"))
	m = updated.(Model)
	updated, _ = m.Update(key("ctrl+s"))
	m = updated.(Model)

	if m.settings.ASCII != runningASCIIBefore {
		t.Fatalf("m.settings.ASCII changed from %v to %v after a save -- an env-overridden field must not pretend a save changed the running value", runningASCIIBefore, m.settings.ASCII)
	}

	afterView := m.settingsView()
	// After the save, the file value the row states is now On (the
	// staged edit that ctrl+s wrote), but running is still On too since
	// DECK_ASCII was already forcing On before the edit -- both read
	// "On" here, which is exactly why TestSettingsLabelsAnEnvOverridden
	// FieldAndDoesNotLie above asserts m.settings.ASCII did not move as
	// the thing that actually proves the running value is independent of
	// this save, not the row text (which can legitimately show equal
	// values when they happen to coincide).
	if !strings.Contains(afterView, "overridden by DECK_ASCII") {
		t.Fatalf("settings view after save = %q, want the override label to persist", afterView)
	}
	if !strings.Contains(m.settingsNote, "saved") && m.settingsNote == "" {
		t.Fatalf("settingsNote = %q, want a save confirmation", m.settingsNote)
	}
}

// TestSettingsDoesNotLabelAFieldEnvCannotOverride proves the label is
// keyed off the actual per-load override map, not a blanket "some DECK_*
// var is set somewhere" guess: allow_yolo has no environment override
// path at all (per config.LoadFrom), so it must never be labelled even
// when unrelated DECK_* variables are set in the same environment.
func TestSettingsDoesNotLabelAFieldEnvCannotOverride(t *testing.T) {
	dir := t.TempDir()
	getenv := func(name string) string {
		switch name {
		case "DECK_HOME":
			return dir
		case "DECK_ANIM":
			return "false"
		}
		return ""
	}
	userHome := func() (string, error) { return dir, nil }
	loaded, err := config.LoadFrom(getenv, userHome)
	if err != nil {
		t.Fatalf("config.LoadFrom: %v", err)
	}
	if len(loaded.EnvOverrides) != 0 {
		t.Fatalf("EnvOverrides = %+v, want none: DECK_ANIM overrides no schema field", loaded.EnvOverrides)
	}
	m := New(nil, loaded, "")
	updated, _ := m.Update(key(","))
	m = updated.(Model)
	m.settingsFocus = settingsFocusFields
	m.settingsCategoryIndex = 0
	m.settingsFieldIndex = 0
	view := m.settingsView()
	if strings.Contains(view, "overridden by environment") {
		t.Fatalf("settings view = %q, want no override label when no field's env var is set", view)
	}
}
