package interactive

import (
	"strings"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
)

// newSelectionTestSession builds a bare *Session around a fresh Grid with
// no tmux pane behind it at all -- SelectedText/AbsoluteRow only ever
// touch s.grid (through s.mu, exactly as RenderRows does), so a Session
// built this way exercises the identical code path a real pipe-fed
// Session would, without needing a live tmux server the way grid_test.go's
// heavier fixtures (newBareInteractiveSession) do.
func newSelectionTestSession(t *testing.T, width, height int) *Session {
	t.Helper()
	return &Session{grid: newGrid(width, height)}
}

// TestSelectedTextWithinOneRowTrimsTrailingSpace proves the simplest
// case: a same-row selection returns exactly the cells between the two
// (inclusive) columns, with the row's own trailing padding trimmed --
// newGrid's blank cells are always " " (this file's own doc comment on
// SelectedText), never real content, so trimming them is never lossy.
func TestSelectedTextWithinOneRowTrimsTrailingSpace(t *testing.T) {
	s := newSelectionTestSession(t, 20, 5)
	if _, err := s.grid.Write([]byte("HELLO WORLD")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got, want := s.SelectedText(0, 0, 4, 0), "HELLO"; got != want {
		t.Fatalf("SelectedText(0,0,4,0) = %q, want %q", got, want)
	}
	if got, want := s.SelectedText(6, 0, 10, 0), "WORLD"; got != want {
		t.Fatalf("SelectedText(6,0,10,0) = %q, want %q", got, want)
	}
	if got, want := s.SelectedText(0, 0, 10, 0), "HELLO WORLD"; got != want {
		t.Fatalf("SelectedText(0,0,10,0) = %q, want %q", got, want)
	}
}

// TestSelectedTextAcrossRowsIsLinearNotRectangular proves the
// stream-style ("linear"/tmux-copy-mode-like) selection shape SelectedText's
// own doc comment commits to: the first selected row keeps only its tail
// (from the anchor column onward), the last keeps only its head (up to
// the release column), and every row strictly between the two is kept
// in FULL regardless of either endpoint's column -- never a rectangular
// block confined to the narrower of the two columns.
func TestSelectedTextAcrossRowsIsLinearNotRectangular(t *testing.T) {
	s := newSelectionTestSession(t, 10, 3)
	if _, err := s.grid.Write([]byte("ABCDEFGHIJ\r\nKLMNOPQRST\r\nUVWXYZ")); err != nil {
		t.Fatalf("write: %v", err)
	}
	// Anchor at column 7 of row 0 (col index 7 == 'H'), release at
	// column 2 of row 2 (col index 2 == 'X'). A rectangular selection
	// would clip row 1 to columns 2-7; a linear one keeps row 1 whole.
	got := s.SelectedText(7, 0, 2, 2)
	want := "HIJ\nKLMNOPQRST\nUVW"
	if got != want {
		t.Fatalf("SelectedText(7,0,2,2) = %q, want %q", got, want)
	}
}

// TestSelectedTextReversedDragMatchesForwardDrag proves the endpoint-swap
// SelectedText's own doc comment describes: dragging from either end of
// the same two cells selects identical text, the same direction-
// independence a real terminal's own click-drag selection guarantees.
func TestSelectedTextReversedDragMatchesForwardDrag(t *testing.T) {
	s := newSelectionTestSession(t, 10, 3)
	if _, err := s.grid.Write([]byte("ABCDEFGHIJ\r\nKLMNOPQRST\r\nUVWXYZ")); err != nil {
		t.Fatalf("write: %v", err)
	}
	forward := s.SelectedText(7, 0, 2, 2)
	backward := s.SelectedText(2, 2, 7, 0)
	if forward != backward {
		t.Fatalf("forward selection %q != backward selection %q; drag direction must not change the result", forward, backward)
	}
}

// TestSelectedTextClampsOutOfRangeCoordinates proves a coordinate beyond
// the grid's own bounds (a caller's hit-test rounding, or a resize
// racing a drag) is clamped rather than panicking or silently returning
// garbage.
func TestSelectedTextClampsOutOfRangeCoordinates(t *testing.T) {
	s := newSelectionTestSession(t, 5, 2)
	if _, err := s.grid.Write([]byte("HELLO\r\nWORLD")); err != nil {
		t.Fatalf("write: %v", err)
	}
	got := s.SelectedText(-3, -3, 999, 999)
	want := "HELLO\nWORLD"
	if got != want {
		t.Fatalf("SelectedText with out-of-range coordinates = %q, want %q", got, want)
	}
}

// TestSelectedTextReachesIntoScrollback proves the scrollback half of
// the absolute row space RenderRows/AbsoluteRow already document: rows
// scrolled off the live grid entirely are still addressable by
// SelectedText through ScrollbackCellAt, not merely the live screen's
// own CellAt.
func TestSelectedTextReachesIntoScrollback(t *testing.T) {
	s := newSelectionTestSession(t, 10, 2)
	for i := 1; i <= 5; i++ {
		if _, err := s.grid.Write([]byte("LINE" + string(rune('0'+i)) + "\r\n")); err != nil {
			t.Fatalf("write line %d: %v", i, err)
		}
	}
	sbLen := s.grid.ScrollbackLen()
	if sbLen == 0 {
		t.Fatalf("test assumption violated: nothing scrolled off yet (sbLen=0)")
	}
	// Row 0 in absolute space is the OLDEST scrollback line, which this
	// fixture guarantees started with "LINE1" (RenderRows/AbsoluteRow's
	// own doc comment: "row 0 is the oldest scrollback line").
	got := s.SelectedText(0, 0, 4, 0)
	if !strings.HasPrefix(got, "LINE") {
		t.Fatalf("SelectedText(0,0,4,0) = %q, want a line starting with LINE (oldest scrollback row)", got)
	}
}

// TestSelectionHighlightRangeAgreesWithSelectedTextAcrossWrappedRows is
// task 206's own agreement property (steer 018 / SPEC §11.8: "it covers
// exactly the run SelectedText would return"): for a drag spanning three
// wrapped rows, the cells SelectionHighlightRange marks -- read back
// directly off the grid through the SAME ScrollbackCellAt/CellAt split
// SelectedText itself uses, never SelectedText's own output reparsed --
// must reproduce SelectedText's return value byte for byte once each
// row's own trailing spaces are trimmed the same way. This is checked at
// the cell level, never against a golden frame or an escape-code
// assertion, so it proves the highlighted set and the copied set are the
// same set, not merely that both exist.
func TestSelectionHighlightRangeAgreesWithSelectedTextAcrossWrappedRows(t *testing.T) {
	s := newSelectionTestSession(t, 10, 3)
	if _, err := s.grid.Write([]byte("ABCDEFGHIJ\r\nKLMNOPQRST\r\nUVWXYZ")); err != nil {
		t.Fatalf("write: %v", err)
	}
	const offset, height = 0, 3
	// Anchor at column 7 of view row 0, release at column 2 of view row
	// 2 -- the same reversed-column shape
	// TestSelectedTextAcrossRowsIsLinearNotRectangular already proves is
	// linear, not rectangular, for SelectedText alone.
	const anchorCol, anchorViewRow = 7, 0
	const curCol, curViewRow = 2, 2

	fromRow := s.AbsoluteRow(offset, height, anchorViewRow)
	toRow := s.AbsoluteRow(offset, height, curViewRow)
	want := s.SelectedText(anchorCol, fromRow, curCol, toRow)

	g := s.Grid()
	sbLen := g.ScrollbackLen()
	cellAt := func(x, i int) *uv.Cell {
		if i < sbLen {
			return g.ScrollbackCellAt(x, i)
		}
		return g.CellAt(x, i-sbLen)
	}

	var rows []string
	for viewRow := 0; viewRow < height; viewRow++ {
		startCol, endCol, ok := s.SelectionHighlightRange(offset, height, viewRow, anchorCol, fromRow, curCol, toRow)
		if !ok {
			continue
		}
		abs := s.AbsoluteRow(offset, height, viewRow)
		var b strings.Builder
		for x := startCol; x <= endCol; x++ {
			if c := cellAt(x, abs); c != nil {
				b.WriteString(c.Content)
			}
		}
		rows = append(rows, strings.TrimRight(b.String(), " "))
	}
	got := strings.Join(rows, "\n")
	if got != want {
		t.Fatalf("highlighted cells joined = %q, SelectedText(%d,%d,%d,%d) = %q, want them equal", got, anchorCol, fromRow, curCol, toRow, want)
	}
}

// TestSelectionHighlightRangeReportsNoHighlightOutsideSelectedRows proves
// ok is false for a view row whose absolute row falls outside
// [fromRow, toRow] -- a row above the anchor or below the release gets no
// highlight at all, not a zero-width one.
func TestSelectionHighlightRangeReportsNoHighlightOutsideSelectedRows(t *testing.T) {
	s := newSelectionTestSession(t, 10, 3)
	if _, err := s.grid.Write([]byte("ABCDEFGHIJ\r\nKLMNOPQRST\r\nUVWXYZ")); err != nil {
		t.Fatalf("write: %v", err)
	}
	const offset, height = 0, 3
	// Selection confined to view row 1 only (columns 2-5).
	fromRow := s.AbsoluteRow(offset, height, 1)
	toRow := fromRow
	if _, _, ok := s.SelectionHighlightRange(offset, height, 0, 2, fromRow, 5, toRow); ok {
		t.Fatalf("view row 0: want no highlight, got one")
	}
	startCol, endCol, ok := s.SelectionHighlightRange(offset, height, 1, 2, fromRow, 5, toRow)
	if !ok || startCol != 2 || endCol != 5 {
		t.Fatalf("view row 1: SelectionHighlightRange = (%d,%d,%v), want (2,5,true)", startCol, endCol, ok)
	}
	if _, _, ok := s.SelectionHighlightRange(offset, height, 2, 2, fromRow, 5, toRow); ok {
		t.Fatalf("view row 2: want no highlight, got one")
	}
}

// TestAbsoluteRowMatchesRenderRowsOwnWindow proves AbsoluteRow's own
// promise directly against RenderRows: for any (offset, height, viewRow)
// RenderRows itself accepted, AbsoluteRow(offset, height, viewRow) names
// the exact absolute row whose content RenderRows placed at that
// viewRow -- resolving it via SelectedText and comparing against
// RenderRows's own returned line (with SelectedText's trailing-space
// trim applied to RenderRows's line too, since RenderRows does not trim
// on its own) is a stronger proof than re-deriving the arithmetic a
// second time by hand.
func TestAbsoluteRowMatchesRenderRowsOwnWindow(t *testing.T) {
	s := newSelectionTestSession(t, 10, 2)
	for i := 1; i <= 6; i++ {
		if _, err := s.grid.Write([]byte("LINE" + string(rune('0'+i)) + "\r\n")); err != nil {
			t.Fatalf("write line %d: %v", i, err)
		}
	}
	const offset, height = 1, 2
	rows, usedOffset := s.RenderRows(offset, height)
	if usedOffset != offset {
		t.Fatalf("test assumption violated: RenderRows clamped offset %d to %d", offset, usedOffset)
	}
	if len(rows) != height {
		t.Fatalf("RenderRows returned %d rows, want %d", len(rows), height)
	}
	for viewRow, wantLine := range rows {
		abs := s.AbsoluteRow(offset, height, viewRow)
		got := s.SelectedText(0, abs, 9, abs)
		want := strings.TrimRight(wantLine, " ")
		if got != want {
			t.Fatalf("viewRow %d: AbsoluteRow(%d,%d,%d)=%d, SelectedText there = %q, want %q (RenderRows's own line)", viewRow, offset, height, viewRow, abs, got, want)
		}
	}
}
