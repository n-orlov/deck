package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/theme"
	"github.com/n-orlov/deck/internal/tui/lineedit"
)

// This file is task 023's launch-inputs editor (SPEC §6.2/§11.4, PRD
// R108): the four editable launch inputs -- pre_launch, post_destroy,
// launch_args and login_shell -- reached ONLY from inside the `i` detail
// dialog, exactly as rename is (rename.go's "l" case is the one and only
// place that ever sets m.launchInputsEditing = true). Every field here is
// restart-to-apply: a launch input is by definition consumed at launch
// (§6.2), so writing one here only sets launch_dirty -- it never reaches
// a pane that is already running, and only `R` (restart) applies it. The
// store mutator underneath a submit (m.launchInputsSetter) is
// store.Store.SetLaunchInputs (task 020); this dialog never mutates
// agent, cwd, slug or captured_path, which have no mutator at all
// (task 022 pins that as a source-scanning guard in internal/store).

// launchInputsFieldCount and the four field indices below are
// launchInputsFieldRows' own order: pre_launch, post_destroy, launch_args,
// login_shell -- SPEC §6.2's own listing order, and the order R108
// documents its "four editable launch inputs" in.
const launchInputsFieldCount = 4

// launchInputsTextFieldCount is how many of the fields above are text, held in
// the shared line editor (§11.11): the first three, in the same order.
const launchInputsTextFieldCount = 3

const (
	launchInputsFieldPreLaunch = iota
	launchInputsFieldPostDestroy
	launchInputsFieldLaunchArgs
	launchInputsFieldLoginShell
)

// launchInputsFieldIsText reports whether field accepts free-typed runes,
// as opposed to being a cycled selection (login_shell, the one boolean
// field) that only left/right/space change -- mirroring
// createFieldIsText's identical role for the create modal's own field set.
func launchInputsFieldIsText(field int) bool {
	return field != launchInputsFieldLoginShell
}

// launchArgsToText renders a session's stored LaunchArgs as the same JSON
// array text the create modal's own "Launch args (JSON array)" field
// holds (createLaunchArgs/parseCreateLaunchArgs), so typing here follows
// the one convention this package already has for launch_args rather than
// inventing a second (space-separated, newline-separated, ...) just for
// this dialog. A nil/empty slice renders as "" -- an empty field, not the
// literal "[]" -- matching parseLaunchInputsArgs' own "\"\" means nil"
// rule below, so opening the dialog on a session with no launch_args and
// submitting without touching the field is a true no-op.
func launchArgsToText(args []string) string {
	if len(args) == 0 {
		return ""
	}
	data, err := json.Marshal(args)
	if err != nil {
		return ""
	}
	return string(data)
}

// parseLaunchInputsArgs is launchArgsToText's inverse, mirroring
// parseCreateLaunchArgs exactly (same JSON-array-of-strings contract, same
// "" is fine rule, same error wording) so the two launch_args fields in
// this package can never silently disagree about what a typed value means.
func parseLaunchInputsArgs(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var args []string
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return nil, fmt.Errorf("launch_args must be a JSON array of strings: %w", err)
	}
	return args, nil
}

// launchInputsFieldRows describes the launch-inputs editor's field set:
// label, current value and a one-line explanation -- the same
// label/value/help table shape createFieldRows uses for the create modal,
// so launchInputsBody/styledLaunchInputsBody can share createBody's own
// walk-the-rows structure rather than inventing a second one. Every help
// line ends in "restart-to-apply" verbatim (SPEC §6.2/§11.4, PRD R108:
// "every field labelled restart-to-apply"), and the two hook fields' help
// also states their own fail-closed/fail-open contract (SPEC §9.1/§9.2)
// so a reader never has to leave this dialog to find out what a hook
// failing here actually does.
func (m Model) launchInputsFieldRows() []struct{ label, value, help string } {
	loginShell := "off"
	if m.launchInputsLoginShell {
		loginShell = "on"
	}
	return []struct{ label, value, help string }{
		{launchInputsLabels[0], m.launchInputsFieldText(launchInputsFieldPreLaunch), "runs before the agent starts on every launch and must be idempotent; a non-zero exit blocks the launch (fail-closed) -- restart-to-apply"},
		{launchInputsLabels[1], m.launchInputsFieldText(launchInputsFieldPostDestroy), "runs after Archive or Delete durably succeeds; a non-zero exit or timeout never blocks teardown (fail-open) -- restart-to-apply"},
		{launchInputsLabels[2], m.launchInputsFieldText(launchInputsFieldLaunchArgs), "extra arguments appended verbatim after the adapter's own argv -- restart-to-apply"},
		{launchInputsLabels[3], loginShell + " (space toggles)", "runs the pane command through $SHELL -lc, letting rc files rewrite PATH -- restart-to-apply"},
	}
}

// launchInputsFieldMarker is this dialog's own ">"/"  " focus marker,
// mirroring createFieldMarker one file over.
func (m Model) launchInputsFieldMarker(field int) string {
	if m.launchInputsField == field {
		return "> "
	}
	return "  "
}

// launchInputsFieldLabel is a row's drawn label (marker included), whose width
// is what a text field's own cells are budgeted against.
func (m Model) launchInputsFieldLabel(field int) string {
	return fmt.Sprintf("%s%s: ", m.launchInputsFieldMarker(field), launchInputsLabels[field])
}

// launchInputsLabels are the four rows' labels, in field order.
var launchInputsLabels = [launchInputsFieldCount]string{"Pre-launch command", "Post-destroy command", "Launch args (JSON array)", "Login shell"}

// launchInputsFieldWidth is the number of cells a text field has inside the
// dialog's box once its label has been drawn: the editor scrolls within it, so
// the row never wraps.
func (m Model) launchInputsFieldWidth(field int) int {
	w := m.dialogWidth() - 4 - stringWidth(m.launchInputsFieldLabel(field))
	if w < 1 {
		w = 1
	}
	return w
}

// launchInputsEditStyle is how the shared editor is drawn here: the clip marks
// follow DECK_ASCII and an offered value carries the theme's selection
// background (none under NO_COLOR). Only the focused field draws a caret.
func (m Model) launchInputsEditStyle(field int) lineedit.Style {
	sel, _ := m.backgroundSGR(theme.Selection)
	return lineedit.Style{ASCII: m.settings.ASCII, Selection: sel, Blurred: field != m.launchInputsField}
}

// launchInputsFieldText is a text field's drawn value: the editor's view, with
// the caret on the focused field only. The hook commands are never masked.
func (m Model) launchInputsFieldText(field int) string {
	return m.launchInputsEdits[field].View(m.launchInputsFieldWidth(field), m.launchInputsEditStyle(field))
}

// launchInputsValue is a text field's current value as typed.
func (m Model) launchInputsValue(field int) string { return m.launchInputsEdits[field].Value() }

// launchInputsFooterLine is the closing legend both launchInputsBody and
// styledLaunchInputsBody render verbatim, shared so the two can never
// state the keymap differently.
const launchInputsFooterLine = "\u2191/\u2193 field \u00b7 Left/Right/Space toggles Login shell \u00b7 Enter submits \u00b7 Esc cancels"

// launchInputsVerbatimNote is the one-line statement SPEC §6.4/§11.4
// requires: the two hook fields above are shown exactly as typed, never
// masked -- they are commands, not secret-shaped values, and §6.4's own
// recommendation is that a command SOURCES a secret rather than
// containing one.
const launchInputsVerbatimNote = "Pre-launch and Post-destroy are shown verbatim above, never masked -- they are commands, not secret values."

// launchInputsBody builds the launch-inputs editor's PLAIN, unstyled
// content -- the one text wrapDialogLines/dialogMaxScroll/PgUp/PgDn
// measure (mirroring createBody's own role), so a colour token
// styledLaunchInputsBody adds can never move where a page boundary falls.
func (m Model) launchInputsBody() string {
	session, _ := m.selectedSession()
	var b strings.Builder
	fmt.Fprintf(&b, "Launch inputs for %s\n\n", session.Name)
	for field, row := range m.launchInputsFieldRows() {
		fmt.Fprintf(&b, "%s%s\n    %s\n", m.launchInputsFieldLabel(field), row.value, row.help)
	}
	b.WriteString("\n" + launchInputsVerbatimNote + "\n")
	b.WriteString(launchInputsFooterLine + "\n")
	if launchInputsFieldIsText(m.launchInputsField) {
		b.WriteString(textFieldEditKeysLine + "\n")
	}
	if m.launchInputsNote != "" {
		fmt.Fprintf(&b, "\n%s\n", m.launchInputsNote)
	}
	return b.String()
}

// launchInputsView renders launchInputsBody inside framedDialogScrollable
// (mirroring detailView/eventLogView -- a themed dialog whose field set
// plus its four help lines can push past the frame budget), themed via
// styledLaunchInputsBody's own §11.6 token pass exactly as R108 requires.
func (m Model) launchInputsView() string {
	return m.framedDialogScrollable(m.styledLaunchInputsBody(), m.launchInputsScroll)
}

// launchInputsLegendKeys is styledLaunchInputsBody's own footer key
// vocabulary, mirroring createFooterKeyTokens/envBrowseLegendKeys one
// file over: the leading token(s) of launchInputsFooterLine's own
// key phrases, used to decide which already-wrapped word gets theme.Key
// instead of theme.Hint.
var launchInputsLegendKeys = map[string]bool{
	"\u2191/\u2193":    true,
	"Left/Right/Space": true,
	"Enter":            true,
	"Esc":              true,
}

// styledLaunchInputsBody re-derives launchInputsBody's exact structure --
// same title, blank line, field-row loop (label/value then its help line),
// verbatim note, footer legend and optional error note, in the same order
// -- but colours each finished PHYSICAL line rather than the logical one,
// exactly like styledCreateBody/styledEnvBody: every helper below wraps a
// plain string via wrap (m.wrapDialogLines) FIRST, so a colour token can
// never straddle a word-wrap boundary wrapDialogLines has not drawn yet.
// Token mapping is SPEC.md:1355 verbatim: the title in `title`, a field's
// label in `hint` and its value in `text` (the focused field carrying the
// same `selection` treatment a selected list row does, via
// renderCreateRowSegments), its help line in `dimmed`, the verbatim note
// in `dimmed`, the footer legend's keys in `key` and the rest of it in
// `hint`, and a validation/failed-submit note in `error`.
func (m Model) styledLaunchInputsBody() string {
	wrap := m.wrapDialogLines
	var out []string

	colorWhole := func(tok theme.Token, line string) {
		for _, l := range wrap(line) {
			out = append(out, m.colorToken(tok, l))
		}
	}
	colorRow := func(label, value string, focused bool) {
		for _, l := range wrap(label + value) {
			var segs []settingsRowSegment
			rest := l
			if strings.HasPrefix(l, label) {
				segs = append(segs, settingsRowSegment{Text: label, Tok: theme.Hint})
				rest = strings.TrimPrefix(l, label)
			}
			if rest != "" {
				segs = append(segs, settingsRowSegment{Text: rest, Tok: theme.Text})
			}
			if len(segs) == 0 {
				segs = []settingsRowSegment{{Text: l, Tok: theme.Text}}
			}
			out = append(out, m.renderCreateRowSegments(focused, segs))
		}
	}
	colorLegendLine := func(line string, keys map[string]bool) {
		for _, l := range wrap(line) {
			fields := strings.Fields(l)
			for i, f := range fields {
				trimmed := strings.TrimRight(f, ",;.")
				if keys[trimmed] {
					fields[i] = m.colorToken(theme.Key, trimmed) + f[len(trimmed):]
				} else {
					fields[i] = m.colorToken(theme.Hint, f)
				}
			}
			out = append(out, strings.Join(fields, " "))
		}
	}

	session, _ := m.selectedSession()
	colorWhole(theme.Title, fmt.Sprintf("Launch inputs for %s", session.Name))
	out = append(out, "")
	for field, row := range m.launchInputsFieldRows() {
		label := m.launchInputsFieldLabel(field)
		colorRow(label, row.value, field == m.launchInputsField)
		colorWhole(theme.Dimmed, "    "+row.help)
	}
	out = append(out, "")
	colorWhole(theme.Dimmed, launchInputsVerbatimNote)
	colorLegendLine(launchInputsFooterLine, launchInputsLegendKeys)
	if launchInputsFieldIsText(m.launchInputsField) {
		out = append(out, m.styledTextFieldEditKeys(wrap)...)
	}
	if m.launchInputsNote != "" {
		out = append(out, "")
		colorWhole(theme.Error, m.launchInputsNote)
	}
	return strings.Join(out, "\n")
}

// updateLaunchInputsDialog handles every key while the launch-inputs
// editor is open. Three of the four fields are free text
// (launchInputsFieldIsText); the fourth (login_shell) is a boolean only
// left/right/space (via the shared contract's Cycle) ever changes --
// TextFocused is what keeps all three off the contract while a text field is
// focused, where they are the shared line editor's (§11.11).
func (m Model) updateLaunchInputsDialog(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if cmd, handled := applyDialogContract(msg, dialogContract{
		Fields: dialogFields{
			Count:          launchInputsFieldCount,
			Index:          &m.launchInputsField,
			Cycle:          m.cycleLaunchInputsField,
			SpaceTypesText: func() bool { return launchInputsFieldIsText(m.launchInputsField) },
			TextFocused:    func() bool { return launchInputsFieldIsText(m.launchInputsField) },
		},
		Cancel: func() {
			m.launchInputsEditing = false
			m.launchInputsNote = ""
		},
		Submit: m.submitLaunchInputs,
	}); handled {
		return m, cmd
	}
	switch msg.String() {
	case "pgup":
		// Mirrors createView's own task 016 (measured off m.launchInputsBody(),
		// the plain body, never the coloured one, so a theme change can
		// never move where a page boundary falls).
		m.launchInputsScroll = m.dialogScrollByPage(m.launchInputsScroll, m.launchInputsBody(), -1)
		return m, nil
	case "pgdown":
		m.launchInputsScroll = m.dialogScrollByPage(m.launchInputsScroll, m.launchInputsBody(), 1)
		return m, nil
	}
	// Every editing key, a typed rune and a bracketed paste on a text field
	// belong to the shared line editor (§11.11): the opening value is an offered
	// value, so a printable key or a paste replaces it and a caret or editing
	// key accepts it and edits it in place. A key the editor does not own is
	// left alone, and the selection field has no editor at all.
	if field := m.launchInputsField; launchInputsFieldIsText(field) {
		if isFieldCopyKey(msg) {
			return m.copyFieldText(m.launchInputsEdits[field].Value(), false), nil
		}
		if edited, ok := m.launchInputsEdits[field].Update(msg); ok {
			m.launchInputsEdits[field] = edited.Fit(m.launchInputsFieldWidth(field), m.launchInputsEditStyle(field))
		}
	}
	return m, nil
}

// cycleLaunchInputsField implements left/right/space's dialogContract.Cycle
// for the login_shell field, mirroring cycleCreateField's own
// case 7 (m.createLoginShell = !m.createLoginShell) exactly: the delta's
// sign is ignored, since a boolean has nothing to cycle THROUGH, only to
// flip. It is only ever reached while the login_shell selection is focused:
// on a text field left, right and space are the shared line editor's
// (dialogFields.TextFocused), so Cycle applies to selection fields only.
func (m *Model) cycleLaunchInputsField(_ int) {
	if m.launchInputsField == launchInputsFieldLoginShell {
		m.launchInputsLoginShell = !m.launchInputsLoginShell
	}
}

// submitLaunchInputs dispatches the launch-inputs editor's Enter (SPEC
// §11.4 submit) through m.launchInputsSetter (nil when no setter is
// wired, e.g. an internal/tui-only test model), mutating the caller's
// local Model in place and returning only the resulting tea.Cmd -- the
// same shape submitRename/submitEnvEdit already use. Validation
// (launch_args must parse as a JSON array of strings) happens in-dialog
// BEFORE the setter is ever consulted, and retains every field exactly
// as typed on failure (SPEC §11.4: "validation is in-dialog and retains
// what the user typed"), mirroring validateCreateFields' own launch_args
// check one file over.
func (m *Model) submitLaunchInputs() tea.Cmd {
	args, err := parseLaunchInputsArgs(m.launchInputsValue(launchInputsFieldLaunchArgs))
	if err != nil {
		m.launchInputsNote = err.Error()
		return nil
	}
	if m.launchInputsSetter == nil {
		m.launchInputsNote = "editing launch inputs is unavailable"
		return nil
	}
	if len(m.sessions) == 0 {
		return nil
	}
	session, _ := m.selectedSession()
	sessionID := session.ID
	preLaunch, postDestroy := m.launchInputsValue(launchInputsFieldPreLaunch), m.launchInputsValue(launchInputsFieldPostDestroy)
	loginShell := m.launchInputsLoginShell
	setter := m.launchInputsSetter
	return func() tea.Msg {
		updated, err := setter(context.Background(), sessionID, preLaunch, postDestroy, args, loginShell)
		return launchInputsSaved{session: updated, err: err}
	}
}
