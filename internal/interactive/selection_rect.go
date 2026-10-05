package interactive

import "strings"

// SelectedRectText is SelectedText's rectangular sibling (SPEC §11.8, R203):
// the plain text of the block spanned by the two corner cells (fromCol,
// fromRow) and (toCol, toRow), in the same absolute row space SelectedText
// uses. The corners may be given in any order: columns and rows are each
// ordered independently, so a drag in any of the four directions selects
// the same block. Every row yields its cells inside the column range with
// trailing blanks trimmed and the rows are joined with "\n". A row that
// ends before the range gives a shorter, possibly empty, line and is never
// padded. A double-width glyph belongs to the cell that holds its left
// half, so one whose left cell is inside the range is returned whole and
// one whose left cell is outside it is not returned at all: half a glyph is
// never emitted.
func (s *Session) SelectedRectText(fromCol, fromRow, toCol, toRow int) string {
	fromCol, toCol = orderPair(fromCol, toCol)
	fromRow, toRow = orderPair(fromRow, toRow)
	s.mu.RLock()
	defer s.mu.RUnlock()
	s.writes.RLock()
	defer s.writes.RUnlock()
	g := s.grid
	width := g.Width()
	sbLen := g.ScrollbackLen()
	total := sbLen + g.Height()
	if total <= 0 || width <= 0 {
		return ""
	}
	fromRow, toRow = clampIndex(fromRow, total), clampIndex(toRow, total)
	fromCol, toCol = clampIndex(fromCol, width), clampIndex(toCol, width)

	lines := make([]string, 0, toRow-fromRow+1)
	for i := fromRow; i <= toRow; i++ {
		lines = append(lines, selectedRowText(g, sbLen, i, fromCol, toCol))
	}
	return strings.Join(lines, "\n")
}

// SelectionRectHighlightRange is SelectionHighlightRange for a rectangular
// selection: for viewRow it names the column range [startCol, endCol]
// (inclusive) the rectangle spanned by the two corners covers, and ok
// reports whether the row carries any of it. The range is the same on every
// covered row, so the marking and SelectedRectText cannot disagree.
func (s *Session) SelectionRectHighlightRange(offset, height, viewRow, fromCol, fromRow, toCol, toRow int) (startCol, endCol int, ok bool) {
	fromCol, toCol = orderPair(fromCol, toCol)
	fromRow, toRow = orderPair(fromRow, toRow)
	abs := s.AbsoluteRow(offset, height, viewRow)
	if abs < fromRow || abs > toRow {
		return 0, 0, false
	}
	width := s.Grid().Width()
	if width <= 0 {
		return 0, 0, false
	}
	return clampIndex(fromCol, width), clampIndex(toCol, width), true
}

// orderPair returns a and b in ascending order.
func orderPair(a, b int) (int, int) {
	if a > b {
		return b, a
	}
	return a, b
}
