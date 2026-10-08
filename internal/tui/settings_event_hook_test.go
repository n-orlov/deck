package tui

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/config"
)

// pressAll feeds each key, in order, through the real Model.Update.
func pressAll(m Model, presses ...string) Model {
	for _, press := range presses {
		updated, _ := m.Update(key(press))
		m = updated.(Model)
	}
	return m
}

// TestSettingsEventHookEventsEditsAsCommaSeparatedText is R229's dialog
// contract: event_hook_events opens a free-text editor prefilled with the
// staged list as comma-separated words, a typed list stages as a list, ctrl+s
// writes it as a TOML array, and the real loader reads it back.
func TestSettingsEventHookEventsEditsAsCommaSeparatedText(t *testing.T) {
	model, configFile, reload := settingsTestModel(t)
	model = settingsFocusFieldByKey(t, model, "event_hook_events")

	m := pressAll(model, "enter")
	if !m.settingsStringEditing || m.settingsStringEditKey != "event_hook_events" {
		t.Fatalf("enter on event_hook_events did not open the text editor (editing=%v key=%q)", m.settingsStringEditing, m.settingsStringEditKey)
	}
	if got, want := m.settingsStringEdit.Value(), "waiting, error, ended"; got != want {
		t.Fatalf("editor prefill = %q, want the default list as %q", got, want)
	}
	if m.settingsDirty() {
		t.Fatal("opening the editor made the takeover dirty")
	}

	m = pressAll(m, "started,idle , killed,", "enter")
	if m.settingsStringEditing {
		t.Fatalf("a valid list left the editor open (note %q)", m.settingsNote)
	}
	if want := []string{"started", "idle", "killed"}; !reflect.DeepEqual(m.settingsEdits.EventHookEvents, want) {
		t.Fatalf("staged EventHookEvents = %v, want %v", m.settingsEdits.EventHookEvents, want)
	}
	if !m.settingsDirty() {
		t.Fatal("staging a different list left settingsDirty() false")
	}
	m = pressAll(m, "ctrl+s")
	if m.settingsDirty() || !strings.HasPrefix(m.settingsNote, "saved ") {
		t.Fatalf("ctrl+s did not save (dirty=%v note=%q)", m.settingsDirty(), m.settingsNote)
	}
	parsed, err := reload()
	if err != nil {
		t.Fatalf("saved config.toml did not parse: %v", err)
	}
	if want := []string{"started", "idle", "killed"}; !reflect.DeepEqual(parsed.File.EventHookEvents, want) {
		t.Fatalf("reloaded File.EventHookEvents = %v, want %v", parsed.File.EventHookEvents, want)
	}
	if !reflect.DeepEqual(m.settings.EventHookEvents, parsed.EventHookEvents) {
		t.Fatalf("running Settings.EventHookEvents = %v, want the saved %v (ScopeGlobal: live)", m.settings.EventHookEvents, parsed.EventHookEvents)
	}

	// Reopening shows the saved list in the same comma-separated form.
	m = pressAll(settingsFocusFieldByKey(t, m, "event_hook_events"), "enter")
	if got, want := m.settingsStringEdit.Value(), "started, idle, killed"; got != want {
		t.Fatalf("reopened editor = %q, want %q", got, want)
	}
	_ = configFile
}

// TestSettingsEventHookEventsRefusesAnUnofferedKind: a word outside the
// offered set is refused with a note naming the key, nothing is staged, and
// the editor stays open on the typed text so it can be corrected.
func TestSettingsEventHookEventsRefusesAnUnofferedKind(t *testing.T) {
	model, _, _ := settingsTestModel(t)
	model = settingsFocusFieldByKey(t, model, "event_hook_events")
	before := model.settingsEdits.EventHookEvents

	m := pressAll(model, "enter", "waiting, prompt", "enter")
	if !m.settingsStringEditing {
		t.Fatal("an unoffered kind closed the editor; it must stay open")
	}
	if !strings.Contains(m.settingsNote, "event_hook_events") || !strings.Contains(m.settingsNote, "prompt") {
		t.Fatalf("note = %q, want one naming event_hook_events and prompt", m.settingsNote)
	}
	if !reflect.DeepEqual(m.settingsEdits.EventHookEvents, before) || m.settingsDirty() {
		t.Fatalf("a refused list was staged: %v", m.settingsEdits.EventHookEvents)
	}
	if got := m.settingsStringEdit.Value(); got != "waiting, prompt" {
		t.Fatalf("editor text = %q, want the typed text kept", got)
	}

	// esc abandons the edit; the staged list is untouched.
	m = pressAll(m, "esc")
	if m.settingsStringEditing || !reflect.DeepEqual(m.settingsEdits.EventHookEvents, before) {
		t.Fatal("esc did not abandon the refused edit cleanly")
	}
}

// TestSettingsEventHookEventsEmptyTextIsTheEmptyList: emptying the field is
// a real value (no kind offered), not a cancel.
func TestSettingsEventHookEventsEmptyTextIsTheEmptyList(t *testing.T) {
	model, _, reload := settingsTestModel(t)
	model = settingsFocusFieldByKey(t, model, "event_hook_events")
	m := pressAll(model, "enter", " ", "enter")
	if m.settingsStringEditing {
		t.Fatalf("empty text left the editor open (note %q)", m.settingsNote)
	}
	if len(m.settingsEdits.EventHookEvents) != 0 || !m.settingsDirty() {
		t.Fatalf("staged = %v dirty=%v, want the empty list and a dirty takeover", m.settingsEdits.EventHookEvents, m.settingsDirty())
	}
	m = pressAll(m, "ctrl+s")
	parsed, err := reload()
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.File.EventHookEvents) != 0 {
		t.Fatalf("reloaded list = %v, want empty", parsed.File.EventHookEvents)
	}
}

// TestSettingsEventHookKeysEditAndSave drives the other three keys through
// their ordinary editors and the real loader: the script (text), the default
// toggle (enter) and the timeout (+, floored at its schema minimum).
func TestSettingsEventHookKeysEditAndSave(t *testing.T) {
	model, _, reload := settingsTestModel(t)

	m := pressAll(settingsFocusFieldByKey(t, model, "event_hook"), "enter", "/opt/hooks/ping --loud", "enter")
	m = pressAll(settingsFocusFieldByKey(t, m, "event_hook_default"), "enter")
	m = pressAll(settingsFocusFieldByKey(t, m, "event_hook_timeout"), "+", "+")
	m = pressAll(m, "ctrl+s")
	if !strings.HasPrefix(m.settingsNote, "saved ") {
		t.Fatalf("save failed: %q", m.settingsNote)
	}
	parsed, err := reload()
	if err != nil {
		t.Fatal(err)
	}
	if parsed.File.EventHook != "/opt/hooks/ping --loud" || !parsed.File.EventHookDefault || parsed.File.EventHookTimeout != 5*time.Second {
		t.Fatalf("reloaded File = %q %v %s, want the script, on, 5s", parsed.File.EventHook, parsed.File.EventHookDefault, parsed.File.EventHookTimeout)
	}
	if m.settings.EventHook != "/opt/hooks/ping --loud" || !m.settings.EventHookDefault || m.settings.EventHookTimeout != 5*time.Second {
		t.Fatalf("running Settings = %q %v %s, want them live after the save", m.settings.EventHook, m.settings.EventHookDefault, m.settings.EventHookTimeout)
	}

	// "-" never stages a non-positive timeout: the schema minimum is 1 s.
	m = pressAll(settingsFocusFieldByKey(t, m, "event_hook_timeout"), "-", "-", "-", "-", "-", "-", "-")
	if m.settingsEdits.EventHookTimeout != time.Second {
		t.Fatalf("timeout after repeated decrease = %s, want the 1s floor", m.settingsEdits.EventHookTimeout)
	}
}

// TestSettingsEventHookRowsRenderTheirValues: the rows show the list as
// comma-separated text, the unset script as the placeholder and the
// timeout with its unit.
func TestSettingsEventHookRowsRenderTheirValues(t *testing.T) {
	model, _, _ := settingsTestModel(t)
	model.width, model.height = 120, 60
	view := stripANSI(model.settingsView())
	for key, want := range map[string]string{
		"event_hook":         "(not set)",
		"event_hook_default": "Off",
		"event_hook_events":  "waiting, error, ended",
		"event_hook_timeout": "3 seconds",
	} {
		row := settingsRowContaining(t, view, settingsFieldLabel(config.Field{Key: key})+": ")
		if !strings.Contains(row, want) {
			t.Errorf("%s row = %q, want it to show %q", key, row, want)
		}
	}
}
