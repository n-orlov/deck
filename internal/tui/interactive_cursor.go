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
	for i := 0; i < len(line); {
		if line[i] == 0x1b {
			n := ansiEscapeLen(line, i)
			out.WriteString(line[i : i+n])
			i += n
			continue
		}
		r, size := utf8.DecodeRuneInString(line[i:])
		w := cellWidth(r)
		if !done && col >= cur && col < cur+max(w, 1) {
			out.WriteString(cursorOpenSGR)
			out.WriteString(line[i : i+size])
			out.WriteString(cursorCloseSGR)
			done = true
		} else {
			out.WriteString(line[i : i+size])
		}
		cur += w
		i += size
	}
	if !done {
		if col > cur {
			out.WriteString(strings.Repeat(" ", col-cur))
		}
		out.WriteString(cursorOpenSGR + " " + cursorCloseSGR)
	}
	return out.String()
}
