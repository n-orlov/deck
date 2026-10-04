// gridmodes_test.go pins R182/R183's grid side: the vt grid tracks cursor
// visibility and the six mouse-reporting modes through callbacks that are
// registered before the first seed byte, and RenderSnapshot reads rows,
// cursor and visibility from one locked read.
package interactive

import (
	"context"
	"strings"
	"testing"
)

func newModesSession() *Session {
	s := &Session{renders: NewRenderCoalescer(renderCoalesceInterval)}
	s.grid = s.newDrainedGrid(20, 5)
	return s
}

func TestGridTracksCursorVisibilityAndMouseModes(t *testing.T) {
	g := newGrid(20, 5)
	if !g.CursorVisible() || g.MouseModes().Any() {
		t.Fatalf("fresh grid: visible=%v mouse=%b, want visible and no mouse modes", g.CursorVisible(), g.MouseModes())
	}
	for _, c := range []struct {
		num int
		bit MouseMode
	}{{1000, MouseNormal}, {1002, MouseButton}, {1003, MouseAny}, {1005, MouseUTF8}, {1006, MouseSGR}, {1015, MouseURXVT}} {
		_, _ = g.Write([]byte("\x1b[?" + itoa(c.num) + "h"))
		if got := g.MouseModes(); got != c.bit {
			t.Fatalf("after ?%dh mouse modes = %b, want only %b", c.num, got, c.bit)
		}
		_, _ = g.Write([]byte("\x1b[?" + itoa(c.num) + "l"))
		if g.MouseModes().Any() {
			t.Fatalf("after ?%dl mouse modes = %b, want none", c.num, g.MouseModes())
		}
	}
	// Modes the grid does not track must not leak into the set.
	_, _ = g.Write([]byte("\x1b[?1004h\x1b[?2004h\x1b[?1001h"))
	if g.MouseModes().Any() {
		t.Fatalf("untracked modes leaked into mouse modes: %b", g.MouseModes())
	}
	_, _ = g.Write([]byte("\x1b[?25l"))
	if g.CursorVisible() {
		t.Fatalf("cursor still visible after ?25l")
	}
	_, _ = g.Write([]byte("\x1b[?25h"))
	if !g.CursorVisible() {
		t.Fatalf("cursor hidden after ?25h")
	}
}

func itoa(n int) string {
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}

// The callbacks are registered before newGrid returns, so the very first
// bytes written -- a seed's mode replay -- are tracked, for both
// constructors.
func TestModeCallbacksRegisteredBeforeSeedForBothConstructors(t *testing.T) {
	seed := []byte("hello\x1b[?25l\x1b[?1000h\x1b[?1006h")
	s := &Session{}
	for name, g := range map[string]*Grid{"newGrid": newGrid(20, 5), "newDrainedGrid": s.newDrainedGrid(20, 5)} {
		if _, err := g.Write(seed); err != nil {
			t.Fatalf("%s write: %v", name, err)
		}
		if g.CursorVisible() {
			t.Errorf("%s: seed's ?25l not tracked", name)
		}
		if got := g.MouseModes(); got != MouseNormal|MouseSGR {
			t.Errorf("%s: mouse modes = %b, want %b", name, got, MouseNormal|MouseSGR)
		}
		retireGrid(g)
	}
}

func TestModesSurviveReseedAndResize(t *testing.T) {
	s := newModesSession()
	defer retireGrid(s.grid)
	// Live stream sets state on the first grid.
	s.writeGrid([]byte("\x1b[?25l\x1b[?1002h\x1b[?1015h"))
	snap := s.RenderSnapshot(0, 5)
	if snap.CursorVisible || snap.Mouse != MouseButton|MouseURXVT {
		t.Fatalf("live stream: visible=%v mouse=%b", snap.CursorVisible, snap.Mouse)
	}

	// A resize rebuilds the grid from the seed; the seed's replay carries
	// the pane's modes (BuildSeed's own mode tail), so they hold again.
	seed := []byte("row\x1b[?25l\x1b[?1002h\x1b[?1015h")
	old := s.Grid()
	if err := s.Resize(context.Background(), 30, 8, func(context.Context) ([]byte, error) { return seed, nil }); err != nil {
		t.Fatal(err)
	}
	if s.Grid() == old {
		t.Fatal("resize did not rebuild the grid")
	}
	snap = s.RenderSnapshot(0, 8)
	if snap.CursorVisible || snap.Mouse != MouseButton|MouseURXVT {
		t.Fatalf("after resize: visible=%v mouse=%b", snap.CursorVisible, snap.Mouse)
	}

	// A reseed whose replay says the modes are off resets them (the rebuilt
	// grid carries nothing over from the one it replaces), and the live
	// stream then works on the new grid.
	if err := s.Resize(context.Background(), 30, 8, func(context.Context) ([]byte, error) {
		return []byte("row\x1b[?25h\x1b[?1002l\x1b[?1015l"), nil
	}); err != nil {
		t.Fatal(err)
	}
	snap = s.RenderSnapshot(0, 8)
	if !snap.CursorVisible || snap.Mouse.Any() {
		t.Fatalf("after reseed to defaults: visible=%v mouse=%b", snap.CursorVisible, snap.Mouse)
	}
	s.writeGrid([]byte("\x1b[?1003h\x1b[?25l"))
	snap = s.RenderSnapshot(0, 8)
	if snap.CursorVisible || snap.Mouse != MouseAny {
		t.Fatalf("live stream after reseed: visible=%v mouse=%b", snap.CursorVisible, snap.Mouse)
	}
	if s.Grid().MouseModes() != MouseAny {
		t.Fatalf("grid accessor disagrees with snapshot")
	}
}

func TestRenderSnapshotRowsAndCursorComeFromOneRead(t *testing.T) {
	s := newModesSession()
	defer retireGrid(s.grid)
	s.writeGrid([]byte("abc\r\ndef\x1b[?25l\x1b[?1006h"))
	snap := s.RenderSnapshot(0, 5)
	if len(snap.Rows) != 5 || !strings.Contains(snap.Rows[1], "def") {
		t.Fatalf("rows = %q", snap.Rows)
	}
	if snap.CursorX != 3 || snap.CursorY != 1 || snap.CursorViewRow != 1 || snap.CursorVisible || snap.Mouse != MouseSGR {
		t.Fatalf("snapshot cursor = (%d,%d) viewRow %d visible=%v mouse=%b", snap.CursorX, snap.CursorY, snap.CursorViewRow, snap.CursorVisible, snap.Mouse)
	}
	// RenderRows is the same composition.
	rows, used := s.RenderRows(0, 5)
	if used != snap.UsedOffset || strings.Join(rows, "\n") != strings.Join(snap.Rows, "\n") {
		t.Fatalf("RenderRows disagrees with RenderSnapshot rows")
	}

	// While a write is in flight the snapshot is served whole from the one
	// previously composed frame: rows AND cursor AND visibility from the
	// same moment, never the in-flight write's partial state.
	s.writes.Lock()
	if _, err := s.grid.Write([]byte("\x1b[?25h\x1b[1;1HZZZ")); err != nil {
		t.Fatal(err)
	}
	stale := s.RenderSnapshot(0, 5)
	s.writes.Unlock()
	if stale.CursorVisible || stale.CursorX != 3 || stale.CursorY != 1 || stale.Mouse != MouseSGR || strings.Contains(strings.Join(stale.Rows, ""), "ZZZ") {
		t.Fatalf("stale snapshot mixed moments: cursor (%d,%d) visible=%v rows=%q", stale.CursorX, stale.CursorY, stale.CursorVisible, stale.Rows)
	}
	fresh := s.RenderSnapshot(0, 5)
	if !fresh.CursorVisible || fresh.CursorX != 3 || fresh.CursorY != 0 || !strings.Contains(fresh.Rows[0], "ZZZ") {
		t.Fatalf("fresh snapshot = cursor (%d,%d) visible=%v rows=%q", fresh.CursorX, fresh.CursorY, fresh.CursorVisible, fresh.Rows)
	}
}

// staleScrollSession builds a 20x5 grid whose scrollback holds lines
// L00..L09 (L10..L14 on screen would need more writes; here the screen holds
// the tail), so offsets 0..sbLen are all meaningful.
func staleScrollSession(t *testing.T) *Session {
	t.Helper()
	s := newModesSession()
	var b strings.Builder
	for i := 0; i < 12; i++ {
		b.WriteString("L" + string(rune('0'+i/10)) + string(rune('0'+i%10)) + "\r\n")
	}
	s.writeGrid([]byte(b.String()))
	return s
}

// Rows served from the cached frame keep the cached frame's own offset and
// cursor labelling no matter what offset the new request carries.
func TestStaleSnapshotKeepsRowsOffsetAndScrollIntent(t *testing.T) {
	// Case 1: frame cached scrolled back, next request asks for live.
	s := staleScrollSession(t)
	defer retireGrid(s.grid)
	hist := s.RenderSnapshot(2, 5)
	if hist.UsedOffset != 2 || hist.Stale {
		t.Fatalf("fixture: history frame used=%d stale=%v", hist.UsedOffset, hist.Stale)
	}
	s.writes.Lock()
	live := s.RenderSnapshot(0, 5)
	s.writes.Unlock()
	if !live.Stale || live.UsedOffset != 2 {
		t.Fatalf("cached history frame requested live: used=%d stale=%v, want used=2 (the rows' own offset)", live.UsedOffset, live.Stale)
	}
	if live.ScrollOffset != 0 {
		t.Fatalf("scroll intent = %d, want 0: the user's request must survive for the next fresh frame", live.ScrollOffset)
	}
	if strings.Join(live.Rows, "\n") != strings.Join(hist.Rows, "\n") || live.CursorViewRow != hist.CursorViewRow {
		t.Fatalf("cached rows/cursor changed: %q vs %q", live.Rows, hist.Rows)
	}
	if rows, off := s.RenderRows(0, 5); off != 0 || len(rows) != 5 {
		t.Fatalf("fresh render after the write: offset=%d rows=%d", off, len(rows))
	}

	// Case 2: frame cached live, next request asks for history.
	s2 := staleScrollSession(t)
	defer retireGrid(s2.grid)
	s2.RenderSnapshot(0, 5)
	s2.writes.Lock()
	h := s2.RenderSnapshot(3, 5)
	s2.writes.Unlock()
	if h.UsedOffset != 0 || h.ScrollOffset != 3 || !h.Stale {
		t.Fatalf("cached live frame requested at 3: used=%d scroll=%d stale=%v, want used=0 scroll=3", h.UsedOffset, h.ScrollOffset, h.Stale)
	}

	// Case 3: the intent is NOT clamped against the cached scrollback
	// length (the write in flight may be growing it), only to the grid's
	// hard cap; the cached frame's offset is untouched, and the next fresh
	// composition clamps the intent against the real length.
	s3 := staleScrollSession(t)
	defer retireGrid(s3.grid)
	s3.RenderSnapshot(1, 5)
	s3.writes.Lock()
	c := s3.RenderSnapshot(10000, 5)
	s3.writes.Unlock()
	if c.UsedOffset != 1 || c.ScrollOffset != ScrollbackMaxLines {
		t.Fatalf("cap: used=%d scroll=%d, want used=1 scroll=%d", c.UsedOffset, c.ScrollOffset, ScrollbackMaxLines)
	}
	realLen := s3.grid.Scrollback().Len()
	if f := s3.RenderSnapshot(c.ScrollOffset, 5); f.Stale || f.UsedOffset != realLen || f.ScrollOffset != realLen {
		t.Fatalf("fresh frame after an over-long intent: stale=%v used=%d scroll=%d, want both %d", f.Stale, f.UsedOffset, f.ScrollOffset, realLen)
	}

	// Case 4: a taller request pads the cached frame at the top, and the
	// cursor row and offset labels stay consistent with the padded rows.
	s4 := staleScrollSession(t)
	defer retireGrid(s4.grid)
	base := s4.RenderSnapshot(2, 5)
	s4.writes.Lock()
	tall := s4.RenderSnapshot(0, 7)
	s4.writes.Unlock()
	if tall.UsedOffset != 2 || len(tall.Rows) != 7 || tall.CursorViewRow != base.CursorViewRow+2 {
		t.Fatalf("padded cached frame: used=%d rows=%d cursorViewRow=%d (base %d)", tall.UsedOffset, len(tall.Rows), tall.CursorViewRow, base.CursorViewRow)
	}
}

// With no cached frame there is nothing to relabel: the request comes back
// as both labels.
func TestStaleSnapshotWithoutFrameEchoesRequest(t *testing.T) {
	s := &Session{}
	snap := s.staleSnapshot(4, 3)
	if snap.UsedOffset != 4 || snap.ScrollOffset != 4 || !snap.Stale || len(snap.Rows) != 3 {
		t.Fatalf("no-frame snapshot = %+v", snap)
	}
}

// The verifier's case for R182's stale path: history GROWS during the write
// that forces the cached frame, so a request past the cached scrollback
// length is valid for the next fresh frame and must not be cut down to the
// cached length (which would discard the user's scroll intent), while the
// cached live rows keep their own live labelling.
func TestStaleSnapshotKeepsScrollIntentWhileHistoryGrows(t *testing.T) {
	s := newModesSession() // 20x5
	defer retireGrid(s.grid)
	s.writeGrid([]byte("A0\r\nA1\r\nA2\r\nA3\r\nA4\r\nA5"))
	live := s.RenderSnapshot(0, 5)
	if cached := s.lastFrame.Load().scrollbackLen; cached != 1 || live.UsedOffset != 0 {
		t.Fatalf("fixture: cached scrollback %d used %d, want 1 and 0", cached, live.UsedOffset)
	}

	s.writes.Lock()
	// The in-flight write pushes eight more lines into history.
	if _, err := s.grid.Write([]byte("\r\nB0\r\nB1\r\nB2\r\nB3\r\nB4\r\nB5\r\nB6\r\nB7")); err != nil {
		s.writes.Unlock()
		t.Fatal(err)
	}
	var got []RenderSnapshot
	for _, req := range []int{2, 5, 9} {
		got = append(got, s.RenderSnapshot(req, 5))
	}
	s.writes.Unlock()

	for i, req := range []int{2, 5, 9} {
		g := got[i]
		if !g.Stale || g.ScrollOffset != req {
			t.Fatalf("request %d during a growing write: stale=%v scroll=%d, want stale scroll=%d (intent kept)", req, g.Stale, g.ScrollOffset, req)
		}
		// The rows are still the cached LIVE frame, so their labels say live
		// and the cursor row is the cached one, in view.
		if g.UsedOffset != 0 || g.CursorViewRow != live.CursorViewRow || strings.Join(g.Rows, "\n") != strings.Join(live.Rows, "\n") {
			t.Fatalf("request %d: cached live frame relabelled: used=%d cursorViewRow=%d (want 0, %d)", req, g.UsedOffset, g.CursorViewRow, live.CursorViewRow)
		}
	}

	// The next fresh frame honours the kept intent at the grown length, and
	// it is history, so the live cursor row is out of its view.
	if n := s.grid.Scrollback().Len(); n != 9 {
		t.Fatalf("fixture: scrollback after the write = %d, want 9", n)
	}
	fresh := s.RenderSnapshot(got[1].ScrollOffset, 5)
	if fresh.Stale || fresh.UsedOffset != 5 || fresh.ScrollOffset != 5 {
		t.Fatalf("fresh frame at the kept intent: stale=%v used=%d scroll=%d, want used=scroll=5", fresh.Stale, fresh.UsedOffset, fresh.ScrollOffset)
	}
	if fresh.CursorViewRow >= 0 && fresh.CursorViewRow < len(fresh.Rows) {
		t.Fatalf("history frame at offset 5 places the live cursor in view at row %d", fresh.CursorViewRow)
	}
	if !strings.HasPrefix(fresh.Rows[0], "A4") || !strings.HasPrefix(fresh.Rows[4], "B2") {
		t.Fatalf("fresh frame at offset 5 rows = %q, want A4..B2", fresh.Rows)
	}
}

// A negative request during a write is the live bottom: the intent comes
// back as 0 and the cached history frame keeps its own offset, so the
// cursor is still not placed on it.
func TestStaleSnapshotNegativeRequestIsLiveIntentNotLiveRows(t *testing.T) {
	s := staleScrollSession(t)
	defer retireGrid(s.grid)
	hist := s.RenderSnapshot(4, 5)
	s.writes.Lock()
	got := s.RenderSnapshot(-3, 5)
	s.writes.Unlock()
	if !got.Stale || got.ScrollOffset != 0 || got.UsedOffset != 4 || got.CursorViewRow != hist.CursorViewRow {
		t.Fatalf("negative request: stale=%v scroll=%d used=%d cursorViewRow=%d, want stale scroll=0 used=4 cursorViewRow=%d", got.Stale, got.ScrollOffset, got.UsedOffset, got.CursorViewRow, hist.CursorViewRow)
	}
	if hist.CursorViewRow >= 0 && hist.CursorViewRow < len(hist.Rows) {
		t.Fatalf("fixture: history frame at offset 4 shows the cursor row %d", hist.CursorViewRow)
	}
}
