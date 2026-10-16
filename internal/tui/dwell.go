package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/n-orlov/deck/internal/store"
)

// R322: a dwell sets seen. A row that stays selected with its pane in view for
// seenDwell sets seen_status_at = status_at, but only when its prompt was
// already on screen at the moment the dwell started. A prompt that appears
// mid-dwell stays unseen until an engagement (R321) sets it. The write itself
// is engageRow's, so it changes no status, writes no event and moves no row.
//
// "In view" means the row is the one the preview shows (the selected row, or
// the interactive target), its status is an unseen waiting/error, the preview
// panel is on screen (not collapsed away below its floor, no modal or takeover over it),
// the terminal window has not reported losing focus, and the pane's capture
// is the row's own. Anything that stops being true ends the dwell: arrowing
// to another row, collapsing the pane, the window blurring. When it is true
// again a new dwell starts and its prompt-at-start is read afresh.
//
// "The prompt on screen" is the row's agent adapter probing the pane text in
// view (the passive capture, or the interactive grid) and reporting the row's
// own attention status. An agent with no probe rule for it never dwells to
// seen.

// seenDwell is the dwell that sets seen.
const seenDwell = 1500 * time.Millisecond

const engageDwell engagementTrigger = "dwell"

// dwellState is the dwell in progress, if any (engagementState.dwelling).
type dwellState struct {
	sessionID     string
	statusAt      int64
	startedAt     time.Time
	promptAtStart bool
}

// dwellClock is the dwell's elapsed-time source: the monotonic clock, which a
// frozen DECK_CLOCK wall clock does not stop. Tests replace it on the cell.
func (e *engagementState) dwellClock() time.Time {
	if e.now != nil {
		return e.now()
	}
	return time.Now()
}

// onWindowFocus records the terminal window's focus (bubbletea's FocusMsg and
// BlurMsg, requested by cmd/deck). A terminal that never reports focus leaves
// the deck "focused". The reports are consumed here and never forwarded to a
// pane.
func (m Model) onWindowFocus(_ tea.FocusMsg) (tea.Model, tea.Cmd) { return m.setWindowFocus(false) }

func (m Model) onWindowBlur(_ tea.BlurMsg) (tea.Model, tea.Cmd) { return m.setWindowFocus(true) }

func (m Model) setWindowFocus(blurred bool) (tea.Model, tea.Cmd) {
	m.engagementCell().blurred = blurred
	m.observeDwell()
	return m, nil
}

// dwellSubject is the row a dwell would be about right now, or ok=false when
// nothing is in view for one.
func (m Model) dwellSubject() (session store.Session, ok bool) {
	if m.engagement != nil && m.engagement.blurred {
		return store.Session{}, false
	}
	session, ok = m.engagementTargetSession()
	if !ok || !unseenAttention(session) || !m.dwellViewLive(session) {
		return store.Session{}, false
	}
	return session, true
}

// unseenAttention reports a waiting or error row whose prompt is still unseen.
func unseenAttention(session store.Session) bool {
	return (session.Status == "waiting" || session.Status == "error") && !session.StatusSeen()
}

// dwellViewLive reports whether the preview (or interactive grid) in view is
// live and shows session, with no takeover covering it.
func (m Model) dwellViewLive(session store.Session) bool {
	if m.takeoverActive() || !m.computeLayout().PreviewShown {
		return false
	}
	if m.interactive {
		return m.interactiveGrid != nil && m.interactiveGrid.Grid() != nil
	}
	return m.previewLive && m.previewSessionID == session.ID
}

// dwellScreenText is the plain text of the pane in view.
func (m Model) dwellScreenText() string {
	if m.interactive {
		_, height := m.previewContentSize()
		if height <= 0 {
			height = interactiveMinInnerRows
		}
		rows, _ := m.interactiveGrid.RenderRows(0, height)
		return ansi.Strip(strings.Join(rows, "\n"))
	}
	return ansi.Strip(string(m.previewBytes))
}

// promptOnScreen reports whether the pane in view shows session's prompt: its
// agent's probe of the pane text names the row's own attention status.
func (m Model) promptOnScreen(session store.Session) bool {
	adapter, ok := m.registry().Lookup(session.Agent)
	if !ok {
		return false
	}
	status, _ := adapter.Probe(m.dwellScreenText())
	return status == session.Status
}

// observeDwell starts, keeps or ends the dwell for the model's current view.
// It is cheap and runs after every message, so a row that is arrowed past and
// back between two preview ticks still starts a new dwell.
func (m *Model) observeDwell() {
	m.observeView() // R325's viewing clock follows every message too
	session, ok := m.dwellSubject()
	if !ok {
		if m.engagement != nil {
			m.engagement.dwelling = false
		}
		return
	}
	cell := m.engagementCell()
	if cell.dwelling && cell.dwell.sessionID == session.ID && cell.dwell.statusAt == session.StatusAt {
		return
	}
	cell.dwell = dwellState{
		sessionID:     session.ID,
		statusAt:      session.StatusAt,
		startedAt:     cell.dwellClock(),
		promptAtStart: m.promptOnScreen(session),
	}
	cell.dwelling = true
}

// engageOnDwell is the dwell trigger, run on the preview tick: once seenDwell
// has passed on a dwell whose prompt was on screen at its start, the row is
// engaged.
func (m *Model) engageOnDwell() tea.Cmd {
	m.observeDwell()
	if m.engagement == nil || !m.engagement.dwelling || !m.engagement.dwell.promptAtStart {
		return nil
	}
	if m.engagement.dwellClock().Sub(m.engagement.dwell.startedAt) < seenDwell {
		return nil
	}
	return m.engage(engageDwell)
}

// observeDwellAfterUpdate is Update's hook: the dwell follows every message.
func observeDwellAfterUpdate(next tea.Model) tea.Model {
	model, ok := next.(Model)
	if !ok {
		return next
	}
	model.observeDwell()
	return model
}
