package tui

import (
	"fmt"

	"github.com/n-orlov/deck/internal/theme"
	"github.com/n-orlov/deck/internal/tui/lineedit"
)

// The create modal's field positions (createFieldRows' own order). Six of the
// ten are text, held in the shared line editor (§11.11, Model.createEdits);
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
	createFieldGroup
)

// createFieldLabels are the ten rows' labels, in field order.
var createFieldLabels = [createFieldCount]string{
	"Name", "Working directory", "Agent", "Permission profile", "Launch args (JSON array)",
	"Env (key=value, comma-separated)", "Pre-launch command", "Post-destroy command", "Login shell", "Group",
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
	return m.createEdits[field].View(width, style)
}
