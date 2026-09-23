// interactive_forward_gate_test.go is R135's own red-first proof (PRD
// Part II §51/SPEC.md §11.9, GH #30): updateInteractive
// (internal/tui/interactive.go) used to zero m.interactiveScrollOffset for
// EVERY key that reached it past the Ctrl+Q/nil-dispatcher checks, before
// ever asking interactiveNamedKey/interactiveLiteralPayload whether that
// key forwards any bytes at all. A key neither helper recognises -- Alt+
// Insert is the one interactiveAltNamedKeys documents as a listed, permanent
// gap (see interactive.go's own comment on that map) -- writes nothing to
// the target pane, so snapping a scrolled-back view to the live bottom for
// it is exactly the surprise PRD II-51 rules out: the screen jumps even
// though the user's keystroke never reached anywhere they could see.
//
// Both tests below drive a real *interactive.Session and a real
// *tmux.Dispatcher through enterInteractive, the same
// newQuietSelectionPane/newShellPaneWithHistory + enterInteractive
// machinery interactive_scroll_heal_test.go already uses in this package,
// so the dispatcher-nil guard is genuinely false and the fix's own
// ordering (dispatcher-live, then a helper's own yes/no) is what is
// actually exercised -- a hand-built Model with no dispatcher would take
// the early nil-guard return and prove nothing about ordering at all.
package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// forwardGateTestModel builds a real, entered interactive Model backed by
// a quiet (silent) tmux pane -- enough for a live, non-nil
// interactiveDispatcher without racing any pane output -- and scrolls it
// back by scrollBy lines before returning. slug is deck's own session
// slug (tmux.SessionName prefixes it with "deck_" to get the real tmux
// session name), so the tmux session created below must carry that
// prefix ITSELF rather than have it added twice.
func forwardGateTestModel(t *testing.T, socket, slug string, scrollBy int) Model {
	t.Helper()
	m := New(nil, config.Settings{Color: true}, "")
	m.width, m.height = 100, 30

	newQuietSelectionPane(t, socket, "deck_"+slug, 80, 24)

	m.tmuxClient = tmux.Client{Socket: socket}
	m.sessions = []store.Session{{ID: "sess-" + slug, Name: slug, Slug: slug, Status: "waiting"}}
	m.selected = rowCursor(0)

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

	got.setInteractiveScrollOffset(scrollBy)
	return got
}

// TestUpdateInteractiveLeavesTheScrollOffsetUntouchedForAKeyNeitherHelperForwards
// is this task's own required negative case: Alt+Insert satisfies neither
// interactiveNamedKey (interactiveAltNamedKeys["Insert"] == "", the listed
// gap) nor interactiveLiteralPayload (KeyInsert's KeyType is negative, so
// it never reaches that function's fixed-byte switch), so updateInteractive
// forwards nothing at all -- and before this fix, the same call still
// zeroed the scroll offset regardless, because the reset ran unconditionally
// ahead of either helper's own verdict.
func TestUpdateInteractiveLeavesTheScrollOffsetUntouchedForAKeyNeitherHelperForwards(t *testing.T) {
	socket := selectionTestSocket("forwardgate-neither")
	m := forwardGateTestModel(t, socket, "forwardgate_neither", 5)

	if _, ok := interactiveNamedKey(tea.KeyMsg(tea.Key{Type: tea.KeyInsert, Alt: true})); ok {
		t.Fatalf("test assumption violated: interactiveNamedKey now forwards Alt+Insert -- pick a different key this test still proves matches neither helper")
	}
	if _, ok := interactiveLiteralPayload(tea.KeyMsg(tea.Key{Type: tea.KeyInsert, Alt: true})); ok {
		t.Fatalf("test assumption violated: interactiveLiteralPayload now forwards Alt+Insert -- pick a different key this test still proves matches neither helper")
	}

	next, cmd := m.updateInteractive(tea.KeyMsg(tea.Key{Type: tea.KeyInsert, Alt: true}))
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("updateInteractive returned a non-Model tea.Model")
	}
	if cmd != nil {
		t.Fatalf("updateInteractive(Alt+Insert) returned a non-nil cmd, want nil (nothing was forwarded)")
	}
	if got.interactiveScrollOffset() != 5 {
		t.Fatalf("updateInteractive(Alt+Insert) left the scroll offset at %d, want it UNCHANGED at 5 -- Alt+Insert forwards no bytes (the listed interactiveAltNamedKeys gap), so a scrolled-back view must not snap to the live bottom for it", got.interactiveScrollOffset())
	}
}

// TestUpdateInteractiveResetsTheScrollOffsetForAForwardedKey is the mirror
// positive case the criteria also names: an ordinary rune key DOES satisfy
// interactiveLiteralPayload, so it is genuinely forwarded -- and the reset
// to 0 must still happen for it, proving the reordering did not simply stop
// resetting the offset altogether.
func TestUpdateInteractiveResetsTheScrollOffsetForAForwardedKey(t *testing.T) {
	socket := selectionTestSocket("forwardgate-forwards")
	m := forwardGateTestModel(t, socket, "forwardgate_forwards", 5)

	if _, ok := interactiveLiteralPayload(tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune("x")})); !ok {
		t.Fatalf("test assumption violated: interactiveLiteralPayload no longer forwards a plain rune key")
	}

	next, cmd := m.updateInteractive(tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune("x")}))
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("updateInteractive returned a non-Model tea.Model")
	}
	if cmd != nil {
		t.Fatalf("updateInteractive(\"x\") returned a non-nil cmd, want nil")
	}
	if got.interactiveScrollOffset() != 0 {
		t.Fatalf("updateInteractive(\"x\") left the scroll offset at %d, want 0 -- a key that really forwards bytes must still snap a scrolled-back view back to the live bottom (PRD II-51)", got.interactiveScrollOffset())
	}
}
