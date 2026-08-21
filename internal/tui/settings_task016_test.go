package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
)

// settingsTestModel opens the takeover against a real, empty config.toml
// under a fresh temp directory (via DECK_HOME, config.LoadFrom's own test
// seam), so ctrl+s (task 016) has somewhere real to write and the test can
// read the file back through the real loader afterwards -- exactly what
// requirement 20 asks be asserted, not merely claimed. The returned reload
// function re-runs config.LoadFrom against the same directory, so a test
// can check the file's PARSED content, not just its raw bytes.
func settingsTestModel(t *testing.T) (model Model, configFile string, reload func() (config.Settings, error)) {
	t.Helper()
	dir := t.TempDir()
	getenv := func(name string) string {
		if name == "DECK_HOME" {
			return dir
		}
		return ""
	}
	userHome := func() (string, error) { return dir, nil }
	loaded, err := config.LoadFrom(getenv, userHome)
	if err != nil {
		t.Fatalf("config.LoadFrom (seed load): %v", err)
	}
	m := New(nil, loaded, "")
	updated, _ := m.Update(key(","))
	m = updated.(Model)
	if !m.settingsOpen {
		t.Fatal(", did not open settings")
	}
	// General (section "") is category 0 and allow_yolo is its first
	// field; move focus onto the field list so enter/ctrl+s tests below
	// can toggle it directly, matching how settingsActivateField requires
	// settingsFocusFields (settingsSelectedField's own guard).
	m.settingsFocus = settingsFocusFields
	m.settingsFieldIndex = 0
	reload = func() (config.Settings, error) { return config.LoadFrom(getenv, userHome) }
	return m, loaded.Paths.ConfigFile, reload
}

// TestSettingsCtrlSSavesThroughAtomicWriter proves ctrl+s writes the
// staged edits to config.toml through config.WriteConfigFile (task 012):
// the file's PARSED content after the save reflects the toggled field, not
// merely that some bytes landed on disk.
func TestSettingsCtrlSSavesThroughAtomicWriter(t *testing.T) {
	model, configFile, reload := settingsTestModel(t)

	before := model.settingsEdits.AllowYolo
	// "General" (section "") is category 0 and allow_yolo is its first
	// field per schema.go's declared order; flip it with enter (task
	// 015's toggle activation) so the save has a real, provable change.
	updated, _ := model.Update(key("enter"))
	model = updated.(Model)
	if model.settingsEdits.AllowYolo == before {
		t.Fatal("enter did not toggle allow_yolo before the save test could proceed")
	}

	updated, _ = model.Update(key("ctrl+s"))
	model = updated.(Model)

	if !model.settingsOpen {
		t.Fatal("ctrl+s closed the takeover; it should only save")
	}
	if model.settingsNote == "" {
		t.Fatal("ctrl+s left no note behind; a save's outcome must be visible")
	}
	if model.settingsDirty() {
		t.Fatal("a successful save left settingsDirty() true; settingsSavedEdits should now match settingsEdits")
	}

	data, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatalf("config file was not written: %v", err)
	}
	if !strings.Contains(string(data), "allow_yolo") {
		t.Fatalf("saved config.toml does not mention allow_yolo:\n%s", data)
	}

	parsed, err := reload()
	if err != nil {
		t.Fatalf("saved config.toml did not parse: %v", err)
	}
	if parsed.AllowYolo != model.settingsEdits.AllowYolo {
		t.Fatalf("parsed AllowYolo = %v, want %v (the staged, saved value)", parsed.AllowYolo, model.settingsEdits.AllowYolo)
	}
}

// TestSettingsEscWithChangesPromptsToDiscard proves requirement 14/20:
// esc with a staged, unsaved change does not close the takeover outright
// -- it opens the discard-confirm prompt instead, and cancelling that
// prompt (any key other than y/enter) returns to editing with the change
// still staged and the file still untouched.
func TestSettingsEscWithChangesPromptsToDiscard(t *testing.T) {
	model, configFile, _ := settingsTestModel(t)

	updated, _ := model.Update(key("enter")) // toggles allow_yolo
	model = updated.(Model)

	updated, _ = model.Update(key("esc"))
	model = updated.(Model)
	if !model.settingsOpen {
		t.Fatal("esc with unsaved changes closed the takeover outright; it must prompt first")
	}
	if !model.settingsDiscardConfirm {
		t.Fatal("esc with unsaved changes did not raise the discard-confirm prompt")
	}
	if _, err := os.Stat(configFile); err == nil {
		t.Fatal("the discard prompt itself must not have written config.toml")
	}

	// Cancel the prompt: back to editing, nothing lost.
	updated, _ = model.Update(key("n"))
	model = updated.(Model)
	if model.settingsDiscardConfirm {
		t.Fatal("n did not dismiss the discard-confirm prompt")
	}
	if !model.settingsOpen {
		t.Fatal("cancelling the discard prompt closed the takeover")
	}
	if !model.settingsDirty() {
		t.Fatal("cancelling the discard prompt lost the staged change")
	}
}

// TestSettingsEscDiscardConfirmedLeavesFileUnchanged proves the other half
// of requirement 20: confirming the discard prompt (y/enter) closes the
// takeover and leaves config.toml exactly as it was -- in this case,
// never written at all, since nothing was ever saved.
func TestSettingsEscDiscardConfirmedLeavesFileUnchanged(t *testing.T) {
	model, configFile, _ := settingsTestModel(t)

	updated, _ := model.Update(key("enter")) // toggles allow_yolo
	model = updated.(Model)
	updated, _ = model.Update(key("esc"))
	model = updated.(Model)
	if !model.settingsDiscardConfirm {
		t.Fatal("esc with unsaved changes did not raise the discard-confirm prompt")
	}

	updated, _ = model.Update(key("y"))
	model = updated.(Model)
	if model.settingsOpen {
		t.Fatal("confirming the discard prompt did not close the takeover")
	}
	if _, err := os.Stat(configFile); err == nil {
		t.Fatal("discarding must leave config.toml unwritten/unchanged")
	}
}

// TestSettingsEscWithNoChangesClosesWithoutPrompting proves the other side
// of requirement 16/20's "esc ... otherwise closes": with nothing staged
// that differs from what settings opened with, esc closes immediately —
// exactly the pre-existing TestSettingsKeyOpensAndEscCloses behaviour,
// re-asserted here against a model that also has a real settingsSavedEdits
// baseline (task 016), not just the zero value.
func TestSettingsEscWithNoChangesClosesWithoutPrompting(t *testing.T) {
	model, _, _ := settingsTestModel(t)

	updated, _ := model.Update(key("esc"))
	model = updated.(Model)
	if model.settingsDiscardConfirm {
		t.Fatal("esc with no changes raised the discard-confirm prompt")
	}
	if model.settingsOpen {
		t.Fatal("esc with no changes did not close the takeover")
	}
}

// TestSettingsSaveAfterDiscardPromptDismissedStillWorks proves a save is
// still possible after a discard prompt is raised and then cancelled: the
// staged edit is not corrupted by the round trip through the prompt.
func TestSettingsSaveAfterDiscardPromptDismissedStillWorks(t *testing.T) {
	model, configFile, _ := settingsTestModel(t)

	updated, _ := model.Update(key("enter"))
	model = updated.(Model)
	updated, _ = model.Update(key("esc"))
	model = updated.(Model)
	updated, _ = model.Update(key("esc")) // cancel the prompt (any non-y/enter key)
	model = updated.(Model)
	if model.settingsDiscardConfirm {
		t.Fatal("esc did not dismiss the discard-confirm prompt")
	}

	updated, _ = model.Update(key("ctrl+s"))
	model = updated.(Model)
	if model.settingsDirty() {
		t.Fatal("save after a dismissed discard prompt left settingsDirty() true")
	}
	if _, err := os.Stat(configFile); err != nil {
		t.Fatalf("save after a dismissed discard prompt did not write config.toml: %v", err)
	}
}

// TestSettingsSaveFailureSurfacesNote proves a write failure (an
// unwritable config directory) is visible in settingsNote rather than
// swallowed, and leaves the staged edit intact for a retry.
func TestSettingsSaveFailureSurfacesNote(t *testing.T) {
	dir := t.TempDir()
	// A config file path inside a directory that does not exist and
	// cannot be created (its parent is a file, not a directory) makes
	// WriteConfigFile's rename fail.
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}
	configFile := filepath.Join(blocker, "nested", "config.toml")
	settings := config.Settings{Paths: config.Paths{ConfigFile: configFile}}
	model := New(nil, settings, "")
	updated, _ := model.Update(key(","))
	model = updated.(Model)
	model.settingsFocus = settingsFocusFields
	model.settingsFieldIndex = 0

	updated, _ = model.Update(key("enter")) // stage a change
	model = updated.(Model)

	updated, _ = model.Update(key("ctrl+s"))
	model = updated.(Model)
	if !model.settingsDirty() {
		t.Fatal("a failed save must not be treated as saved")
	}
	if model.settingsNote == "" {
		t.Fatal("a failed save left no note behind")
	}
}
