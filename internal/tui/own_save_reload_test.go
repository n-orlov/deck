package tui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// pickTheme drives the `t` picker to name and confirms it with enter, the way
// the terminal would.
func (h *reloadHarness) pickTheme(name string) {
	h.t.Helper()
	h.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	if !h.m.themePicking {
		h.t.Fatal("`t` did not open the theme picker")
	}
	for i := 0; i <= len(h.m.themePickerNames()) && h.m.themePickerValue != name; i++ {
		h.key(tea.KeyMsg{Type: tea.KeyRight})
	}
	if h.m.themePickerValue != name {
		h.t.Fatalf("could not cycle the picker onto %q", name)
	}
	h.key(tea.KeyMsg{Type: tea.KeyEnter})
	if h.m.themePicking || h.m.settings.Theme.Name != name {
		h.t.Fatalf("enter did not apply %q (picking=%v, theme=%q)", name, h.m.themePicking, h.m.settings.Theme.Name)
	}
}

func TestThemePickerOwnSaveTriggersNoSecondApply(t *testing.T) {
	h := newReloadHarness(t, "[ui]\ntheme = \"empire\"\nsort_order = \"name\"\n")
	h.pickTheme("matrix")
	if !strings.Contains(h.configText(), "matrix") {
		t.Fatalf("setup: the picker did not persist the theme:\n%s", h.configText())
	}
	if got := h.m.settings.File.Theme; got != "matrix" {
		t.Fatalf("the running file snapshot still says theme %q after the picker saved matrix", got)
	}
	applied := h.m.reloadApplied
	for i := 0; i < 3; i++ { // every later unchanged poll stays silent
		if polled := h.pollOnce(); polled.changed {
			t.Fatalf("poll %d after the picker's own save reported a change", i)
		}
	}
	if h.m.reloadApplied != applied || h.m.reloaded != nil {
		t.Fatalf("picker save applied again: counter %d -> %d, reloaded=%v", applied, h.m.reloadApplied, h.m.reloaded)
	}
	// A genuinely external write is still picked up, exactly once.
	h.writeConfig("[ui]\ntheme = \"empire\"\nsort_order = \"name\"\n")
	if !h.pollOnce().changed || h.m.reloadApplied != applied+1 || h.m.settings.Theme.Name != "empire" {
		t.Fatalf("a later external write was not applied once (applied=%d, theme=%q)", h.m.reloadApplied, h.m.settings.Theme.Name)
	}
	if h.pollOnce().changed {
		t.Fatal("the external write was applied twice")
	}
}

func TestThemePickerOwnSaveStillSeesExternalUserThemeEdit(t *testing.T) {
	h := newReloadHarness(t, "[ui]\ntheme = \"empire\"\n")
	h.pickTheme("matrix")
	h.write(filepath.Join(h.dir, "themes", "extra.toml"), "name = \"extra\"\n")
	applied := h.m.reloadApplied
	h.pollOnce()
	if h.m.reloadApplied != applied+1 {
		t.Fatalf("a theme file added after the picker's save was not picked up (applied %d -> %d)", applied, h.m.reloadApplied)
	}
}

func TestPickerSaveClearsInvalidConfigNoticeAndSettingsSaveAfterItStaysQuiet(t *testing.T) {
	h := newReloadHarness(t, "[ui]\ntheme = \"empire\"\n")
	h.writeConfig("[ui\n")
	if h.pollOnce().err == nil || h.m.reloadErr == nil {
		t.Fatal("setup: the invalid file raised no notice")
	}
	h.pickTheme("matrix")
	if h.m.reloadErr != nil {
		t.Fatalf("the notice survived our own successful save: %v", h.m.reloadErr)
	}
	// A second own writer path right after the first stays quiet too.
	h.openSettings()
	h.m.settingsEdits.SortOrder = "activity"
	h.key(tea.KeyMsg{Type: tea.KeyCtrlS})
	applied := h.m.reloadApplied
	if h.pollOnce().changed || h.m.reloadApplied != applied {
		t.Fatal("a settings save after a picker save was applied back")
	}
}
