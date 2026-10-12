package tui

import (
	"os"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
)

// TestBookmarksSettingIsToggledFromTheSettingsMenu pins R274's menu toggle:
// the bookmarks setting is off by default, is a row in the settings takeover
// that `enter` flips, and `ctrl+s` applies it to the running client and
// writes it to config.toml.
func TestBookmarksSettingIsToggledFromTheSettingsMenu(t *testing.T) {
	field, ok := config.FieldByFullKey("bookmarks")
	if !ok {
		t.Fatal("config.Schema has no bookmarks key")
	}
	ci, fi, ok := settingsFindFieldIndex(field)
	if !ok {
		t.Fatal("the settings takeover has no bookmarks row")
	}
	m := settingsLiveApplyTestModel(t)
	if m.settings.Bookmarks {
		t.Fatal("bookmarks is on by default, want off")
	}
	m = settingsOpenAndSelect(t, m, ci, fi, settingsFieldLabel(field))
	if screen := m.View(); !strings.Contains(screen, settingsFieldLabel(field)) {
		t.Fatalf("settings screen does not show the bookmarks row:\n%s", screen)
	}
	updated, _ := m.Update(key("enter"))
	m = updated.(Model)
	if !m.settingsEdits.Bookmarks {
		t.Fatal("enter on the bookmarks row did not stage it on")
	}
	updated, _ = m.Update(key("ctrl+s"))
	m = updated.(Model)
	if !m.settings.Bookmarks {
		t.Fatalf("ctrl+s did not turn bookmarks on in the running client (note %q)", m.settingsNote)
	}
	raw, err := os.ReadFile(m.settings.Paths.ConfigFile)
	if err != nil || !strings.Contains(string(raw), "bookmarks = true") {
		t.Fatalf("config.toml lacks bookmarks = true after the save: %v\n%s", err, raw)
	}
	updated, _ = m.Update(key("enter"))
	m = updated.(Model)
	updated, _ = m.Update(key("ctrl+s"))
	m = updated.(Model)
	if m.settings.Bookmarks {
		t.Fatal("a second flip and save did not turn bookmarks back off")
	}
}
