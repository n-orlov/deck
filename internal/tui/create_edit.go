package tui

import (
	"fmt"
	"strings"

	"github.com/n-orlov/deck/internal/theme"
	"github.com/n-orlov/deck/internal/tui/lineedit"
)

// The create modal's field positions (createFieldRows' own order). Seven of the
// twelve are text, held in the shared line editor (§11.11, Model.createEdits);
// the rest are selections that left, right and space cycle.
const (
	createFieldName = iota
	createFieldCWD
	createFieldAgent
	createFieldProfile
	createFieldLaunchArgs
	createFieldEnv
	createFieldPreLaunch
	createFieldPostDestroy
	createFieldLoginShell
	// The next two are the session's own event-hook controls (SPEC §10.2).
	createFieldEventHook
	createFieldEventHookEvents
	createFieldGroup
)

// createFieldLabels are the twelve rows' labels, in field order.
var createFieldLabels = [createFieldCount]string{
	"Name", "Working directory", "Agent", "Permission profile", "Launch args (JSON array)",
	"Env (key=value, comma-separated)", "Pre-launch command", "Post-destroy command", "Login shell",
	"Event hook (event_hook_enabled)", "Event hook kinds (event_hook_events)", "Group",
}

// createText is a text field's current value as typed ("" for a selection).
func (m Model) createText(field int) string { return m.createEdits[field].Value() }

// setCreateText replaces a text field's value, caret at its end and not
// offered: a programmatic value (a recent_cwds step, a completion) is the
// user's own text from then on.
func (m *Model) setCreateText(field int, text string) {
	m.createEdits[field] = lineedit.New(text).Fit(m.createFieldWidth(field), m.createEditStyle(field))
}

// createFieldLabel is a row's drawn label (marker included), whose width is
// what a text field's own cells are budgeted against.
func (m Model) createFieldLabel(field int) string {
	return fmt.Sprintf("%s%s: ", m.createFieldMarker(field), createFieldLabels[field])
}

// createFieldWidth is the number of cells a text field has inside the dialog's
// box once its label has been drawn: the editor scrolls within it, so the row
// never wraps.
func (m Model) createFieldWidth(field int) int {
	w := m.dialogWidth() - 4 - stringWidth(m.createFieldLabel(field))
	if w < 1 {
		w = 1
	}
	return w
}

// createEditStyle is how the shared editor is drawn here: the clip marks
// follow DECK_ASCII and an offered value carries the theme's selection
// background (none under NO_COLOR). Only the focused field draws a caret.
func (m Model) createEditStyle(field int) lineedit.Style {
	sel, _ := m.backgroundSGR(theme.Selection)
	return lineedit.Style{ASCII: m.settings.ASCII, Selection: sel, Blurred: field != m.createField}
}

// createFieldText is a text field's drawn value: the editor's view, with the
// caret on the focused field only. The cwd's ghost completion trails it and
// carries the caret itself (createCWDGhostView), so its cells come out of the
// field's budget and the editor draws no caret of its own there.
func (m Model) createFieldText(field int) string {
	width, style := m.createFieldWidth(field), m.createEditStyle(field)
	if field == createFieldCWD {
		if ghost := m.createCWDGhostSuffix(); ghost != "" {
			width -= stringWidth(ghost)
			if width < 1 {
				width = 1
			}
			style.Blurred = true
		}
	}
	ed := m.createEdits[field]
	if field == createFieldEnv {
		// SPEC §6.4: a secret-shaped key's value is masked in every view, the
		// create modal's Env field included, until this view's own reveal
		// toggle (Ctrl+R on the field) is on. The draw swaps in the masked
		// text with the caret at its end; the stored value is untouched.
		if masked := m.maskCreateEnvText(ed.Value(), m.createEnvReveal); masked != ed.Value() {
			ed = lineedit.New(masked)
		}
	}
	return ed.View(width, style)
}

// maskCreateEnvText applies maskEnvValue to every `key=value` entry of the
// Env field's comma-separated text. A key's own text and the entry's
// separators are kept; only a non-empty value of a secret-shaped key becomes
// the fixed placeholder (never the real length). revealed returns text as is.
func (m Model) maskCreateEnvText(text string, revealed bool) string {
	if revealed || text == "" {
		return text
	}
	entries := strings.Split(text, ",")
	for i, entry := range entries {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || value == "" {
			continue
		}
		entries[i] = key + "=" + m.maskEnvValue(strings.TrimSpace(key), value, false)
	}
	return strings.Join(entries, ",")
}

// createEnvMasked reports whether the Env field is currently drawing a
// masked secret: copying it then is refused, as the env editor's is.
func (m Model) createEnvMasked() bool {
	text := m.createText(createFieldEnv)
	return m.maskCreateEnvText(text, m.createEnvReveal) != text
}

// createEnvHelp is the Env row's one-line explanation, naming the §6.4 reveal
// toggle for as long as it is relevant.
func (m Model) createEnvHelp() string {
	help := "session env, wins PATH resolution; secrets masked, Ctrl+R reveals"
	if m.createEnvReveal {
		help = "session env, wins PATH resolution; secrets shown, Ctrl+R masks"
	}
	return help
}
