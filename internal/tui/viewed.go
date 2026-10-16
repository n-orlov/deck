package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/n-orlov/deck/internal/store"
)

// R325: what counts as viewed. A row is viewed while the passive preview has
// been visible on it for seenDwell (1.5 s), or while interactive mode is on it.
// Viewing copies the row's last_output_at into last_seen_at (Store.MarkViewed),
// and while the row stays in view that keeps happening on every preview tick,
// so output landing under a watched row never leaves it unseen. It writes no
// status, no seen_status_at, no event, and moves no row.
//
// "Visible" is the dwell's in-view test (dwellViewLive): the row is the one the
// preview shows, the preview panel is on screen (a collapsed preview views
// nothing), no modal or takeover covers it, the pane's capture is the row's own
// and live, and the terminal window has not reported losing focus. Arrowing
// away restarts the clock, so rows passed over on the way down a list stay
// unseen.

// viewState is the viewing clock: which row has been in view since when.
type viewState struct {
	sessionID string
	startedAt int64 // unix nanoseconds on the dwell clock
	viewing   bool
	// issued maps a session id to the last_output_at its view write named, so
	// a second tick before the reload lands issues nothing.
	issued map[string]int64
}

// observeView starts, keeps or ends the viewing clock for the current view.
func (m *Model) observeView() {
	if m.engagement == nil {
		return
	}
	if m.engagement.blurred {
		m.engagement.view.viewing = false
		return
	}
	session, ok := m.engagementTargetSession()
	if !ok || !m.dwellViewLive(session) {
		m.engagement.view.viewing = false
		return
	}
	cell := m.engagement
	if cell.view.viewing && cell.view.sessionID == session.ID {
		return
	}
	cell.view.sessionID, cell.view.startedAt = session.ID, cell.dwellClock().UnixNano()
	cell.view.viewing = true
}

// viewDue reports whether the row in view has been viewed long enough: at
// once in interactive mode, after seenDwell otherwise.
func (m Model) viewDue() bool {
	cell := m.engagement
	if m.interactive {
		return true
	}
	return cell.dwellClock().UnixNano()-cell.view.startedAt >= seenDwell.Nanoseconds()
}

// viewWriteNeeded reports whether session has output the view has not copied
// and no write for that output is already in flight.
func (m Model) viewWriteNeeded(session store.Session) bool {
	if session.LastOutputAt == session.LastSeenAt {
		return false
	}
	stamp, done := m.engagement.view.issued[session.ID]
	return !done || stamp != session.LastOutputAt
}

// engageOnView is the viewing trigger, run on the preview tick.
func (m *Model) engageOnView() tea.Cmd {
	m.observeView()
	if m.markViewed == nil || m.engagement == nil || !m.engagement.view.viewing || !m.viewDue() {
		return nil
	}
	session, ok := m.engagementTargetSession()
	if !ok || !m.viewWriteNeeded(session) {
		return nil
	}
	return m.issueViewWrite(session)
}

// issueViewWrite records that session's current last_output_at is being
// copied into last_seen_at and returns the write; the one place both the
// viewing trigger and a landing (Alt+U, R326) build it.
func (m *Model) issueViewWrite(session store.Session) tea.Cmd {
	cell := m.engagement
	if m.markViewed == nil || cell == nil {
		return nil
	}
	if cell.view.issued == nil {
		cell.view.issued = map[string]int64{}
	}
	cell.view.issued[session.ID] = session.LastOutputAt
	markViewed, id := m.markViewed, session.ID
	return func() tea.Msg {
		changed, err := markViewed(context.Background(), id)
		return sessionViewed{err: err, changed: changed}
	}
}

// sessionViewed is the viewing write's result.
type sessionViewed struct {
	err     error
	changed bool
}

func (m Model) onSessionViewed(msg sessionViewed) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.setAttachError("Cannot mark viewed: " + msg.err.Error())
		return m, nil
	}
	if !msg.changed {
		return m, nil
	}
	return m, m.loadSessions
}
