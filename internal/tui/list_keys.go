package tui

import (
	"context"
	"errors"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tui/lineedit"
)

// keyHandler is one entry of the list-mode key table: what a bound key does
// once every overlay and dialog has declined it.
type keyHandler func(Model, tea.KeyMsg) (tea.Model, tea.Cmd)

// keyOverlay is one layer that owns the keyboard while it is open: active
// says whether the layer is up, handle is its own key updater.
type keyOverlay struct {
	active func(Model) bool
	handle keyHandler
}

// keyOverlays lists the layers that intercept a key before the list-mode
// keymap sees it, in precedence order (the first open one wins). Like
// listKeyHandlers it is filled in by init: the handlers can reach Update
// again, which an initialiser would turn into an initialisation cycle.
var keyOverlays []keyOverlay

// onKeyMsg is Update's handler for a key press: coalesced runes are replayed
// one by one, the refusal banner's Esc goes first, then the open overlay (if
// any) takes the key, then the dd chord's second key, then the shared
// session-scoped guard, and last the list-mode key table.
func (m Model) onKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// GH #52: any key at all cancels a pending auto-enter, before any
	// layer below gets to act on it (auto_enter.go).
	m.cancelAutoEnter()
	m.fieldCopyNote = ""
	if msg.Type == tea.KeyRunes && len(msg.Runes) > 1 && !msg.Paste {
		return m.replayCoalescedKeys(msg)
	}

	// cure-01-02 (R143/SPEC §11.9): the banner-dismissing Esc has to run
	// before EVERY other Esc-cleared layer's own dispatch below --
	// pendingDelete's intercept, help/settings/filtering's own updaters,
	// deleteConfirming/archiveConfirming and the rest all used to sit
	// ahead of this check (it lived just above guardSessionScopedKey),
	// so an Esc pressed while, say, pendingDelete was ALSO armed hit
	// pendingDelete's own intercept first and never reached here at all
	// (TestReviewRefusalEscPrecedesPendingDelete, TestReviewRefusalEscPrecedesHelp
	// at ee7f5a5d3, artifacts/review/reviewer_refusal_test.go). Checking
	// it first, before any layer's own dispatch, is what makes "the
	// banner always goes first" true regardless of what else is open;
	// activeEntryRefusalForSelection already answers false while
	// m.interactive is true (no banner exists to dismiss there), so this
	// cannot change interactive's own Esc handling, and every other
	// layer below is reached completely unchanged on the very next Esc
	// once the banner (if any) is gone.
	if msg.String() == "esc" {
		if _, ok := m.activeEntryRefusalForSelection(); ok {
			m.clearEntryRefusal()
			return m, nil
		}
	}
	if overlay, ok := m.activeKeyOverlay(); ok {
		return overlay.handle(m, msg)
	}
	if m.pendingDelete {
		return m.handlePendingDeleteKey(msg)
	}
	// task 013/D.2: the one shared guard every session-scoped binding
	// below now runs through (session_scoped_guard.go) instead of each
	// one deciding for itself whether the cursor names a session.
	// Reverting just this call, leaving guardSessionScopedKey itself in
	// place, restores the per-site gaps it closed.
	//
	// cure-01-02: the refusal-banner Esc used to be checked here (right
	// before this guard), which put it AFTER pendingDelete's own
	// intercept and every dialog's own updater above -- see the comment
	// on the earlier, now sole "esc" check just above the coalesced-rune
	// split, where it runs before all of those instead.
	if m.guardSessionScopedKey(msg.String()) {
		return m, nil
	}
	if handle, ok := listKeyHandlers[msg.String()]; ok {
		return handle(m, msg)
	}
	return m, nil
}

// activeKeyOverlay returns the first open overlay in keyOverlays' precedence
// order, if any.
func (m Model) activeKeyOverlay() (keyOverlay, bool) {
	for _, overlay := range keyOverlays {
		if overlay.active(m) {
			return overlay, true
		}
	}
	return keyOverlay{}, false
}

// replayCoalescedKeys splits one multi-rune KeyMsg back into single-rune
// ones and feeds them through Update in order.
func (m Model) replayCoalescedKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Bubble Tea's own PTY reader coalesces multiple keystrokes that
	// land in the same read into a single KeyMsg whose Runes holds
	// every character (e.g. two quick presses of 'j' and 'x' can
	// arrive as one KeyMsg{Runes: "jx"}). Every case below (and every
	// dialog's own update* method) matches msg.String() against a
	// single key's own string ("j", "x", ...); a coalesced "jx" would
	// match NONE of them and the whole event would be silently
	// dropped, leaving neither rune to act (task 118, requirement 51).
	// Rather than teach every case and every dialog about multi-rune
	// strings, split the coalesced KeyMsg back into one single-rune
	// tea.KeyMsg per character and dispatch them through Update in
	// order, exactly as if they had arrived as separate keystrokes. A
	// single keypress (len(Runes)==1) is untouched and falls straight
	// through to the handling below, unchanged.
	//
	// A bracketed-paste KeyMsg (msg.Paste) is deliberately exempted:
	// Key.String() already wraps a paste's runes in "[...]" so it can
	// never match a single-letter shortcut by accident (bubbletea's own
	// key.go). Splitting pasted text into individual keystrokes would
	// turn a paste of, say, "dd" into an actual delete chord -- the
	// deliberate decision (docs/reports/phase3-findings.md, task 118)
	// is that a paste into the list is ignored outright, never
	// dispatched rune by rune.
	var cmds []tea.Cmd
	next := tea.Model(m)
	for _, r := range msg.Runes {
		var cmd tea.Cmd
		next, cmd = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}, Alt: msg.Alt})
		cmds = append(cmds, cmd)
	}
	return next, tea.Batch(cmds...)
}

// handlePendingDeleteKey takes the key after a lone `d`.
func (m Model) handlePendingDeleteKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// pendingDelete intercepts the very next key after a lone `d`
	// (SPEC's dd chord): a second `d` opens the confirm dialog; every
	// other key -- Esc included -- clears the pending indicator and is
	// otherwise swallowed, so "d followed by any other key performs no
	// destructive action" holds without also having to reason about
	// whatever that other key would normally have done.
	m.pendingDelete = false
	// task cure-01-01 (F1, R137, SPEC §11): the second `d` asks the
	// ONE shared guard the header question -- for BOTH the batch
	// path and the single-row path -- before either one opens the
	// confirm dialog. It cannot run after the guard (this intercept
	// has to swallow and clear the indicator for every key, guarded
	// ones included) so it calls the guard itself rather than
	// carrying a second selectedSession check of its own.
	//
	// A non-empty mark set (task 112's batch dd) used to be exempt
	// from this question entirely -- the confirm opened for the
	// MARKED set regardless of the cursor. Review found that
	// exemption wrong (F1): the batch path is exactly as inert on a
	// header as the single-row path, so it now asks the same guard.
	if msg.String() == "d" && len(m.sessions) > 0 && !m.guardSessionScopedKey("d") {
		if len(m.marked) > 0 {
			m.deleteConfirming = true
			m.deleteNote = ""
			m.deleteScroll = 0
			// cure-01-05: the bulk confirm offers the same
			// non-default purge choice the single-session dialog
			// does, just resolved per session at submit time
			// (transcriptPathFor has no single session to call
			// eagerly here) -- so only the cycled VALUE resets;
			// deletePurgePath/OK stay meaningless for a batch and
			// are left alone.
			m.bulkDeletePurgeValue = "keep"
			m.deletePurgeValue = ""
			m.deletePurgePath = ""
			m.deletePurgeOK = false
		} else {
			// The guard having let this through means the cursor
			// resolves to a session, so selectedSession is ok here.
			session, _ := m.selectedSession()
			if canDelete(session) {
				m.deleteConfirming = true
				m.deleteNote = ""
				m.deleteScroll = 0
				m.deletePurgeValue = "keep"
				m.deletePurgePath, m.deletePurgeOK = m.transcriptPathFor(session)
			}
		}
	}
	return m, nil
}

// listKeyHandlers is the list-mode keymap: the raw bubbletea key string
// (msg.String()) mapped to its handler. Several keys may share one handler.
var listKeyHandlers map[string]keyHandler

func init() {
	keyOverlays = []keyOverlay{
		{func(m Model) bool { return m.lostAttach }, Model.updateLostAttachView},
		{func(m Model) bool { return m.interactive }, Model.updateInteractive},
		{func(m Model) bool { return m.creating }, Model.updateCreate},
		{func(m Model) bool { return m.profileSwitching }, Model.updateProfileSwitch},
		{func(m Model) bool { return m.pinning }, Model.updatePinDialog},
		{func(m Model) bool { return m.envEditing }, Model.updateEnvDialog},
		{func(m Model) bool { return m.restartChoosing }, Model.updateRestartChoice},
		{func(m Model) bool { return m.deleteConfirming }, Model.updateDeleteConfirm},
		// R72 (issue #10): the archive confirm intercepts every key while it
		// is open, exactly as deleteConfirming does -- which is what makes a
		// second `A` inside the dialog a no-op rather than a re-entrant archive,
		// and what keeps `x`/`dd`/`r` from acting on the row the dialog asks about.
		{func(m Model) bool { return m.archiveConfirming }, Model.updateArchiveConfirm},
		{func(m Model) bool { return m.settingsOpen }, func(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) { return m.updateSettings(msg) }},
		{func(m Model) bool { return m.themePicking }, Model.updateThemePicker},
		{func(m Model) bool { return m.help }, Model.updateHelpView},
		{func(m Model) bool { return m.renaming }, Model.updateRenameDialog},
		{func(m Model) bool { return m.launchInputsEditing }, Model.updateLaunchInputsDialog},
		{func(m Model) bool { return m.movingGroup }, Model.updateMoveGroupDialog},
		{func(m Model) bool { return m.detail }, Model.updateDetailView},
		{func(m Model) bool { return m.eventLogOpen }, Model.updateEventLog},
		{func(m Model) bool { return m.filtering }, Model.updateFilter},
	}
	listKeyHandlers = map[string]keyHandler{
		"q":      Model.keyQuit,
		"ctrl+c": Model.keyQuit,
		"?":      Model.keyHelp,
		"esc":    Model.keyEscape,
		"i":      Model.keyDetail,
		",":      Model.keySettings,
		"t":      Model.keyThemePicker,
		"n":      Model.keyNewSession,
		"up":     Model.keyCursorUp,
		"k":      Model.keyCursorUp,
		"down":   Model.keyCursorDown,
		"j":      Model.keyCursorDown,
		"pgup":   Model.keyPageUp,
		"pgdown": Model.keyPageDown,
		"Y":      Model.keyAcknowledge,
		"x":      Model.keyKill,
		"m":      Model.keyMark,
		"p":      Model.keyPin,
		"A":      Model.keyArchive,
		"U":      Model.keyUnarchive,
		"d":      Model.keyDeleteChord,
		"u":      Model.keyUndo,
		"r":      Model.keyResume,
		"R":      Model.keyRestart,
		"e":      Model.keyEditEnv,
		"E":      Model.keyEventLog,
		"/":      Model.keyFilter,
		" ":      Model.keyNextAttention,
		"c":      Model.keyToggleGroup,
		"left":   Model.keyCollapseGroup,
		"right":  Model.keyExpandGroup,
		"g":      Model.keyFirstRow,
		"G":      Model.keyLastRow,
		"|":      Model.keyCycleLayout,
		"<":      Model.keyNarrowSidebar,
		">":      Model.keyWidenSidebar,
		"enter":  Model.keyEnter,
		"F":      Model.keyEnterFull,
		"a":      Model.keyAttach,
	}
}

// keyQuit handles "q", "ctrl+c".
func (m Model) keyQuit(_ tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m, tea.Quit
}

// keyHelp handles "?".
func (m Model) keyHelp(_ tea.KeyMsg) (tea.Model, tea.Cmd) {
	// This switch is only ever reached with m.help == false (task 078:
	// updateHelpView, dispatched above, now intercepts every key,
	// including a second "?"/esc, while help is open), so this only
	// ever opens it; helpScroll resets so a reopen never starts
	// scrolled from wherever a previous visit left off.
	m.help = true
	m.helpScroll = 0
	return m, nil
}

// keyEscape handles "esc".
func (m Model) keyEscape(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// detailView has no fields to submit or cycle, so the only §11.4
	// contract key it binds is esc — shared here through the same
	// applyDialogContract implementation createView, profileSwitchView
	// and pinView defer to, rather than a sixth hand-written cancel.
	// Task 112: a plain top-level Esc also clears the mark set ("the
	// marks clear on the action and on esc"). Neither m.help nor
	// m.detail is ever true here (task 013's updateDetailView and
	// task 078's updateHelpView each intercept every key, including
	// esc, while their own overlay is open) -- this branch cannot
	// close either today, but keeps clearing both anyway so a caller
	// that somehow reaches it with one already true is not left
	// stuck open.
	//
	// Task 027: a filter query left in force by `enter` closing the
	// `/` text field (m.filtering == false, m.filterQuery != "") is
	// ALSO only ever reachable here -- filter.go's own updateFilter
	// intercepts esc itself while the field still has focus, so this
	// branch never doubles up with that one. It is deliberately an
	// else, not a second unconditional clear: SPEC's "esc cancels
	// [one thing]" means a press that lands on a non-empty mark set
	// clears the marks and leaves the filter (if any) held, exactly
	// as it leaves every other state alone -- one press never clears
	// two things at once.
	hadMarks := len(m.marked) > 0
	_, _ = applyDialogContract(msg, dialogContract{Cancel: func() {
		m.help = false
		m.detail = false
		m.marked = nil
	}})
	// cure-01-01-4 (R136/SPEC §11, binding ruling 002): clearing the
	// held query used to hand the OLD row index straight to
	// nearestVisibleSelection, which walks by POSITION, not identity --
	// unfiltering never removes rows, only adds ones the filter had
	// hidden, so the same numeric index can now name a different
	// session entirely (TestReview154HeldFilterEscapePreservesSessionID),
	// and neither branch ever called followSelectionViewport, so a
	// selection that survived (by luck of position) could still sit
	// outside the visible scroll window
	// (TestReview152ClosedFilterEscapeKeepsSelectionVisible,
	// TestReview152HeldFilterEscapeKeepsHeaderVisible). A header cursor
	// carries its own group id rather than a session index, so it never
	// needs this preserve-by-id step -- unfiltering cannot make a group
	// id go stale the way it can a row's numeric position.
	if !hadMarks && m.filterQuery != "" {
		var selectedID string
		selectedWasRow := false
		if idx, ok := m.selected.SessionIndex(); ok {
			selectedWasRow = true
			if idx >= 0 && idx < len(m.sessions) {
				selectedID = m.sessions[idx].ID
			}
		}
		m.filterQuery = ""
		m.sessions = m.filteredSessions()
		if selectedWasRow {
			if idx := indexOfSessionID(m.sessions, selectedID); idx >= 0 {
				m.selected = rowCursor(idx)
			}
		}
		if !m.cursorNamesVisibleStop(m.selected) {
			m.selected = m.nearestVisibleSelection(m.selected)
		}
		m.followSelectionViewport()
	}
	return m, nil
}

// keyDetail handles "i".
func (m Model) keyDetail(_ tea.KeyMsg) (tea.Model, tea.Cmd) {
	// m.detail is never true here (task 013's updateDetailView
	// intercepts every key, including a second "i", while it is), and
	// m.help is never true here either (task 078's updateHelpView
	// intercepts every key first), so this only ever opens it;
	// detailScroll resets for the same reason helpScroll does above.
	// R90/task 033, R61: whether a hook was recently declined for
	// THIS session is fetched fresh on every open, as a tea.Cmd
	// (loadDetailDroppedHook), rather than read from m.store inline
	// in View() -- the previous visit's result is cleared first so a
	// slow reply landing after a different session's "i" reopened the
	// dialog is caught by the session-id mismatch guard in the
	// detailDroppedHookLoaded case below, never rendered against the
	// wrong row.
	// task 013/D.2: guardSessionScopedKey above already refused this
	// keypress entirely when the cursor has no selected session, so
	// selectedSession is guaranteed ok here.
	session, _ := m.selectedSession()
	m.detail = true
	m.detailScroll = 0
	target := session.ID
	m.detailDroppedHookSessionID = target
	m.detailDroppedHookFound = false
	m.detailDroppedHookEvent = store.Event{}
	m.detailHookSessionID = target
	m.detailHookFound = false
	m.detailHookRun = store.EventHookRun{}
	return m, m.loadDetailDroppedHook(target)
}

// keySettings handles ",".
func (m Model) keySettings(_ tea.KeyMsg) (tea.Model, tea.Cmd) {
	if !m.help {
		m.settingsOpen = true
		m.settingsCategoryIndex = 0
		m.settingsFieldIndex = 0
		m.settingsFocus = settingsFocusCategories
		m.settingsSearchActive = false
		m.settingsSearchEdit = lineedit.Editor{}
		m.settingsSearchIndex = 0
		m.settingsEdits = settingsEditsFromSettings(m.settings)
		m.settingsSavedEdits = settingsEditsFromSettings(m.settings)
		m.settingsDiscardConfirm = false
		m.settingsNote = ""
		m.settingsEnvOpen = false
		m.settingsEnvEditing = false
		m.settingsEnvIndex = 0
		m.settingsStringEditing = false
		m.settingsStringEditKey = ""
		m.settingsStringEdit = lineedit.Editor{}
		m.settingsGroups = m.computeAvailableGroups()
		m.settingsGroupIndex = 0
		m.settingsGroupCreating = false
		m.settingsGroupRenaming = false
		m.settingsGroupEditID = 0
		m.settingsGroupEdit = lineedit.Editor{}
		m.settingsGroupNote = ""
	}
	return m, nil
}

// keyThemePicker handles "t".
func (m Model) keyThemePicker(_ tea.KeyMsg) (tea.Model, tea.Cmd) {
	if !m.help {
		m = m.openThemePicker()
	}
	return m, nil
}

// keyNewSession handles "n".
func (m Model) keyNewSession(_ tea.KeyMsg) (tea.Model, tea.Cmd) {
	if !m.help {
		m.creating, m.createError, m.createField = true, "", 0
		m.createScroll = 0
		m.createEnvReveal = false
		m.createEdits = [createFieldCount]lineedit.Editor{}
		prefill, lastUsed := m.prefillCreateCWD()
		m.createEdits[createFieldCWD], m.createCWDLastUsed = lineedit.NewOffered(prefill), lastUsed
		m.createCWDRecents, m.createCWDRecentIndex = nil, -1
		m.createCWDPreCycleEdit = lineedit.Editor{}
		m.closeCreateCWDCandidates()
		m.createAvailableAgentKinds = m.computeAvailableAgentKinds()
		m.createAgent, m.createAgentLastUsed = m.pickCreateAgent()
		m.createProfile = m.defaultCreateProfile(m.createAgent)
		m.createProfileTouched, m.createProfileRequested = false, ""
		m.createLoginShell = false
		m.createEventHook = eventHookInherit
		m.createGroups = m.computeAvailableGroups()
		m.createGroupID, m.createGroupLastUsed = m.pickCreateGroup()
	}
	return m, nil
}

// keyCursorUp handles "up", "k".
func (m Model) keyCursorUp(_ tea.KeyMsg) (tea.Model, tea.Cmd) {
	if next, ok := m.prevVisibleSelection(m.selected); ok {
		m.setSelection(next)
	}
	return m, nil
}

// keyCursorDown handles "down", "j".
func (m Model) keyCursorDown(_ tea.KeyMsg) (tea.Model, tea.Cmd) {
	if next, ok := m.nextVisibleSelection(m.selected); ok {
		m.setSelection(next)
	}
	return m, nil
}

// keyPageUp handles "pgup".
func (m Model) keyPageUp(_ tea.KeyMsg) (tea.Model, tea.Cmd) {
	// ·11.3 requirement 19: PgUp/PgDn always drive the list, since the
	// sidebar is the only focusable region and there is no tab panel
	// cycle to move the page keys onto instead. pageSelection walks
	// VISUAL rows (002-steering.md), not raw m.sessions index
	// arithmetic, so a page of hidden/non-adjacent rows can't skew it.
	m.setSelection(m.pageSelection(-m.sidebarRowsPerPage()))
	return m, nil
}

// keyPageDown handles "pgdown".
func (m Model) keyPageDown(_ tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.setSelection(m.pageSelection(m.sidebarRowsPerPage()))
	return m, nil
}

// keyAcknowledge handles "Y".
func (m Model) keyAcknowledge(_ tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.acknowledge == nil {
		return m, nil
	}
	// task 013/D.2: the guard above already refused this keypress when
	// the cursor has no selected session.
	session, _ := m.selectedSession()
	if !canAcknowledge(session) {
		return m, nil
	}
	sessionID := session.ID
	return m, func() tea.Msg {
		return sessionAcknowledged{err: m.acknowledge(context.Background(), sessionID)}
	}
}

// keyKill handles "x".
func (m Model) keyKill(_ tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.kill == nil {
		return m, nil
	}
	if len(m.marked) > 0 {
		// Task 112: a non-empty mark set means x acts on the WHOLE
		// batch instead of just the selected row. An already-stopped
		// marked row is silently skipped (never an error, unlike the
		// single-row refusal below) since a batch action naming
		// rows that need nothing done would be noise, not a
		// destructive-action guard. Marks clear immediately -- "the
		// action" here IS pressing x, not waiting for the kills to
		// finish.
		sessions := m.markedSessions()
		m.marked = nil
		if len(sessions) == 0 {
			return m, nil
		}
		kill := m.kill
		return m, func() tea.Msg {
			result := sessionsBulkKilled{}
			for _, s := range sessions {
				if !canKill(s) {
					continue
				}
				result.sessions = append(result.sessions, s)
				result.errs = append(result.errs, kill(context.Background(), s))
			}
			return result
		}
	}
	// task 013/D.2 + cure-01-01 (F1): the guard above already refused
	// this keypress outright when the cursor rests on a header, marks
	// or no marks -- reaching here (with no marks) means selectedSession
	// is guaranteed ok.
	session, _ := m.selectedSession()
	// Task 807 (review finding 2): the single-row path now consults
	// the same canKill the footer's x slot already used, instead of
	// deferring the already-stopped refusal to the service's own
	// verdict. That deferral existed only because a stopped row
	// could still be hiding a live tmux pane under `remain-on-exit
	// failed` (#6); the reconcile side has since closed that gap for
	// good: reconcile.go's crashed-pane collect-on-sight
	// (reconcile.go:68-79) collects and kills a dead pane's tmux
	// session regardless of the stored status, and
	// repairTerminalRowWithLivePane (SPEC §7, reconcile.go:215-235)
	// corrects a stopped row -- or an error row that itself already
	// carries a pane-exit or tmux/user-sourced verdict -- that still has
	// a genuinely live pane back to a non-terminal status before the row
	// is ever read here (a hook- or probe-sourced error with no
	// pane-exit verdict is left alone, finding F40, task 901). So a row canKill reads as stopped has already had its
	// corpse collected or its status repaired -- there is no longer a
	// retained corpse for a locally-read Status to miss, and no kill
	// command needs to reach the service to say so. The wording
	// matches service.Kill's own refusal (kill.go) so a genuinely
	// stopped row still shows "Cannot kill: session is already
	// stopped" via the sessionKilled branch.
	if !canKill(session) {
		return m, func() tea.Msg {
			return sessionKilled{session: session, err: errors.New("session is already stopped")}
		}
	}
	return m, func() tea.Msg {
		return sessionKilled{session: session, err: m.kill(context.Background(), session)}
	}
}

// keyMark handles "m".
func (m Model) keyMark(_ tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Task 112, requirement 28: m toggles a mark on the selected row
	// keyed by session id (never a visual index), so the set
	// survives a re-sort or re-group untouched -- markedSessions()
	// re-resolves it through visualOrder() fresh every time it is
	// consulted rather than caching anything positional.
	// task 013/D.2: the guard above already refused this keypress
	// when the cursor has no selected session.
	session, _ := m.selectedSession()
	id := session.ID
	if m.marked == nil {
		m.marked = map[string]bool{}
	}
	if m.marked[id] {
		delete(m.marked, id)
	} else {
		m.marked[id] = true
	}
	return m, nil
}

// keyPin handles "p".
func (m Model) keyPin(_ tea.KeyMsg) (tea.Model, tea.Cmd) {
	// task 010 (SPEC §11's pin rule, R159): the write happens only
	// in the returned tea.Cmd (setSessionsPinnedCmd) -- this case
	// itself never calls the store, only decides which ids and
	// which direction. A non-empty mark set switches the whole
	// batch to whichever direction the SPEC rule names ("unpins
	// them all when every marked row is pinned, and pins them all
	// otherwise") -- mirroring x's own marked-set-vs-single-row
	// split above, but, unlike x/dd, p never clears m.marked: SPEC
	// names no such side effect for it, so a mark set survives a
	// p press exactly as it survives everything but x, dd and esc.
	if len(m.marked) > 0 {
		sessions := m.markedSessions()
		if len(sessions) == 0 {
			return m, nil
		}
		pin := false
		for _, s := range sessions {
			if s.PinnedAt == 0 {
				pin = true
				break
			}
		}
		ids := make([]string, len(sessions))
		for i, s := range sessions {
			ids[i] = s.ID
		}
		return m, m.setSessionsPinnedCmd(ids, pin)
	}
	// task 013/D.2: the guard above already refused this keypress
	// when the cursor has no selected session.
	session, _ := m.selectedSession()
	return m, m.setSessionsPinnedCmd([]string{session.ID}, session.PinnedAt == 0)
}

// keyArchive handles "A".
func (m Model) keyArchive(_ tea.KeyMsg) (tea.Model, tea.Cmd) {
	// R72 (issue #10), SPEC.md:752: `A` writes NOTHING on the keypress.
	// It only opens the confirm dialog below, whose Enter then performs
	// SPEC requirement 27's archive: on a stopped row that only sets
	// archived_at; on any other row archiveSvc
	// (internal/service.Service.Archive) kills the live pane first and
	// archives in the same action ("kill and archive", §4's invariant)
	// rather than refusing the keypress the way x refuses an
	// already-stopped row -- that decision stays archiveSvc's, never
	// this switch's, and the dialog says so in as many words before
	// anything is written. `A` is deliberately NOT rebound and grows no
	// chord here (the operator declined both): the confirm alone is what
	// stops `A` -- one Shift away from `a`, attach -- from killing a live
	// agent on a single keystroke.
	if m.archiveSvc == nil {
		m.attachError = "Archiving is unavailable"
		return m, nil
	}
	// canArchive is review finding 2/R80's single A eligibility
	// definition, shared with the footer: a row that is already
	// archived writes nothing and opens no confirm here, exactly as
	// the footer already refuses to offer A for it -- and the
	// refusal names U, the only route back for that row. This is a
	// call to the footer's own predicate by name, never a local
	// copy of `ArchivedAt == 0`: archive_eligibility_test.go's
	// source parse fails if this case stops naming it.
	//
	// task 013/D.2: the guard above already refused this keypress
	// when the cursor has no selected session, so selectedSession is
	// guaranteed ok here.
	session, _ := m.selectedSession()
	if !canArchive(session) {
		m.attachError = "Cannot archive: session is already archived; press U to unarchive"
		return m, nil
	}
	m.archiveConfirming = true
	m.archiveNote = ""
	return m, nil
}

// keyUnarchive handles "U".
func (m Model) keyUnarchive(_ tea.KeyMsg) (tea.Model, tea.Cmd) {
	// R71 (issue #8), SPEC.md:323-332: `A` is reversible, so `U`
	// clears archived_at on the selected row. It acts on m.sessions --
	// the DISPLAYED list -- which is exactly what makes it reachable
	// from inside requirement 33's `/` filter results: an archived row
	// is absent from the default list entirely and only ever appears
	// while a query is in force (filteredSessions widens the pool to
	// m.archivedSessions), and after Enter closes the text field the
	// freed keymap -- this switch -- acts on that narrowed list. A row
	// that is not archived is refused here rather than being handed to
	// the store, so `U` can never record an "unarchived" event for a
	// row that was never archived.
	if m.unarchiveSvc == nil {
		m.attachError = "Unarchiving is unavailable"
		return m, nil
	}
	// task 013/D.2: the guard above already refused this keypress
	// when the cursor has no selected session, so selectedSession is
	// guaranteed ok here.
	session, _ := m.selectedSession()
	if !canUnarchive(session) {
		m.attachError = "Cannot unarchive: session is not archived"
		return m, nil
	}
	return m, func() tea.Msg {
		unarchived, err := m.unarchiveSvc(context.Background(), session.ID)
		return sessionUnarchived{session: unarchived, err: err}
	}
}

// keyDeleteChord handles "d".
func (m Model) keyDeleteChord(_ tea.KeyMsg) (tea.Model, tea.Cmd) {
	// First half of task 105's dd chord: a visible pending indicator,
	// changing nothing in the store. The very next key (handled by
	// the m.pendingDelete intercept above, on the NEXT tea.KeyMsg) is
	// either another `d` (opens the confirm dialog) or clears this
	// with no destructive action.
	//
	// task 013/D.2 + cure-01-01 (F1): the shared guard above has
	// already refused this key outright when the cursor rests on a
	// header, marks or no marks -- there is no exemption left, so
	// the indicator can no longer be raised at all for a header
	// cursor. The len check stays only as a defensive belt for an
	// empty session list.
	if len(m.sessions) > 0 {
		m.pendingDelete = true
	}
	return m, nil
}

// undoWindow is one of keyUndo's undo windows: it answers ok=false when its
// window is not open (so the next one is consulted), and ok=true -- with the
// model and command to return -- once its window is the one `u` acts on,
// whether or not the service behind it is wired.
type undoWindow func(m Model) (Model, tea.Cmd, bool)

// undoWindows lists keyUndo's windows in precedence order: kill, batch kill,
// delete, batch delete, and the archive window last, so adding a later one
// cannot change what `u` does for any earlier window.
var undoWindows = []undoWindow{
	Model.undoKill, Model.undoBatchKill, Model.undoDelete, Model.undoBatchDelete, Model.undoArchive,
}

// keyUndo handles "u".
func (m Model) keyUndo(_ tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Requirement 22: u undoes the most recent x, not whatever row
	// happens to be selected right now -- undoSessionID is only ever
	// set by a successful kill and cleared by either this key or the
	// DECK_UNDO_MS expiry tick, so an expired or never-killed state
	// makes u a no-op exactly as the requirement states. Requirement
	// 23 (task 106): once the kill-undo trio is empty, u falls back
	// to undoing the most recent dd delete within its own
	// DECK_DELETE_GRACE_MS window, tracked by the separate
	// deleteUndoSessionID trio (never merged with the one above).
	for _, window := range undoWindows {
		if next, cmd, ok := window(m); ok {
			return next, cmd
		}
	}
	return m, nil
}

// undoKill is the single-kill window: resume the session `x` just killed.
func (m Model) undoKill() (Model, tea.Cmd, bool) {
	if m.undoSessionID == "" {
		return m, nil, false
	}
	if m.resume == nil {
		return m, nil, true
	}
	sessionID := m.undoSessionID
	m.undoSessionID, m.undoSessionName = "", ""
	m.undoGeneration++
	return m, func() tea.Msg {
		resumed, outcome, err := m.resume(context.Background(), sessionID)
		return sessionResumed{session: resumed, outcome: outcome, err: err}
	}, true
}

// undoBatchKill is the batch-kill window. Task 112: it is checked next,
// still ahead of the delete-undo trio -- a batch x's undo is "undo the most
// recent x" too, exactly like the single-session case, just covering N
// sessions with one keypress.
func (m Model) undoBatchKill() (Model, tea.Cmd, bool) {
	if len(m.batchUndoSessionIDs) == 0 {
		return m, nil, false
	}
	if m.resume == nil {
		return m, nil, true
	}
	ids := m.batchUndoSessionIDs
	m.batchUndoSessionIDs = nil
	m.batchUndoGeneration++
	resume := m.resume
	return m, func() tea.Msg {
		result := sessionsBulkResumed{}
		for _, id := range ids {
			resumed, outcome, err := resume(context.Background(), id)
			result.sessionIDs = append(result.sessionIDs, resumed.ID)
			result.outcomes = append(result.outcomes, outcome)
			result.errs = append(result.errs, err)
		}
		return result
	}, true
}

// undoDelete is the single dd-delete window: restore the tombstoned session.
func (m Model) undoDelete() (Model, tea.Cmd, bool) {
	if m.deleteUndoSessionID == "" {
		return m, nil, false
	}
	if m.restoreSvc == nil {
		return m, nil, true
	}
	sessionID := m.deleteUndoSessionID
	m.deleteUndoSessionID, m.deleteUndoSessionName = "", ""
	m.deleteUndoGeneration++
	return m, func() tea.Msg {
		restored, err := m.restoreSvc(context.Background(), sessionID)
		return sessionRestored{session: restored, err: err}
	}, true
}

// undoBatchDelete is the batch-delete window (task 112), mirroring the
// single dd undo case: one shared window restoring every session a
// marked-set dd tombstoned.
func (m Model) undoBatchDelete() (Model, tea.Cmd, bool) {
	if len(m.batchDeleteUndoSessionIDs) == 0 {
		return m, nil, false
	}
	if m.restoreSvc == nil {
		return m, nil, true
	}
	ids := m.batchDeleteUndoSessionIDs
	m.batchDeleteUndoSessionIDs = nil
	m.batchDeleteUndoGeneration++
	restoreSvc := m.restoreSvc
	return m, func() tea.Msg {
		result := sessionsBulkRestored{}
		for _, id := range ids {
			_, err := restoreSvc(context.Background(), id)
			result.errs = append(result.errs, err)
		}
		return result
	}, true
}

// undoArchive is the archive window. R72 (issue #10, SPEC.md:752): it is
// checked LAST, behind all four kill/delete trios, so adding it cannot change
// what `u` does for any pre-existing window -- an archive undo only ever
// runs when no kill and no delete undo is outstanding. Its reversal is
// unarchiveSvc (R71's store.UnarchiveSession), the same service `U` uses, so
// the row returns to the default list reading stopped/resumable; `A`'s
// kill is deliberately NOT resumed here (the toast says so in as many
// words), since undoing a hide must never silently relaunch an agent.
func (m Model) undoArchive() (Model, tea.Cmd, bool) {
	if m.archiveUndoSessionID == "" {
		return m, nil, false
	}
	if m.unarchiveSvc == nil {
		return m, nil, true
	}
	sessionID := m.archiveUndoSessionID
	hookRan := m.archiveUndoHookRan
	m.archiveUndoSessionID, m.archiveUndoSessionName = "", ""
	m.archiveUndoKilled = false
	m.archiveUndoHookRan = false
	m.archiveUndoGeneration++
	return m, func() tea.Msg {
		unarchived, err := m.unarchiveSvc(context.Background(), sessionID)
		return sessionUnarchived{session: unarchived, err: err, teardownHookRan: hookRan}
	}, true
}

// keyResume handles "r".
func (m Model) keyResume(_ tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.resume == nil {
		return m, nil
	}
	// task 013/D.2: the guard above already refused this keypress
	// when the cursor has no selected session.
	session, _ := m.selectedSession()
	if !canResume(session) {
		m.attachError = "Cannot resume: session is not stopped"
		return m, nil
	}
	if m.resumableWithNoConversationIDYet(session) {
		m.attachError = "Cannot resume: " + session.Agent + " has not started a conversation yet (no id to resume)"
		return m, nil
	}
	sessionID := session.ID
	return m, func() tea.Msg {
		resumed, outcome, err := m.resume(context.Background(), sessionID)
		return sessionResumed{session: resumed, outcome: outcome, err: err, fromResumeKey: true}
	}
}

// keyRestart handles "R".
func (m Model) keyRestart(_ tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.restart == nil {
		return m, nil
	}
	// task 013/D.2: the guard above already refused this keypress
	// when the cursor has no selected session.
	session, _ := m.selectedSession()
	if !canRestart(session) {
		m.attachError = "Cannot restart: session is not running (use r to resume it)"
		return m, nil
	}
	if m.resumableWithNoConversationIDYet(session) {
		m.attachError = "Cannot restart: " + session.Agent + " has not started a conversation yet (no id to resume)"
		return m, nil
	}
	if session.Agent == "shell" {
		// Task 023: a shell session gets the restart/inject-instead
		// choice instead of restarting immediately -- restarting a
		// plain shell loses whatever state (cwd, history, running
		// commands) that shell had, which inject-instead avoids.
		m.restartChoosing = true
		m.restartChoiceValue = "restart"
		m.restartChoiceNote = ""
		return m, nil
	}
	sessionID := session.ID
	return m, func() tea.Msg {
		restarted, outcome, err := m.restart(context.Background(), sessionID)
		return sessionRestarted{session: restarted, outcome: outcome, err: err}
	}
}

// keyEditEnv handles "e".
func (m Model) keyEditEnv(_ tea.KeyMsg) (tea.Model, tea.Cmd) {
	// SPEC §6.1/§6.3, task 020: opens for any selected session (unlike
	// `P`/`p`, which gate on adapter capabilities) since every session,
	// including a plain shell one, has an effective environment worth
	// showing -- there is no "not applicable here" case to refuse.
	// task 013/D.2: the guard above already refused this keypress
	// when the cursor has no selected session, so no local check is
	// needed here anymore.
	m.envEditing = true
	m.envCursor = 0
	m.envEditKey, m.envEdit, m.envNote = "", lineedit.Editor{}, ""
	m.envReveal = false
	m.envScroll = 0
	return m, nil
}

// keyEventLog handles "E".
func (m Model) keyEventLog(_ tea.KeyMsg) (tea.Model, tea.Cmd) {
	// SPEC §12/requirement 32, task 124: unlike `e`, this is global
	// -- not gated on a selected session -- since the event log
	// lists every session's events, not one row's own environment.
	// eventLogScroll resets (task 078) so a reopen never starts
	// scrolled from wherever a previous visit left off. R61 (steer
	// 3e-001 §6.3): the store read itself is dispatched here, once,
	// as a tea.Cmd (loadEventLog) rather than performed inline in
	// View() -- eventLogRows/eventLogErr also reset so a reopen
	// never renders the previous visit's rows before the fresh
	// fetch lands.
	m.eventLogOpen = true
	m.eventLogScroll = 0
	m.eventLogRows = nil
	m.eventLogErr = nil
	return m, m.loadEventLog
}

// keyFilter handles "/".
func (m Model) keyFilter(_ tea.KeyMsg) (tea.Model, tea.Cmd) {
	// SPEC.md:984/requirement 33, task 123: global like `E` above --
	// not gated on a selected session, since an empty list is still
	// worth filtering into (e.g. to prove nothing archived matches).
	// Reopening with an existing query keeps it rather than clearing
	// it, mirroring the rename dialog's own prefill convention, so a
	// second `/` refines an already-applied filter instead of
	// discarding it.
	if !m.help {
		m.filtering = true
		// A held query is kept, with the caret at its end (not an
		// offer: a second `/` refines it).
		m.filterEdit = lineedit.New(m.filterQuery).Fit(m.filterFieldWidth(), m.filterEditStyle())
		return m, m.loadArchivedSessions
	}
	return m, nil
}

// keyNextAttention handles " ".
func (m Model) keyNextAttention(_ tea.KeyMsg) (tea.Model, tea.Cmd) {
	// SPEC requirements 31, 32: move to the next session needing
	// attention, wrapping, via the one shared NeedsAttention answer
	// (internal/tui/attention.go). Nothing needing attention (ok
	// false) leaves selection — and every session's status —
	// untouched: this must never behave like §7's attach, which
	// clears "waiting" on the attached session.
	if !m.help && len(m.sessions) > 0 {
		if next, ok := m.nextAttentionSelection(m.selected); ok {
			m.setSelection(next)
		}
	}
	return m, nil
}

// keyToggleGroup handles "c".
func (m Model) keyToggleGroup(_ tea.KeyMsg) (tea.Model, tea.Cmd) {
	// SPEC §11.8 gap (requirement 30's collapsible headers had no key):
	// toggle the group whose HEADER IS UNDER THE CURSOR collapsed/
	// expanded -- task 014/D.3 re-aimed this from "the selected row's
	// own group" (true only before task 012/D.1 made a header a cursor
	// stop in its own right) to cursorGroupID's own resolution: the
	// cursor's own header id when it rests on one, or the group id of
	// the session under a row cursor otherwise. Keyed by the group's id
	// (task 013/R129 part 3, sessionGroupID) rather than its display
	// name, via the identical helper the mouse header click calls
	// (toggleGroupCollapse, internal/tui/mouse.go), so neither path is
	// ever the only way to reach this capability, and the result is
	// persisted to ui_state's collapsed_groups (SPEC §11: "collapse
	// state persists in ui_state") the same way `|`/`<`/`>` persist
	// layout_mode/sidebar_width. A no-op, like every other bare-letter
	// binding, while help or the `i` detail overlay covers the
	// sidebar, or when there is no row to resolve a group from.
	//
	// Task 119: this was originally bound to `g`, which collides with
	// SPEC.md:952's own keymap entry "g/G top/bottom" -- `g`/`G` were
	// never actually wired to anything, so every keypress of `g` was
	// silently doing collapse instead of the documented top/bottom jump.
	// `c` (collapse) does not appear anywhere in SPEC §11's keymap list.
	//
	// Deliberately NOT gated on len(m.sessions) > 0 (cure-01-04, F2/R137,
	// same cure-012-01 reasoning as g/G below): a group -- including the
	// implicit default group (id 0) -- can be entirely empty while still
	// holding a real, addressable header cursor stop, and that header
	// must still fold/unfold. cursorGroupID resolves a header cursor's
	// own id directly, with no m.sessions lookup, so it is already safe
	// with zero total sessions; only this stale guard blocked it.
	if !m.help && !m.detail {
		if groupID, ok := m.cursorGroupID(); ok {
			m.toggleGroupCollapse(groupID)
			m.setSelection(m.selected)
			return m, m.persistCollapsedGroups()
		}
	}
	return m, nil
}

// keyCollapseGroup handles "left".
func (m Model) keyCollapseGroup(_ tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Task 014/D.3's explicit-direction companions to `c` above: left
	// FOLDS the group under the cursor, right (below) UNFOLDS it --
	// never toggling, so a repeated left on an already-folded group
	// (or a repeated right on an already-unfolded one) is a no-op
	// rather than flipping back. Same guards, same group resolution
	// (cursorGroupID: the cursor's own header id, or the group id of
	// the session under a row cursor) and the same ui_state
	// persistence as `c`. Deliberately NOT gated on len(m.sessions) > 0
	// (cure-01-04, F2/R137) -- see `c`'s own comment above.
	if !m.help && !m.detail {
		if groupID, ok := m.cursorGroupID(); ok {
			m.setGroupCollapsed(groupID, true)
			m.setSelection(m.selected)
			return m, m.persistCollapsedGroups()
		}
	}
	return m, nil
}

// keyExpandGroup handles "right".
func (m Model) keyExpandGroup(_ tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Same cure-01-04 reasoning as `c`/left above.
	if !m.help && !m.detail {
		if groupID, ok := m.cursorGroupID(); ok {
			m.setGroupCollapsed(groupID, false)
			m.setSelection(m.selected)
			return m, m.persistCollapsedGroups()
		}
	}
	return m, nil
}

// keyFirstRow handles "g".
func (m Model) keyFirstRow(_ tea.KeyMsg) (tea.Model, tea.Cmd) {
	// SPEC.md:952 "g/G top/bottom": jump to the first visible visual
	// stop (task 012/D.1: a header counts now, not only a row) --
	// mirrors up/down's own visualOrder-based navigation, so a
	// collapsed group's hidden rows are skipped exactly like a
	// single up/down press would skip them.
	//
	// Deliberately NOT gated on len(m.sessions) > 0 (cure-012-01):
	// since task 012 a header is a visual stop in its own right, so a
	// sidebar holding only headers (every persisted group empty, or
	// just the implicit default group's own header) still has a first
	// and a last stop for g/G to land on. visibleSessionIndices()'s
	// own emptiness check is the only guard this needs.
	if !m.help && !m.detail {
		if visible := m.visibleSessionIndices(); len(visible) > 0 {
			m.setSelection(visible[0])
		}
	}
	return m, nil
}

// keyLastRow handles "G".
func (m Model) keyLastRow(_ tea.KeyMsg) (tea.Model, tea.Cmd) {
	// SPEC.md:952 "g/G top/bottom": jump to the last visible stop.
	// Same cure-012-01 reasoning as `g` above: header-only sidebars
	// have a last stop too, so no len(m.sessions) gate here either.
	if !m.help && !m.detail {
		if visible := m.visibleSessionIndices(); len(visible) > 0 {
			m.setSelection(visible[len(visible)-1])
		}
	}
	return m, nil
}

// keyCycleLayout handles "|".
func (m Model) keyCycleLayout(_ tea.KeyMsg) (tea.Model, tea.Cmd) {
	if !m.help {
		return m.cycleLayoutMode()
	}
	return m, nil
}

// keyNarrowSidebar handles "<".
func (m Model) keyNarrowSidebar(_ tea.KeyMsg) (tea.Model, tea.Cmd) {
	if !m.help {
		m.sidebarWidth = m.adjustSidebarWidth(-1)
		return m, m.persistSidebarWidth()
	}
	return m, nil
}

// keyWidenSidebar handles ">".
func (m Model) keyWidenSidebar(_ tea.KeyMsg) (tea.Model, tea.Cmd) {
	if !m.help {
		m.sidebarWidth = m.adjustSidebarWidth(1)
		return m, m.persistSidebarWidth()
	}
	return m, nil
}

// keyEnter handles "enter".
func (m Model) keyEnter(_ tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m.enterInteractive()
}

// keyEnterFull handles "F".
func (m Model) keyEnterFull(_ tea.KeyMsg) (tea.Model, tea.Cmd) {
	// SPEC.md §11.9's force-attach: steal the interactive preview over
	// any existing holder. This is `↵`'s own enterInteractiveBody with
	// force=true (task 105) -- every refusal in that ladder still
	// applies except the attached-client one, which is exactly what
	// force exists to skip; the claim itself is taken via
	// ForceClaimWindowOwnership (task 101), not ClaimWindowOwnership.
	return m.enterInteractiveBody(true)
}

// keyAttach handles "a".
func (m Model) keyAttach(_ tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m.attachSelected()
}
