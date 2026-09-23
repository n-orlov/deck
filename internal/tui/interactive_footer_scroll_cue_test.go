// interactive_footer_scroll_cue_test.go is R133 part 2's own red-first
// proof (PRD phase4b, GH #30): the interactive footer must state how far
// back the scrolled-back view is and that it is NOT live, drawn from the
// CLAMPED, HEALED offset (R133 part 1) rather than whatever raw value
// m.interactiveScrollOffset happens to hold at the moment the footer is
// composed.
//
// TestInteractiveFooterCueReportsTheClampedScrolledBackPosition below
// drives the assertion through m.View() rather than by calling
// interactiveFooterLine directly on an already-healed Model, because
// calling interactiveBodyLines and interactiveFooterLine on the SAME
// addressable model (the pattern interactive_scroll_heal_test.go uses)
// would pass even against a naive interactiveFooterLine that just reads
// m.interactiveScrollOffset -- that field is already healed on that one
// shared object by the direct interactiveBodyLines call, so a naive
// footer reading it there would coincidentally see the healed value too.
// View() is a VALUE receiver (as is every render function in its own
// call chain: mainView, renderStackedFrame, renderSideBySideFrame,
// previewBodyLines), so each one gets its OWN copy of the Model; the heal
// interactiveBodyLines performs deep in that chain lands on a copy that
// is discarded long before footerLine ever runs, UNLESS the healed value
// is deliberately threaded back out and rewritten onto the copy
// footerLine actually reads (mainView's own doc explains the wiring this
// test is proving). This is exactly the failure mode the handoff notes
// call out: "the healed offset is only reliably fresh WITHIN the same
// call chain that invoked RenderRows... it does NOT persist across
// Update calls" -- and, as this test demonstrates, not across a fresh,
// independent View() call either, unless it is threaded through.
//
// Before the footer cue was wired to mainView's threaded, healed offset
// (rather than reading m.interactiveScrollOffset as a bare field from
// inside interactiveFooterLine, called from a value-receiver copy that
// never observed the heal), this test failed: the cue reported
// interactive.ScrollbackMaxLines (2000, the stale value scrolled past the
// real length) instead of the fixture's real, much smaller scrollback
// length.
package tui

import (
	"strconv"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/interactive"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/theme"
	"github.com/n-orlov/deck/internal/tmux"
)

// footerLineOf splits a full View() render into its lines and returns the
// last one -- mainView's own composition always appends m.footerLine()
// last (tui.go), so the last line of the whole rendered frame is always
// the footer, in every layout this package supports.
func footerLineOf(view string) string {
	lines := strings.Split(view, "\n")
	return lines[len(lines)-1]
}

func TestInteractiveFooterCueReportsTheClampedScrolledBackPosition(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	m.width, m.height = 100, 30
	if _, height := m.previewContentSize(); height < interactiveMinInnerRows {
		t.Fatalf("test assumption violated: preview content height %d is below the %d-row floor", height, interactiveMinInnerRows)
	}

	const printed = 400
	socket := selectionTestSocket("footercue")
	newShellPaneWithHistory(t, socket, "deck_footercue", 80, 10, printed)

	m.tmuxClient = tmux.Client{Socket: socket}
	m.sessions = []store.Session{{ID: "sess-footercue-1", Name: "footercue", Slug: "footercue", Status: "waiting"}}
	m.selected = rowCursor(0)

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
		t.Fatalf("the fixture pane's grid holds %d scrollback lines, want > 0", realScrollbackLen)
	}
	if realScrollbackLen >= interactive.ScrollbackMaxLines {
		t.Fatalf("the fixture pane's grid holds %d scrollback lines, which is not below interactive.ScrollbackMaxLines (%d) -- this test's premise (a stored offset set to the bound must land past the real length) would be vacuous", realScrollbackLen, interactive.ScrollbackMaxLines)
	}

	// Scroll past the real scrollback length, exactly like
	// interactive_scroll_heal_test.go's own fixture -- set directly so
	// this test exercises the heal regardless of scrollInteractiveByLines'
	// own separate clamp.
	got.setInteractiveScrollOffset(interactive.ScrollbackMaxLines)

	footer := footerLineOf(got.View())

	staleNeedle := strconv.Itoa(interactive.ScrollbackMaxLines)
	if strings.Contains(footer, staleNeedle) {
		t.Fatalf("footer = %q, contains the stale, unhealed offset %s -- the cue must read the clamped/healed position, not the raw stored field", footer, staleNeedle)
	}
	wantNeedle := strconv.Itoa(realScrollbackLen)
	if !strings.Contains(footer, wantNeedle) {
		t.Fatalf("footer = %q, want it to contain the clamped position %s (the grid's own real scrollback length)", footer, wantNeedle)
	}
	if !strings.Contains(strings.ToLower(footer), "not live") {
		t.Fatalf("footer = %q, want it to state the view is not live", footer)
	}
}

// TestInteractiveFooterCueTopOfScrollbackWordingDiffersFromOrdinary is
// R133 part 2's second claim: the top-of-scrollback cue (there is no more
// history to scroll into) must read differently from the ordinary
// scrolled-back cue (there IS more, further back), since the two convey
// different, actionable information -- one says "you have reached the
// end", the other does not.
func TestInteractiveFooterCueTopOfScrollbackWordingDiffersFromOrdinary(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	m.width, m.height = 100, 30
	if _, height := m.previewContentSize(); height < interactiveMinInnerRows {
		t.Fatalf("test assumption violated: preview content height %d is below the %d-row floor", height, interactiveMinInnerRows)
	}

	const printed = 400
	socket := selectionTestSocket("footercuewords")
	newShellPaneWithHistory(t, socket, "deck_footercuewords", 80, 10, printed)

	m.tmuxClient = tmux.Client{Socket: socket}
	m.sessions = []store.Session{{ID: "sess-footercuewords-1", Name: "footercuewords", Slug: "footercuewords", Status: "waiting"}}
	m.selected = rowCursor(0)

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
	if realScrollbackLen < 2 {
		t.Fatalf("the fixture pane's grid holds %d scrollback lines, want at least 2 so an ordinary, non-top offset is distinguishable from the top", realScrollbackLen)
	}

	// Ordinary case: scrolled back, but not all the way -- one line short
	// of the top.
	ordinary := got
	ordinary.setInteractiveScrollOffset(realScrollbackLen - 1)
	ordinaryFooter := footerLineOf(ordinary.View())

	// Top-of-scrollback case: scrolled back exactly to the real length.
	atTop := got
	atTop.setInteractiveScrollOffset(realScrollbackLen)
	topFooter := footerLineOf(atTop.View())

	if ordinaryFooter == topFooter {
		t.Fatalf("the ordinary scrolled-back footer and the top-of-scrollback footer are identical (%q); the top-of-scrollback wording must differ from the ordinary scrolled-back wording", ordinaryFooter)
	}
	if !strings.Contains(strings.ToLower(topFooter), "top") {
		t.Fatalf("top-of-scrollback footer = %q, want it to name the top of scrollback", topFooter)
	}
}

// TestInteractiveFooterAtLiveBottomRendersNoCueAndMatchesPreChangeFooter is
// R133 part 2's third claim: at the live bottom (offset 0, the ordinary,
// unscrolled state every interactive session starts in) the footer must
// render with no cue at all, byte-identical to the line
// interactiveFooterLine produced before this task -- forwardNote, the
// `·`/`-` separator, the Ctrl+Q key and its hint, and nothing else.
func TestInteractiveFooterAtLiveBottomRendersNoCueAndMatchesPreChangeFooter(t *testing.T) {
	m := New(nil, config.Settings{Color: true}, "")
	m.width, m.height = 100, 30
	socket := selectionTestSocket("footercuezero")
	newQuietSelectionPane(t, socket, "deck_footercuezero", 80, 24)

	m.tmuxClient = tmux.Client{Socket: socket}
	m.sessions = []store.Session{{ID: "sess-footercuezero-1", Name: "footercuezero", Slug: "footercuezero", Status: "waiting"}}
	m.selected = rowCursor(0)

	next, _ := m.enterInteractive()
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("enterInteractive returned a non-Model tea.Model")
	}
	if got.attachError != "" {
		t.Fatalf("enterInteractive refused: %q", got.attachError)
	}
	if !got.interactive {
		t.Fatalf("enterInteractive did not enter interactive mode")
	}
	defer got.exitInteractive()

	if got.interactiveScrollOffset() != 0 {
		t.Fatalf("test assumption violated: interactiveScrollOffset = %d at entry, want 0 (the live bottom)", got.interactiveScrollOffset())
	}

	footer := footerLineOf(got.View())

	width, _ := got.frameSize()
	forwardNote := got.colorToken(theme.Hint, "keystrokes forward to the live pane")
	sep := got.glyph(" · ", " - ")
	key := got.colorToken(theme.Key, got.glyph("Ctrl+Q", "Ctrl+Q"))
	hint := got.colorToken(theme.Hint, "leave interactive mode")
	preChangeContent := forwardNote + sep + key + " " + hint
	want := got.canvasFillLine(theme.Background, preChangeContent, width)

	if footer != want {
		t.Fatalf("footer at offset 0 = %q, want the pre-change footer line %q (no cue)", footer, want)
	}

	if strings.Contains(strings.ToLower(footer), "scroll") {
		t.Fatalf("footer = %q, must not mention scrolling at the live bottom", footer)
	}
}
