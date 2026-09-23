package tui

import (
	"reflect"
	"testing"

	"github.com/n-orlov/deck/internal/store"
)

// sidebarHeaderOrder walks m.sidebarEntries at the model's own computed
// layout width -- the exact same call the real renderer makes -- and
// returns the group headers' names in the order they were painted, so a
// test asserting "the rendered sidebar" changed is checking the render
// path itself, not m.groupSessions() called out of context.
func sidebarHeaderOrder(m Model) []string {
	layout := m.computeLayout()
	contentWidth := max(layout.Sidebar.Width-2, 0)
	var names []string
	for _, e := range m.sidebarEntries(contentWidth) {
		if e.kind == sidebarLineHeader {
			names = append(names, e.groupName)
		}
	}
	return names
}

// defaultGroupFirstLiveApplyTestSessions seeds three sessions, one in each
// of group "aaa", the implicit default group (GroupName == "") and group
// "zzz" -- default_group_first == false (R129's baseline) sorts these
// aaa, zzz, "" (default last); flipping the flag to true moves the
// default group alone to the front: "", aaa, zzz. Both orders differ from
// each other in every position, so a live reorder is unmissable.
func defaultGroupFirstLiveApplyTestSessions() []store.Session {
	return []store.Session{
		{ID: "a1", Name: "session-a", Agent: "shell", Status: "idle", GroupName: "aaa"},
		{ID: "d1", Name: "session-d", Agent: "shell", Status: "idle", GroupName: ""},
		{ID: "z1", Name: "session-z", Agent: "shell", Status: "idle", GroupName: "zzz"},
	}
}

// TestSettingsSaveReordersGroupsLivePreservingSelection is task 003's own
// success criterion: saving [ui] default_group_first through the real `,`
// takeover reorders the ALREADY-RUNNING sidebar's group headers on the
// very next render -- no restart, and (per settingsSave's own doc
// comment) no re-read of config.toml either, since this drives the save
// through the in-memory settingsEdits/settingsApplyLiveFields path only --
// while the selected session's id stays exactly what it was, even though
// the reorder moves that session's row to a different position.
func TestSettingsSaveReordersGroupsLivePreservingSelection(t *testing.T) {
	model, _ := settingsLiveApplyTestModel(t)
	if model.settings.DefaultGroupFirst {
		t.Fatal("seed DefaultGroupFirst = true, want false")
	}

	updated, _ := model.Update(sessionsLoaded{sessions: defaultGroupFirstLiveApplyTestSessions()})
	model = updated.(Model)

	before := sidebarHeaderOrder(model)
	wantBefore := []string{"aaa", "zzz", ""}
	if !reflect.DeepEqual(before, wantBefore) {
		t.Fatalf("setup: sidebar header order = %v, want %v (default_group_first=false baseline)", before, wantBefore)
	}

	// Select the session sitting in the implicit default group -- last
	// under the current order, so flipping the flag moves its ROW from
	// the bottom group to the top one.
	targetID := "d1"
	idx := indexOfSessionID(model.sessions, targetID)
	if idx < 0 {
		t.Fatalf("setup: %s not found in m.sessions", targetID)
	}
	model.selected = rowCursor(idx)

	model.settingsEdits = settingsEditsFromSettings(model.settings)
	model.settingsEdits.DefaultGroupFirst = true

	m := &model
	m.settingsSave()
	model = *m

	if !model.settings.DefaultGroupFirst {
		t.Fatal("ctrl+s did not refresh the running m.settings.DefaultGroupFirst to true -- settingsApplyLiveFields did not copy the edit live")
	}

	after := sidebarHeaderOrder(model)
	wantAfter := []string{"", "aaa", "zzz"}
	if !reflect.DeepEqual(after, wantAfter) {
		t.Fatalf("rendered sidebar header order after live save = %v, want %v -- group order did not reorder live", after, wantAfter)
	}
	if reflect.DeepEqual(before, after) {
		t.Fatal("rendered sidebar header order unchanged after a live default_group_first save")
	}

	if testSelectedSession(model).ID != targetID {
		t.Fatalf("selected session = %q after live reorder, want %q -- selection followed the OLD index, not the session id", testSelectedSession(model).ID, targetID)
	}
}

// TestSettingsSaveDefaultGroupFirstRespectsEnvOverride proves
// default_group_first's live-apply guard follows the exact same
// EnvOverrides-precedence shape ui.ascii/ui.mouse/ui.sort_order's own
// settingsApplyLiveFields cases do (requirement 21, SPEC §6.5: "the
// environment always outranks the file", extended to the already-running
// client, never just the next launch). config.LoadFrom defines no real
// DECK_DEFAULT_GROUP_FIRST override today (task 001's own note), so this
// drives the identical guard settings.go checks by setting
// m.settings.EnvOverrides directly -- exactly the map
// settingsFieldEnvOverride/settingsApplyLiveFields themselves consult --
// rather than waiting on an env var config.LoadFrom does not yet resolve.
func TestSettingsSaveDefaultGroupFirstRespectsEnvOverride(t *testing.T) {
	model, _ := settingsLiveApplyTestModel(t)
	if model.settings.EnvOverrides == nil {
		model.settings.EnvOverrides = map[string]string{}
	}
	model.settings.EnvOverrides["ui.default_group_first"] = "DECK_DEFAULT_GROUP_FIRST"

	updated, _ := model.Update(sessionsLoaded{sessions: defaultGroupFirstLiveApplyTestSessions()})
	model = updated.(Model)

	before := sidebarHeaderOrder(model)

	model.settingsEdits = settingsEditsFromSettings(model.settings)
	model.settingsEdits.DefaultGroupFirst = true

	m := &model
	m.settingsSave()
	model = *m

	if model.settings.DefaultGroupFirst {
		t.Fatal("ctrl+s changed the running m.settings.DefaultGroupFirst even though ui.default_group_first is env-overridden -- requirement 21 (\u00a76.5) demands the environment stay pinned for the already-running client too")
	}
	after := sidebarHeaderOrder(model)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("rendered sidebar header order changed under an env override: before %v, after %v", before, after)
	}
	if model.settings.File.DefaultGroupFirst != true {
		t.Fatalf("m.settings.File.DefaultGroupFirst = %v after save, want true -- the file value must still be written even though the resolved/running value stays pinned to the environment", model.settings.File.DefaultGroupFirst)
	}
}
