package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/theme"
)

// This file is task 013's `i` detail dialog's rename action (SPEC §11.4,
// PRD requirement 31, I-8). Rename is reachable ONLY from inside the `i`
// detail dialog: updateDetailView below is the one and only place that
// ever sets m.renaming = true, and there is no case anywhere in this
// package's top-level key switch (Model.Update's main tea.KeyMsg handling)
// that does the same -- a plain top-level "r" keeps meaning resume
// (case "r" there), exactly like "r" inside the `e` env editor already
// means something else again (toggle reveal) purely because envEditing has
// its own dedicated update dispatch the same way m.detail now does.
//
// m.detail stays true for as long as the rename sub-dialog is open: it is
// an action INSIDE detail, not a sibling of it, so cancelling or
// submitting a rename returns to detailView (still showing the selected
// session), never all the way out to the main list.

// updateDetailView handles every key while the `i` detail dialog is open
// and no rename or launch-inputs edit is in progress (m.renaming ==
// false and m.launchInputsEditing == false; see updateRenameDialog and
// updateLaunchInputsDialog, launch_inputs.go, for those nested states).
// detailView itself has no §11.4 fields to submit or cycle, so the only
// shared contract key it binds is esc; "i" (declared inline, mirroring
// detailView's own footer text "i or Esc closes detail"), "r" (this
// task's additional load-bearing key, declared inline exactly as §11.4
// allows), "l" (task 023's own load-bearing key, opening the
// launch-inputs editor exactly as "r" opens rename), pgup/
// pgdown (task 078's whole-dialog scroll, requirement 39 residual) and
// "q"/"ctrl+c" (task 079: a bare q while m.detail is true used to be a
// silent no-op here -- helpText's own "q or Ctrl+C quit deck" line in
// the Keys section already promises this unconditionally, matching
// updateHelpView's identical case below, so this is bringing the
// implementation in line with copy that was already shipped, not
// documenting a new binding) are the keys detailView adds beyond that
// shared contract.
func (m Model) updateDetailView(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if cmd, handled := applyDialogContract(msg, dialogContract{
		Cancel: func() {
			m.help = false
			m.detail = false
			m.marked = nil
		},
	}); handled {
		return m, cmd
	}
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "i":
		m.detail = false
	case "r":
		if len(m.sessions) > 0 {
			session := m.sessions[m.selected]
			m.renaming = true
			m.renameValue = session.Name
			m.renamePrefilled = true
			m.renameNote = ""
		}
	case "l":
		// Task 023 (SPEC §6.2/§11.4, PRD R108): the launch-inputs editor,
		// reachable ONLY from inside `i` detail, exactly like "r" above.
		if len(m.sessions) > 0 {
			session := m.sessions[m.selected]
			m.launchInputsEditing = true
			m.launchInputsField = 0
			m.launchInputsPreLaunch = session.PreLaunch
			m.launchInputsPostDestroy = session.PostDestroy
			m.launchInputsLaunchArgs = launchArgsToText(session.LaunchArgs)
			m.launchInputsLoginShell = session.LoginShell
			m.launchInputsNote = ""
			m.launchInputsScroll = 0
		}
	case "pgup":
		// Task 078 (requirement 39 residual): the whole dialog scrolls
		// uniformly via detailBody's own content, never a per-field bound.
		m.detailScroll = m.dialogScrollByPage(m.detailScroll, m.detailBody(), -1)
	case "pgdown":
		m.detailScroll = m.dialogScrollByPage(m.detailScroll, m.detailBody(), 1)
	case "up", "k":
		// R73 (issue #7): one line per press, so the arrows are not a
		// second PgUp/PgDn.
		m.detailScroll = m.dialogScrollByLines(m.detailScroll, m.detailBody(), -1)
	case "down", "j":
		m.detailScroll = m.dialogScrollByLines(m.detailScroll, m.detailBody(), 1)
	}
	return m, nil
}

// updateRenameDialog handles every key while the rename sub-dialog (task
// 013) is open. It is a single free-text field, so §11.4's tab/shift-tab
// field navigation and left/right/space field-cycling have nothing to do
// here (dialogFields' zero value, like detailView/helpView) -- SpaceTypesText
// always reports true, so space is ordinary typed input (a name containing
// a space) rather than the contract's "change a selection", exactly like
// createView's free-text fields already declare for themselves.
func (m Model) updateRenameDialog(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if cmd, handled := applyDialogContract(msg, dialogContract{
		Fields: dialogFields{SpaceTypesText: func() bool { return true }},
		Cancel: func() {
			m.renaming = false
			m.renameValue, m.renamePrefilled, m.renameNote = "", false, ""
		},
		Submit: m.submitRename,
	}); handled {
		return m, cmd
	}
	switch msg.String() {
	case "backspace", "ctrl+h":
		// Mirrors createView's cwd field and the env editor's value field:
		// while the buffer still holds nothing but the untouched prefill
		// (the session's current name), backspace clears it wholesale
		// rather than trimming one rune off the end of it.
		if m.renamePrefilled {
			m.renameValue, m.renamePrefilled = "", false
			return m, nil
		}
		if m.renameValue != "" {
			runes := []rune(m.renameValue)
			m.renameValue = string(runes[:len(runes)-1])
		}
		return m, nil
	}
	if runes := msg.Runes; len(runes) > 0 {
		// The prefilled current name is replaced wholesale by the first
		// keystroke rather than appended to (same rule as above): once
		// the user has typed anything, the field holds only what they
		// typed.
		if m.renamePrefilled {
			m.renameValue, m.renamePrefilled = "", false
		}
		m.renameValue += string(runes)
	}
	return m, nil
}

// submitRename dispatches the rename sub-dialog's Enter (SPEC §11.4
// submit) through m.renamer (nil when no renamer is wired, e.g. an
// internal/tui-only test model), mutating the caller's local Model in
// place and returning only the resulting tea.Cmd -- the same shape
// submitCreate/submitEnvEdit already use.
func (m *Model) submitRename() tea.Cmd {
	if m.renamer == nil {
		m.renameNote = "renaming is unavailable"
		return nil
	}
	if len(m.sessions) == 0 {
		return nil
	}
	session := m.sessions[m.selected]
	sessionID, newName := session.ID, m.renameValue
	renamer := m.renamer
	return func() tea.Msg {
		updated, err := renamer(context.Background(), sessionID, newName)
		return sessionRenamed{session: updated, err: err}
	}
}

// renameView renders the rename sub-dialog: a single free-text field
// prefilled with the session's current name, and the on-screen statement
// SPEC §11.4/PRD requirement 31 both require -- the tmux session name is
// NOT changing, deck's display name and tmux's own session name are
// deliberately decoupled -- so the user is told this rather than left to
// discover it later. Task 021: rendered through styledRenameBody, never
// renameBody itself, the same split styledArchiveConfirmBody/
// styledProfileSwitchBody already draw for their own dialogs, so §11.6's
// tokens land at render time without ever being baked into the plain
// string a test or the box's own word-wrap measures.
func (m Model) renameView() string {
	return m.framedDialog(m.styledRenameBody())
}

// renameBody builds renameView's text before framedDialog's box-width
// padTrunc touches it, split out for the same reason archiveConfirmBody
// is: a test can assert the exact wording without a terminal-rendering
// concern in between.
func (m Model) renameBody() string {
	session := m.sessions[m.selected]
	var b strings.Builder
	fmt.Fprintf(&b, "Rename %s\n\n", session.Name)
	fmt.Fprintf(&b, "%s\n", m.detailField("New name:  ", m.renameValue))
	fmt.Fprintf(&b, "\nThis changes only the display name. The tmux session stays named\n%q; it is never renamed, so a rename can never move or disturb a\nlive pane's identity.\n", "deck_"+session.Slug)
	b.WriteString("\nType a new name · Enter confirms · Esc cancels\n")
	if m.renameNote != "" {
		fmt.Fprintf(&b, "\n%s\n", m.renameNote)
	}
	return b.String()
}

// renameFooterKeyTokens is styledRenameBody's own footer vocabulary (task
// 021), the same shape as archiveConfirmFooterKeyTokens: the leading
// token of each key phrase in renameBody's own submit line ("Type a new
// name · Enter confirms · Esc cancels"), used to decide which
// already-wrapped word gets theme.Key instead of theme.Hint.
var renameFooterKeyTokens = map[string]bool{
	"Enter": true,
	"Esc":   true,
}

// renderRenameFieldRow renders the rename sub-dialog's single "New
// name:" field. Unlike detailField (which self-resets each half and is
// explicitly documented there as never composing under a shared
// selection background), this field is the ONE field the rename dialog
// has, so it is always the focused field whenever the dialog is showing
// -- SPEC.md:1355's "focused field carries the same selection treatment a
// selected list row does" therefore applies unconditionally here, the
// same way task 016 applied it to the create modal's currently-focused
// row (renderCreateRowSegments, tui.go). The label/value foregrounds are
// opened via settingsRenderRowOpen (settings.go) -- which opens each
// segment's colour but never closes it -- and the whole composed string
// is wrapped in exactly one bgColorToken(theme.Selection, ...) reset at
// the end; a per-segment colorToken reset would double as clearing that
// outer background the instant the label's own text ended
// (foregroundSGR's own doc comment, theme_color.go), which is why this
// cannot reuse detailField's plain self-resetting halves.
func (m Model) renderRenameFieldRow() string {
	segs := []settingsRowSegment{
		{Text: "New name:  ", Tok: theme.Hint},
		{Text: m.renameValue, Tok: theme.Text},
	}
	return m.bgColorToken(theme.Selection, m.settingsRenderRowOpen(segs))
}

// styledRenameBody re-derives renameBody's exact structure -- same
// title/field/explanation/footer/note order -- but colours each finished
// PHYSICAL line rather than the logical one, exactly like
// styledArchiveConfirmBody: every string below is wrapped via
// m.wrapDialogLines FIRST, and only the strings that call already
// returned are ever coloured, so a colour token can never straddle a
// word-wrap boundary wrapDialogLines has not drawn yet. Token mapping is
// SPEC.md:1355: the title in `title`, the New name field via detailField
// (already hint/text, task 022 -- untouched here), the tmux-name
// explanation in `dimmed`, the footer legend's keys in `key` and the rest
// of it in `hint`, and a failed-submit note in `error`.
func (m Model) styledRenameBody() string {
	session := m.sessions[m.selected]
	var out []string
	colorWhole := func(tok theme.Token, line string) {
		for _, l := range m.wrapDialogLines(line) {
			out = append(out, m.colorToken(tok, l))
		}
	}
	colorFooterLine := func(line string) {
		for _, l := range m.wrapDialogLines(line) {
			fields := strings.Fields(l)
			for i, f := range fields {
				if renameFooterKeyTokens[f] {
					fields[i] = m.colorToken(theme.Key, f)
				} else {
					fields[i] = m.colorToken(theme.Hint, f)
				}
			}
			out = append(out, strings.Join(fields, " "))
		}
	}

	colorWhole(theme.Title, fmt.Sprintf("Rename %s", session.Name))
	out = append(out, "")
	out = append(out, m.renderRenameFieldRow())
	out = append(out, "")
	colorWhole(theme.Dimmed, fmt.Sprintf("This changes only the display name. The tmux session stays named\n%q; it is never renamed, so a rename can never move or disturb a\nlive pane's identity.", "deck_"+session.Slug))
	out = append(out, "")
	colorFooterLine("Type a new name · Enter confirms · Esc cancels")
	if m.renameNote != "" {
		out = append(out, "")
		colorWhole(theme.Error, m.renameNote)
	}
	return strings.Join(out, "\n")
}
