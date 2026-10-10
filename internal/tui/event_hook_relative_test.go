package tui

import (
	"os"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
)

// R241 (SPEC §10.1): saving a relative event_hook path from the settings view
// is refused with a note, the file is left alone and the running setting does
// not follow; an absolute path saves.
func TestSettingsSaveRefusesARelativeEventHookPath(t *testing.T) {
	for _, relative := range []string{"./hook.sh", "scripts/hook.sh --quiet", "~/hook.sh"} {
		model, configFile, reload := settingsTestModel(t)
		model = settingsFocusFieldByKey(t, model, "event_hook")
		model = pressAll(model, "enter")
		if !model.settingsStringEditing {
			t.Fatalf("enter on event_hook did not open the editor (note %q)", model.settingsNote)
		}
		model = pressAll(model, relative, "enter", "ctrl+s")
		if !strings.Contains(model.settingsNote, "save refused") || !strings.Contains(model.settingsNote, "absolute") {
			t.Errorf("%q: note = %q, want a refusal naming the absolute-path rule", relative, model.settingsNote)
		}
		if raw, err := os.ReadFile(configFile); err == nil && strings.Contains(string(raw), "event_hook") {
			t.Errorf("%q: the refused value reached config.toml:\n%s", relative, raw)
		}
		if model.settings.EventHook != "" {
			t.Errorf("%q: running event_hook = %q after a refused save", relative, model.settings.EventHook)
		}
		if !model.settingsDirty() {
			t.Errorf("%q: the refused edit must stay staged and dirty", relative)
		}
		if parsed, err := reload(); err != nil || parsed.EventHook != "" {
			t.Errorf("%q: reloaded event_hook = %q, %v", relative, parsed.EventHook, err)
		}
	}

	model, _, reload := settingsTestModel(t)
	model = settingsFocusFieldByKey(t, model, "event_hook")
	model = pressAll(model, "enter", "/opt/hooks/notify.sh --quiet", "enter", "ctrl+s")
	if !strings.HasPrefix(model.settingsNote, "saved ") {
		t.Fatalf("an absolute path must save, note = %q", model.settingsNote)
	}
	if parsed, err := reload(); err != nil || parsed.EventHook != "/opt/hooks/notify.sh --quiet" {
		t.Fatalf("reloaded event_hook = %q, %v", parsed.EventHook, err)
	}
}

// A relative path already in the file does not lock the settings view: an
// unrelated edit still saves, the value itself is the health view's warning.
func TestSettingsSaveKeepsAnAlreadyRelativeEventHookEditable(t *testing.T) {
	model, _, _ := settingsTestModel(t)
	model.settings.File.EventHook = "scripts/hook.sh"
	model.settingsEdits.EventHook = "scripts/hook.sh"
	model.settingsEdits.Theme = "light"
	model.settingsSave()
	if !strings.HasPrefix(model.settingsNote, "saved ") {
		t.Fatalf("an unchanged relative event_hook refused an unrelated save: %q", model.settingsNote)
	}
}

// R241: a loaded config.toml whose event_hook executable is relative is not a
// load error; the health view carries a WARNING naming the script, for a path
// with a slash and with fixed arguments, and nothing for a bare name or an
// absolute path.
func TestLoadedRelativeEventHookWarnsInTheHealthView(t *testing.T) {
	dir := t.TempDir()
	body := "event_hook = \"scripts/hook.sh --quiet\"\n"
	if err := os.WriteFile(dir+"/config.toml", []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	getenv := func(name string) string {
		if name == "DECK_HOME" {
			return dir
		}
		return ""
	}
	loaded, err := config.LoadFrom(getenv, func() (string, error) { return dir, nil })
	if err != nil {
		t.Fatalf("a relative event_hook must load: %v", err)
	}
	m := New(nil, loaded, "")
	m.hookHealth = m.loadHookHealth()
	lines := strings.Join(m.eventHookHealthLines(100), "\n")
	for _, want := range []string{"Event hook WARNING", "scripts/hook.sh", "relative", "absolute path"} {
		if !strings.Contains(lines, want) {
			t.Errorf("health lines lack %q:\n%s", want, lines)
		}
	}
	if strings.Contains(lines, "does not exist") {
		t.Errorf("the relative warning must replace the missing-file verdict:\n%s", lines)
	}
}
