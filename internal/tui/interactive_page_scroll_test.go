// interactive_page_scroll_test.go holds the unit-level half of PRD II-51's
// Shift+PgUp/PgDn contract: that one press moves the interactive grid's own
// scroll offset by a WHOLE PAGE -- the preview's content height -- and not by
// one line, and not by the wheel's few-line notch.
//
// This coverage was missing from the moment scrollInteractiveByPage was
// written. The only thing in the repo that ever asserted the granularity was
// the literal page counts in features/interactive_scroll.feature's
// "sends shift+pgup 3 times" steps, and issue #29 had to replace those with
// an until-found search (the entry seed now prepends the pane's own tmux
// history, so a fixed count no longer lands anywhere predictable). Measured
// at that point: degrading scrollInteractiveByPage to `height := 1` -- one
// line per keypress -- left the whole repo green, because the search just
// pressed Shift+PgUp 33-35 times instead of 2 and still found its marker.
// The feature file now also bounds the page count it needed
// (shiftPageScrollMaxPagesToMarker), but a real-tmux, real-pty scenario is
// the wrong and slowest place for the arithmetic to be pinned -- it can only
// bound the count loosely, and it costs twenty seconds to learn -- so the
// exact step size is pinned here as well, directly against the offset.
package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/interactive"
	"github.com/n-orlov/deck/internal/store"
)

// shiftPgUpRawCSI/shiftPgDownRawCSI are the bytes a real xterm-class
// terminal sends for Shift+PgUp/PgDn, named here exactly as
// features/interactive_scroll_test.go names them for the real pty it drives.
const (
	shiftPgUpRawCSI   = "\x1b[5;2~"
	shiftPgDownRawCSI = "\x1b[6;2~"
)

// unknownCSIStringerMsg stands in for the message those bytes actually arrive
// at Update as. Bubble Tea's decoder has no Key type for either sequence at
// all (shiftPgUpCSIString's own doc in interactive_scroll.go records why, and
// what it falls through to), and the type it does produce --
// unknownCSISequenceMsg -- is unexported in another module, so no test in
// this package can construct or name the real one. What it CAN do is
// reproduce the only handle the product has on it: the fmt.Stringer form.
//
// It is built from the raw terminal bytes above, through the same
// fmt.Sprintf("?CSI%+v?", bytes-after-the-CSI-introducer) shape bubbletea's
// own String() uses, rather than from this package's shiftPgUpCSIString
// vars. Deriving it independently is the point: a test that reused the
// product's own strings would assert nothing about whether those strings
// match what a terminal really sends, and would keep passing if both sides
// drifted together. TestShiftPageScrollRecognisesRealTerminalBytes below
// closes that loop explicitly, so a stand-in that stopped being recognised
// fails as its own named problem instead of quietly making every offset
// assertion here vacuous.
type unknownCSIStringerMsg string

func (m unknownCSIStringerMsg) String() string {
	return fmt.Sprintf("?CSI%+v?", []byte(strings.TrimPrefix(string(m), "\x1b[")))
}

// pageScrollTestModel is an interactive model with a non-nil grid, the least
// this package needs to exercise the scroll path: scrollInteractiveByLines
// gates on m.interactive and a non-nil m.interactiveGrid and then touches
// nothing else on the session, so an empty interactive.Session is enough
// here and keeps the test free of a real tmux server (the seeded, real-pane
// end of the same behaviour is covered by
// features/interactive_scroll.feature and
// interactive_entry_history_test.go).
func pageScrollTestModel(width, height int) Model {
	m := mouseTestModel([]store.Session{{ID: "a1", Name: "a1", CWD: "/work/infra"}})
	m.width, m.height = width, height
	m.interactive = true
	m.interactiveGrid = &interactive.Session{}
	return m
}

// pressPageKey feeds one Shift+PgUp/PgDn through the real Model.Update, so
// what is asserted is the whole dispatch the product uses (Update's tail
// calls shiftPageScrollDir, which calls scrollInteractiveByPage) rather than
// scrollInteractiveByPage in isolation -- a page step wired to nothing would
// otherwise pass.
func pressPageKey(t *testing.T, m Model, raw string) Model {
	t.Helper()
	next, _ := m.Update(unknownCSIStringerMsg(raw))
	updated, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, not tui.Model", next)
	}
	return updated
}

// TestShiftPageScrollRecognisesRealTerminalBytes proves the stand-in above
// is a faithful one: the fmt.Stringer form derived from the raw bytes a real
// terminal sends is exactly what shiftPageScrollDir recognises, with
// Shift+PgUp meaning "back into scrollback" (dir=1) and Shift+PgDn "toward
// the live bottom" (dir=-1).
//
// Without this, a bubbletea version bump that changed the String() format
// would leave every offset assertion in this file passing for the wrong
// reason -- an unrecognised message is a plain no-op, and "the offset did
// not move" is not what any of them check.
func TestShiftPageScrollRecognisesRealTerminalBytes(t *testing.T) {
	for _, tc := range []struct {
		name    string
		raw     string
		wantDir int
	}{
		{"Shift+PgUp", shiftPgUpRawCSI, 1},
		{"Shift+PgDn", shiftPgDownRawCSI, -1},
	} {
		dir, ok := shiftPageScrollDir(unknownCSIStringerMsg(tc.raw))
		if !ok {
			t.Fatalf("shiftPageScrollDir did not recognise %s (%q, Stringer form %q): every offset assertion in this file would silently be asserting a no-op", tc.name, tc.raw, unknownCSIStringerMsg(tc.raw).String())
		}
		if dir != tc.wantDir {
			t.Errorf("shiftPageScrollDir(%s) = %d, want %d", tc.name, dir, tc.wantDir)
		}
	}
}

// TestShiftPageScrollStepsTheWholePreviewContentHeight is the assertion
// nothing in this package made before: ONE Shift+PgUp moves the interactive
// scroll offset by the preview's whole content height, Shift+PgDn moves it
// back by the same, and presses accumulate exactly (so consecutive pages
// tile the grid's rows -- no row is stepped past unseen, and none is shown
// twice).
//
// The two wrong step sizes it rules out are named explicitly, because they
// are the two the product itself has to hand and either would keep every
// other test in the repo green: 1 line (a `height := 1` slip in
// scrollInteractiveByPage) and interactiveWheelStepLines (the wheel's own
// notch, the plausible mistake of routing Shift+PgUp through the wheel's
// delta). Both are ruled out by the model's own geometry assumptions below
// rather than by hoping the numbers differ.
func TestShiftPageScrollStepsTheWholePreviewContentHeight(t *testing.T) {
	m := pageScrollTestModel(100, 40)

	// The page height the product is required to use: previewContentSize's
	// content height, the exact height interactiveBodyLines renders into
	// (scrollInteractiveByPage's own doc).
	_, page := m.previewContentSize()
	if page < interactiveMinInnerRows {
		t.Fatalf("test assumption violated: preview content height %d is below the %d-row floor, so this model would not be allowed to be interactive at all", page, interactiveMinInnerRows)
	}
	if page <= 1 || page == interactiveWheelStepLines {
		t.Fatalf("test assumption violated: preview content height %d cannot be told apart from a one-line step (1) or a wheel notch (%d) -- pick a terminal size whose content height is neither", page, interactiveWheelStepLines)
	}
	if 2*page > interactive.ScrollbackMaxLines {
		t.Fatalf("test assumption violated: two pages (%d lines) exceed the grid's own %d-line bound, so scrollInteractiveByLines' clamp would answer these assertions instead of the page arithmetic", 2*page, interactive.ScrollbackMaxLines)
	}

	if m.interactiveScrollOffset() != 0 {
		t.Fatalf("a freshly entered interactive model starts at scroll offset %d, want 0 (the live bottom)", m.interactiveScrollOffset())
	}

	back1 := pressPageKey(t, m, shiftPgUpRawCSI)
	if back1.interactiveScrollOffset() != page {
		t.Fatalf("one Shift+PgUp moved the interactive scroll offset to %d, want %d -- the preview's whole content height (PRD II-51). A step of 1 would be a LINE, a step of %d would be the mouse wheel's notch (interactiveWheelStepLines); Shift+PgUp must page", back1.interactiveScrollOffset(), page, interactiveWheelStepLines)
	}

	back2 := pressPageKey(t, back1, shiftPgUpRawCSI)
	if back2.interactiveScrollOffset() != 2*page {
		t.Fatalf("a second Shift+PgUp moved the offset to %d, want %d (two whole pages) -- consecutive pages must tile the grid's rows exactly, sharing no row and skipping none", back2.interactiveScrollOffset(), 2*page)
	}

	forward1 := pressPageKey(t, back2, shiftPgDownRawCSI)
	if forward1.interactiveScrollOffset() != page {
		t.Fatalf("one Shift+PgDn from two pages back moved the offset to %d, want %d -- Shift+PgDn must undo exactly one Shift+PgUp, which is what lets a scenario mirror N pages back with N pages forward", forward1.interactiveScrollOffset(), page)
	}

	forward2 := pressPageKey(t, forward1, shiftPgDownRawCSI)
	if forward2.interactiveScrollOffset() != 0 {
		t.Fatalf("Shift+PgDn back to the live view left the offset at %d, want 0: an equal number of pages each way must land ON the live bottom, never short of it", forward2.interactiveScrollOffset())
	}

	// One more at the bottom: scrollInteractiveByLines clamps at 0, so a
	// page step cannot walk the offset negative and desynchronise a
	// mirrored count.
	forward3 := pressPageKey(t, forward2, shiftPgDownRawCSI)
	if forward3.interactiveScrollOffset() != 0 {
		t.Fatalf("Shift+PgDn at the live bottom moved the offset to %d, want it clamped at 0", forward3.interactiveScrollOffset())
	}
}

// TestShiftPageScrollStepTracksTheTerminalHeight proves the page step is
// really taken from the CURRENT preview geometry rather than from any
// constant that merely happens to equal it at one terminal size: two
// different terminal heights must each step by their own
// previewContentSize height, and those two heights must differ.
//
// This is the half a single-size test cannot see. A `height :=
// someFixedRowCount` would satisfy the test above at whichever size that
// constant was tuned for, while silently scrolling past unseen rows on every
// other terminal -- and scrollInteractiveByPage's own doc promises the
// opposite ("so a resize never leaves this stepping by a stale row count").
func TestShiftPageScrollStepTracksTheTerminalHeight(t *testing.T) {
	short := pageScrollTestModel(100, 24)
	tall := pageScrollTestModel(100, 60)

	_, shortPage := short.previewContentSize()
	_, tallPage := tall.previewContentSize()
	if shortPage < interactiveMinInnerRows || tallPage < interactiveMinInnerRows {
		t.Fatalf("test assumption violated: content heights %d/%d must both clear the %d-row floor", shortPage, tallPage, interactiveMinInnerRows)
	}
	if shortPage == tallPage {
		t.Fatalf("test assumption violated: a 24-row and a 60-row terminal both give a %d-row preview content height, so this test cannot tell a geometry-derived step from a constant one", shortPage)
	}

	if got := pressPageKey(t, short, shiftPgUpRawCSI).interactiveScrollOffset(); got != shortPage {
		t.Errorf("Shift+PgUp in a 24-row terminal moved the offset by %d, want that terminal's own preview content height %d", got, shortPage)
	}
	if got := pressPageKey(t, tall, shiftPgUpRawCSI).interactiveScrollOffset(); got != tallPage {
		t.Errorf("Shift+PgUp in a 60-row terminal moved the offset by %d, want that terminal's own preview content height %d -- the page step must come from the live geometry, not a constant tuned to one size", got, tallPage)
	}
}
