package tui

import (
	"os"
	"strings"
	"testing"
)

// settingsLocateField walks settingsCategories() (never a hand-maintained
// index) to find fullKey's category/field position, so this test tracks a
// future reordering of config.Schema instead of silently toggling the
// wrong field.
func settingsLocateField(t *testing.T, fullKey string) (categoryIndex, fieldIndex int) {
	t.Helper()
	for ci, cat := range settingsCategories() {
		for fi, f := range cat.Fields {
			if f.FullKey() == fullKey {
				return ci, fi
			}
		}
	}
	t.Fatalf("settingsCategories() has no field %s", fullKey)
	return 0, 0
}

// TestSettingsGroupByWorkspaceStagesUntilSave proves I-4/requirement 34's
// settings surface obeys the same explicit-save contract as every other
// field (requirement 20): toggling [ui] group_by_workspace stages the
// change in settingsEdits only -- no keypress writes config.toml -- and
// only ctrl+s commits it, after which the file's PARSED content (not
// merely its raw bytes) reflects the toggle.
func TestSettingsGroupByWorkspaceStagesUntilSave(t *testing.T) {
	model, configFile, reload := settingsTestModel(t)
	ci, fi := settingsLocateField(t, "ui.group_by_workspace")
	model.settingsCategoryIndex = ci
	model.settingsFieldIndex = fi

	before := model.settingsEdits.GroupByWorkspace

	updated, _ := model.Update(key("enter"))
	model = updated.(Model)
	if model.settingsEdits.GroupByWorkspace == before {
		t.Fatal("enter did not toggle group_by_workspace; test cannot proceed")
	}
	if !model.settingsDirty() {
		t.Fatal("toggling group_by_workspace did not mark the takeover dirty")
	}
	if _, err := os.Stat(configFile); err == nil {
		t.Fatal("toggling group_by_workspace alone wrote config.toml; only ctrl+s may write")
	}

	updated, _ = model.Update(key("ctrl+s"))
	model = updated.(Model)
	if model.settingsDirty() {
		t.Fatal("ctrl+s left settingsDirty() true after a successful save")
	}

	data, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatalf("ctrl+s did not write config.toml: %v", err)
	}
	if !strings.Contains(string(data), "group_by_workspace") {
		t.Fatalf("saved config.toml does not mention group_by_workspace:\n%s", data)
	}

	parsed, err := reload()
	if err != nil {
		t.Fatalf("saved config.toml did not parse: %v", err)
	}
	if parsed.GroupByWorkspace != model.settingsEdits.GroupByWorkspace {
		t.Fatalf("parsed GroupByWorkspace = %v, want %v (the staged, saved value)", parsed.GroupByWorkspace, model.settingsEdits.GroupByWorkspace)
	}
}
