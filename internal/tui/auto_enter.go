package tui

import tea "github.com/charmbracelet/bubbletea"

// autoEnterTickBudget is how many previewTicks the auto-enter intent (GH
// #52; SPEC §11's newly-created bullet, §9.1, §11.9) waits for its
// session's pane to be reported live before it is dropped silently. It is
// counted from the create's or resume's own success, so it also covers the
// reload that first lists the row (or its no-longer-stopped status): at
// the default 250ms tick that is 2.5s, far longer than a fresh pane takes
// to exist (the create or resume itself starts the tmux session before its
// result message ever lands), and still short enough that a session which
// never goes live does not leave the intent armed for minutes.
const autoEnterTickBudget = 10

// armAutoEnter records the one auto-enter intent this client can hold (GH
// #52): once sessionID is selected and its pane is live, enter the
// interactive preview on it exactly as `↵` would. It is armed from exactly
// two places -- a successful create from `n` when [ui] attach_on_new is on
// (shellCreated), and a successful `r` resume or `R` restart when
// [ui] attach_on_resume is on (sessionResumed/sessionRestarted) -- with
// enabled carrying whichever of the two keys applies. It lives on this
// Model and nowhere durable, so only the client that asked ever acts on
// it. Any earlier intent is replaced, not kept: a second create or resume
// is a new aim.
func (m *Model) armAutoEnter(sessionID string, enabled bool) {
	m.cancelAutoEnter()
	if !enabled || sessionID == "" {
		return
	}
	m.pendingAutoEnterSessionID = sessionID
	m.pendingAutoEnterTicks = autoEnterTickBudget
}

// cancelAutoEnter drops the intent for good. Update calls it on every key
// and every mouse report before anything else sees them, which is what
// makes "the user always wins" hold for every layer at once: a selection
// move, a dialog, an overlay, the filter, settings and another `n` or `r`
// are all reached only through a key or a click. It is also how the intent
// is consumed when it fires, so it can never fire twice.
func (m *Model) cancelAutoEnter() {
	m.pendingAutoEnterSessionID = ""
	m.pendingAutoEnterTicks = 0
}

// autoEnterTargetReady reports whether the intent is armed and the row it
// is waiting for is the one selected right now, as a row `↵` may act on.
// The selection is the precondition, not something this intent sets:
// pendingSelectSessionID selects a created row on the first load that
// lists it, and `r`/`R` act on the row already selected. canReachPane is
// part of the wait rather than left to entry's own refusal because a
// resumed row still reads `stopped` in this client's list until the reload
// the resume issues lands, and a capture of the new pane can arrive first.
func (m Model) autoEnterTargetReady() bool {
	if m.pendingAutoEnterSessionID == "" || m.creating {
		return false
	}
	session, ok := m.selectedSession()
	return ok && session.ID == m.pendingAutoEnterSessionID && canReachPane(session)
}

// tickAutoEnter is the intent's previewTick step: it spends one tick of
// the budget, drops the intent once the budget is gone, and fires it when
// the preview panel is not shown at all. That last case is the one where
// no previewCaptured will ever arrive to report the pane live
// (capturePreview issues nothing while the panel is hidden) and where `↵`
// would refuse on the panel's size before looking at the pane anyway, so
// entering now produces exactly the refusal `↵` would.
func (m Model) tickAutoEnter() (Model, tea.Cmd) {
	if m.pendingAutoEnterSessionID == "" {
		return m, nil
	}
	if m.pendingAutoEnterTicks <= 0 {
		m.cancelAutoEnter()
		return m, nil
	}
	m.pendingAutoEnterTicks--
	if m.autoEnterTargetReady() && !m.computeLayout().PreviewShown {
		return m.fireAutoEnter()
	}
	return m, nil
}

// captureAutoEnter is the intent's previewCaptured step: the capture
// engine's own report that the selected target's pane is live is the
// signal the intent waits for, so entry never meets `↵`'s "no live pane"
// refusal merely because the row landed as `starting` a tick before tmux
// had a pane to show.
func (m Model) captureAutoEnter(msg previewCaptured) (Model, tea.Cmd) {
	if msg.err != nil || !msg.capture.Live || msg.sessionID != m.pendingAutoEnterSessionID || !m.autoEnterTargetReady() {
		return m, nil
	}
	return m.fireAutoEnter()
}

// fireAutoEnter consumes the intent and runs `↵`'s own entry
// (enterInteractive, SPEC §11.9) -- never a second implementation -- so a
// successful entry claims, fits and records the attachment exactly as `↵`
// does, and a refused one sets the same banner and leaves the session
// selected in the list. The intent is spent either way: there is no retry.
func (m Model) fireAutoEnter() (Model, tea.Cmd) {
	m.cancelAutoEnter()
	next, cmd := m.enterInteractive()
	return next.(Model), cmd
}
