package tui

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// newDyingSelectionPane starts a real tmux session named target (already
// the full deck_<slug> window name, not a bare slug) whose pane will be
// swapped to a dead, RETAINED corpse: remain-on-exit is set window-scoped
// (never server-wide, unlike tmux.Client.Bootstrap's own "failed" -- this
// fixture needs "on" so even the exit-0 respawn below stays retained
// deterministically) before anything is asked to exit, so there is no race
// between setting the option and the pane's process leaving. The initial
// process is a long sleep purely so the window exists to set the option
// against; deadenSelectionPane below is what actually kills it.
func newDyingSelectionPane(t *testing.T, socket, target string, width, height int) {
	t.Helper()
	args := []string{
		"-L", socket, "new-session", "-d", "-s", target,
		"-x", strconv.Itoa(width), "-y", strconv.Itoa(height),
		"sleep", "600",
	}
	if out, err := exec.Command("tmux", args...).CombinedOutput(); err != nil {
		t.Fatalf("start tmux session %q: %v: %s", target, err, out)
	}
	t.Cleanup(func() {
		_ = exec.Command("tmux", "-L", socket, "kill-server").Run()
	})
	if out, err := exec.Command("tmux", "-L", socket, "set-window-option", "-t", target, "remain-on-exit", "on").CombinedOutput(); err != nil {
		t.Fatalf("set remain-on-exit on %q: %v: %s", target, err, out)
	}
}

// deadenSelectionPane kills target's own pane process in place (respawn-pane
// -k with a command that exits immediately) so the retained corpse
// tmux.Client.PreviewPane treats as "no live pane" is real, not merely
// simulated -- respawnAlivePane below is the exact mirror, bringing the
// SAME window back to life the way a session's own agent restarting would.
func deadenSelectionPane(t *testing.T, socket, target string) {
	t.Helper()
	if out, err := exec.Command("tmux", "-L", socket, "respawn-pane", "-k", "-t", target, "sh", "-c", "exit 7").CombinedOutput(); err != nil {
		t.Fatalf("respawn-pane (die) %q: %v: %s", target, err, out)
	}
}

func respawnAlivePane(t *testing.T, socket, target string) {
	t.Helper()
	if out, err := exec.Command("tmux", "-L", socket, "respawn-pane", "-k", "-t", target, "sleep", "600").CombinedOutput(); err != nil {
		t.Fatalf("respawn-pane (revive) %q: %v: %s", target, err, out)
	}
}

// waitForPreviewPaneLiveness polls client.PreviewPane(ctx, slug) until its
// ok return matches want, or fails the test -- both PreviewPane's own dead-
// pane detection and tmux's own asynchronous bookkeeping around
// respawn-pane are not instantaneous.
func waitForPreviewPaneLiveness(t *testing.T, client tmux.Client, slug string, want bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, ok, err := client.PreviewPane(context.Background(), slug)
		if err == nil && ok == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("PreviewPane(%q) liveness never reached %v (last ok=%v err=%v)", slug, want, ok, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestReviewNoLivePaneRefusalClearsWhenTickFindsRespawnedPane is
// cure-01-01-2's real-private-tmux regression: a no-live-pane refusal set
// by a genuine ↵ against a genuine retained-dead pane (Model.Update, not a
// bare setEntryRefusal call) must persist across a previewTick that still
// finds the pane dead, and then clear on the FIRST previewTick after the
// pane is respawned alive again -- all without ever moving the selection
// or pressing any other key. capturePreview only ever captures the
// CURRENTLY SELECTED row (tui.go), so leaving the selection on the refused
// session throughout is exactly what proves the clearing is driven by the
// tick's own observation, not by a move.
func TestReviewNoLivePaneRefusalClearsWhenTickFindsRespawnedPane(t *testing.T) {
	socket := selectionTestSocket("norefusal1")
	target, err := tmux.SessionName("norefusal1")
	if err != nil {
		t.Fatalf("SessionName: %v", err)
	}
	newDyingSelectionPane(t, socket, target, 80, 24)
	deadenSelectionPane(t, socket, target)

	client := tmux.Client{Socket: socket}
	waitForPreviewPaneLiveness(t, client, "norefusal1", false)

	m := New(nil, config.Settings{}, "")
	m.width, m.height = 100, 30
	if _, height := m.previewContentSize(); height < interactiveMinInnerRows {
		t.Fatalf("test assumption violated: preview content height %d is below the %d-row floor", height, interactiveMinInnerRows)
	}
	m.tmuxClient = client
	m.previewCapture = func(ctx context.Context, slug string) (tmux.PreviewCapture, error) {
		return client.CapturePreview(ctx, slug)
	}
	m.sessions = []store.Session{{ID: "sess-nolive-1", Name: "norefusal1", Slug: "norefusal1", Status: "waiting"}}
	m.baseSessions = m.sessions
	m.selected = rowCursor(0)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if !m.entryRefusal.active || m.entryRefusal.kind != entryRefusalNoLivePane || m.entryRefusal.sessionID != "sess-nolive-1" {
		t.Fatalf("↵ against a real retained-dead pane = %+v, want an active no-live-pane refusal for sess-nolive-1", m.entryRefusal)
	}

	// A tick that still finds the pane dead must not clear the refusal.
	m = runOnePreviewTick(t, m)
	if m.previewLive {
		t.Fatalf("previewTick reported the still-dead pane as live: previewSessionID=%q previewLive=%v", m.previewSessionID, m.previewLive)
	}
	if !m.entryRefusal.active {
		t.Fatalf("entryRefusal cleared while the pane is still dead: want it to remain active")
	}

	// Respawn the SAME window alive, then let the next tick observe it.
	respawnAlivePane(t, socket, target)
	waitForPreviewPaneLiveness(t, client, "norefusal1", true)

	m = runOnePreviewTick(t, m)
	if m.previewSessionID != "sess-nolive-1" || !m.previewLive {
		t.Fatalf("previewTick after the respawn = (id=%q live=%v), want the refused session reported live", m.previewSessionID, m.previewLive)
	}
	if m.entryRefusal.active {
		t.Fatalf("entryRefusal is still active after a tick observed a live pane for the same refused session: %+v, want cleared", m.entryRefusal)
	}
	if _, ok := m.activeEntryRefusalForSelection(); ok {
		t.Fatalf("banner is still showing after the refusal cleared")
	}
	cw, ch := m.previewContentSize()
	drawn, _, _ := m.previewBodyLines(cw, ch)
	rendered := stripANSI(strings.Join(drawn, "\n"))
	if strings.Contains(rendered, "NOT ATTACHED") {
		t.Fatalf("live pane observed by tick but the no-live-pane banner still renders:\n%s", rendered)
	}
}

// TestReviewNoLivePaneRefusalIgnoresAnotherSessionsCapture proves the
// sessionID guard in clearEntryRefusalIfPreviewLive: a live capture for a
// DIFFERENT session than the one refused must never clear this refusal,
// even though it is the only capture in flight and it does report Live.
func TestReviewNoLivePaneRefusalIgnoresAnotherSessionsCapture(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	m.sessions = []store.Session{{ID: "sess-a", Name: "a", Slug: "a", Status: "running"}}
	m.selected = rowCursor(0)
	m.setEntryRefusal("sess-a", entryRefusalNoLivePane, "no live pane")

	next, _ := m.Update(previewCaptured{sessionID: "sess-b", capture: tmux.PreviewCapture{Live: true}})
	got := next.(Model)
	if !got.entryRefusal.active {
		t.Fatalf("entryRefusal cleared by a live capture naming a DIFFERENT session (sess-b), want sess-a's refusal untouched")
	}

	next2, _ := got.Update(previewCaptured{sessionID: "sess-a", capture: tmux.PreviewCapture{Live: true}})
	got2 := next2.(Model)
	if got2.entryRefusal.active {
		t.Fatalf("entryRefusal is still active after a live capture for the SAME session: %+v, want cleared", got2.entryRefusal)
	}
}

// TestReviewFilteringToAnotherSessionDoesNotResurrectRefusal is
// cure-01-01-2's Model.Update regression for the filter-editing clause: a
// refusal set on alpha (row cursor 0) must clear the moment `/`+"b"
// actually selects beta, EVEN THOUGH the row cursor index never changes
// (0 both before and after -- filtering simply narrows the list down to
// beta at that same position) -- and clearing the filter with Esc back to
// the full, unfiltered list (which lands the cursor on alpha again) must
// never resurrect the old banner.
func TestReviewFilteringToAnotherSessionDoesNotResurrectRefusal(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	m.sessions = []store.Session{
		{ID: "s-alpha", Name: "alpha", Slug: "alpha", Status: "stopped"},
		{ID: "s-beta", Name: "beta", Slug: "beta", Status: "running"},
	}
	m.baseSessions = m.sessions
	m.selected = rowCursor(0)
	m.setEntryRefusal("s-alpha", entryRefusalStopped, stoppedSessionRefusalTail)

	if _, ok := m.activeEntryRefusalForSelection(); !ok {
		t.Fatalf("test setup: alpha's refusal banner is not showing before filtering -- fixture is vacuous")
	}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = next.(Model)
	if !m.filtering {
		t.Fatalf("`/` did not open the filter")
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	m = next.(Model)
	if m.selected != rowCursor(0) {
		t.Fatalf("selected cursor after filtering to \"b\" = %v, want row 0 (the cursor index must stay put -- that is the whole point of this regression)", m.selected)
	}
	session, ok := m.selectedSession()
	if !ok || session.ID != "s-beta" {
		t.Fatalf("selected session after filtering to \"b\" = (%+v, %v), want beta at the same cursor index", session, ok)
	}
	if m.entryRefusal.active {
		t.Fatalf("alpha's refusal is still active after filtering actually selected beta at the SAME cursor index: %+v, want cleared", m.entryRefusal)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if m.filtering || m.filterQuery != "" {
		t.Fatalf("esc did not clear the filter: filtering=%v filterQuery=%q", m.filtering, m.filterQuery)
	}
	session, ok = m.selectedSession()
	if !ok || session.ID != "s-alpha" {
		t.Fatalf("selected session after clearing the filter = (%+v, %v), want alpha again (test assumption for the resurrection check)", session, ok)
	}
	if m.entryRefusal.active {
		t.Fatalf("clearing the filter resurrected alpha's old refusal: %+v, want it to stay cleared", m.entryRefusal)
	}
	if _, ok := m.activeEntryRefusalForSelection(); ok {
		t.Fatalf("alpha's banner reappeared after clearing the filter, with no new entry attempt")
	}
}
