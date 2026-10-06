package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/interactive"
)

// R202/GH #67, SPEC §11.8: over the interactive preview every mouse event
// that is not an ON-mode left selection drag belongs to the pane program.
// A button's press, the motion while it is held, its release, motion with
// no button held, and the horizontal wheel are encoded as mouse reports and
// sent to a program that tracks the mouse, by the same routing rule as the
// vertical wheel (mouseForwardModes). With [ui] select_on_drag false the
// left button is the program's too. Nothing forwarded is selected,
// highlighted or copied, and a program that does not track the mouse
// receives nothing.

// mouseButtonCodeNone is the button code of motion with no button held.
const mouseButtonCodeNone = 3

// mouseButtonCodeExtraBase is the first code of the additional buttons
// (8-11): SGR (1006) only, X10 has no form for them.
const mouseButtonCodeExtraBase = 128

// forwardedButtonCode is the protocol code of a button that has a
// press/motion/release lifecycle or is a horizontal wheel notch. The
// vertical wheel is onInteractiveWheel's.
func forwardedButtonCode(b tea.MouseButton) (int, bool) {
	switch b {
	case tea.MouseButtonLeft:
		return mouseButtonCodeLeft, true
	case tea.MouseButtonMiddle:
		return mouseButtonCodeMiddle, true
	case tea.MouseButtonRight:
		return mouseButtonCodeRight, true
	case tea.MouseButtonWheelLeft:
		return mouseButtonCodeWheelLeft, true
	case tea.MouseButtonWheelRight:
		return mouseButtonCodeWheelRight, true
	case tea.MouseButtonBackward, tea.MouseButtonForward, tea.MouseButton10, tea.MouseButton11:
		return mouseButtonCodeExtraBase + int(b-tea.MouseButtonBackward), true
	}
	return 0, false
}

// motionReportsWanted: modes 1002 (button-event) and 1003 (any-event) ask
// for motion while a button is held; plain 1000 asks for press and release
// only.
func motionReportsWanted(modes interactive.MouseMode) bool {
	return modes.Has(interactive.MouseButton) || modes.Has(interactive.MouseAny)
}

// buttonBit is the held-button mask bit of a button.
func buttonBit(b tea.MouseButton) uint16 { return 1 << uint(b) }

// normalizeInteractiveRelease gives an X10 release, which cannot say which
// button came up (bubbletea reports it as no button), the button it ends:
// the selection's left press, else a forwarded button still held. Each
// anonymous release ends exactly one held gesture, so with several buttons
// down every one of them still gets its release.
func (m Model) normalizeInteractiveRelease(msg tea.MouseMsg) tea.MouseMsg {
	if msg.Action != tea.MouseActionRelease || msg.Button != tea.MouseButtonNone {
		return msg
	}
	if m.interactiveSelecting {
		msg.Button = tea.MouseButtonLeft
	} else if held, ok := m.heldForwardedButton(); ok {
		msg.Button = held
	}
	return msg
}

// heldForwardedButton is the forwarded button an anonymous release ends:
// the last one pressed while it is still held, else the held button with
// the lowest code. ok is false when no forwarded button is held.
func (m Model) heldForwardedButton() (tea.MouseButton, bool) {
	held := m.interactiveForwardedButtons
	if held == 0 {
		return tea.MouseButtonNone, false
	}
	if held&buttonBit(m.interactiveForwardedLast) != 0 {
		return m.interactiveForwardedLast, true
	}
	b := tea.MouseButtonNone
	for held&buttonBit(b) == 0 {
		b++
	}
	return b, true
}

// interactiveForwardedMouse is every interactive-preview mouse event that
// is not the left button's selection route: the other buttons, motion with
// no button and the horizontal wheel.
func (m Model) interactiveForwardedMouse(msg tea.MouseMsg) Model {
	if msg.Button == tea.MouseButtonNone {
		return m.forwardInteractiveHover(msg)
	}
	code, ok := forwardedButtonCode(msg.Button)
	if !ok {
		return m
	}
	if code == mouseButtonCodeWheelLeft || code == mouseButtonCodeWheelRight {
		return m.forwardInteractiveHorizontalWheel(msg, code)
	}
	switch msg.Action {
	case tea.MouseActionPress:
		return m.forwardInteractiveButtonPress(msg)
	case tea.MouseActionMotion:
		return m.forwardInteractiveButtonStep(msg, mouseReportMotion)
	case tea.MouseActionRelease:
		return m.forwardInteractiveButtonStep(msg, mouseReportRelease)
	}
	return m
}

// forwardInteractiveButtonPress forwards the press of a button that began
// inside the preview's content box and arms the forwarding of the rest of
// its gesture. A press elsewhere, one with Shift held, or one the program
// does not want, arms nothing or sends nothing.
func (m Model) forwardInteractiveButtonPress(msg tea.MouseMsg) Model {
	if !m.interactive || m.interactiveGrid == nil || msg.Shift {
		return m
	}
	col, row, ok := m.previewCellAt(msg.X, msg.Y)
	if !ok {
		return m
	}
	code, _ := forwardedButtonCode(msg.Button)
	m.interactiveForwardedButtons |= buttonBit(msg.Button)
	m.interactiveForwardedLast = msg.Button
	return m.sendMouseReport(code, mouseReportPress, false, col, row, false)
}

// forwardInteractiveButtonStep forwards the motion or release of a
// forwarded button at the cell the pointer is over, clamped into the
// content box so a gesture that runs off the panel's edge keeps reporting
// its nearest cell. The release ends the gesture whether or not anything
// could be sent. Shift was judged at the press: a gesture already handed
// to the program is finished for it, so a Shift pressed mid-drag cannot
// strand a release.
func (m Model) forwardInteractiveButtonStep(msg tea.MouseMsg, kind mouseReportKind) Model {
	bit := buttonBit(msg.Button)
	if m.interactiveForwardedButtons&bit == 0 {
		return m
	}
	if kind == mouseReportRelease {
		m.interactiveForwardedButtons &^= bit
	}
	code, _ := forwardedButtonCode(msg.Button)
	col, row := m.previewClampToContent(msg.X, msg.Y)
	return m.sendMouseReport(code, kind, false, col, row, false)
}

// forwardInteractiveHover forwards motion with no button held, which only a
// program in any-event mode (1003) asks for. It is reported at the cell
// under the pointer and only over the preview's content box.
func (m Model) forwardInteractiveHover(msg tea.MouseMsg) Model {
	if msg.Action != tea.MouseActionMotion {
		return m
	}
	col, row, ok := m.previewCellAt(msg.X, msg.Y)
	if !ok {
		return m
	}
	return m.sendMouseReport(mouseButtonCodeNone, mouseReportMotion, msg.Shift, col, row, true)
}

// forwardInteractiveHorizontalWheel forwards a sideways wheel notch as a
// press-shaped report. Deck has no horizontal scroll of its own, so a
// program that does not track the mouse simply gets nothing.
func (m Model) forwardInteractiveHorizontalWheel(msg tea.MouseMsg, code int) Model {
	col, row, ok := m.previewCellAt(msg.X, msg.Y)
	if !ok {
		return m
	}
	return m.sendMouseReport(code, mouseReportPress, msg.Shift, col, row, false)
}

// sendMouseReport encodes and sends one report when the routing rule lets
// it through. Motion is further limited to programs that asked for it:
// button motion to modes 1002 and 1003, hover to 1003.
func (m Model) sendMouseReport(code int, kind mouseReportKind, shift bool, col, row int, hover bool) Model {
	modes, forwards := m.mouseForwardModes(shift)
	if !forwards || (kind == mouseReportMotion && !motionWanted(modes, hover)) {
		return m
	}
	report, ok := encodeMouseReport(modes, code, kind, col, row)
	if !ok {
		return m
	}
	return m.sendMouseBytes(report)
}

// motionWanted reports whether the program's modes ask for this motion.
func motionWanted(modes interactive.MouseMode, hover bool) bool {
	if hover {
		return modes.Has(interactive.MouseAny)
	}
	return motionReportsWanted(modes)
}
