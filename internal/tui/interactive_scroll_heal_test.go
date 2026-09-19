// interactive_scroll_heal_test.go is R133 part 1's own red-first proof
// (PRD phase4b, GH #30): interactiveBodyLines (interactive.go:532 at
// launch sha c3b530a) used to discard RenderRows' own second return --
// the offset it actually used, after clamping the caller's requested
// offset against the grid's REAL scrollback length -- so
// m.interactiveScrollOffset could sit far past that length while the
// view was really pinned at the top of scrollback.
//
// Both tests below drive a real *interactive.Session (via a real tmux
// server, the same newShellPaneWithHistory/newQuietSelectionPane +
// enterInteractive machinery interactive_entry_history_test.go and
// interactive_notice_background_test.go already use elsewhere in this
// package) rather than a hand-built fixture, because the property under
// test is specifically about the REAL clamp RenderRows applies against
// the grid's own scrollback length -- a bound nothing in internal/tui
// itself can compute or fake convincingly.
package tui

import (
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/interactive"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// TestInteractiveBodyLinesHealsTheStoredOffsetToTheClampedUsedOffset
// scrolls a model PAST the real scrollback length -- interactive.
// ScrollbackMaxLines (2000) is set directly on m.interactiveScrollOffset,
// far beyond the ~390 real lines the fixture pane below actually holds
// -- and asserts the stored offset ends up equal to the CLAMPED offset
// RenderRows really used (the live grid's own ScrollbackLen(), read
// straight off *interactive.Session.Grid(), never a value this test
// invents or recomputes independently).
//
// Before R133 part 1 this fails: the old `lines, _ :=
// m.interactiveGrid.RenderRows(...)` left m.interactiveScrollOffset at
// interactive.ScrollbackMaxLines untouched, so the stored value would
// stay far past the real scrollback length no matter what RenderRows
// itself actually used underneath.
func TestInteractiveBodyLinesHealsTheStoredOffsetToTheClampedUsedOffset(t *testing.T) {
	m := New(nil, config.Settings{Color: true}, "")
	m.width, m.height = 100, 30
	if _, height := m.previewContentSize(); height < interactiveMinInnerRows {
		t.Fatalf("test assumption violated: preview content height %d is below the %d-row floor", height, interactiveMinInnerRows)
	}

	// A pane far too short to hold everything it prints, so most of it is
	// real tmux scrollback -- the exact fixture
	// TestEnterInteractiveSeedsTheGridWithThePanesOwnTmuxHistory already
	// established as this package's own way to get a real, deterministic
	// (via the #{history_size} wait loop) scrollback length without
	// racing a live pane's own output.
	const printed = 400
	socket := selectionTestSocket("scrollheal")
	newShellPaneWithHistory(t, socket, "deck_scrollheal", 80, 10, printed)

	m.tmuxClient = tmux.Client{Socket: socket}
	m.sessions = []store.Session{{ID: "sess-scrollheal-1", Name: "scrollheal", Slug: "scrollheal", Status: "waiting"}}
	m.selected = 0

	next, _ := m.enterInteractive()
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("enterInteractive returned a non-Model tea.Model")
	}
	if got.attachError != "" {
		t.Fatalf("enterInteractive refused: %q", got.attachError)
	}
	if !got.interactive || got.interactiveGrid == nil {
		t.Fatalf("enterInteractive did not enter interactive mode with a live grid")
	}
	defer got.exitInteractive()

	realScrollbackLen := got.interactiveGrid.Grid().ScrollbackLen()
	if realScrollbackLen <= 0 {
		t.Fatalf("the fixture pane's grid holds %d scrollback lines, want > 0 -- this test needs a real, positive scrollback length to clamp against", realScrollbackLen)
	}
	if realScrollbackLen >= interactive.ScrollbackMaxLines {
		t.Fatalf("the fixture pane's grid holds %d scrollback lines, which is not below interactive.ScrollbackMaxLines (%d) -- this test's whole premise (a stored offset set to the bound must land PAST the real length) would be vacuous", realScrollbackLen, interactive.ScrollbackMaxLines)
	}

	// Scroll past the real scrollback length: the bound
	// scrollInteractiveByLines itself clamps to, set directly so this
	// test exercises interactiveBodyLines' own healing regardless of
	// scrollInteractiveByLines' own clamp.
	got.interactiveScrollOffset = interactive.ScrollbackMaxLines

	contentWidth, contentHeight := got.previewContentSize()
	_, _ = got.interactiveBodyLines(contentWidth, contentHeight)

	if got.interactiveScrollOffset != realScrollbackLen {
		t.Fatalf("after interactiveBodyLines, m.interactiveScrollOffset = %d, want the clamped used offset %d (the grid's own real scrollback length) -- a stored offset stale-high past the real scrollback length must be healed back onto the model, not left at the value scrolled past it", got.interactiveScrollOffset, realScrollbackLen)
	}
}

// TestInteractiveBodyLinesHealingReenablesTheNotRepaintedNotice is the
// PRD's own named consequence of the fix above: the II-49 "has not
// repainted" notice (interactiveNotRepaintedNotice) is gated on
// `m.interactiveScrollOffset == 0`, so a stale-high stored offset used
// to suppress the notice even while the LIVE screen -- what offset 0
// really means once healed -- is genuinely blank.
//
// The fixture pane here (newQuietSelectionPane) runs only `sleep 600`:
// it never prints a single byte, so its grid has ZERO real scrollback
// and a wholly blank live screen for the life of the test -- the same
// fixture interactive_notice_background_test.go already uses to prove
// the notice fires at offset 0. Setting m.interactiveScrollOffset to
// interactive.ScrollbackMaxLines before calling interactiveBodyLines
// reproduces exactly the bug this task fixes: without the heal, offset
// stays non-zero and the notice's own `offset == 0` gate stays false
// even though the grid is blank; with the heal, RenderRows clamps the
// request down to the real scrollback length (0 here), the stored field
// is healed back to 0, and the notice reappears.
func TestInteractiveBodyLinesHealingReenablesTheNotRepaintedNotice(t *testing.T) {
	m := New(nil, config.Settings{Color: true}, "")
	m.width, m.height = 100, 30
	if _, height := m.previewContentSize(); height < interactiveMinInnerRows {
		t.Fatalf("test assumption violated: preview content height %d is below the %d-row floor", height, interactiveMinInnerRows)
	}

	socket := selectionTestSocket("noticeheal")
	newQuietSelectionPane(t, socket, "deck_noticeheal", 80, 24)

	m.tmuxClient = tmux.Client{Socket: socket}
	m.sessions = []store.Session{{ID: "sess-noticeheal-1", Name: "noticeheal", Slug: "noticeheal", Status: "waiting"}}
	m.selected = 0

	next, _ := m.enterInteractive()
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("enterInteractive returned a non-Model tea.Model")
	}
	if got.attachError != "" {
		t.Fatalf("enterInteractive refused: %q", got.attachError)
	}
	if !got.interactive || got.interactiveGrid == nil {
		t.Fatalf("enterInteractive did not enter interactive mode with a live grid")
	}
	defer got.exitInteractive()

	if realScrollbackLen := got.interactiveGrid.Grid().ScrollbackLen(); realScrollbackLen != 0 {
		t.Fatalf("the quiet fixture pane's grid holds %d scrollback lines, want exactly 0 -- it must never print anything for this test's premise (healing lands back at offset 0) to hold", realScrollbackLen)
	}

	// A stale-high stored offset, exactly like the scenario above: past
	// the real (zero) scrollback length.
	got.interactiveScrollOffset = interactive.ScrollbackMaxLines

	contentWidth, contentHeight := got.previewContentSize()
	lines, _ := got.interactiveBodyLines(contentWidth, contentHeight)

	if got.interactiveScrollOffset != 0 {
		t.Fatalf("after interactiveBodyLines, m.interactiveScrollOffset = %d, want 0 (the clamped used offset against a zero-length real scrollback)", got.interactiveScrollOffset)
	}
	if len(lines) == 0 || lines[0] != interactiveNotRepaintedNotice {
		t.Fatalf("interactiveBodyLines' line 0 = %q, want the not-repainted notice %q now that the stale-high stored offset has been healed back to 0 against a genuinely blank live screen", firstOrEmpty(lines), interactiveNotRepaintedNotice)
	}
}

// firstOrEmpty is a small failure-message helper: lines[0] would itself
// panic on an empty slice inside a Fatalf's own argument evaluation.
func firstOrEmpty(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return lines[0]
}
