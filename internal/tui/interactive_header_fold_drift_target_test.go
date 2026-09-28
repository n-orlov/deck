// interactive_header_fold_drift_target_test.go is cure-01-06's own
// regression pair (R149/R157, SPEC §11): a genuine sidebar-wheel drift
// that started AFTER a header fold had already moved m.selected off the
// bound interactive target -- onto that group's own header, and hid the
// target's row outright -- reveals the TARGET's row when the drift ends,
// not merely wherever the fold had left m.selected. Before
// revealInteractiveDriftTarget (interactive.go), followSelectionViewport
// followed m.selected as-is: with the target's group still collapsed,
// that revealed nothing belonging to the target at all (a collapsed
// group hides every one of its rows, isSessionVisible), and even once
// the fold had happened, the header, not the target row, is what would
// have come into view.
//
// Both fixtures below use a REAL tmux pane behind the target session
// (driftEndDispatchFixtureModel's own discipline, this file's own
// headerFoldDriftDispatchFixtureModel) so the input-forwarding case
// proves the exact byte reached the pane, not merely that the model's
// own bookkeeping looks right.
package tui

import (
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// headerFoldDriftDispatchFixtureModel builds a real, entered interactive
// Model whose target session is the SOLE member of its own small group
// ("aaa-target", sorting first), beside a second, much longer group
// ("zzz-filler") -- long enough that a real sidebar-wheel notch can drift
// the viewport well away from the top, where both the target's row and
// its own header live. Only the target session has a real tmux session
// behind it.
func headerFoldDriftDispatchFixtureModel(t *testing.T, socket, slug string) (Model, int64) {
	t.Helper()
	targetGroupID := int64(101)
	fillerGroupID := int64(102)

	m := New(nil, config.Settings{Mouse: true, Color: true}, "")
	sessions := []store.Session{
		{ID: "target-" + slug, Name: slug, Slug: slug, CWD: "/work", Status: "waiting", GroupName: "aaa-target", GroupID: &targetGroupID},
	}
	for i := 0; i < 30; i++ {
		id := fmt.Sprintf("filler%02d", i)
		sessions = append(sessions, store.Session{ID: id, Name: id, Slug: id, CWD: "/work/infra", Status: "idle", GroupName: "zzz-filler", GroupID: &fillerGroupID})
	}
	m.sessions = sessions
	m.baseSessions = append([]store.Session(nil), sessions...)
	m.width, m.height = 100, 30
	m.selected = rowCursor(0)

	newQuietSelectionPane(t, socket, "deck_"+slug, 80, 24)
	m.tmuxClient = tmux.Client{Socket: socket}

	next, _ := m.enterInteractive()
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("enterInteractive returned a non-Model tea.Model")
	}
	if got.attachError != "" {
		t.Fatalf("enterInteractive refused: %q", got.attachError)
	}
	if !got.interactive || got.interactiveGrid == nil || got.interactiveDispatcher == nil {
		t.Fatalf("enterInteractive did not enter interactive mode with a live grid and dispatcher (grid=%v dispatcher=%v)", got.interactiveGrid, got.interactiveDispatcher)
	}
	t.Cleanup(func() { got.exitInteractive() })
	return got, targetGroupID
}

// foldTargetHeaderThenDrift folds the target's own group by the SAME
// mouse press mouse_interactive_header_press_test.go already proves
// reaches resolveSidebarPress while interactive (a left press on its
// header), asserts the fold moved m.selected onto that header and hid
// the target row, then arms a genuine sidebar-wheel drift all the way to
// the bottom of the (now header-plus-filler) list, off both the header
// and the target row.
func foldTargetHeaderThenDrift(t *testing.T, m Model, targetGroupID int64) Model {
	t.Helper()
	hx, hy := findHeader(t, m, "aaa-target")
	if hit := m.hitTest(hx, hy); hit.panel != hitPanelSidebar || hit.target != hitTargetHeader || hit.groupID != targetGroupID {
		t.Fatalf("test setup: hitTest(%d,%d) = %+v, want sidebar/header groupID=%d", hx, hy, hit, targetGroupID)
	}
	next, _ := m.Update(press(hx, hy))
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update(header press) returned a non-Model tea.Model")
	}
	if !got.isGroupCollapsed(targetGroupID) {
		t.Fatalf("test setup: header press did not collapse the target's own group")
	}
	if _, ok := got.selected.GroupID(); !ok {
		t.Fatalf("test setup: header fold left m.selected = %+v, want it re-targeted onto the folded group's own header", got.selected)
	}
	if got.isSessionVisible(0) {
		t.Fatalf("test setup: target row (session 0) still reports visible after folding its own group")
	}

	got = armSidebarWheelDriftPastSelection(t, got)
	return got
}

// assertTargetRowRevealed is this file's own positive assertion: the
// target's group is expanded again, its row (session index 0) is
// actually inside the current sidebar viewport window, and m.selected
// names that same row -- not the header the fold left behind.
func assertTargetRowRevealed(t *testing.T, m Model, targetGroupID int64, label string) {
	t.Helper()
	if m.isGroupCollapsed(targetGroupID) {
		t.Fatalf("%s: target's own group is still collapsed -- its row cannot be on screen", label)
	}
	idx, ok := m.selected.SessionIndex()
	if !ok || idx != 0 {
		t.Fatalf("%s: m.selected = %+v, want the target row (session index 0)", label, m.selected)
	}
	assertSelectionInView(t, m, label)
}

// TestReviewForwardedInputRevealsTargetAfterHeaderFoldAndDrift is the
// review's own forwarded-input case: a genuine sidebar-wheel drift armed
// AFTER a header fold hid the bound interactive target's row sends "x"
// while still drifted -- the byte still reaches the real pane, exactly
// once, and the target's row (not wherever the fold left m.selected) is
// what comes back into view.
func TestReviewForwardedInputRevealsTargetAfterHeaderFoldAndDrift(t *testing.T) {
	socket := selectionTestSocket("headerdrift-input")
	slug := "headerdrift_input"
	m, targetGroupID := headerFoldDriftDispatchFixtureModel(t, socket, slug)
	m = foldTargetHeaderThenDrift(t, m, targetGroupID)
	target := "deck_" + slug
	verificationsBefore := m.interactiveDispatcher.Verifications()

	next, cmd := m.updateInteractive(tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune("x")}))
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("updateInteractive returned a non-Model tea.Model")
	}
	if cmd != nil {
		t.Fatalf(`updateInteractive("x") returned a non-nil cmd, want nil`)
	}
	if got.sidebarScrollDrifted {
		t.Fatalf(`updateInteractive("x") left m.sidebarScrollDrifted true -- the drift did not end on the forwarded press`)
	}
	assertTargetRowRevealed(t, got, targetGroupID, `updateInteractive("x")`)

	if v := got.interactiveDispatcher.Verifications(); v != verificationsBefore+1 {
		t.Fatalf("dispatcher Verifications() = %d, want %d (exactly one Send on this press)", v, verificationsBefore+1)
	}
	waitForPaneCaptureLine0(t, socket, target, "x")
}

// TestReviewCtrlQRevealsTargetAfterHeaderFoldAndDrift is the review's own
// Ctrl+Q case: the same fold-then-drift setup, ended by a real
// tea.KeyCtrlQ instead of a forwarded key. Interactive mode must exit AND
// the target's row must be what the list view returns to, not the
// header the fold left behind.
func TestReviewCtrlQRevealsTargetAfterHeaderFoldAndDrift(t *testing.T) {
	socket := selectionTestSocket("headerdrift-ctrlq")
	slug := "headerdrift_ctrlq"
	m, targetGroupID := headerFoldDriftDispatchFixtureModel(t, socket, slug)
	m = foldTargetHeaderThenDrift(t, m, targetGroupID)

	next, _ := m.updateInteractive(key("ctrl+q"))
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("updateInteractive(ctrl+q) returned a non-Model tea.Model")
	}
	if got.interactive {
		t.Fatalf("updateInteractive(ctrl+q) did not leave interactive mode")
	}
	if got.sidebarScrollDrifted {
		t.Fatalf("Ctrl+Q left m.sidebarScrollDrifted true -- exitInteractive did not end the drift")
	}
	assertTargetRowRevealed(t, got, targetGroupID, "Ctrl+Q")
}
