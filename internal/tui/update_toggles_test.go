package tui

import (
	"os"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
)

// TestMainMenuUpdateToggles pins R297's menu: the `,` settings menu has a
// toggle row for update_check (on by default) and one for auto_update (off by
// default); `enter` stages a flip, `ctrl+s` applies it to the running client
// and writes it to config.toml.
func TestMainMenuUpdateToggles(t *testing.T) {
	rows := []struct {
		key         string
		defaultOn   bool
		running     func(Model) bool
		staged      func(Model) bool
		wantWritten string
	}{
		{"update_check", true, func(m Model) bool { return m.settings.UpdateCheck }, func(m Model) bool { return m.settingsEdits.UpdateCheck }, "update_check = false"},
		{"auto_update", false, func(m Model) bool { return m.settings.AutoUpdate }, func(m Model) bool { return m.settingsEdits.AutoUpdate }, "auto_update = true"},
	}
	for _, r := range rows {
		field, ok := config.FieldByFullKey(r.key)
		if !ok {
			t.Fatalf("config.Schema has no %s key", r.key)
		}
		ci, fi, ok := settingsFindFieldIndex(field)
		if !ok {
			t.Fatalf("the settings menu has no %s row", r.key)
		}
		m := settingsLiveApplyTestModel(t)
		if r.running(m) != r.defaultOn {
			t.Fatalf("%s running default = %v, want %v", r.key, r.running(m), r.defaultOn)
		}
		m = settingsOpenAndSelect(t, m, ci, fi, settingsFieldLabel(field))
		if screen := m.View(); !strings.Contains(screen, settingsFieldLabel(field)) {
			t.Fatalf("settings screen does not show the %s row:\n%s", r.key, screen)
		}
		updated, _ := m.Update(key("enter"))
		m = updated.(Model)
		if r.staged(m) == r.defaultOn {
			t.Fatalf("enter on the %s row did not stage a flip", r.key)
		}
		updated, _ = m.Update(key("ctrl+s"))
		m = updated.(Model)
		if r.running(m) == r.defaultOn {
			t.Fatalf("ctrl+s did not apply %s to the running client (note %q)", r.key, m.settingsNote)
		}
		raw, err := os.ReadFile(m.settings.Paths.ConfigFile)
		if err != nil || !strings.Contains(string(raw), r.wantWritten) {
			t.Fatalf("config.toml lacks %q after the save: %v\n%s", r.wantWritten, err, raw)
		}
	}
}
