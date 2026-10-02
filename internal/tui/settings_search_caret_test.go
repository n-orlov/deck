package tui

import (
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
)

// TestSettingsSearchLeftMovesTheCaretNotTheLists: in the `/` search, left and
// right move the query's caret and tab never switches lists.
func TestSettingsSearchLeftMovesTheCaretNotTheLists(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	m.settingsOpen = true
	m = settingsPress(m, "/", "vrdict", "left", "left", "left", "left", "left", "e")
	focus := m.settingsFocus
	m = settingsPress(m, "right", "tab", "left")
	if !m.settingsSearchActive || m.settingsFocus != focus {
		t.Fatalf("search left/right/tab switched lists or left search: active=%v focus %d->%d", m.settingsSearchActive, focus, m.settingsFocus)
	}
	m = settingsPress(m, "enter")
	cat := settingsCategories()[m.settingsCategoryIndex]
	if cat.Fields[m.settingsFieldIndex].FullKey() != "stale_after" {
		t.Fatalf("enter jumped to %s, want stale_after", cat.Fields[m.settingsFieldIndex].FullKey())
	}
}

// TestSettingsSearchViewDrawsTheQueryRow: the search box is a row with the
// editor's caret, and the results follow it.
func TestSettingsSearchViewDrawsTheQueryRow(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	m.width, m.height = 100, 30
	m.settingsOpen = true
	m = settingsPress(m, "/", "verdict")
	view := m.View()
	if !strings.Contains(view, "Search: verdict") || !strings.Contains(view, "\x1b[7m") {
		t.Fatalf("search row or caret missing:\n%s", view)
	}
}
