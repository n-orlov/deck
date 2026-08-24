// scrollback_test.go proves PRD phase3b II-51's two claims about the
// grid's own scrollback directly against a bare *Grid (newGrid), with no
// live tmux pane needed at all: it stays bounded no matter how much more
// than the bound is written through it, and RenderRows composes a
// scrolled view out of it correctly (matching Grid().Render() itself at
// offset 0, and reaching genuinely older content at a positive offset).
package interactive

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"
)

// feedNumberedLines writes n lines of the form "line NNNNNN" (padded to
// the grid's own width so vt.Scrollback.Push's own trailing-empty-cell
// trim -- see grid.go's RenderRows doc -- never shortens a line this
// test later expects to find at its full, padded width) followed by
// "\r\n", so each one advances the cursor to a fresh line and, once the
// screen's own bottom row is reached, scrolls the previous top row off
// into the grid's scrollback exactly the way a real shell's own output
// would.
func feedNumberedLines(g *Grid, width, from, to int) {
	for i := from; i < to; i++ {
		line := fmt.Sprintf("line %06d", i)
		if len(line) < width {
			line += strings.Repeat(".", width-len(line))
		}
		_, _ = g.Write([]byte(line + "\r\n"))
	}
}

// TestScrollbackStaysBoundedRegardlessOfInputVolume is II-51's first,
// deterministic claim: feeding a grid FAR more than ScrollbackMaxLines
// worth of lines never grows its scrollback past that bound, because vt's
// own Scrollback.Push (charmbracelet/x/vt@.../scrollback.go) evicts the
// oldest line every time a Push would otherwise exceed maxLines --
// newGrid's own SetScrollbackSize call (grid.go) is what makes that bound
// ScrollbackMaxLines rather than vt's own, larger library default
// (vt.DefaultScrollbackSize, 10000).
func TestScrollbackStaysBoundedRegardlessOfInputVolume(t *testing.T) {
	const width, height = 80, 24
	g := newGrid(width, height)

	feedNumberedLines(g, width, 0, ScrollbackMaxLines)
	if got := g.ScrollbackLen(); got > ScrollbackMaxLines {
		t.Fatalf("after feeding exactly the bound's worth of lines, ScrollbackLen() = %d, want <= %d", got, ScrollbackMaxLines)
	}

	// Now feed TWENTY TIMES the bound's worth of additional lines -- far
	// more than the bound, the successCriteria's own wording -- and
	// confirm the bound still holds rather than the scrollback simply
	// growing to match however much was written.
	const floodMultiple = 20
	feedNumberedLines(g, width, ScrollbackMaxLines, ScrollbackMaxLines*(1+floodMultiple))
	if got := g.ScrollbackLen(); got > ScrollbackMaxLines {
		t.Fatalf("after feeding %dx the bound's worth of MORE lines, ScrollbackLen() = %d, want <= %d (the bound must cap it, not merely describe a starting size)", floodMultiple, got, ScrollbackMaxLines)
	}
	if got := g.ScrollbackLen(); got != ScrollbackMaxLines {
		// Not fatal on its own (a shorter-than-the-bound scrollback would
		// still satisfy "does not grow without limit"), but a flood this
		// much larger than the bound should have filled it exactly, and a
		// mismatch here is worth surfacing rather than silently passing.
		t.Errorf("ScrollbackLen() = %d after a %dx flood, want exactly %d (the bound, now full)", got, floodMultiple, ScrollbackMaxLines)
	}
}

// heapAllocNow forces a full GC and OS-memory return before sampling
// runtime.MemStats.HeapAlloc, the same discipline
// docs/reports/phase3b.md's II-51 measurement section used to get a
// stable, comparable number instead of one dominated by not-yet-
// collected garbage from the parsing this test itself does. keepAlive is
// kept reachable (runtime.KeepAlive) until AFTER the GC/sample, not
// merely lexically in scope: Go's own liveness analysis is by last real
// use, not declaration scope, so without this a caller's own "last use"
// of its *Grid before calling into here (its last real read, e.g.
// ScrollbackLen()) would let the compiler consider it already dead by
// the time THIS function's own runtime.GC() actually runs, collecting
// exactly the content this measurement exists to weigh and silently
// making baseline and content-filled samples come out identical.
func heapAllocNow(keepAlive any) uint64 {
	runtime.GC()
	debug.FreeOSMemory()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	runtime.KeepAlive(keepAlive)
	return m.HeapAlloc
}

// TestScrollbackMemoryDoesNotGrowWithoutBound is II-51's second claim,
// the one the successCriteria phrases as "a test feeds far more than the
// bound and asserts memory does not grow without limit": once a grid's
// scrollback is already full (TestScrollbackStaysBoundedRegardlessOfInputVolume
// above proves it cannot grow past ScrollbackMaxLines lines), pushing a
// LOT more input through it should cost roughly constant heap, not heap
// that keeps growing with however much more is written -- each new Push
// evicts an old line as it adds a new one, so the live, retained set of
// lines never grows once the bound is reached.
//
// This asserts a generous ratio rather than an exact number (heap
// accounting always carries some noise from goroutine stacks, GC
// bookkeeping and this very test's own temporaries), but the ratio is
// chosen to make the ACTUAL failure mode -- heap scaling with the input
// volume because the bound was not actually enforced -- fail loudly: a
// 40x larger flood producing more than double the heap of a 1x flood
// would mean something is retaining far more than one bound's worth of
// lines.
func TestScrollbackMemoryDoesNotGrowWithoutBound(t *testing.T) {
	const width, height = 120, 40

	measure := func(totalLines int) uint64 {
		g := newGrid(width, height)
		feedNumberedLines(g, width, 0, totalLines)
		if got := g.ScrollbackLen(); got > ScrollbackMaxLines {
			t.Fatalf("ScrollbackLen() = %d after feeding %d lines, want <= %d", got, totalLines, ScrollbackMaxLines)
		}
		return heapAllocNow(g)
	}

	// A baseline empty grid's own heap cost, subtracted below so the
	// comparison is about the scrollback content itself, not whatever a
	// bare *Grid always costs regardless of how it is fed.
	baselineGrid := newGrid(width, height)
	baseline := heapAllocNow(baselineGrid)

	heapAfterOneBoundWorth := measure(ScrollbackMaxLines)
	const floodMultiple = 40
	heapAfterFlood := measure(ScrollbackMaxLines * floodMultiple)

	t.Logf("docs/reports/phase3b.md II-51 measurement: baseline empty %dx%d grid HeapAlloc=%d bytes; after feeding exactly %d lines (the bound) HeapAlloc=%d bytes; after feeding %dx that many (%d lines total, far more than the bound) HeapAlloc=%d bytes",
		width, height, baseline, ScrollbackMaxLines, heapAfterOneBoundWorth, floodMultiple, ScrollbackMaxLines*floodMultiple, heapAfterFlood)

	if heapAfterOneBoundWorth <= baseline {
		t.Fatalf("HeapAlloc after filling the scrollback (%d) <= the empty baseline (%d); this measurement is not sensitive enough to prove anything below", heapAfterOneBoundWorth, baseline)
	}
	boundCost := heapAfterOneBoundWorth - baseline
	floodCost := heapAfterFlood - baseline
	if floodCost > boundCost*2 {
		t.Fatalf("feeding %dx more input than the bound cost %d bytes over baseline, more than double the %d bytes one bound's worth costs -- the scrollback appears to be retaining far more than its stated bound", floodMultiple, floodCost, boundCost)
	}
}

// TestRenderRowsMatchesLiveRenderAtZeroOffset proves RenderRows(0, h) is
// exactly Grid().Render() split on "\n" -- the pre-task-068 behaviour
// interactiveBodyLines relied on -- so switching interactiveBodyLines
// over to RenderRows changed nothing about the ordinary, unscrolled view.
func TestRenderRowsMatchesLiveRenderAtZeroOffset(t *testing.T) {
	const width, height = 40, 10
	g := newGrid(width, height)
	feedNumberedLines(g, width, 0, height*3) // enough to have scrolled some lines off already

	want := strings.Split(g.Render(), "\n")
	s := &Session{grid: g}
	got, usedOffset := s.RenderRows(0, height)
	if usedOffset != 0 {
		t.Fatalf("RenderRows(0, %d) usedOffset = %d, want 0", height, usedOffset)
	}
	if len(got) != len(want) {
		t.Fatalf("RenderRows(0, %d) returned %d rows, want %d (Render() split on \\n)", height, len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("RenderRows(0, %d) row %d = %q, want %q (Render()'s own row %d)", height, i, got[i], want[i], i)
		}
	}
}

// TestRenderRowsScrollsIntoScrollbackAndBack proves the actual scrolling
// contract: a line that has been pushed off the live screen into
// scrollback is invisible at offset 0 (the live view), visible once the
// offset scrolls back far enough to include it, and invisible again once
// the offset returns to 0.
func TestRenderRowsScrollsIntoScrollbackAndBack(t *testing.T) {
	const width, height = 40, 10
	g := newGrid(width, height)
	// Comfortably overflow the screen so SOME scrollback exists; exactly
	// how many lines a given amount of input scrolls off depends on vt's
	// own newline/scroll-margin handling (not this test's business), so
	// this reads back whatever g.ScrollbackLen() actually is rather than
	// assume a specific count.
	feedNumberedLines(g, width, 0, height+5)
	sbLen := g.ScrollbackLen()
	if sbLen == 0 {
		t.Fatalf("expected some scrollback after writing %d lines through a %d-row screen, got ScrollbackLen() = 0", height+5, height)
	}

	// Line 0 is the very first line ever written, so it is the OLDEST
	// scrollback entry if it survived eviction at all -- and with
	// ScrollbackMaxLines (2000) far larger than the handful of lines this
	// test pushes, it always has.
	needle := fmt.Sprintf("line %06d", 0)
	s := &Session{grid: g}

	live, usedOffset := s.RenderRows(0, height)
	if usedOffset != 0 {
		t.Fatalf("RenderRows(0, %d) usedOffset = %d, want 0", height, usedOffset)
	}
	for _, row := range live {
		if strings.Contains(row, needle) {
			t.Fatalf("live view (offset 0) unexpectedly contains %q, want it already scrolled off screen: %v", needle, live)
		}
	}

	// Scrolling back by the full scrollback length brings the oldest
	// line (line 0) into view: RenderRows composes a window covering
	// exactly [oldest scrollback line .. as much of the live screen as
	// still fits], so at offset == sbLen the very first row is the
	// oldest scrollback line.
	scrolledBack, usedOffset := s.RenderRows(sbLen, height)
	if usedOffset != sbLen {
		t.Fatalf("RenderRows(%d, %d) usedOffset = %d, want %d (the grid's actual scrollback length)", sbLen, height, usedOffset, sbLen)
	}
	found := false
	for _, row := range scrolledBack {
		if strings.Contains(row, needle) {
			found = true
		}
	}
	if !found {
		t.Fatalf("scrolled-back view (offset %d) does not contain %q, want the oldest scrolled-off line visible: %v", sbLen, needle, scrolledBack)
	}

	backToLive, usedOffset := s.RenderRows(0, height)
	if usedOffset != 0 {
		t.Fatalf("RenderRows(0, %d) usedOffset = %d, want 0", height, usedOffset)
	}
	for _, row := range backToLive {
		if strings.Contains(row, needle) {
			t.Fatalf("returning to offset 0 unexpectedly still shows %q: %v", needle, backToLive)
		}
	}
}

// TestRenderRowsClampsOffsetToActualScrollbackLength proves the
// usedOffset return value (RenderRows' own doc: "a caller's own stored
// offset can be kept in bounds from this single call") actually reflects
// reality rather than echoing back whatever was asked for: requesting an
// offset far beyond however much scrollback genuinely exists must clamp
// down to that real amount, not silently show blank rows above it.
func TestRenderRowsClampsOffsetToActualScrollbackLength(t *testing.T) {
	const width, height = 40, 10
	g := newGrid(width, height)
	feedNumberedLines(g, width, 0, height+3)
	sbLen := g.ScrollbackLen()
	if sbLen == 0 || sbLen >= ScrollbackMaxLines {
		t.Fatalf("test fixture assumption broken: ScrollbackLen() = %d, want a small, nonzero amount well under ScrollbackMaxLines (%d)", sbLen, ScrollbackMaxLines)
	}

	s := &Session{grid: g}
	_, usedOffset := s.RenderRows(ScrollbackMaxLines, height)
	if usedOffset != sbLen {
		t.Fatalf("RenderRows(%d, %d) usedOffset = %d, want %d (the grid's actual, much smaller scrollback length)", ScrollbackMaxLines, height, usedOffset, sbLen)
	}
}
