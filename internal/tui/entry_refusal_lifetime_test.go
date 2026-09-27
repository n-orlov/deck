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

// TestIndependentCaptureFromBeforeRefusalCannotDismissIt is cure-01-02-3's
// (R143/R148, SPEC §11.9) own real-private-tmux Model.Update regression:
// clearEntryRefusalIfPreviewLive used to accept ANY successful same-session
// previewCaptured, even one whose underlying tmux capture-pane call was
// issued BEFORE the refusal it would go on to clear ever existed --
// exactly the race a slow reply from a tick issued while the pane was
// still alive can produce once the pane dies and Enter is refused before
// that old reply lands. This drives a genuine capturePreview call while
// the pane is alive, HOLDS its real result, deadens the SAME real pane,
// obtains a fresh no-live-pane refusal via a real Enter through
// Model.Update, and only THEN delivers the held stale result: the banner
// must remain, because a real PreviewPane probe issued now would still
// report live=false. A genuinely later tick's own fresh capture (issued
// AFTER the refusal exists, once the pane is respawned alive) still
// clears it -- the fix must not also break the ordinary recovery path
// TestReviewNoLivePaneRefusalClearsWhenTickFindsRespawnedPane above
// already covers.
func TestIndependentCaptureFromBeforeRefusalCannotDismissIt(t *testing.T) {
	socket := selectionTestSocket("norefusal2")
	target, err := tmux.SessionName("norefusal2")
	if err != nil {
		t.Fatalf("SessionName: %v", err)
	}
	newDyingSelectionPane(t, socket, target, 80, 24)

	client := tmux.Client{Socket: socket}
	waitForPreviewPaneLiveness(t, client, "norefusal2", true)

	m := New(nil, config.Settings{}, "")
	m.width, m.height = 100, 30
	if _, height := m.previewContentSize(); height < interactiveMinInnerRows {
		t.Fatalf("test assumption violated: preview content height %d is below the %d-row floor", height, interactiveMinInnerRows)
	}
	m.tmuxClient = client
	m.previewCapture = func(ctx context.Context, slug string) (tmux.PreviewCapture, error) {
		return client.CapturePreview(ctx, slug)
	}
	m.sessions = []store.Session{{ID: "sess-nolive-2", Name: "norefusal2", Slug: "norefusal2", Status: "waiting"}}
	m.baseSessions = m.sessions
	m.selected = rowCursor(0)

	// A genuine capture while the pane is still alive -- this is the reply
	// that will be held and delivered LATE, after the refusal it predates.
	cmd := m.capturePreview()
	if cmd == nil {
		t.Fatalf("fixture: capturePreview returned nil while the pane is alive and selected")
	}
	staleMsg, ok := cmd().(previewCaptured)
	if !ok || staleMsg.err != nil || !staleMsg.capture.Live || staleMsg.sessionID != "sess-nolive-2" {
		t.Fatalf("fixture: capture while alive = %+v, want a live success for sess-nolive-2", staleMsg)
	}

	deadenSelectionPane(t, socket, target)
	waitForPreviewPaneLiveness(t, client, "norefusal2", false)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if !m.entryRefusal.active || m.entryRefusal.kind != entryRefusalNoLivePane || m.entryRefusal.sessionID != "sess-nolive-2" {
		t.Fatalf("↵ against the now-dead pane = %+v, want a fresh no-live-pane refusal for sess-nolive-2", m.entryRefusal)
	}
	if staleMsg.refusalGeneration >= m.entryRefusal.generation {
		t.Fatalf("fixture: the pre-refusal capture's own generation (%d) is not strictly less than the fresh refusal's (%d) -- the race this test exists to prove is not actually set up", staleMsg.refusalGeneration, m.entryRefusal.generation)
	}

	// Deliver the STALE, pre-refusal result. It must not dismiss a banner
	// it never observed the reason for -- a real probe against the pane
	// right now would still say live=false.
	next, _ = m.Update(staleMsg)
	m = next.(Model)
	if !m.entryRefusal.active {
		t.Fatalf("a capture issued BEFORE the current refusal cleared it on late delivery: %+v, want the still-applicable banner to remain", m.entryRefusal)
	}
	if live, ok, perr := client.PreviewPane(context.Background(), "norefusal2"); ok || perr != nil {
		t.Fatalf("sanity: the real pane is not actually dead right now (live=%v ok=%v err=%v) -- the stale-result race this test proves would be vacuous", live, ok, perr)
	}
	if _, ok := m.activeEntryRefusalForSelection(); !ok {
		t.Fatalf("banner is not showing after the stale capture was correctly refused")
	}

	// A genuinely LATER tick -- issued after the refusal exists, once the
	// pane is actually respawned alive -- must still clear it.
	respawnAlivePane(t, socket, target)
	waitForPreviewPaneLiveness(t, client, "norefusal2", true)

	m = runOnePreviewTick(t, m)
	if m.entryRefusal.active {
		t.Fatalf("a genuinely later tick's own live capture did not clear the refusal: %+v", m.entryRefusal)
	}
	if _, ok := m.activeEntryRefusalForSelection(); ok {
		t.Fatalf("banner still showing after a genuinely later tick observed the respawned pane live")
	}
}

// TestIndependentRefusalSupersedesStaleCaptureForSameSession is
// cure-01-02-3's non-tmux unit-level companion: a capture whose
// refusalGeneration names an EARLIER refusal for the exact same session
// (the refusal was cleared and a fresh one set again for that same
// session, rather than a brand-new session entirely) must not clear the
// newer refusal either -- "results for a superseded refusal cannot clear
// a newer refusal" is a generation check, not merely a session-identity
// check, and TestReviewNoLivePaneRefusalIgnoresAnotherSessionsCapture
// above only ever proves the session-identity half.
func TestIndependentRefusalSupersedesStaleCaptureForSameSession(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	m.sessions = []store.Session{{ID: "sess-a", Name: "a", Slug: "a", Status: "running"}}
	m.selected = rowCursor(0)

	m.setEntryRefusal("sess-a", entryRefusalNoLivePane, "no live pane")
	staleGeneration := m.entryRefusal.generation

	// The refusal clears (e.g. the tick's own successful capture at the
	// time), and a FRESH refusal for the SAME session is set again later.
	m.clearEntryRefusal()
	m.setEntryRefusal("sess-a", entryRefusalNoLivePane, "no live pane")
	if m.entryRefusal.generation <= staleGeneration {
		t.Fatalf("fixture: re-set refusal's generation (%d) is not strictly greater than the stale one's (%d)", m.entryRefusal.generation, staleGeneration)
	}

	next, _ := m.Update(previewCaptured{sessionID: "sess-a", capture: tmux.PreviewCapture{Live: true}, refusalGeneration: staleGeneration})
	got := next.(Model)
	if !got.entryRefusal.active {
		t.Fatalf("a capture naming the OLD, superseded refusal's generation cleared the newer refusal for the same session: %+v", got.entryRefusal)
	}

	next, _ = got.Update(previewCaptured{sessionID: "sess-a", capture: tmux.PreviewCapture{Live: true}, refusalGeneration: got.entryRefusal.generation})
	got2 := next.(Model)
	if got2.entryRefusal.active {
		t.Fatalf("a capture naming the CURRENT refusal's own generation did not clear it: %+v", got2.entryRefusal)
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

	next, _ := m.Update(previewCaptured{sessionID: "sess-b", capture: tmux.PreviewCapture{Live: true}, refusalGeneration: m.entryRefusal.generation})
	got := next.(Model)
	if !got.entryRefusal.active {
		t.Fatalf("entryRefusal cleared by a live capture naming a DIFFERENT session (sess-b), want sess-a's refusal untouched")
	}

	next2, _ := got.Update(previewCaptured{sessionID: "sess-a", capture: tmux.PreviewCapture{Live: true}, refusalGeneration: got.entryRefusal.generation})
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
