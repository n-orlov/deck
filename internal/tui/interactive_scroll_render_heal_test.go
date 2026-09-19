// interactive_scroll_render_heal_test.go is cure-01-01-2's own red-first
// proof (PRD phase4b, GH #30, review finding filed at
// artifacts/review2/tui-reviewer-prd-test.go's
// TestReviewerScrollHealAfterVisibleOnlyReseed): cure-01-01
// (interactive_scroll_persist_test.go) healed the stored offset back
// onto the model bubbletea keeps between Update calls, but ONLY from
// inside scrollInteractiveByLines/Page themselves. Anything ELSE that
// changes the grid's real scrollback length -- a resize that reseeds it
// with a shorter or empty real history, or (interactive_transport =
// capture) a background poll that replaces the grid wholesale with a
// visible-only re-seed -- left the stored offset stale-high until the
// next scroll command happened to heal it, which could be arbitrarily
// long after (or never, if the user's next input forwards bytes
// instead).
//
// The review's own reproducer asserted m.interactiveScrollOffset == 0
// directly after calling m.View() on the very same, already-addressable
// model -- something no value-receiver method can ever do, in any Go
// program, since View() (and everything in its call chain: mainView,
// renderStackedFrame/renderSideBySideFrame, previewBodyLines,
// interactiveBodyLines's own pointer-receiver heal included) only ever
// mutates ITS OWN copy of the receiver, discarded the instant the call
// returns; interactive_footer_scroll_cue_test.go's own doc comment
// already documents this for the footer text specifically. Proving the
// FIX (rather than re-demonstrating that same impossibility) means
// observing the healed value the one way Go actually allows a method to
// hand state back to its caller: through the MODEL A CALL RETURNS, the
// same pattern every other test in this package already uses
// (`next, _ := m.Something(); m = next.(Model)`).
//
// The fix itself (interactive_scroll.go's
// healInteractiveScrollOffsetFromRender, called both from
// scrollInteractiveByLines and from the top of Update in tui.go) runs
// the heal on EVERY Update call while m.interactive is true, not only
// inside the two scroll helpers -- which is exactly what this test
// drives with a deliberately unrelated, unrecognised message type
// instead of another scroll command.
package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/interactive"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// interactiveScrollHealNoOpMsg stands in for "whatever the next tea.Msg
// happens to be": Update's own catch-all falls through to `return m, nil`
// for any message type none of its cases recognise (the same path a
// stray/unknown tea.Msg would take in production), so healing on this
// message type specifically proves the heal fires from Update's own top,
// not from some case arm that happens to already touch the offset for an
// unrelated reason.
type interactiveScrollHealNoOpMsg struct{}

func TestInteractiveScrollOffsetHealsAfterVisibleOnlyReseedOnTheNextUpdate(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	m.width, m.height = 100, 30
	socket := selectionTestSocket("scrollheal-reseed")
	newShellPaneWithHistory(t, socket, "deck_scrollheal-reseed", 80, 10, 100)
	m.tmuxClient = tmux.Client{Socket: socket}
	m.sessions = []store.Session{{ID: "reseed-1", Name: "scrollheal-reseed", Slug: "scrollheal-reseed", Status: "waiting"}}

	next, _ := m.enterInteractive()
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("enterInteractive returned a non-Model tea.Model")
	}
	m = got
	if !m.interactive || m.interactiveGrid == nil {
		t.Fatalf("entry refused: %s", m.attachError)
	}
	defer m.exitInteractive()

	// Scroll into history first -- PgUp/PgDn/wheel already heal
	// themselves (cure-01-01), so the gap this task closes needs a
	// stored position that genuinely reflects real history before the
	// reseed below wipes it out from under that position.
	next, _ = m.scrollInteractiveByLines(interactive.ScrollbackMaxLines)
	m = next.(Model)
	if m.interactiveScrollOffset < 2 {
		t.Fatalf("fixture pane has no real history to scroll into: stored=%d", m.interactiveScrollOffset)
	}

	// A history-changing resize: the same fresh, visible-only grid
	// replacement a real resize or an interactive_transport = capture
	// background reseed installs -- real history drops to zero, wiping
	// out the very history the stored offset above still points at.
	w, h := m.previewContentSize()
	if err := m.interactiveGrid.Resize(context.Background(), w, h, func(context.Context) ([]byte, error) {
		return []byte("\x1b[2J\x1b[H"), nil
	}); err != nil {
		t.Fatalf("resize: %v", err)
	}
	if real := m.interactiveGrid.Grid().ScrollbackLen(); real != 0 {
		t.Fatalf("test assumption violated: resize left %d lines of real scrollback, want 0", real)
	}

	// Render: interactiveBodyLines' own heal (R133 part 1) makes THIS
	// call's footer correct already, on its own value-receiver copy,
	// regardless of what the persisted model still says.
	if footer := footerLineOf(m.View()); strings.Contains(footer, "not live") {
		t.Fatalf("footer still claims scrolled-back/not-live against a freshly reseeded, empty grid: %q", footer)
	}

	// The NEXT input event -- deliberately an unrelated, unrecognised
	// message, never another scroll command -- must start from the
	// healed position: the model Update hands back for it must already
	// carry interactiveScrollOffset == 0, the real (empty) scrollback
	// length RenderRows now clamps to, not the pre-reseed stored value.
	next, _ = m.Update(interactiveScrollHealNoOpMsg{})
	m = next.(Model)
	if m.interactiveScrollOffset != 0 {
		t.Fatalf("stored offset after the next Update call = %d, want 0 (healed against the reseeded grid's real, empty scrollback) -- the heal must not depend on a scroll command", m.interactiveScrollOffset)
	}

	// Subsequent ordinary output creates NEW, short history; one line
	// back from the healed position must land on offset 1 -- never the
	// stale pre-reseed offset, and never the grid's new maximum either.
	if _, err := m.interactiveGrid.Grid().Write([]byte(strings.Repeat("new output after reseed\r\n", h+8))); err != nil {
		t.Fatalf("write: %v", err)
	}
	next, _ = m.scrollInteractiveByLines(1)
	m = next.(Model)
	if m.interactiveScrollOffset != 1 {
		t.Fatalf("one line back after the reseed = %d, want 1; footer=%q", m.interactiveScrollOffset, footerLineOf(m.View()))
	}
}

// TestInteractiveScrollHealAtUpdateTopLeavesNonInteractiveModelsUntouched
// guards the new, broadened call site (the top of Update in tui.go)
// against the exact over-broad heal
// TestInteractiveDispatcherNilDoesNotSnapStoredOffset already guards
// scrollInteractiveByLines/Page against: healInteractiveScrollOffsetFromRender
// must stay a no-op whenever m.interactive is false, or m.interactive is
// true with no live grid installed at all -- never a blanket zeroing of
// whatever m.interactiveScrollOffset already holds.
func TestInteractiveScrollHealAtUpdateTopLeavesNonInteractiveModelsUntouched(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	m.interactiveScrollOffset = 42

	next, _ := m.Update(interactiveScrollHealNoOpMsg{})
	if got := next.(Model).interactiveScrollOffset; got != 42 {
		t.Fatalf("non-interactive model: Update healed interactiveScrollOffset to %d, want it untouched at 42", got)
	}

	m.interactive = true
	next, _ = m.Update(interactiveScrollHealNoOpMsg{})
	if got := next.(Model).interactiveScrollOffset; got != 42 {
		t.Fatalf("interactive model with no live grid: Update healed interactiveScrollOffset to %d, want it untouched at 42", got)
	}
}
