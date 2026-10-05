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

// mouseReportKind is which of the three things a mouse report can say.
type mouseReportKind int

const (
	mouseReportPress mouseReportKind = iota
	mouseReportRelease
	mouseReportMotion
)

// Base button codes of a mouse report: the three buttons and the two wheel
// directions, as the xterm protocol numbers them.
const (
	mouseButtonCodeLeft      = 0
	mouseButtonCodeMiddle    = 1
	mouseButtonCodeRight     = 2
	mouseButtonCodeWheelUp   = 64
	mouseButtonCodeWheelDown = 65

	// mouseMotionBit is added to the button code of a motion report;
	// x10ReleaseButton is the button code an X10 release carries, because
	// X10 cannot say which button came up.
	mouseMotionBit   = 32
	x10ReleaseButton = 3
)

// encodeMouseReport returns the bytes a terminal would send the program for
// one mouse event of the given kind at 0-based pane cell (col, row). button
// is a base code (mouseButtonCode*). Under mode 1006 it is the SGR report
// `ESC [ < b ; col+1 ; row+1 M`, where a release keeps the button's own
// code and ends in `m`, and motion adds 32 to b; otherwise the X10 report
// `ESC [ M` then the three bytes 32+b, 32+col+1, 32+row+1, where a release
// carries button code 3 and motion adds 32 to b. X10 only exists while both
// 1-based coordinates are at most 223. A report past that under X10, and any
// report under 1005 (UTF-8 coordinates) or 1015 (urxvt decimal), which deck
// does not encode, reports ok == false and is dropped, never guessed at.
// The caller has already established that reporting is on.
func encodeMouseReport(modes interactive.MouseMode, button int, kind mouseReportKind, col, row int) (report []byte, ok bool) {
	x, y := col+1, row+1
	if x < 1 || y < 1 {
		return nil, false
	}
	if modes.Has(interactive.MouseSGR) {
		return encodeSGRMouseReport(button, kind, x, y), true
	}
	if modes.Has(interactive.MouseUTF8) || modes.Has(interactive.MouseURXVT) {
		return nil, false
	}
	if x > x10CoordMax || y > x10CoordMax {
		return nil, false
	}
	code := button
	switch kind {
	case mouseReportRelease:
		code = x10ReleaseButton
	case mouseReportMotion:
		code += mouseMotionBit
	}
	return []byte{0x1b, '[', 'M', byte(32 + code), byte(32 + x), byte(32 + y)}, true
}

// encodeSGRMouseReport builds the mode-1006 report for 1-based (x, y).
func encodeSGRMouseReport(button int, kind mouseReportKind, x, y int) []byte {
	code, final := button, byte('M')
	switch kind {
	case mouseReportRelease:
		final = 'm'
	case mouseReportMotion:
		code += mouseMotionBit
	}
	b := make([]byte, 0, 16)
	b = append(b, 0x1b, '[', '<')
	b = strconv.AppendInt(b, int64(code), 10)
	b = append(b, ';')
	b = strconv.AppendInt(b, int64(x), 10)
	b = append(b, ';')
	b = strconv.AppendInt(b, int64(y), 10)
	return append(b, final)
}

// encodeWheelReport is encodeMouseReport for one wheel notch: button 64 for
// a notch up and 65 for a notch down, always a press-shaped report.
func encodeWheelReport(modes interactive.MouseMode, up bool, col, row int) (report []byte, ok bool) {
	button := mouseButtonCodeWheelDown
	if up {
		button = mouseButtonCodeWheelUp
	}
	return encodeMouseReport(modes, button, mouseReportPress, col, row)
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
