package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/theme"
)

// themePickerTestModel builds a colour-enabled Model with a real
// config.toml on disk (so themePickerConfirm's write has somewhere to
// land) and one session loaded, on a generous frame so the picker's own
// banner line and the sidebar's coloured rows are both fully on screen.
func themePickerTestModel(t *testing.T) (Model, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("allow_yolo = false\n"), 0o644); err != nil {
		t.Fatalf("seed config.toml: %v", err)
	}
	def, ok := theme.Builtin(theme.DefaultName)
	if !ok {
		t.Fatalf("default builtin %q missing", theme.DefaultName)
	}
	m := New(nil, config.Settings{Color: true, Theme: def, Paths: config.Paths{ConfigFile: path}}, "")
	m.width, m.height = 120, 30
	m.sessions = []store.Session{
		{ID: "s1", Name: "alpha", Agent: "shell", Status: "running", CWD: "/repo/alpha"},
	}
	m.selected = 0
	return m, path
}

// otherBuiltinName returns a built-in theme name other than exclude, so
// tests can cycle the picker onto a theme guaranteed to differ from
// whatever is already active.
func otherBuiltinName(t *testing.T, exclude string) string {
	t.Helper()
	for _, th := range theme.Builtins() {
		if th.Name != exclude {
			return th.Name
		}
	}
	t.Fatalf("no second built-in theme besides %q; task 008 requires at least two", exclude)
	return ""
}

// TestThemePickerTKeyOpensListingBuiltinsAndUserThemes proves `t` opens the
// picker (requirement 27) with both a built-in and a discovered user theme
// reachable, generated the same way settingsThemeOptions already resolves
// them for the `,` takeover (task 015) -- never a second, hand-written
// theme name list.
func TestThemePickerTKeyOpensListingBuiltinsAndUserThemes(t *testing.T) {
	m, path := themePickerTestModel(t)
	themesDir := theme.ThemesDir(path)
	if err := os.MkdirAll(themesDir, 0o755); err != nil {
		t.Fatalf("mkdir themes dir: %v", err)
	}
	userTheme := "name = \"my-user-theme\"\nappearance = \"dark\"\n" + builtinThemeBodyForTest(t)
	if err := os.WriteFile(filepath.Join(themesDir, "mine.toml"), []byte(userTheme), 0o644); err != nil {
		t.Fatalf("write user theme: %v", err)
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	m = updated.(Model)
	if !m.themePicking {
		t.Fatalf("`t` did not open the theme picker")
	}
	names := m.themePickerNames()
	foundBuiltin, foundUser := false, false
	for _, n := range names {
		if n == theme.DefaultName {
			foundBuiltin = true
		}
		if n == "my-user-theme" {
			foundUser = true
		}
	}
	if !foundBuiltin {
		t.Fatalf("picker names %v missing built-in %q", names, theme.DefaultName)
	}
	if !foundUser {
		t.Fatalf("picker names %v missing discovered user theme", names)
	}
}

// builtinThemeBodyForTest returns a full, valid [colors] body copied from
// the default built-in (minus its own top-level `name` line, which the
// caller supplies), so a test-authored user theme file parses cleanly
// without hand-maintaining every token here.
func builtinThemeBodyForTest(t *testing.T) string {
	t.Helper()
	def, ok := theme.Builtin(theme.DefaultName)
	if !ok {
		t.Fatalf("default builtin missing")
	}
	var b strings.Builder
	b.WriteString("[colors]\n")
	for _, tok := range theme.AllTokens {
		hex, err := def.Color(tok)
		if err != nil {
			t.Fatalf("default theme lacks token %q: %v", tok, err)
		}
		b.WriteString(string(tok) + " = \"" + hex + "\"\n")
	}
	return b.String()
}

// TestThemePickerPreviewsLiveOnRealList proves requirement 27's "previews
// the theme live on the real list while you move through the options":
// moving the picker onto a different built-in changes the ACTIVE theme
// (activeTheme(), which every coloured render call goes through), without
// ever touching m.settings.Theme -- the field that would otherwise still
// hold the pre-picker value.
func TestThemePickerPreviewsLiveOnRealList(t *testing.T) {
	m, _ := themePickerTestModel(t)
	original := m.settings.Theme
	target := otherBuiltinName(t, original.Name)

	m = m.openThemePicker()
	// Cycle until the picker highlights target -- built-ins are few, so a
	// bounded walk is simplest and never risks an infinite loop on a
	// broken cycle.
	for i := 0; i < len(m.themePickerNames())+1 && m.themePickerValue != target; i++ {
		updated, _ := m.updateThemePicker(tea.KeyMsg{Type: tea.KeyRight})
		m = updated.(Model)
	}
	if m.themePickerValue != target {
		t.Fatalf("could not cycle picker onto %q, stuck at %q", target, m.themePickerValue)
	}

	if got := m.activeTheme().Name; got != target {
		t.Fatalf("activeTheme() = %q while picker highlights %q, want live preview to follow it", got, target)
	}
	if m.settings.Theme.Name != original.Name {
		t.Fatalf("m.settings.Theme changed to %q while only previewing; must stay %q until enter", m.settings.Theme.Name, original.Name)
	}
}

// TestThemePickerEscRevertsByteForByte proves requirement 27's actual
// assertion: preview, esc, and the frame's cells/attributes are
// byte-for-byte what they were before the picker opened -- read straight
// off View()'s output, not merely "the setting looks unchanged".
func TestThemePickerEscRevertsByteForByte(t *testing.T) {
	m, _ := themePickerTestModel(t)
	before := m.View()

	m = m.openThemePicker()
	target := otherBuiltinName(t, m.settings.Theme.Name)
	for i := 0; i < len(m.themePickerNames())+1 && m.themePickerValue != target; i++ {
		updated, _ := m.updateThemePicker(tea.KeyMsg{Type: tea.KeyRight})
		m = updated.(Model)
	}
	if mid := m.View(); mid == before {
		t.Fatalf("picker's live preview produced a frame identical to before opening it; the preview is not actually live")
	}

	updated, _ := m.updateThemePicker(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.themePicking {
		t.Fatalf("esc did not close the picker")
	}
	after := m.View()
	if after != before {
		t.Fatalf("esc did not revert byte-for-byte:\n--- before ---\n%s\n--- after ---\n%s", before, after)
	}
}

// TestThemePickerEnterSelectsAndPersists proves enter both applies the
// highlighted theme to the running settings (so every later render, not
// just the picker's own preview, reflects it) and writes it through task
// 012's atomic writer, so the choice survives a restart -- the same write
// path settingsSave uses, so `t` and `,` can never disagree about how
// [ui] theme lands in config.toml.
func TestThemePickerEnterSelectsAndPersists(t *testing.T) {
	m, path := themePickerTestModel(t)
	target := otherBuiltinName(t, m.settings.Theme.Name)

	m = m.openThemePicker()
	for i := 0; i < len(m.themePickerNames())+1 && m.themePickerValue != target; i++ {
		updated, _ := m.updateThemePicker(tea.KeyMsg{Type: tea.KeyRight})
		m = updated.(Model)
	}

	updated, _ := m.updateThemePicker(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.themePicking {
		t.Fatalf("enter did not close the picker")
	}
	if m.settings.Theme.Name != target {
		t.Fatalf("m.settings.Theme = %q after enter, want %q", m.settings.Theme.Name, target)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config.toml after enter: %v", err)
	}
	if !strings.Contains(string(data), `theme = "`+target+`"`) {
		t.Fatalf("config.toml after enter missing theme = %q:\n%s", target, string(data))
	}
}

// TestThemePickerNoteNamesLoadBearingKeysOnly proves the picker's own
// banner names exactly the keys that are load bearing while it is open
// (SPEC §11.4: "nothing undeclared is load-bearing") -- Enter and Esc.
func TestThemePickerNoteNamesLoadBearingKeysOnly(t *testing.T) {
	m, _ := themePickerTestModel(t)
	m = m.openThemePicker()
	view := m.View()
	for _, want := range []string{"Enter selects", "Esc reverts"} {
		if !strings.Contains(view, want) {
			t.Fatalf("picker view missing %q:\n%s", want, view)
		}
	}
}

// TestThemePickerDegenerateCandidateLeavesActiveThemeUnchanged proves
// requirement 27's "behaviour with a degenerate list is defined": a
// highlighted name that no longer resolves to any theme (e.g. a user
// theme file removed out from under an open picker) previews nothing --
// activeTheme falls back to exactly what it would have rendered had the
// picker never opened -- rather than a nil-pointer panic or a stale
// colour, and enter closes the picker without writing config.toml.
func TestThemePickerDegenerateCandidateLeavesActiveThemeUnchanged(t *testing.T) {
	m, path := themePickerTestModel(t)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read seeded config.toml: %v", err)
	}
	original := m.settings.Theme.Name

	m.themePicking = true
	m.themePickerValue = "a-name-that-does-not-resolve-to-any-theme"

	if got := m.activeTheme().Name; got != original {
		t.Fatalf("activeTheme() = %q with an unresolvable highlight, want fallback to %q", got, original)
	}

	updated, _ := m.updateThemePicker(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.themePicking {
		t.Fatalf("enter on an unresolvable candidate did not close the picker")
	}
	if m.settings.Theme.Name != original {
		t.Fatalf("m.settings.Theme changed to %q from an unresolvable candidate, want unchanged %q", m.settings.Theme.Name, original)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config.toml after enter: %v", err)
	}
	if string(after) != string(before) {
		t.Fatalf("config.toml changed from an unresolvable candidate:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

// TestThemePickerMouseIgnoredWhileOpen proves SPEC §11.4/§11.8: no dialog
// action is reachable by mouse alone, and the picker is no exception --
// mirroring the identical guard already proven for creating/profileSwitching/
// pinning/detail (mouse_test.go).
func TestThemePickerMouseIgnoredWhileOpen(t *testing.T) {
	m, _ := themePickerTestModel(t)
	m.settings.Mouse = true
	m = m.openThemePicker()
	before := m.themePickerValue

	updated, _ := m.Update(tea.MouseMsg{X: 0, Y: 0, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	m = updated.(Model)
	if !m.themePicking || m.themePickerValue != before {
		t.Fatalf("a mouse click while the picker is open changed state: themePicking=%v value=%q, want unchanged", m.themePicking, m.themePickerValue)
	}
}
