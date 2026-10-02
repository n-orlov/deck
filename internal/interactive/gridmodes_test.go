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

const allMouseModes = MouseNormal | MouseButton | MouseAny | MouseUTF8 | MouseSGR | MouseURXVT

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
