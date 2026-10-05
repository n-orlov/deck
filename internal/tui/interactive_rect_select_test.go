// interactive_rect_select_test.go covers R203 (#66): Alt+drag and Ctrl+drag
// select the rectangle spanned by two corner cells in the interactive
// preview; a plain drag still selects by line; with [ui] select_on_drag OFF
// neither kind of selection happens.
package tui

import (
	"io"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// rectScript paints a fixed block at the bottom of a 24-row pane (1-based
// rows 18-24) and then runs cat, which does not track the mouse:
//
//	a你b好c       cells a=0 你=1-2 b=3 好=4-5 c=6
//	ABCDEFGHIJ
//	KLM
//	(blank)
//	UVWXYZ
//	(blank)
//	ready
const rectScript = `printf '\033[2J\033[18;1Ha你b好c\033[19;1HABCDEFGHIJ\033[20;1HKLM\033[22;1HUVWXYZ\033[24;1Hready'; exec cat`

// rectRows are the preview rows of the fixture lines.
type rectRows struct{ wide, alpha, short, blank, tail int }

// rectModifiers are the keys held during a drag.
type rectModifiers struct{ alt, ctrl, shift bool }

var (
	modAlt     = rectModifiers{alt: true}
	modCtrl    = rectModifiers{ctrl: true}
	modPlain   = rectModifiers{}
	modCtrlAlt = rectModifiers{alt: true, ctrl: true}
	modAltShft = rectModifiers{alt: true, shift: true}
)

// rectFixture enters interactive mode on rectScript and waits until the
// grid shows the block; it also keeps the OSC 52 half off the test terminal.
func rectFixture(t *testing.T, name string, selectOnDrag bool) (Model, string, rectRows) {
	t.Helper()
	previous := oscClipboardWriter
	oscClipboardWriter = io.Discard
	t.Cleanup(func() { oscClipboardWriter = previous })

	m, socket, _ := wheelFixtureModelScript(t, name, rectScript, "")
	m.settings.SelectOnDrag = selectOnDrag
	_, height := m.previewContentSize()
	find := func(rows []string, text string) int {
		for i, r := range rows {
			if strings.HasPrefix(strings.TrimRight(stripANSI(r), " "), text) {
				return i
			}
		}
		return -1
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		rows, _ := m.interactiveGrid.RenderRows(0, height)
		got := rectRows{wide: find(rows, "a你b好c"), alpha: find(rows, "ABCDEFGHIJ"), short: find(rows, "KLM"), tail: find(rows, "ready")}
		if got.wide >= 0 && got.alpha >= 0 && got.short >= 0 && got.tail >= 0 {
			got.blank = got.alpha + 2
			if got.short != got.alpha+1 || got.wide != got.alpha-1 {
				t.Fatalf("fixture rows are not contiguous: %+v", got)
			}
			return m, socket, got
		}
		if time.Now().After(deadline) {
			t.Fatalf("the grid never showed the fixture; rows: %q", rows)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// dragCells drags from pane cell (c1, r1) to (c2, r2) with the modifiers
// held, returning the model after the press and motion, and the release msg.
func dragCells(t *testing.T, m Model, c1, r1, c2, r2 int, mod rectModifiers) (Model, tea.MouseMsg) {
	t.Helper()
	at := func(col, row int) tea.MouseMsg {
		w := wheelAtPreviewCell(t, m, col, row, true, false)
		return tea.MouseMsg{X: w.X, Y: w.Y, Button: tea.MouseButtonLeft, Alt: mod.alt, Ctrl: mod.ctrl, Shift: mod.shift}
	}
	down, end := at(c1, r1), at(c2, r2)
	down.Action = tea.MouseActionPress
	end.Action = tea.MouseActionMotion
	m = updateModel(t, m, down)
	m = updateModel(t, m, end)
	end.Action = tea.MouseActionRelease
	return m, end
}

// dragCopy drags and releases, returning the model and the copied text.
func dragCopy(t *testing.T, m Model, socket string, c1, r1, c2, r2 int, mod rectModifiers) (Model, string) {
	t.Helper()
	m, rel := dragCells(t, m, c1, r1, c2, r2, mod)
	m = updateModel(t, m, rel)
	out, err := selectionBufferText(socket)
	if err != nil {
		t.Fatalf("nothing was copied: %v: %s", err, out)
	}
	return m, out
}

// TestRectangleCornersInAllFourDirections: the same two corners, dragged in
// each direction, copy the same block.
func TestRectangleCornersInAllFourDirections(t *testing.T) {
	m, socket, r := rectFixture(t, "rectdirs", true)
	for _, tc := range []struct {
		name           string
		c1, r1, c2, r2 int
	}{
		{"down-right", 2, r.alpha, 4, r.short},
		{"down-left", 4, r.alpha, 2, r.short},
		{"up-right", 2, r.short, 4, r.alpha},
		{"up-left", 4, r.short, 2, r.alpha},
	} {
		_, got := dragCopy(t, m, socket, tc.c1, tc.r1, tc.c2, tc.r2, modAlt)
		if want := "CDE\nM"; got != want {
			t.Errorf("%s: copied %q, want %q", tc.name, got, want)
		}
	}
}

// TestRectangleIsHighlightedAsARectangleAndClearsOnRelease: while the drag
// is in progress exactly the block's cells carry the selection background.
func TestRectangleIsHighlightedAsARectangleAndClearsOnRelease(t *testing.T) {
	m, _, r := rectFixture(t, "recthl", true)
	m, rel := dragCells(t, m, 2, r.alpha, 4, r.blank, modAlt)
	cells := previewHighlight(t, m)
	if len(cells) != 3 {
		t.Fatalf("highlighted rows = %v, want the 3 rows of the block", cells)
	}
	for row := r.alpha; row <= r.blank; row++ {
		if len(cells[row]) != 3 {
			t.Fatalf("row %d highlights columns %v, want exactly 2-4", row, cells[row])
		}
		for col := 2; col <= 4; col++ {
			if _, ok := cells[row][col]; !ok {
				t.Fatalf("row %d column %d is not highlighted: %v", row, col, cells[row])
			}
		}
	}
	m = updateModel(t, m, rel)
	if len(previewHighlight(t, m)) != 0 {
		t.Fatalf("the highlight survived the release")
	}
}

// TestRectangleKeepsMiddleRowsToTheColumnRange: unlike a line selection, a
// row between the corners is confined to the columns.
func TestRectangleKeepsMiddleRowsToTheColumnRange(t *testing.T) {
	m, socket, r := rectFixture(t, "rectmid", true)
	_, got := dragCopy(t, m, socket, 1, r.alpha, 3, r.alpha+3, modAlt)
	if want := "BCD\nLM\n\nVWX"; got != want {
		t.Fatalf("copied %q, want %q", got, want)
	}
}

// TestRectangleTrimsTrailingBlanksAndNeverPadsAShortRow: a row that ends
// before the range is empty or shorter, never padded to the range.
func TestRectangleTrimsTrailingBlanksAndNeverPadsAShortRow(t *testing.T) {
	m, socket, r := rectFixture(t, "rectshort", true)
	_, got := dragCopy(t, m, socket, 1, r.short, 8, r.blank+1, modAlt)
	if want := "LM\n\nVWXYZ"; got != want {
		t.Fatalf("copied %q, want %q", got, want)
	}
	// A range wholly past a short row's end copies an empty line for it.
	_, got = dragCopy(t, m, socket, 5, r.alpha, 8, r.short, modAlt)
	if want := "FGHI\n"; got != want {
		t.Fatalf("copied %q, want %q", got, want)
	}
}

// TestRectangleDoubleWidthGlyphAtTheLeftAndRightEdge: a glyph is copied
// whole when its left cell is inside the rectangle and left out when only
// its right cell is; half a glyph is never copied.
func TestRectangleDoubleWidthGlyphAtTheLeftAndRightEdge(t *testing.T) {
	m, socket, r := rectFixture(t, "rectwide", true)
	for _, tc := range []struct {
		name   string
		c1, c2 int
		want   string
	}{
		{"left edge in the glyph's right cell", 2, 3, "b"},
		{"right edge in the glyph's left cell", 3, 4, "b好"},
		{"right edge in the glyph's right cell", 3, 5, "b好"},
		{"both edges inside a glyph", 2, 4, "b好"},
		{"left edge on the glyph's left cell", 1, 3, "你b"},
	} {
		if _, got := dragCopy(t, m, socket, tc.c1, r.wide, tc.c2, r.wide, modAlt); got != tc.want {
			t.Errorf("%s: copied %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestRectangleIsClampedToThePaneTheDragStartedIn: a drag that runs off the
// preview onto the sidebar or the screen's far corner stops at the preview's
// own edge, so sidebar text never reaches the copy.
func TestRectangleIsClampedToThePaneTheDragStartedIn(t *testing.T) {
	m, socket, r := rectFixture(t, "rectclamp", true)
	rel := func(m Model, x, y int) string {
		down := wheelAtPreviewCell(t, m, 3, r.alpha, true, false)
		press := tea.MouseMsg{X: down.X, Y: down.Y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, Alt: true}
		m = updateModel(t, m, press)
		m = updateModel(t, m, tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion, Alt: true})
		_ = updateModel(t, m, tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease, Alt: true})
		out, err := selectionBufferText(socket)
		if err != nil {
			t.Fatalf("nothing copied: %v", err)
		}
		return out
	}
	// Onto the sidebar at the top-left: the rectangle runs to the preview's
	// own (0, 0), so the sidebar's text is not in it.
	got := rel(m, 0, 0)
	want := make([]string, r.alpha+1)
	want[r.wide] = "a你b"
	want[r.alpha] = "ABCD"
	if w := strings.Join(want, "\n"); got != w {
		t.Fatalf("drag onto the sidebar copied %q, want %q", got, w)
	}
	// Past the bottom-right corner of the screen: clamps to the preview's
	// last column and row, so the rectangle reaches the far edge only.
	got = rel(m, m.width-1, m.height-1)
	if !strings.HasPrefix(got, "DEFGHIJ\n\n\nXYZ\n") {
		t.Fatalf("drag past the corner copied %q, want a block starting at column 3 of ABCDEFGHIJ", got)
	}
	_, height := m.previewContentSize()
	if lines := strings.Count(got, "\n") + 1; lines != height-r.alpha {
		t.Fatalf("drag past the corner copied %d rows, want %d (down to the preview's last row)", lines, height-r.alpha)
	}
}

// TestAltDragAndCtrlDragEachSelectARectangle: both bindings select the same
// block, and the confirmation names the lines copied.
func TestAltDragAndCtrlDragEachSelectARectangle(t *testing.T) {
	m, socket, r := rectFixture(t, "rectmods", true)
	for name, mod := range map[string]rectModifiers{"alt": modAlt, "ctrl": modCtrl} {
		after, got := dragCopy(t, m, socket, 2, r.alpha, 4, r.short, mod)
		if want := "CDE\nM"; got != want {
			t.Errorf("%s drag copied %q, want %q", name, got, want)
		}
		if !strings.Contains(after.selectionCopyNote, "2 lines") {
			t.Errorf("%s drag confirmation %q does not name 2 lines", name, after.selectionCopyNote)
		}
	}
}

// TestPlainDragStillSelectsByLine: no modifier, or a combination that is not
// a binding (Ctrl+Alt, Shift+Alt), keeps the line-based selection.
func TestPlainDragStillSelectsByLine(t *testing.T) {
	m, socket, r := rectFixture(t, "rectplain", true)
	for name, mod := range map[string]rectModifiers{"plain": modPlain, "ctrl+alt": modCtrlAlt, "shift+alt": modAltShft} {
		_, got := dragCopy(t, m, socket, 2, r.alpha, 4, r.short, mod)
		if want := "CDEFGHIJ\nKLM"; got != want {
			t.Errorf("%s drag copied %q, want the line run %q", name, got, want)
		}
	}
}

// TestSelectOnDragOffDisablesBothRectangleBindings: Alt+drag and Ctrl+drag
// highlight and copy nothing when [ui] select_on_drag is off.
func TestSelectOnDragOffDisablesBothRectangleBindings(t *testing.T) {
	m, socket, r := rectFixture(t, "rectoff", false)
	for name, mod := range map[string]rectModifiers{"alt": modAlt, "ctrl": modCtrl} {
		after, rel := dragCells(t, m, 2, r.alpha, 4, r.short, mod)
		if after.interactiveSelecting || len(previewHighlight(t, after)) != 0 {
			t.Errorf("%s drag with select_on_drag OFF began a selection", name)
		}
		after = updateModel(t, after, rel)
		if after.selectionCopyNote != "" {
			t.Errorf("%s drag with select_on_drag OFF copied: %q", name, after.selectionCopyNote)
		}
		if out, err := selectionBufferText(socket); err == nil {
			t.Errorf("%s drag with select_on_drag OFF reached the tmux buffer: %q", name, out)
		}
	}
}
