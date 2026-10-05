package tui

import tea "github.com/charmbracelet/bubbletea"

// onMouseMsg is Update's handler for a mouse report.
func (m Model) onMouseMsg(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	m.cancelAutoEnter()
	// [ui] mouse / DECK_MOUSE (requirement 3, 37): bubbletea's own input
	// reader decodes an SGR/X10 mouse report from raw input bytes
	// unconditionally, regardless of whether tea.WithMouseCellMotion
	// was passed at startup (that option only controls whether the
	// *enable* escape sequence is written to the terminal in the first
	// place) -- a real, compliant terminal simply never emits a mouse
	// report deck did not ask for, but this second, product-side gate
	// makes the opt-out authoritative even if one arrives anyway (a
	// terminal that ignores the missing enable sequence, or a replayed
	// byte stream), rather than relying solely on a well-behaved
	// terminal's cooperation.
	if !m.settings.Mouse {
		return m, nil
	}
	// PRD II-51: the wheel scrolls the interactive grid's own bounded
	// scrollback while interactive mode owns the keyboard, the one
	// mouse gesture interactive mode accepts at all -- every other
	// gesture (press/drag/release/double-click) stays a no-op below,
	// exactly as the whole of interactive mode already was before this
	// (a click over a live pane makes no more sense here than it does
	// over the passive preview in list mode, which also ignores
	// clicks).
	if m.interactive {
		return m.onInteractiveMouse(msg)
	}
	// R73 (issue #7): a wheel notch over one of the three scrollable
	// overlays scrolls THAT overlay's own viewport, by the same one-line
	// step up/down and j/k bind. This is tested BEFORE the blanket
	// suppression below -- "scrollable overlay AND wheel event" and
	// nothing else -- so the action-suppressing rule underneath stays
	// exactly as strict as it was for every other gesture: a click, a
	// drag, a release or any other button still returns early for all
	// fifteen overlay flags, and a wheel notch over one of the twelve
	// unscrollable overlays still does nothing either (scrollWheelOverlay
	// reports false and this falls through to that same return).
	//
	// Why no hit test on msg.X/msg.Y: an overlay is modal -- it owns the
	// keyboard outright and nothing underneath it is reachable while it is
	// up -- so there is no second thing a wheel notch could have been
	// meant for, exactly as PgUp/PgDn need no pointer to decide what they
	// page. (Interactive mode's wheel, handled above, DOES hit-test,
	// because there the sidebar next to the pane is genuinely live.)
	//
	// SPEC.md:1250 is not violated: it forbids the mouse *cancelling or
	// confirming* a dialog and forbids reaching a dialog action by mouse
	// alone. Moving a read-only viewport cancels nothing, confirms
	// nothing, takes no focus and moves no selection -- the same test
	// §11.8 already applies to drag-to-select over the preview ("selecting
	// text is reading rather than acting").
	if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
		dir := 1
		if msg.Button == tea.MouseButtonWheelUp {
			dir = -1
		}
		if scrolled, ok := m.scrollWheelOverlay(dir); ok {
			return scrolled, nil
		}
	}
	// SPEC §11.4/§11.8: the mouse can neither cancel nor confirm a dialog,
	// and no dialog action is reachable by mouse alone, so every overlay
	// that already makes the bare-letter keymap a no-op ignores the mouse
	// exactly the same way.
	if m.mouseBlockedByOverlay() {
		return m, nil
	}
	return m.handleMouse(msg)
}

// mouseSuppressingOverlays are the layers that own the mouse outright: while
// any is open, every gesture but a wheel notch over a scrollable overlay is
// a no-op.
var mouseSuppressingOverlays = []func(Model) bool{
	func(m Model) bool { return m.help },
	func(m Model) bool { return m.creating },
	func(m Model) bool { return m.profileSwitching },
	func(m Model) bool { return m.pinning },
	func(m Model) bool { return m.detail },
	func(m Model) bool { return m.renaming },
	func(m Model) bool { return m.launchInputsEditing },
	func(m Model) bool { return m.themePicking },
	func(m Model) bool { return m.settingsOpen },
	func(m Model) bool { return m.settingsDiscardConfirm },
	func(m Model) bool { return m.envEditing },
	func(m Model) bool { return m.restartChoosing },
	func(m Model) bool { return m.deleteConfirming },
	func(m Model) bool { return m.archiveConfirming },
	func(m Model) bool { return m.eventLogOpen },
	func(m Model) bool { return m.filtering },
	func(m Model) bool { return m.interactive },
	func(m Model) bool { return m.lostAttach },
}

// mouseBlockedByOverlay reports whether any layer in mouseSuppressingOverlays is open.
func (m Model) mouseBlockedByOverlay() bool {
	for _, open := range mouseSuppressingOverlays {
		if open(m) {
			return true
		}
	}
	return false
}

// onInteractiveMouse is the mouse while interactive mode owns the keyboard.
func (m Model) onInteractiveMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
		return m.onInteractiveWheel(msg)
	}
	// Task 313/R54, SPEC §11.8: hit-test a left PRESS first, before
	// ever assuming it is task 216's drag-to-copy gesture -- a press
	// that resolves to the sidebar is resolved by the SAME shared
	// resolver list mode's own handleMousePress calls (task 005/#33,
	// R138): a header press toggles that group's collapse (and
	// persists it) and a collapsed-strip press restores the previous
	// non-collapsed mode, byte-identically to list mode; a row hit
	// re-targets interactive mode onto that session instead (leaving
	// the current one, restoring its window geometry byte-exact, then
	// entering the new one; a press on the row that is ALREADY the
	// interactive target is a no-op: no leave, no re-enter, no
	// resize). A press over the preview or the seam falls straight
	// through, unchanged, to the drag-to-copy path below.
	//
	// R144/GH #37: a press that resolves to the sidebar but to none
	// of resolveSidebarPress's three targets (hitTargetNone -- the
	// blank padding below the last row, or a border row) is list
	// mode's own no-op (TestClickSidebarPaddingBelowLastRowIsANoOp),
	// but while interactive mode owns the keyboard there is no row
	// to fall through to, and letting it reach drag-to-copy below
	// would silently start a text selection over blank sidebar
	// space that gesture was never meant to cover. SPEC names
	// Ctrl+Q as the only deliberate way out; this gives the operator
	// a second, equally deliberate one -- a press on empty sidebar
	// space runs the exact same exitInteractive Ctrl+Q itself
	// calls, never a second, divergent teardown.
	if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress {
		if updated, cmd, ok := m.interactiveSidebarPress(msg); ok {
			return updated, cmd
		}
	}
	// Steer 017 item 3/task 216, SPEC §11.8: a left-button drag
	// beginning inside the interactive preview's own content box
	// selects text; releasing after a genuine drag copies it. Every
	// OTHER gesture (a plain click, any button but left, anything
	// outside the content box) stays the no-op interactive mode
	// already made every non-wheel gesture before this.
	return m.interactiveSelectionMouse(msg)
}

// onInteractiveWheel is a wheel notch while interactive.
func (m Model) onInteractiveWheel(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// R149/GH #47: a wheel over the sidebar while interactive does
	// exactly what list mode's own scrollSidebar (mouse.go:208)
	// does -- move the sidebar viewport and arm
	// m.sidebarScrollDrifted, never the selection, the interactive
	// target or the grid. A wheel over the preview still scrolls
	// the interactive scrollback exactly as before.
	switch hit := m.hitTest(msg.X, msg.Y); hit.panel {
	case hitPanelSidebar:
		delta := 1
		if msg.Button == tea.MouseButtonWheelUp {
			delta = -1
		}
		return m.scrollSidebar(msg, delta), nil
	case hitPanelPreview:
		// R183/GH #59: a program that tracks the mouse owns
		// the notch (interactive_wheel.go); otherwise it
		// scrolls the grid, as before.
		if forwarded, handled := m.forwardInteractiveWheel(msg); handled {
			return forwarded, nil
		}
		delta := interactiveWheelStepLines
		if msg.Button == tea.MouseButtonWheelDown {
			delta = -delta
		}
		return m.scrollInteractiveByLines(delta)
	}
	return m, nil
}

// interactiveSidebarPress handles a left press that resolves to the sidebar; ok is false when the press is elsewhere.
func (m Model) interactiveSidebarPress(msg tea.MouseMsg) (tea.Model, tea.Cmd, bool) {
	hit := m.hitTest(msg.X, msg.Y)
	if hit.panel != hitPanelSidebar {
		return m, nil, false
	}
	if updated, cmd, ok := m.resolveSidebarPress(hit, func(mm Model, h hitResult) (tea.Model, tea.Cmd) {
		return mm.retargetInteractiveSidebarClick(h.sessionIndex)
	}); ok {
		return updated, cmd, true
	}
	updated, cmd := m.exitInteractive()
	return updated, cmd, true
}

// interactiveSelectionMouse is the interactive preview's non-wheel mouse: a
// left-button drag selects and copies text; a click of any button is
// forwarded to a program that tracks the mouse (R202, SPEC §11.8).
func (m Model) interactiveSelectionMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch msg.Button {
	case tea.MouseButtonLeft:
		return m.interactiveLeftMouse(msg), nil
	case tea.MouseButtonMiddle, tea.MouseButtonRight:
		return m.interactiveOtherButtonClick(msg), nil
	}
	return m, nil
}

// interactiveLeftMouse: press arms a selection, motion makes it a drag, and
// the release either copies the drag or, when no motion happened, is a click
// forwarded at the press cell.
//
// With [ui] select_on_drag false the left button is the pane program's
// instead (interactive_drag_forward.go): a press without Shift, the motion
// that follows it and the release are forwarded as reports, and nothing is
// selected. Shift on the press keeps the selection route.
func (m Model) interactiveLeftMouse(msg tea.MouseMsg) Model {
	switch msg.Action {
	case tea.MouseActionPress:
		if !m.settings.SelectOnDrag && !msg.Shift {
			return m.forwardInteractiveDragPress(msg)
		}
		if updated, ok := m.beginInteractiveSelection(msg.X, msg.Y); ok {
			updated.interactiveSelectRect = rectangularSelectionPress(msg)
			return updated
		}
	case tea.MouseActionMotion:
		if m.interactiveSelecting {
			return m.updateInteractiveSelection(msg.X, msg.Y)
		}
		return m.forwardInteractiveDragStep(msg, mouseReportMotion)
	case tea.MouseActionRelease:
		if m.interactiveSelecting {
			return m.commitInteractiveSelection(msg.Shift)
		}
		return m.forwardInteractiveDragStep(msg, mouseReportRelease)
	}
	return m
}

// interactiveOtherButtonClick forwards a middle or right click as a press
// and release at the press cell; the release event itself carries nothing
// more to say.
func (m Model) interactiveOtherButtonClick(msg tea.MouseMsg) Model {
	if msg.Action != tea.MouseActionPress {
		return m
	}
	col, row, ok := m.previewCellAt(msg.X, msg.Y)
	if !ok {
		return m
	}
	button := mouseButtonCodeRight
	if msg.Button == tea.MouseButtonMiddle {
		button = mouseButtonCodeMiddle
	}
	return m.forwardInteractiveClick(button, msg.Shift, col, row)
}
