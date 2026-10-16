package tui

import (
	"context"
	"hash/fnv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/n-orlov/deck/internal/store"
)

// R321: engagement sets seen. In attach or the interactive preview, the first
// forwarded key, the first scroll (a passive-preview wheel notch included)
// and the first change of the screen's content on an unseen waiting/error row
// set seen_status_at = status_at, and every one of those triggers goes through
// Model.engageRow, the one function that decides and issues the write (the
// preview's triggers via Model.engage, a full attach's via onAttachFinished
// and the watcher in attach_engagement.go).
// Engagement sets seen only: it writes no status, no event, no notify_epoch
// and moves no row. The seen predicate (R319) would drop an engaged error
// row to the seen-error tier, so the attention sort holds an engaged row in
// the tier it had before engagement until its status_at next changes
// (engagementHold, attentionSortRank); the count and `space` read the
// predicate as it is.

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
	// issued maps a session id to the status episode (status_at AND
	// status_seq: a stamp alone can repeat) its engagement write named. It is
	// also the sort hold: a row still in its entry's episode keeps its
	// pre-engagement attention tier. An entry only ever moves forward in
	// status_seq: a delayed engagement of an older episode never replaces it.
	issued map[string]store.StatusEpisode
	// screenTarget/screenHash are the baseline of the interactive preview's
	// live screen; screenTarget is "" outside interactive mode.
	screenTarget string
	screenHash   uint64
	// dwell is the R322 dwell in progress (dwell.go); dwelling is false while
	// no row is in view for one. blurred is the terminal window's reported
	// focus loss. now replaces the dwell's clock in tests.
	dwell    dwellState
	dwelling bool
	blurred  bool
	now      func() time.Time
	// view is the R325 viewing clock (viewed.go).
	view viewState
}

func (m *Model) engagementCell() *engagementState {
	if m.engagement == nil {
		m.engagement = &engagementState{issued: map[string]store.StatusEpisode{}}
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
	session, ok := m.engagementTargetSession()
	if !ok {
		return nil
	}
	return m.engageRow(trigger, session)
}

// engageRow is the one function every engagement trigger ends in -- the
// interactive preview's (through engage) and a full attach's (through
// onAttachFinished) alike. It decides and issues the single seen write for
// session's current status episode, and records that episode so the row keeps
// its sidebar position (engagementHold).
func (m *Model) engageRow(trigger engagementTrigger, session store.Session) tea.Cmd {
	if m.markSeen == nil {
		return nil
	}
	if (session.Status != "waiting" && session.Status != "error") || session.StatusSeen() {
		return nil
	}
	cell := m.engagementCell()
	episode := session.Episode()
	if issued, done := cell.issued[session.ID]; done && (issued == episode || issued.Seq > episode.Seq) {
		// Already issued for this episode, or a NEWER episode of the row was
		// engaged before this older one's delayed trigger arrived (an attach
		// completion). status_seq counts the row's transitions, so it orders
		// episodes where status_at cannot (equal stamps, a stepped-back
		// clock). The older trigger must neither replace the newer entry --
		// which is the cached decision and the sidebar hold -- nor issue a
		// write the store would refuse anyway.
		return nil
	}
	cell.issued[session.ID] = episode
	markSeen, id := m.markSeen, session.ID
	return func() tea.Msg {
		_, err := markSeen(context.Background(), id, episode)
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

// engagementHold is the attention sort's hold set: session id -> the status
// episode an engagement write named. Nil when nothing was engaged.
func (m Model) engagementHold() map[string]store.StatusEpisode {
	if m.engagement == nil || len(m.engagement.issued) == 0 {
		return nil
	}
	return m.engagement.issued
}
