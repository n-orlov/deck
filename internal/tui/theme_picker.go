package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/theme"
)

// This file is task 025's `t` theme picker (SPEC §11.6, requirement 27).
// Unlike every other overlay in this package (m.creating, m.settingsOpen,
// m.profileSwitching, m.pinning, m.help), the picker deliberately does NOT
// get its own full-screen View() branch: SPEC requirement 27 asks for a
// preview "live on the real list", so the real sidebar/preview panels
// (mainView) keep rendering throughout, coloured by activeTheme() (which
// consults themePickerValue while m.themePicking -- see theme_color.go),
// and the picker itself is a small banner mainView grows the same way
// themeBanner/attachErrorLines already do. Moving through the options
// changes only m.themePickerValue; esc never writes m.settings at all, so
// the revert this task's success criteria requires ("byte-for-byte what
// they were") is a structural consequence of never having mutated
// anything durable in the first place, not a separately-maintained undo.

// themePickerNames is the ordered list of names the picker cycles
// through: builtins first (alphabetical), then discovered user themes
// (alphabetical) -- settingsThemeOptions' existing order (task 015), so
// settings (`,`) and the picker (`t`) never disagree about theme
// ordering, and neither hand-maintains a second theme name list next to
// theme.Builtins()/theme.DiscoverUserThemes.
func (m Model) themePickerNames() []string {
	return settingsThemeOptions(m.settings.Paths)
}

// themePickerCandidateTheme resolves m.themePickerValue back to a real
// *theme.Theme, or nil when the picker's list is empty or the highlighted
// name no longer resolves (e.g. a user theme file removed out from under
// an open picker) -- both are the picker's own defined "degenerate list"
// behaviour (requirement 27): activeTheme falls back to whatever it would
// have rendered had the picker never opened, so a degenerate list previews
// nothing rather than a nil-pointer panic or a stale colour.
func (m Model) themePickerCandidateTheme() *theme.Theme {
	if m.themePickerValue == "" {
		return nil
	}
	if t, ok := theme.Builtin(m.themePickerValue); ok {
		return t
	}
	userThemes, _ := theme.DiscoverUserThemes(theme.ThemesDir(m.settings.Paths.ConfigFile))
	if t, ok := userThemes[m.themePickerValue]; ok {
		return t
	}
	return nil
}

// openThemePicker is `t`'s effect (SPEC §11.6): it starts the picker
// highlighting whatever theme is currently active (falling back to the
// list's first entry, or "" for an empty list -- requirement 27's
// degenerate-list case), and never touches m.settings itself.
func (m Model) openThemePicker() Model {
	m.themePicking = true
	m.themePickerNote = ""
	names := m.themePickerNames()
	current := ""
	if m.settings.Theme != nil {
		current = m.settings.Theme.Name
	}
	m.themePickerValue = ""
	for _, n := range names {
		if n == current {
			m.themePickerValue = n
			break
		}
	}
	if m.themePickerValue == "" && len(names) > 0 {
		m.themePickerValue = names[0]
	}
	return m
}

// updateThemePicker handles key input while the picker is open. Only the
// keys named on screen (themePickerLines) are load-bearing, per the
// §11.4 contract's "a dialog may declare additional load-bearing keys of
// its own, but only if it states them inline" -- left/right/up/down/space
// change the highlighted selection (mirroring updateProfileSwitch/updatePin
// Dialog's identical single-value-cycle shape elsewhere in this package),
// enter confirms, esc reverts. j/k are deliberately NOT bound here even
// though several other overlays in this codebase treat them as vi-style
// aliases: themePickerLines' banner only names Left/Right, Up/Down, Enter
// and Esc, and binding an unnamed key would violate the same rule this
// comment is quoting.
func (m Model) updateThemePicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	names := m.themePickerNames()
	switch msg.String() {
	case "esc":
		m.themePicking = false
		m.themePickerValue = ""
		m.themePickerNote = ""
		return m, nil
	case "left", "up":
		if len(names) > 0 {
			m.themePickerValue = cycleOption(names, m.themePickerValue, -1)
		}
		return m, nil
	case "right", "down", " ":
		if len(names) > 0 {
			m.themePickerValue = cycleOption(names, m.themePickerValue, 1)
		}
		return m, nil
	case "enter":
		return m.themePickerConfirm()
	}
	return m, nil
}

// themePickerConfirm is enter's effect: it resolves the highlighted name
// to a real theme, persists the choice to config.toml through task 012's
// atomic writer (exactly the write settingsSave uses, so a picker
// selection and a settings-takeover edit of the same [ui] theme key can
// never disagree about how it lands on disk), and only on success applies
// it to the running m.settings so every other render -- not just the
// picker's own preview -- reflects the choice from here on. A degenerate
// empty list, or a highlighted name that no longer resolves, closes the
// picker without writing or changing anything (there is nothing valid to
// select), which is requirement 27's degenerate-list behaviour on enter.
func (m Model) themePickerConfirm() (tea.Model, tea.Cmd) {
	candidate := m.themePickerCandidateTheme()
	if candidate == nil {
		m.themePicking = false
		m.themePickerValue = ""
		m.themePickerNote = ""
		return m, nil
	}
	if path := m.settings.Paths.ConfigFile; path != "" {
		cfg := settingsEditsFromSettings(m.settings)
		cfg.Theme = candidate.Name
		if err := config.WriteConfigFile(path, cfg); err != nil {
			m.themePickerNote = "save failed: " + err.Error()
			return m, nil
		}
	}
	m.settings.Theme = candidate
	m.settings.ThemeReason = ""
	m.themePicking = false
	m.themePickerValue = ""
	m.themePickerNote = ""
	return m, nil
}

// themePickerLines is the picker's own "banner" (mirroring themeBanner/
// attachErrorLines' shape): mainView appends it, and computeLayout
// reserves rows for it, exactly like those, so the picker never pushes the
// frame past the terminal's actual row count. It returns no lines at all
// while the picker is closed, matching every other banner's "costs
// nothing in the common case" convention.
func (m Model) themePickerLines(width int) []string {
	if !m.themePicking {
		return nil
	}
	names := m.themePickerNames()
	if len(names) == 0 {
		lines := wrapText("Theme picker: no themes available (no built-ins embedded and no user themes discovered) -- Esc closes.", width)
		return append(lines, "")
	}
	position := 0
	for i, n := range names {
		if n == m.themePickerValue {
			position = i + 1
			break
		}
	}
	sep := m.glyph(" · ", " - ")
	header := fmt.Sprintf("Theme picker: %s (%d of %d)%sLeft/Right or Up/Down changes%sEnter selects%sEsc reverts",
		m.themePickerValue, position, len(names), sep, sep, sep)
	lines := wrapText(header, width)
	if m.themePickerNote != "" {
		lines = append(lines, wrapText(m.themePickerNote, width)...)
	}
	return append(lines, "")
}
