package tui

import (
	"context"
	"hash/fnv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/n-orlov/deck/internal/store"
)

// R321: engagement sets seen. In attach or the interactive preview, the first
// forwarded key, the first scroll (a passive-preview wheel notch included)
// and the first change of the screen's content on an unseen waiting/error row
// set seen_status_at = status_at, and every one of those triggers goes through
// Model.engage, the one function that decides and issues the write.
// Engagement sets seen only: it writes no status, no event, no notify_epoch
// and moves no row by itself (the sort tiers read the seen predicate, R319,
// and nothing else).

type engagementTrigger string

const (
	engageKey           engagementTrigger = "key"
	engageScroll        engagementTrigger = "scroll"
	engagePassiveScroll engagementTrigger = "passive-preview scroll"
	engageScreen        engagementTrigger = "screen content change"
)

// engagementState is the shared cell (every Model copy points at the same
// one) that remembers which status stamps already carried an engagement
// write, so a second trigger before the reload lands issues nothing, and the
// screen baseline the content-change trigger compares against.
type engagementState struct {
	// issued maps a session id to the status_at its engagement write named.
	issued map[string]int64
	// screenTarget/screenHash are the baseline of the interactive preview's
	// live screen; screenTarget is "" outside interactive mode.
	screenTarget string
	screenHash   uint64
}

func (m *Model) engagementCell() *engagementState {
	if m.engagement == nil {
		m.engagement = &engagementState{issued: map[string]int64{}}
	}
	return m.engagement
}

// engagementTargetSession is the row a trigger engages: the interactive
// target while interactive mode owns the keyboard, otherwise the selected
// row (the one the passive preview shows).
func (m Model) engagementTargetSession() (store.Session, bool) {
	if m.interactive {
		return m.interactiveTargetSession()
	}
	if !m.hasSelectedSession() {
		return store.Session{}, false
	}
	return m.selectedSession()
}

// engage is the single code path that sets seen on engagement. It returns the
// command that performs the one write, or nil when there is nothing to do: no
// target, a status other than waiting/error, a row already seen, or a write
// already issued for this status stamp.
func (m *Model) engage(trigger engagementTrigger) tea.Cmd {
	if m.markSeen == nil {
		return nil
	}
	session, ok := m.engagementTargetSession()
	if !ok || (session.Status != "waiting" && session.Status != "error") || session.StatusSeen() {
		return nil
	}
	cell := m.engagementCell()
	if stamp, done := cell.issued[session.ID]; done && stamp == session.StatusAt {
		return nil
	}
	cell.issued[session.ID] = session.StatusAt
	markSeen, id, statusAt := m.markSeen, session.ID, session.StatusAt
	return func() tea.Msg {
		_, err := markSeen(context.Background(), id, statusAt)
		return sessionEngaged{err: err, trigger: trigger}
	}
}

// sessionEngaged is engage's write result.
type sessionEngaged struct {
	err     error
	trigger engagementTrigger
}

func (m Model) onSessionEngaged(msg sessionEngaged) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.setAttachError("Cannot mark seen: " + msg.err.Error())
		return m, nil
	}
	return m, m.loadSessions
}

// interactiveScreenHash fingerprints the interactive grid's live screen (the
// bottom of the grid, whatever the scrollback offset shows).
func (m Model) interactiveScreenHash() (uint64, bool) {
	if !m.interactive || m.interactiveGrid == nil || m.interactiveGrid.Grid() == nil {
		return 0, false
	}
	_, height := m.previewContentSize()
	if height <= 0 {
		height = interactiveMinInnerRows
	}
	rows, _ := m.interactiveGrid.RenderRows(0, height)
	h := fnv.New64a()
	_, _ = h.Write([]byte(strings.Join(rows, "\n")))
	return h.Sum64(), true
}

// baselineInteractiveScreen records the screen as it is now; a later change
// of it is the content-change trigger.
func (m *Model) baselineInteractiveScreen() {
	cell := m.engagementCell()
	if sum, ok := m.interactiveScreenHash(); ok {
		cell.screenTarget, cell.screenHash = m.interactiveWindowTarget, sum
		return
	}
	cell.screenTarget, cell.screenHash = "", 0
}

// engageOnScreenChange is the content-change trigger, run on the preview tick
// while interactive. The first tick after a target change only baselines.
func (m *Model) engageOnScreenChange() tea.Cmd {
	if !m.interactive {
		return nil
	}
	cell := m.engagementCell()
	sum, ok := m.interactiveScreenHash()
	if !ok {
		return nil
	}
	if cell.screenTarget != m.interactiveWindowTarget {
		cell.screenTarget, cell.screenHash = m.interactiveWindowTarget, sum
		return nil
	}
	if sum == cell.screenHash {
		return nil
	}
	cell.screenHash = sum
	return m.engage(engageScreen)
}
