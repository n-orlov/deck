// interactive_scroll_persist_test.go is R133 part 2's own red-first proof
// (PRD phase4b, GH #30, review finding at 840fafb): the review found that
// interactiveBodyLines' heal (R133 part 1, interactive.go) lands only on
// the value-receiver copy of the model that a render call builds several
// stack frames deep inside View() -- a copy discarded the instant that
// render returns -- so the model bubbletea actually keeps between Update
// calls (the one the NEXT input event starts from) never observed the
// healed, clamped offset at all. Driving the real Update/View cycle past
// a short history boundary and then scrolling one line toward live used
// to keep reporting "Top of scrollback" at the stale-high value instead
// of the one line closer to live the real, clamped position actually is.
//
// This test is the exact reproducer the review filed
// (TestReviewStoredOffsetHealedAcrossViewAndNextScroll in
// artifacts/review/tui-review-prd-test.go), kept under this task's own
// file so its provenance is traceable to the R133 part 2 fix
// (scrollInteractiveByLines, interactive_scroll.go) rather than to the
// review's own scratch file.
package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/interactive"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

func TestStoredScrollOffsetHealedAcrossViewAndNextScroll(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	m.width, m.height = 100, 30
	socket := selectionTestSocket("persistheal")
	newShellPaneWithHistory(t, socket, "deck_persistheal", 80, 10, 100)
	m.tmuxClient = tmux.Client{Socket: socket}
	m.sessions = []store.Session{{ID: "persist-heal", Name: "persistheal", Slug: "persistheal", Status: "waiting"}}
	next, _ := m.enterInteractive()
	m = next.(Model)
	if !m.interactive || m.interactiveGrid == nil {
		t.Fatalf("entry refused: %s", m.attachError)
	}
	defer m.exitInteractive()

	real := m.interactiveGrid.Grid().ScrollbackLen()
	if real <= 1 || real >= interactive.ScrollbackMaxLines {
		t.Fatalf("bad fixture history %d", real)
	}

	// Scroll far past the real scrollback length -- the bound
	// scrollInteractiveByLines itself still clamps to
	// [0, interactive.ScrollbackMaxLines].
	next, _ = m.scrollInteractiveByLines(interactive.ScrollbackMaxLines)
	m = next.(Model)

	// The model scrollInteractiveByLines just handed back -- the one the
	// NEXT input event actually starts from -- must already carry the
	// clamped, real offset, not the raw arithmetic bound value.
	if m.interactiveScrollOffset != real {
		t.Fatalf("after scrollInteractiveByLines, stored offset=%d, want the clamped used offset %d (the grid's own real scrollback length) -- the heal must land on the model returned for the next input event, not merely a render-local copy", m.interactiveScrollOffset, real)
	}

	footer := footerLineOf(m.View())
	if !strings.Contains(footer, fmt.Sprint(real)) {
		t.Fatalf("cue itself wrong: %s", footer)
	}

	// One line toward live from the healed position must report the
	// PRECEDING history line immediately -- real-1 -- not "Top of
	// scrollback" at some stale-high value that never healed.
	next, _ = m.scrollInteractiveByLines(-1)
	m = next.(Model)
	footer = footerLineOf(m.View())
	if strings.Contains(footer, "Top of scrollback") || !strings.Contains(footer, fmt.Sprintf("Scrolled back %d lines", real-1)) {
		t.Fatalf("one line towards live did not leave top: stored=%d footer=%q; want %d", m.interactiveScrollOffset, footer, real-1)
	}
}

// TestInteractiveDispatcherNilDoesNotSnapStoredOffset guards against an
// over-broad heal: updateInteractive's own nil-dispatcher no-op path
// (interactive_test.go's sibling coverage) must still leave
// m.interactiveScrollOffset completely untouched -- this fix only heals
// the offset inside scrollInteractiveByLines/scrollInteractiveByPage
// themselves, never as some blanket side effect of every interactive
// Update call.
func TestInteractiveDispatcherNilDoesNotSnapStoredOffset(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	m.interactive = true
	m.interactiveScrollOffset = 27
	next, _ := m.updateInteractive(key("x"))
	m = next.(Model)
	if m.interactiveScrollOffset != 27 {
		t.Fatalf("nil dispatcher snapped offset to %d", m.interactiveScrollOffset)
	}
}
