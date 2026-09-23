package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/store"
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
			session, _ := m.selectedSession()
			m.renaming = true
			m.renameValue = session.Name
			m.renamePrefilled = true
			m.renameNote = ""
		}
	case "l":
		// Task 023 (SPEC §6.2/§11.4, PRD R108): the launch-inputs editor,
		// reachable ONLY from inside `i` detail, exactly like "r" above.
		if len(m.sessions) > 0 {
			session, _ := m.selectedSession()
			m.launchInputsEditing = true
			m.launchInputsField = 0
			m.launchInputsPreLaunch = session.PreLaunch
			m.launchInputsPostDestroy = session.PostDestroy
			m.launchInputsLaunchArgs = launchArgsToText(session.LaunchArgs)
			m.launchInputsLoginShell = session.LoginShell
			m.launchInputsNote = ""
			m.launchInputsScroll = 0
		}
	case "g":
		// R130 part 2 (SPEC §11): the group-move picker, reachable ONLY
		// from inside `i` detail, exactly like "r"/"l" above. Collision
		// checked against list-level g/G (top/bottom, tui.go's own
		// visibleSessionIndices navigation): both are guarded by
		// !m.detail, so there is no dispatch conflict with this case.
		//
		// task 013/D.2: this used to mutate unconditionally with no check
		// at all -- the one site the guard's own doc comment calls out by
		// name as the defect it exists to close. Routed through the same
		// shared guardSessionScopedKey (session_scoped_guard.go) every
		// top-level session-scoped binding now uses, under the synthetic
		// "detail:g" key so it can never collide with the unrelated
		// top-level `g` (jump to first stop).
		if m.guardSessionScopedKey("detail:g") {
			return m, nil
		}
		session, _ := m.selectedSession()
		m.movingGroup = true
		m.moveGroupOptions = m.computeAvailableGroups()
		m.moveGroupValue = sessionGroupID(session)
		m.moveGroupNote = ""
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
	session, _ := m.selectedSession()
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
	session, _ := m.selectedSession()
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
	session, _ := m.selectedSession()
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

// moveGroupCycleOptions returns the `g` move picker's full cycle order
// (R130 part 2), mirroring createGroupCycleOptions exactly: every group in
// m.moveGroupOptions (already alphabetical, case-insensitive --
// store.ListGroups' own order), plus the structural default group
// appended last, matching R129's sidebar order (default always sorts
// after every real group, never among them, since it is not a row at all
// -- store.Group's own doc comment).
func (m Model) moveGroupCycleOptions() []store.Group {
	options := make([]store.Group, 0, len(m.moveGroupOptions)+1)
	options = append(options, m.moveGroupOptions...)
	options = append(options, store.Group{ID: 0, Name: "default"})
	return options
}

// moveGroupName resolves id against moveGroupCycleOptions, falling back to
// "default" for 0 or for any id no longer present in the current cycle set
// (e.g. another client deleted the group while this picker was open) --
// SPEC §11's "a group_id that no longer resolves renders under default
// rather than vanishing" applies here exactly as it does to a session row
// (createGroupName's identical precedent).
func (m Model) moveGroupName(id int64) string {
	for _, g := range m.moveGroupCycleOptions() {
		if g.ID == id {
			return g.Name
		}
	}
	return "default"
}

// updateMoveGroupDialog handles keys while the `g` group-move picker
// (R130 part 2, reachable ONLY from inside the `i` detail dialog) is
// open. It only ever cycles a locally-held candidate group id and, on
// confirmation, persists it through m.groupMover; it never touches any
// other column -- mirroring updateProfileSwitch's identical shape.
func (m Model) updateMoveGroupDialog(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	options := m.moveGroupCycleOptions()
	cycle := func(delta int) {
		idx := 0
		for i, g := range options {
			if g.ID == m.moveGroupValue {
				idx = i
				break
			}
		}
		idx = (idx + delta + len(options)) % len(options)
		m.moveGroupValue = options[idx].ID
	}
	if cmd, handled := applyDialogContract(msg, dialogContract{
		Fields: dialogFields{Cycle: cycle},
		Cancel: func() {
			m.movingGroup = false
			m.moveGroupNote = ""
		},
		Submit: m.submitGroupMove,
	}); handled {
		return m, cmd
	}
	return m, nil
}

// submitGroupMove dispatches the group-move picker's Enter (SPEC §11.4
// submit) through m.groupMover (nil when no mover is wired, e.g. an
// internal/tui-only test model), mutating the caller's local Model in
// place and returning only the resulting tea.Cmd -- the same shape
// submitRename/submitProfileSwitch already use.
func (m *Model) submitGroupMove() tea.Cmd {
	if m.groupMover == nil {
		m.moveGroupNote = "moving a session's group is unavailable"
		return nil
	}
	if len(m.sessions) == 0 {
		return nil
	}
	session, _ := m.selectedSession()
	sessionID, groupID := session.ID, m.moveGroupValue
	mover := m.groupMover
	return func() tea.Msg {
		updated, err := mover(context.Background(), sessionID, groupID)
		return sessionGroupMoved{session: updated, err: err}
	}
}

// moveGroupView renders the `g` group-move picker: a single left/right
// cycle field, mirroring profileSwitchView's shape exactly (task 020's
// own precedent for a single-field cycle dialog -- the closest existing
// analog to a move rather than a free-text edit).
func (m Model) moveGroupView() string {
	return m.framedDialog(m.styledMoveGroupBody())
}

// moveGroupBody builds moveGroupView's text before framedDialog's
// box-width padTrunc touches it, split out for the same reason
// profileSwitchBody/renameBody are: a test can assert the exact wording
// without a terminal-rendering concern in between.
func (m Model) moveGroupBody() string {
	session, _ := m.selectedSession()
	options := m.moveGroupCycleOptions()
	names := make([]string, 0, len(options))
	for _, g := range options {
		names = append(names, g.Name)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Move %s to a different group\n\n", session.Name)
	fmt.Fprintf(&b, "%s\n", m.detailField("Current group: ", sessionGroupLabel(session)))
	fmt.Fprintf(&b, "%s\n", m.detailField("New group:     ", fmt.Sprintf("%s (left/right cycles: %s)", m.moveGroupName(m.moveGroupValue), strings.Join(names, ", "))))
	b.WriteString("\nLeft/Right cycles · Enter confirms · Esc cancels\n")
	if m.moveGroupNote != "" {
		fmt.Fprintf(&b, "\n%s\n", m.moveGroupNote)
	}
	return b.String()
}

// styledMoveGroupBody re-derives moveGroupBody's exact structure -- same
// title/fields/footer/note order -- but colours each finished PHYSICAL
// line rather than the logical one, exactly like styledProfileSwitchBody:
// every string is wrapped via m.wrapDialogLines FIRST, so a colour token
// can never straddle a word-wrap boundary wrapDialogLines has not drawn
// yet. Token mapping is SPEC.md:1355: the title in `title`, the footer
// legend's keys in `key` and the rest in `hint`, a failed-submit note in
// `error`, and the "New group:" row (the only field this dialog ever lets
// left/right cycle) carrying the same `selection` treatment a selected
// list row does (renderCreateRowSegments, reused verbatim from
// styledProfileSwitchBody's identical precedent) -- the "Current group:"
// row is not a cycle target, so it renders through the ordinary unfocused
// branch of the same helper.
func (m Model) styledMoveGroupBody() string {
	session, _ := m.selectedSession()
	options := m.moveGroupCycleOptions()
	names := make([]string, 0, len(options))
	for _, g := range options {
		names = append(names, g.Name)
	}
	wrap := m.wrapDialogLines
	var out []string
	colorWhole := func(tok theme.Token, line string) {
		for _, l := range wrap(line) {
			out = append(out, m.colorToken(tok, l))
		}
	}
	colorField := func(label, value string, focused bool) {
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
	colorFooterLine := func(line string) {
		for _, l := range wrap(line) {
			fields := strings.Fields(l)
			for i, f := range fields {
				if cycleConfirmFooterKeyTokens[f] {
					fields[i] = m.colorToken(theme.Key, f)
				} else {
					fields[i] = m.colorToken(theme.Hint, f)
				}
			}
			out = append(out, strings.Join(fields, " "))
		}
	}

	colorWhole(theme.Title, fmt.Sprintf("Move %s to a different group", session.Name))
	out = append(out, "")
	colorField("Current group: ", sessionGroupLabel(session), false)
	colorField("New group:     ", fmt.Sprintf("%s (left/right cycles: %s)", m.moveGroupName(m.moveGroupValue), strings.Join(names, ", ")), true)
	out = append(out, "")
	colorFooterLine("Left/Right cycles · Enter confirms · Esc cancels")
	if m.moveGroupNote != "" {
		out = append(out, "")
		colorWhole(theme.Error, m.moveGroupNote)
	}
	return strings.Join(out, "\n")
}
