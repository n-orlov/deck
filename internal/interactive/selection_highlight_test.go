package interactive

import "testing"

// TestSelectionHighlightRangeIsDirectionIndependent proves a drag
// performed backwards -- a release above its own press, or leftward on the
// same row -- highlights exactly the cells the same drag performed forwards
// does (SPEC §11.8: the highlight covers the run SelectedText would
// return, and SelectedText swaps a backwards drag first).
func TestSelectionHighlightRangeIsDirectionIndependent(t *testing.T) {
	s := newSelectionTestSession(t, 10, 3)
	if _, err := s.grid.Write([]byte("ABCDEFGHIJ\r\nKLMNOPQRST\r\nUVWXYZ")); err != nil {
		t.Fatalf("write: %v", err)
	}
	const offset, height = 0, 3
	row := func(v int) int { return s.AbsoluteRow(offset, height, v) }
	cases := []struct {
		name                             string
		fromCol, fromView, toCol, toView int
	}{
		{"leftward on one row", 2, 1, 6, 1},
		{"upward across rows", 3, 0, 7, 2},
	}
	for _, tc := range cases {
		fromRow, toRow := row(tc.fromView), row(tc.toView)
		for viewRow := 0; viewRow < height; viewRow++ {
			fs, fe, fok := s.SelectionHighlightRange(offset, height, viewRow, tc.fromCol, fromRow, tc.toCol, toRow)
			bs, be, bok := s.SelectionHighlightRange(offset, height, viewRow, tc.toCol, toRow, tc.fromCol, fromRow)
			if fs != bs || fe != be || fok != bok {
				t.Fatalf("%s, view row %d: forward = (%d,%d,%v), backward = (%d,%d,%v), want identical", tc.name, viewRow, fs, fe, fok, bs, be, bok)
			}
		}
	}
	// And the swap really happened: the backwards same-row drag still
	// yields the forward range, not an empty one.
	r := row(1)
	if st, en, ok := s.SelectionHighlightRange(offset, height, 1, 6, r, 2, r); !ok || st != 2 || en != 6 {
		t.Fatalf("backwards same-row drag = (%d,%d,%v), want (2,6,true)", st, en, ok)
	}
}

// TestSelectionHighlightRangeClampsColumnsToTheGrid proves a drag whose
// endpoint columns lie outside the grid (a cell reported left of column 0
// or past the right edge) highlights the nearest real columns, never an
// out-of-range one.
func TestSelectionHighlightRangeClampsColumnsToTheGrid(t *testing.T) {
	s := newSelectionTestSession(t, 10, 3)
	const offset, height = 0, 3
	from, to := s.AbsoluteRow(offset, height, 0), s.AbsoluteRow(offset, height, 2)
	if st, en, ok := s.SelectionHighlightRange(offset, height, 0, -4, from, 99, to); !ok || st != 0 || en != 9 {
		t.Fatalf("first row of (-4 .. 99) = (%d,%d,%v), want (0,9,true)", st, en, ok)
	}
	if st, en, ok := s.SelectionHighlightRange(offset, height, 2, -4, from, 99, to); !ok || st != 0 || en != 9 {
		t.Fatalf("last row of (-4 .. 99) = (%d,%d,%v), want (0,9,true)", st, en, ok)
	}
	if st, en, ok := s.SelectionHighlightRange(offset, height, 0, 12, from, 15, from); !ok || st != 9 || en != 9 {
		t.Fatalf("both columns past the right edge = (%d,%d,%v), want (9,9,true)", st, en, ok)
	}
}

// TestSelectionHighlightRangeOnAZeroWidthGridHighlightsNothing proves a
// grid with no columns reports no highlight rather than a range into
// cells that do not exist.
func TestSelectionHighlightRangeOnAZeroWidthGridHighlightsNothing(t *testing.T) {
	s := newSelectionTestSession(t, 0, 3)
	if st, en, ok := s.SelectionHighlightRange(0, 3, 0, 0, 0, 5, 2); ok {
		t.Fatalf("zero-width grid highlighted (%d,%d), want no highlight", st, en)
	}
}

// TestOrderSelectionEndsSwapsOnlyBackwardsDrags pins the endpoint
// ordering both SelectedText and SelectionHighlightRange share.
func TestOrderSelectionEndsSwapsOnlyBackwardsDrags(t *testing.T) {
	cases := []struct {
		name                           string
		fc, fr, tc, tr                 int
		wantFC, wantFR, wantTC, wantTR int
	}{
		{"forward across rows", 1, 0, 5, 2, 1, 0, 5, 2},
		{"forward on one row", 1, 3, 5, 3, 1, 3, 5, 3},
		{"single cell", 4, 3, 4, 3, 4, 3, 4, 3},
		{"upward", 5, 2, 1, 0, 1, 0, 5, 2},
		{"leftward on one row", 5, 3, 1, 3, 1, 3, 5, 3},
	}
	for _, c := range cases {
		a, b, d, e := orderSelectionEnds(c.fc, c.fr, c.tc, c.tr)
		if a != c.wantFC || b != c.wantFR || d != c.wantTC || e != c.wantTR {
			t.Errorf("%s: orderSelectionEnds(%d,%d,%d,%d) = (%d,%d,%d,%d), want (%d,%d,%d,%d)", c.name, c.fc, c.fr, c.tc, c.tr, a, b, d, e, c.wantFC, c.wantFR, c.wantTC, c.wantTR)
		}
	}
}
