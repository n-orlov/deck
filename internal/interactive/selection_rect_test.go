package interactive

import "testing"

// rectFixture is a 10x5 grid:
//
//	row 0  ABCDEFGHIJ
//	row 1  KLM
//	row 2  (blank)
//	row 3  UVWXYZ
func rectFixture(t *testing.T) *Session {
	t.Helper()
	s := newSelectionTestSession(t, 10, 5)
	if _, err := s.grid.Write([]byte("ABCDEFGHIJ\r\nKLM\r\n\r\nUVWXYZ")); err != nil {
		t.Fatalf("write: %v", err)
	}
	return s
}

// TestSelectedRectTextCornersInAllFourDirections: the block between two
// corners is the same whichever corner the drag began at.
func TestSelectedRectTextCornersInAllFourDirections(t *testing.T) {
	s := rectFixture(t)
	want := "CDE\nM"
	// The rectangle covers columns 2-4 of rows 0-1.
	corners := []struct {
		name                   string
		fromCol, fromRow, tCol int
		toRow                  int
	}{
		{"down-right", 2, 0, 4, 1},
		{"down-left", 4, 0, 2, 1},
		{"up-right", 2, 1, 4, 0},
		{"up-left", 4, 1, 2, 0},
	}
	for _, c := range corners {
		if got := s.SelectedRectText(c.fromCol, c.fromRow, c.tCol, c.toRow); got != want {
			t.Errorf("%s: SelectedRectText = %q, want %q", c.name, got, want)
		}
	}
}

// TestSelectedRectTextIsNotLinear: unlike SelectedText, a middle row is
// confined to the column range.
func TestSelectedRectTextIsNotLinear(t *testing.T) {
	s := rectFixture(t)
	if got, want := s.SelectedRectText(1, 0, 3, 3), "BCD\nLM\n\nVWX"; got != want {
		t.Fatalf("SelectedRectText = %q, want %q", got, want)
	}
}

// TestSelectedRectTextTrimsTrailingBlanksAndNeverPads: a row that ends
// before the range gives an empty line, one that ends inside it a shorter,
// unpadded line.
func TestSelectedRectTextTrimsTrailingBlanksAndNeverPads(t *testing.T) {
	s := rectFixture(t)
	if got, want := s.SelectedRectText(1, 1, 8, 3), "LM\n\nVWXYZ"; got != want {
		t.Fatalf("SelectedRectText = %q, want %q", got, want)
	}
	if got, want := s.SelectedRectText(5, 1, 8, 1), ""; got != want {
		t.Fatalf("short row beyond its end = %q, want empty", got)
	}
	if got, want := s.SelectedRectText(0, 2, 8, 2), ""; got != want {
		t.Fatalf("blank row = %q, want empty", got)
	}
}

// TestSelectedRectTextWideGlyphAtTheEdges: a double-width glyph whose left
// cell is inside the range is whole; one whose left cell is left of the
// range is left out, never halved.
func TestSelectedRectTextWideGlyphAtTheEdges(t *testing.T) {
	s := newSelectionTestSession(t, 10, 3)
	// Cells: a=0 你=1-2 b=3 好=4-5 c=6
	if _, err := s.grid.Write([]byte("a你b好c")); err != nil {
		t.Fatalf("write: %v", err)
	}
	// Left edge at the glyph's right half (col 2): glyph excluded.
	if got, want := s.SelectedRectText(2, 0, 3, 0), "b"; got != want {
		t.Errorf("left edge straddling: %q, want %q", got, want)
	}
	// Right edge at the glyph's left half (col 4): whole glyph included.
	if got, want := s.SelectedRectText(3, 0, 4, 0), "b好"; got != want {
		t.Errorf("right edge straddling: %q, want %q", got, want)
	}
	// Both: left edge in 你's right half, right edge in 好's left half.
	if got, want := s.SelectedRectText(2, 0, 4, 0), "b好"; got != want {
		t.Errorf("both edges straddling: %q, want %q", got, want)
	}
	// Right edge in the right half of the glyph: still whole.
	if got, want := s.SelectedRectText(3, 0, 5, 0), "b好"; got != want {
		t.Errorf("right edge at glyph's right cell: %q, want %q", got, want)
	}
}

// TestSelectedRectTextClampsToTheGrid: corners past the grid are pulled in.
func TestSelectedRectTextClampsToTheGrid(t *testing.T) {
	s := rectFixture(t)
	if got, want := s.SelectedRectText(8, -3, 99, 1), "IJ\n"; got != want {
		t.Fatalf("SelectedRectText = %q, want %q", got, want)
	}
}

// TestSelectionRectHighlightRangeMatchesSelectedRectText: every covered row
// carries the same column range, other rows none.
func TestSelectionRectHighlightRangeMatchesSelectedRectText(t *testing.T) {
	s := rectFixture(t)
	const offset, height = 0, 5
	row := func(v int) int { return s.AbsoluteRow(offset, height, v) }
	// Corners given bottom-right to top-left.
	fromCol, fromRow, toCol, toRow := 4, row(3), 2, row(1)
	for view := 0; view < height; view++ {
		start, end, ok := s.SelectionRectHighlightRange(offset, height, view, fromCol, fromRow, toCol, toRow)
		covered := view >= 1 && view <= 3
		if ok != covered {
			t.Fatalf("view row %d: ok = %v, want %v", view, ok, covered)
		}
		if ok && (start != 2 || end != 4) {
			t.Fatalf("view row %d: range = %d-%d, want 2-4", view, start, end)
		}
	}
}
