package tui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/n-orlov/deck/internal/config"
)

// settingsEnvOverrideFileTestModel writes a real config.toml containing an
// explicit `[ui]\nascii = false` and opens the takeover with DECK_ASCII=1
// set in the environment, so the running config.Settings.ASCII is true
// (per §6.5, environment outranks the file) while the file itself still
// says false. It returns a reload function that re-parses the file
// through config.LoadFrom, so a test can check what a save actually wrote,
// not merely what the running model claims.
func settingsEnvOverrideFileTestModel(t *testing.T) (model Model, reload func() (config.Settings, error)) {
	t.Helper()
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(configFile, []byte("[ui]\nascii = false\n"), 0o644); err != nil {
		t.Fatalf("seed config.toml: %v", err)
	}
	getenv := func(name string) string {
		switch name {
		case "DECK_HOME":
			return dir
		case "DECK_ASCII":
			return "1"
		}
		return ""
	}
	userHome := func() (string, error) { return dir, nil }
	loaded, err := config.LoadFrom(getenv, userHome)
	if err != nil {
		t.Fatalf("config.LoadFrom (seed load): %v", err)
	}
	if !loaded.ASCII {
		t.Fatal("seed load Settings.ASCII = false, want true: DECK_ASCII=1 should override the file")
	}
	if loaded.EnvOverrides["ui.ascii"] != "DECK_ASCII" {
		t.Fatalf("seed load EnvOverrides = %+v, want ui.ascii overridden by DECK_ASCII", loaded.EnvOverrides)
	}
	if loaded.File.ASCII {
		t.Fatal("seed load Settings.File.ASCII = true, want false: the raw file value must not be affected by the env override")
	}
	m := New(nil, loaded, "")
	updated, _ := m.Update(key(","))
	m = updated.(Model)
	if !m.settingsOpen {
		t.Fatal(", did not open settings")
	}
	// "UI" is category 1 (General is 0) and ascii is field 1 within it
	// (theme, ascii, mouse, recent_cwd_limit), matching
	// settingsEnvOverrideTestModel's own documented order in task017's test.
	m.settingsFocus = settingsFocusFields
	m.settingsCategoryIndex = 1
	m.settingsFieldIndex = 1
	if got := settingsFieldLabel(settingsCategories()[1].Fields[1]); got != "Ascii" {
		t.Fatalf("category 1 field 1 = %q, want the Ascii field (schema order changed?)", got)
	}
	reload = func() (config.Settings, error) { return config.LoadFrom(getenv, userHome) }
	return m, reload
}

// TestSettingsSaveWithNoEditLeavesEnvOverriddenFileValueUntouched proves
// requirement 21: opening the takeover and saving without touching an
// env-overridden field must never rewrite that field's file value to the
// environment-supplied one. This reproduces the reviewer's probe exactly
// -- config.toml says ascii=false, DECK_ASCII=1 makes the running value
// true, `,` opens the takeover, no edit is made, ctrl+s is pressed -- and
// asserts the file, re-parsed, still says ascii=false.
func TestSettingsSaveWithNoEditLeavesEnvOverriddenFileValueUntouched(t *testing.T) {
	m, reload := settingsEnvOverrideFileTestModel(t)

	if m.settingsEdits.ASCII {
		t.Fatal("settingsEdits.ASCII seeded true from the env-resolved value; it must seed from the file's own false")
	}

	updated, _ := m.Update(key("ctrl+s"))
	m = updated.(Model)
	if m.settingsNote == "" {
		t.Fatal("ctrl+s left no note behind; a save's outcome must be visible")
	}

	reloaded, err := reload()
	if err != nil {
		t.Fatalf("reload after save: %v", err)
	}
	if reloaded.File.ASCII {
		t.Fatal("a no-edit ctrl+s rewrote config.toml's ui.ascii from false to true -- the file must keep its own value for an env-overridden key that was never actually edited")
	}
}

// TestSettingsSaveWithExplicitEditOfEnvOverriddenFieldIsWritten proves the
// fix from the test above is not "never write an overridden key at all":
// an explicit user edit of that same field (enter toggles it, task 015)
// must still land in the file on ctrl+s. Only a no-edit save must leave
// the file's value alone.
func TestSettingsSaveWithExplicitEditOfEnvOverriddenFieldIsWritten(t *testing.T) {
	m, reload := settingsEnvOverrideFileTestModel(t)

	updated, _ := m.Update(key("enter"))
	m = updated.(Model)
	if !m.settingsEdits.ASCII {
		t.Fatal("enter did not toggle the staged ascii edit to true")
	}

	updated, _ = m.Update(key("ctrl+s"))
	m = updated.(Model)
	if m.settingsNote == "" {
		t.Fatal("ctrl+s left no note behind; a save's outcome must be visible")
	}

	reloaded, err := reload()
	if err != nil {
		t.Fatalf("reload after save: %v", err)
	}
	if !reloaded.File.ASCII {
		t.Fatal("an explicit edit of an env-overridden field was not written to config.toml; the fix must not suppress genuine edits")
	}
}
