// interactive_scroll_render_heal_test.go is cure-01-01-2's own regression
// for R133 (PRD phase4b, GH #30), filed against the review finding
// artifacts/review2/tui-reviewer-prd-test.go's
// TestReviewerScrollHealAfterVisibleOnlyReseed and kept in that
// reproducer's exact shape: a real interactive session, scrolled into
// history, whose history is then replaced by a visible-only seed, then
// RENDERED -- with nothing at all in between the render and the check.
//
// cure-01-01 (interactive_scroll_persist_test.go) healed the stored
// offset only from inside scrollInteractiveByLines/Page, and this file's
// first attempt healed it only from the top of Update; both left the gap
// the finding names, because nothing runs either one when a resize
// reseeds the grid with a shorter or empty real history, or when
// interactive_transport = capture's poll loop replaces the grid
// wholesale from a visible-only re-seed. The offset RENDERING actually
// used (RenderRows' own clamp against the grid's real scrollback length)
// was written onto whichever value-receiver copy of Model happened to be
// on the stack -- View's, mainView's, previewBodyLines',
// interactiveBodyLines' -- and died with it.
//
// The fix stores that position in ONE cell every copy of Model points at
// (interactiveScrollState in interactive_scroll.go, reached through
// m.interactiveScrollOffset()/m.setInteractiveScrollOffset()), so the
// clamped position a render used IS the stored position afterwards, with
// no Update needed in between, and scrollInteractiveByLines steps from
// that healed position rather than from the stale request.
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

func TestInteractiveScrollOffsetHealsFromTheRenderAfterVisibleOnlyReseed(t *testing.T) {
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
	scrolled := m.interactiveScrollOffset()
	if scrolled < 2 {
		t.Fatalf("fixture pane has no real history to scroll into: stored=%d", scrolled)
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

	// Render, and nothing else: no scroll command, no Update, no second
	// call of any kind between the render and the two checks below.
	if footer := footerLineOf(m.View()); strings.Contains(footer, "not live") {
		t.Fatalf("footer still claims scrolled-back/not-live against a freshly reseeded, empty grid: %q", footer)
	}
	if stored := m.interactiveScrollOffset(); stored != 0 {
		t.Fatalf("stored position after the render = %d (real scrollback is %d, pre-reseed stored was %d), want 0 -- the offset RENDERING used must BE the stored position, with no Update needed to heal it", stored, m.interactiveGrid.Grid().ScrollbackLen(), scrolled)
	}

	// Subsequent ordinary output creates NEW, short history; one line
	// back from the healed position must land on offset 1 -- never the
	// stale pre-reseed offset, and never the grid's new maximum either.
	if _, err := m.interactiveGrid.Grid().Write([]byte(strings.Repeat("new output after reseed\r\n", h+8))); err != nil {
		t.Fatalf("write: %v", err)
	}
	newReal := m.interactiveGrid.Grid().ScrollbackLen()
	if newReal < 2 {
		t.Fatalf("test assumption violated: the new output left %d lines of real scrollback, want at least 2 so offset 1 is distinguishable from the new maximum", newReal)
	}
	next, _ = m.scrollInteractiveByLines(1)
	m = next.(Model)
	if stored := m.interactiveScrollOffset(); stored != 1 {
		t.Fatalf("one line back after the reseed = %d, want 1 (never the pre-reseed %d, never the new maximum %d); footer=%q", stored, scrolled, newReal, footerLineOf(m.View()))
	}
}

// TestInteractiveScrollOffsetHealsOnTheNextUpdateWithNoRenderInBetween
// covers the OTHER order: the grid's real scrollback length changes and
// the next thing to happen is an ordinary message, with no render at all
// in between. Update's own top-of-function heal (tui.go) is what catches
// that one, so the position the message's own handling starts from is
// already clamped against the reseeded grid.
func TestInteractiveScrollOffsetHealsOnTheNextUpdateWithNoRenderInBetween(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	m.width, m.height = 100, 30
	socket := selectionTestSocket("scrollheal-update")
	newShellPaneWithHistory(t, socket, "deck_scrollheal-update", 80, 10, 100)
	m.tmuxClient = tmux.Client{Socket: socket}
	m.sessions = []store.Session{{ID: "reseed-2", Name: "scrollheal-update", Slug: "scrollheal-update", Status: "waiting"}}

	next, _ := m.enterInteractive()
	m = next.(Model)
	if !m.interactive || m.interactiveGrid == nil {
		t.Fatalf("entry refused: %s", m.attachError)
	}
	defer m.exitInteractive()

	next, _ = m.scrollInteractiveByLines(interactive.ScrollbackMaxLines)
	m = next.(Model)
	if m.interactiveScrollOffset() < 2 {
		t.Fatalf("fixture pane has no real history to scroll into: stored=%d", m.interactiveScrollOffset())
	}

	w, h := m.previewContentSize()
	if err := m.interactiveGrid.Resize(context.Background(), w, h, func(context.Context) ([]byte, error) {
		return []byte("\x1b[2J\x1b[H"), nil
	}); err != nil {
		t.Fatalf("resize: %v", err)
	}

	next, _ = m.Update(interactiveScrollHealNoOpMsg{})
	m = next.(Model)
	if stored := m.interactiveScrollOffset(); stored != 0 {
		t.Fatalf("stored position after the next Update call = %d, want 0 (healed against the reseeded grid's real, empty scrollback) -- the heal must not depend on a scroll command", stored)
	}
}

// TestInteractiveScrollHealAtUpdateTopLeavesNonInteractiveModelsUntouched
// guards the broadened call sites (the top of Update in tui.go, and the
// render's own store) against the exact over-broad heal
// TestInteractiveDispatcherNilDoesNotSnapStoredOffset already guards
// scrollInteractiveByLines/Page against: healInteractiveScrollOffsetFromRender
// must stay a no-op whenever m.interactive is false, or m.interactive is
// true with no live grid installed at all -- never a blanket zeroing of
// whatever the stored position already holds.
func TestInteractiveScrollHealAtUpdateTopLeavesNonInteractiveModelsUntouched(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	m.setInteractiveScrollOffset(42)

	next, _ := m.Update(interactiveScrollHealNoOpMsg{})
	if got := next.(Model).interactiveScrollOffset(); got != 42 {
		t.Fatalf("non-interactive model: Update healed the stored offset to %d, want it untouched at 42", got)
	}

	m.interactive = true
	next, _ = m.Update(interactiveScrollHealNoOpMsg{})
	if got := next.(Model).interactiveScrollOffset(); got != 42 {
		t.Fatalf("interactive model with no live grid: Update healed the stored offset to %d, want it untouched at 42", got)
	}
}
