package tui

import (
	"strings"
	"unicode/utf8"
)

// Interactive cursor (R182, GH #60): the pane's own cursor cell is drawn
// in reverse video over the grid rows. The vt emulator's Render() never
// paints a cursor, so without this the preview gives no sign of where
// typed text will land.
const (
	cursorOpenSGR  = "\x1b[7m"
	cursorCloseSGR = "\x1b[27m"
)

// drawInteractiveCursor marks the cursor cell of rows in reverse video
// when, and only when, the snapshot it came from says there is a cursor to
// show: it is visible, the view is the live one (usedOffset == 0 -- a
// scrolled-back view shows history, and the live cursor's row index would
// land on unrelated content), and the cursor's row lies inside rows.
//
// cursorViewRow, cursorX and visible all come from the one
// interactive.RenderSnapshot that produced rows, so a stale-frame answer
// pairs the cached rows with the cached cursor, never a fresh one.
func drawInteractiveCursor(rows []string, usedOffset, cursorViewRow, cursorX int, visible bool) []string {
	if !visible || usedOffset != 0 || cursorX < 0 || cursorViewRow < 0 || cursorViewRow >= len(rows) {
		return rows
	}
	out := make([]string, len(rows))
	copy(out, rows)
	out[cursorViewRow] = reverseCellSGR(out[cursorViewRow], cursorX)
	return out
}

// reverseCellSGR wraps the display column col of one rendered row in
// reverse video, walking the row like highlightRangeSGR does (escape
// sequences cost no columns, every printable rune its cellWidth). A row
// that ends before col -- the emulator trims nothing, but a shorter row
// is still possible after cropping -- is padded with blanks so the cursor
// still has a cell to sit on.
func reverseCellSGR(line string, col int) string {
	var out strings.Builder
	cur := 0
	done := false
	// reverse tracks the program's own SGR 7 state, so a cursor that sits
	// inside a reverse run the program painted is left as painted instead
	// of closing the run with 27 (which would un-reverse the cursor cell
	// and every cell of the run after it).
	var (
		reverse bool
		fg, bg  sgrColor
	)
	for i := 0; i < len(line); {
		if line[i] == 0x1b {
			n := ansiEscapeLen(line, i)
			if isSGREscape(line[i : i+n]) {
				applySGR(line[i+2:i+n-1], &fg, &bg, &reverse)
			}
			out.WriteString(line[i : i+n])
			i += n
			continue
		}
		r, size := utf8.DecodeRuneInString(line[i:])
		w := cellWidth(r)
		if !done && col >= cur && col < cur+max(w, 1) {
			out.WriteString(cursorCellSGR(line[i:i+size], reverse))
			done = true
		} else {
			out.WriteString(line[i : i+size])
		}
		cur += w
		i += size
	}
	if !done {
		out.WriteString(strings.Repeat(" ", max(col-cur, 0)))
		out.WriteString(cursorCellSGR(" ", reverse))
	}
	return out.String()
}

// cursorCellSGR paints one cursor cell: the glyph wrapped in the cursor
// SGR pair, or left exactly as painted when the program's own reverse
// video already covers it.
func cursorCellSGR(glyph string, reverse bool) string {
	if reverse {
		return glyph
	}
	return cursorOpenSGR + glyph + cursorCloseSGR
}
