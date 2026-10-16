package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// R326: Alt+U / Alt+Shift+U in the list. Alt+U jumps to the next unseen
// session in list order from the current row, wrapping; Alt+Shift+U to the
// previous one. "Unseen" is store.Session.Unseen, the one predicate the `◆`
// marker and the N unseen header already use, so a seen `error` row stays
// reachable exactly until its output is viewed. `space` is a different
// question (blocked on me, by status) and is untouched.
//
// Landing views the row (the same last_seen_at write R325's viewing makes) and
// nothing else: it sets no seen_status_at, writes no status or event, and the
// rows do not move. The unseen marker may clear on the landed row while its
// status stays unseen until engagement (R322) sets it.
const (
	unseenNextKey     = "alt+u"
	unseenPreviousKey = "alt+U"
	noUnseenNote      = "No unseen sessions"
)

// keyNextUnseen handles "alt+u".
func (m Model) keyNextUnseen(_ tea.KeyMsg) (tea.Model, tea.Cmd) { return m.jumpToUnseen(1) }

// keyPreviousUnseen handles "alt+U" (Alt+Shift+U).
func (m Model) keyPreviousUnseen(_ tea.KeyMsg) (tea.Model, tea.Cmd) { return m.jumpToUnseen(-1) }

// jumpToUnseen moves the selection to the nearest unseen visible session in
// direction dir and views it; with none it shows a one-line message.
func (m Model) jumpToUnseen(dir int) (tea.Model, tea.Cmd) {
	if m.help || len(m.sessions) == 0 {
		return m, nil
	}
	next, ok := m.stepSelection(m.selected, dir, store.Session.Unseen)
	if !ok {
		m.setFieldCopyNote(noUnseenNote)
		return m, nil
	}
	m.setSelection(next)
	idx, _ := next.SessionIndex()
	session := m.sessions[idx]
	if m.engagement == nil || !m.viewWriteNeeded(session) {
		return m, nil
	}
	return m, m.issueViewWrite(session)
}

// R327: the same two chords while a session is attached (interactive mode).
// They are intercepted next to Ctrl+Q and the bookmark keys, so no byte of
// them reaches a pane, and they switch the attached view to the next / previous
// unseen session exactly as a bookmark jump switches it: interactive mode
// leaves the current window and enters the target's, without a list round
// trip. They are not gated by the bookmarks setting. A stopped session cannot
// be attached, so it is not a candidate. With nothing else to switch to the
// view stays and the footer's note slot shows noUnseenNote until the next key.

// handleInteractiveUnseenKey is the attached-mode layer of the unseen jumps.
func (m Model) handleInteractiveUnseenKey(msg tea.KeyMsg) (next Model, cmd tea.Cmd, handled bool) {
	if m.fieldCopyNote == noUnseenNote {
		m.setFieldCopyNote("")
	}
	var dir int
	switch msg.String() {
	case unseenNextKey:
		dir = 1
	case unseenPreviousKey:
		dir = -1
	default:
		return m, nil, false
	}
	next, cmd = m.jumpInteractiveUnseen(dir)
	return next, cmd, true
}

// jumpInteractiveUnseen switches the attached view to the nearest unseen
// attachable session in direction dir, counted from the session the view is
// bound to (not from the list selection, which a sidebar wheel drift or a
// header click can leave elsewhere).
func (m Model) jumpInteractiveUnseen(dir int) (Model, tea.Cmd) {
	from := m.selected
	if idx, ok := m.interactiveTargetIndex(); ok {
		from = rowCursor(idx)
	}
	found, ok := m.stepSelection(from, dir, func(s store.Session) bool {
		return s.Unseen() && canReachPane(s)
	})
	if !ok {
		m.setFieldCopyNote(noUnseenNote)
		return m, nil
	}
	idx, _ := found.SessionIndex()
	if name, err := tmux.SessionName(m.sessions[idx].Slug); err == nil && name == m.interactiveWindowTarget {
		return m, nil
	}
	left, _ := m.exitInteractive()
	next := left.(Model)
	next.setSelection(found)
	entered, cmd := next.enterInteractive()
	return entered.(Model), cmd
}
