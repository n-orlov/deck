package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/interactive"
)

// R202c/GH #67, SPEC §11.8: with [ui] select_on_drag false a left-button
// gesture over the interactive preview belongs to the pane program. The
// press, the motion while it is held and the release are encoded as mouse
// reports and sent to a program that tracks the mouse, by the same routing
// rule as the wheel and clicks (mouseForwardModes). Nothing is selected,
// highlighted or copied, so a program that does not track the mouse
// receives nothing and the drag does nothing.

// motionReportsWanted: modes 1002 (button-event) and 1003 (any-event) ask
// for motion; plain 1000 asks for press and release only.
func motionReportsWanted(modes interactive.MouseMode) bool {
	return modes.Has(interactive.MouseButton) || modes.Has(interactive.MouseAny)
}

// forwardInteractiveDragPress forwards the press of a left drag that began
// inside the preview's content box and arms the forwarding of the rest of
// the gesture. A press elsewhere, or one the program does not want, arms
// nothing.
func (m Model) forwardInteractiveDragPress(msg tea.MouseMsg) Model {
	if !m.interactive || m.interactiveGrid == nil {
		return m
	}
	col, row, ok := m.previewCellAt(msg.X, msg.Y)
	if !ok {
		return m
	}
	m.interactiveForwardingDrag = true
	return m.sendDragReport(mouseReportPress, msg.Shift, col, row)
}

// forwardInteractiveDragStep forwards the motion or release of a forwarded
// drag at the cell the pointer is over, clamped into the content box so a
// drag that runs off the panel's edge keeps reporting its nearest cell. The
// release ends the gesture whether or not anything could be sent.
func (m Model) forwardInteractiveDragStep(msg tea.MouseMsg, kind mouseReportKind) Model {
	if !m.interactiveForwardingDrag {
		return m
	}
	if kind == mouseReportRelease {
		m.interactiveForwardingDrag = false
	}
	col, row := m.previewClampToContent(msg.X, msg.Y)
	// Shift was judged at the press; a gesture already handed to the program
	// is finished for it, so a Shift pressed mid-drag cannot strand a release.
	return m.sendDragReport(kind, false, col, row)
}

// sendDragReport encodes and sends one left-button report when the routing
// rule lets it through; motion is further limited to programs that asked
// for it.
func (m Model) sendDragReport(kind mouseReportKind, shift bool, col, row int) Model {
	modes, forwards := m.mouseForwardModes(shift)
	if !forwards || (kind == mouseReportMotion && !motionReportsWanted(modes)) {
		return m
	}
	report, ok := encodeMouseReport(modes, mouseButtonCodeLeft, kind, col, row)
	if !ok {
		return m
	}
	return m.sendMouseBytes(report)
}
