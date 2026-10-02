package tui

import (
	"context"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/interactive"
)

// R183/GH #59, SPEC §11.8/§11.9: over the interactive preview a wheel notch
// is the pane program's own input when that program has turned mouse
// reporting on, and scrolls deck's grid only otherwise.

// wheelForwardsToProgram is the routing rule. A notch is forwarded when
// the pane program tracks the mouse (1000, 1002 or 1003), the grid is at
// live (offset 0: a scrolled-back view is deck's own and the notch moves
// it) and Shift is not held (Shift is the terminal convention for "do not
// give this to the program", so Shift+wheel always scrolls the grid).
func wheelForwardsToProgram(modes interactive.MouseMode, offset int, shift bool) bool {
	reporting := modes.Has(interactive.MouseNormal) || modes.Has(interactive.MouseButton) || modes.Has(interactive.MouseAny)
	return reporting && offset == 0 && !shift
}

// x10CoordMax is the largest 1-based column or row an X10 report can
// carry: each coordinate is one byte holding 32+value, and 255-32 = 223.
const x10CoordMax = 223

// encodeWheelReport returns the bytes a terminal would send the program for
// one wheel notch at 0-based pane cell (col, row): button 64 for a notch up
// and 65 for a notch down. Under mode 1006 it is the SGR report
// `ESC [ < b ; col+1 ; row+1 M`; otherwise the X10 report `ESC [ M` then
// the three bytes 32+b, 32+col+1, 32+row+1, which only exists while both
// 1-based coordinates are at most 223. A notch past that under X10, and any
// notch under 1005 (UTF-8 coordinates) or 1015 (urxvt decimal), which deck
// does not encode, reports ok == false and is dropped, never guessed at.
// The caller has already established that reporting is on.
func encodeWheelReport(modes interactive.MouseMode, up bool, col, row int) (report []byte, ok bool) {
	button := 65
	if up {
		button = 64
	}
	x, y := col+1, row+1
	if x < 1 || y < 1 {
		return nil, false
	}
	if modes.Has(interactive.MouseSGR) {
		b := make([]byte, 0, 16)
		b = append(b, 0x1b, '[', '<')
		b = strconv.AppendInt(b, int64(button), 10)
		b = append(b, ';')
		b = strconv.AppendInt(b, int64(x), 10)
		b = append(b, ';')
		b = strconv.AppendInt(b, int64(y), 10)
		b = append(b, 'M')
		return b, true
	}
	if modes.Has(interactive.MouseUTF8) || modes.Has(interactive.MouseURXVT) {
		return nil, false
	}
	if x > x10CoordMax || y > x10CoordMax {
		return nil, false
	}
	return []byte{0x1b, '[', 'M', byte(32 + button), byte(32 + x), byte(32 + y)}, true
}

// forwardInteractiveWheel handles a wheel notch over the interactive
// preview. handled is true when the notch belongs to the pane program
// (whether or not bytes could be encoded for it: a dropped notch must not
// fall through and scroll the grid under a program that asked for the
// mouse); false means the caller scrolls the grid as before.
//
// Everything it reads is already in memory: the grid's mode bits are kept
// by the vt callbacks (Grid.MouseModes), the cell comes from previewCellAt,
// and the bytes go through the interactive dispatcher's verified-identity
// `send-keys -H` path (Dispatcher.SendBytes). No tmux read is made per notch
// beyond that dispatcher's own identity check.
func (m Model) forwardInteractiveWheel(msg tea.MouseMsg) (out Model, handled bool) {
	if m.interactiveGrid == nil || m.interactiveDispatcher == nil {
		return m, false
	}
	grid := m.interactiveGrid.Grid()
	if grid == nil {
		return m, false
	}
	offset := m.interactiveScrollOffset()
	if offset > 0 {
		offset = m.healInteractiveScrollOffsetFromRender().interactiveScrollOffset()
	}
	modes := grid.MouseModes()
	if !wheelForwardsToProgram(modes, offset, msg.Shift) {
		return m, false
	}
	col, row, ok := m.previewCellAt(msg.X, msg.Y)
	if !ok {
		return m, true
	}
	report, ok := encodeWheelReport(modes, msg.Button == tea.MouseButtonWheelUp, col, row)
	if !ok {
		return m, true
	}
	// A forwarded notch is input to the pane, so it ends a sidebar wheel
	// drift exactly as a forwarded key does (updateInteractive).
	if m.sidebarScrollDrifted {
		m.revealInteractiveDriftTarget()
		m.sidebarScrollDrifted = false
	}
	_ = m.interactiveDispatcher.SendBytes(context.Background(), report)
	return m, true
}
