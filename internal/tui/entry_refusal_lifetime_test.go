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

// TestRefusalDoesNotSurviveCreatedSelection is cure-01-01-3's (R143/R148,
// SPEC §11.9) own regression for the create-selection lifetime edge: a
// refusal set on alpha (Enter against a stopped session) must clear
// PERMANENTLY the moment the newly-created beta is actually selected via
// pendingSelectSessionID's own sessionsLoaded fulfillment -- and once
// cleared, beta subsequently disappearing from the list (another client's
// deletion) must never resurrect alpha's old banner, because that would
// require a fresh entry attempt against alpha, not merely a reload.
func TestRefusalDoesNotSurviveCreatedSelection(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	m.width, m.height = 100, 30
	m.tmuxClient = tmux.Client{Socket: "dummy"}
	alpha := store.Session{ID: "alpha", Name: "alpha", Slug: "alpha", Status: "stopped"}
	beta := store.Session{ID: "beta", Name: "beta", Slug: "beta", Status: "starting"}
	m.sessions = []store.Session{alpha}
	m.baseSessions = m.sessions
	m.selected = rowCursor(0)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if !m.entryRefusal.active || m.entryRefusal.sessionID != "alpha" {
		t.Fatalf("fixture: Enter did not refuse stopped alpha: %+v", m.entryRefusal)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	m = next.(Model)
	if !m.creating {
		t.Fatalf("fixture: n did not open the create dialog")
	}

	next, _ = m.Update(shellCreated{session: beta})
	m = next.(Model)
	if m.pendingSelectSessionID != "beta" {
		t.Fatalf("fixture: shellCreated did not record beta as the pending selection intent: %q", m.pendingSelectSessionID)
	}

	next, _ = m.Update(sessionsLoaded{sessions: []store.Session{alpha, beta}})
	m = next.(Model)
	session, ok := m.selectedSession()
	if !ok || session.ID != "beta" {
		t.Fatalf("fixture: created beta was not selected after sessionsLoaded: (%+v, %v)", session, ok)
	}
	if m.entryRefusal.active {
		t.Fatalf("selecting newly-created beta did not clear alpha's refusal: %+v", m.entryRefusal)
	}

	// Beta subsequently leaves the visible list (another client deletes it).
	// The selection lands back on alpha's row, but alpha's refusal is gone
	// for good -- no old banner without a fresh entry attempt.
	next, _ = m.Update(sessionsLoaded{sessions: []store.Session{alpha}})
	m = next.(Model)
	if m.entryRefusal.active {
		t.Fatalf("alpha's old refusal reappeared after beta left with no new entry attempt: %+v", m.entryRefusal)
	}
	if _, ok := m.activeEntryRefusalForSelection(); ok {
		t.Fatalf("alpha's NOT ATTACHED banner resurrected after beta left the list, with no new entry attempt")
	}
}

// TestRefusalDoesNotSurviveRemovedAndRestoredSelection is cure-01-01-3's own
// regression for the disappearance lifetime edge: the selected row itself
// (not a different session taking its place) vanishing from a reload must
// clear its refusal permanently, so restoring the SAME session later never
// resurrects the stale banner.
func TestRefusalDoesNotSurviveRemovedAndRestoredSelection(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	m.width, m.height = 100, 30
	m.tmuxClient = tmux.Client{Socket: "dummy"}
	alpha := store.Session{ID: "alpha", Name: "alpha", Slug: "alpha", Status: "stopped"}
	m.sessions = []store.Session{alpha}
	m.baseSessions = m.sessions
	m.selected = rowCursor(0)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if !m.entryRefusal.active {
		t.Fatalf("fixture: Enter did not refuse alpha")
	}

	// Another client tombstones the only row.
	next, _ = m.Update(sessionsLoaded{sessions: nil})
	m = next.(Model)
	if _, ok := m.selectedSession(); ok {
		t.Fatalf("fixture: a session is still selected after the only row was removed")
	}
	if m.entryRefusal.active {
		t.Fatalf("refusal survived the selected row's own removal: %+v", m.entryRefusal)
	}

	// The same session comes back (undo, or the tombstone itself expiring).
	next, _ = m.Update(sessionsLoaded{sessions: []store.Session{alpha}})
	m = next.(Model)
	session, ok := m.selectedSession()
	if !ok || session.ID != "alpha" {
		t.Fatalf("fixture: alpha was not reselected after restoration: (%+v, %v)", session, ok)
	}
	if m.entryRefusal.active {
		t.Fatalf("removed selection's refusal returned after restoration with no entry attempt: %+v", m.entryRefusal)
	}
	if _, ok := m.activeEntryRefusalForSelection(); ok {
		t.Fatalf("alpha's NOT ATTACHED banner resurrected after restoration, with no new entry attempt")
	}
}

// TestRefusalOnSameSelectedSessionSurvivesResort proves the identity check
// (not merely a row-index check) in clearEntryRefusalIfSelectedSessionChanged:
// a reload that reorders the sidebar but keeps the SAME session selected
// (alpha, now at a different index because beta's status promoted it ahead
// in attention order) must retain alpha's still-applicable refusal.
func TestRefusalOnSameSelectedSessionSurvivesResort(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	m.width, m.height = 100, 30
	m.tmuxClient = tmux.Client{Socket: "dummy"}
	alpha := store.Session{ID: "alpha", Name: "alpha", Slug: "alpha", Status: "stopped"}
	beta := store.Session{ID: "beta", Name: "beta", Slug: "beta", Status: "stopped"}
	m.sessions = []store.Session{alpha, beta}
	m.baseSessions = m.sessions
	m.selected = rowCursor(0)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if !m.entryRefusal.active || m.entryRefusal.sessionID != "alpha" {
		t.Fatalf("fixture: Enter did not refuse alpha: %+v", m.entryRefusal)
	}

	beta.Status = "waiting"
	next, _ = m.Update(sessionsLoaded{sessions: []store.Session{beta, alpha}})
	m = next.(Model)
	session, ok := m.selectedSession()
	if !ok || session.ID != "alpha" {
		t.Fatalf("fixture: alpha was not retained as the selected session across the resort: (%+v, %v)", session, ok)
	}
	if !m.entryRefusal.active {
		t.Fatalf("same selected session's still-applicable refusal was incorrectly cleared by a resort that kept it selected")
	}
	if _, ok := m.activeEntryRefusalForSelection(); !ok {
		t.Fatalf("alpha's banner is not showing after a resort that kept alpha selected")
	}
}

// TestRefusalOnRemovedArchivedSelectionDoesNotSurvive is cure-01-01-3's own
// regression for the archived-list refresh half of the same lifetime rule:
// an archivedSessionsLoaded reload that drops the selected session out of
// the (archive-inclusive) filtered list must clear its refusal permanently,
// exactly like an ordinary sessionsLoaded removal above.
func TestRefusalOnRemovedArchivedSelectionDoesNotSurvive(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	m.width, m.height = 100, 30
	m.tmuxClient = tmux.Client{Socket: "dummy"}
	// alpha is reachable ONLY through the archived pool (never in
	// baseSessions), and a live filter query is what makes filteredSessions
	// (filter.go) search that pool at all -- so an archivedSessionsLoaded
	// reload that drops alpha out of msg.sessions is what actually removes
	// it from m.sessions here, exactly the archive-side mirror of an
	// ordinary sessionsLoaded removal.
	alpha := store.Session{ID: "alpha", Name: "alpha", Slug: "alpha", Status: "stopped"}
	m.baseSessions = nil
	m.filterQuery = "alpha"
	m.archivedSessions = []store.Session{alpha}
	m.sessions = m.filteredSessions()
	if len(m.sessions) != 1 || m.sessions[0].ID != "alpha" {
		t.Fatalf("fixture: filteredSessions did not surface archived alpha: %+v", m.sessions)
	}
	m.selected = rowCursor(0)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if !m.entryRefusal.active {
		t.Fatalf("fixture: Enter did not refuse alpha")
	}

	next, _ = m.Update(archivedSessionsLoaded{sessions: nil})
	m = next.(Model)
	if _, ok := m.selectedSession(); ok {
		t.Fatalf("fixture: alpha is still selected after the archived-list refresh dropped it")
	}
	if m.entryRefusal.active {
		t.Fatalf("refusal survived an archived-list refresh that dropped the selected session: %+v", m.entryRefusal)
	}

	next, _ = m.Update(archivedSessionsLoaded{sessions: []store.Session{alpha}})
	m = next.(Model)
	if m.entryRefusal.active {
		t.Fatalf("alpha's old refusal reappeared after an archived-list refresh restored it, with no new entry attempt")
	}
}
