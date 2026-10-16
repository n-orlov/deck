package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/n-orlov/deck/internal/store"
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
