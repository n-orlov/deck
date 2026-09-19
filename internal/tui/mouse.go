package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// SPEC §11.8 mouse navigation: every binding here duplicates a key from
// §11's keymap and the key remains the primary, documented path (no
// capability below is ever mouse-only). Hit-testing resolves a click by
// consulting the same layout (ComputeLayout, task 011) and the same sidebar
// content (sidebarEntries, task 028) the renderer itself drew from — there
// is exactly one geometry implementation, never a second one recomputed
// independently for the mouse.

// hitPanel names which of the two panels (or the seam between them) a
// terminal cell belongs to.
type hitPanel int

const (
	hitPanelNone hitPanel = iota
	hitPanelSidebar
	hitPanelPreview
	hitPanelSeam
)

// hitTarget further refines a hitPanelSidebar hit into what, specifically,
// is at that cell.
type hitTarget int

const (
	hitTargetNone hitTarget = iota
	hitTargetRow
	hitTargetHeader
	hitTargetCollapsedStrip
)

// hitResult is hitTest's answer for one (x, y) cell. groupID (task 013/
// R129 part 3) is only meaningful when target is hitTargetHeader -- the
// header's durable group identity, never its display name, since a
// header hit exists to drive toggleGroupCollapse, which is itself keyed
// by id (sessionGroupID).
type hitResult struct {
	panel        hitPanel
	target       hitTarget
	sessionIndex int
	groupID      int64
}

// hitTest resolves one absolute terminal cell (as bubbletea's tea.MouseMsg
// reports it, 0-indexed) to whatever mainView actually drew there this
// frame. It reads m.frameSize/m.computeLayout/m.startupBanner/
// m.sidebarVisibleEntries — the exact same calls mainView's own render path
// makes — rather than re-deriving panel offsets from scratch, which is what
// SPEC §11.8 calls out as the failure mode that silently selects the wrong
// row the moment grouping, elision or a mode change touches only one of two
// independent implementations.
func (m Model) hitTest(x, y int) hitResult {
	if x < 0 || y < 0 {
		return hitResult{}
	}
	width, _ := m.frameSize()
	banner := len(m.startupBanner(width)) + len(m.themeBanner(width))
	frameY := y - banner
	if frameY < 0 {
		return hitResult{}
	}
	layout := m.computeLayout()
	if layout.Effective == LayoutStacked {
		return m.hitTestStacked(layout, x, frameY)
	}
	return m.hitTestSideBySide(layout, x, frameY)
}

// hitTestSideBySide covers both the side-by-side and collapsed layouts
// (mainView's renderSideBySideFrame draws both the same way: one shared
// seam, sidebar on the left).
func (m Model) hitTestSideBySide(layout LayoutResult, x, y int) hitResult {
	sw, height := layout.Sidebar.Width, layout.Sidebar.Height
	if y >= height {
		return hitResult{}
	}
	collapsed := layout.Effective == LayoutCollapsed
	if x >= sw {
		if x == sw && !collapsed {
			return hitResult{panel: hitPanelSeam}
		}
		return hitResult{panel: hitPanelPreview}
	}
	if collapsed {
		return hitResult{panel: hitPanelSidebar, target: hitTargetCollapsedStrip}
	}
	if y == 0 || y == height-1 {
		// Top/bottom border row: within the sidebar panel, but on no
		// particular row or header.
		return hitResult{panel: hitPanelSidebar}
	}
	contentRow := y - 1
	contentHeight := height - 2
	visible := m.sidebarVisibleEntries(sidebarEntryContentWidth(layout), contentHeight)
	if contentRow < 0 || contentRow >= len(visible) {
		return hitResult{panel: hitPanelSidebar}
	}
	return sidebarEntryHit(visible[contentRow])
}

// hitTestStacked covers the below-80-column fallback (renderStackedFrame):
// the list box, then the preview box, stacked top to bottom with no seam
// between them. The below-minimum notice (SPEC requirement 14) lives on
// the footer now, not above these panels, so it never shifts this offset
// math.
func (m Model) hitTestStacked(layout LayoutResult, x, y int) hitResult {
	lw, lh := layout.Sidebar.Width, layout.Sidebar.Height
	pw, ph := layout.Preview.Width, layout.Preview.Height
	if lh >= 2 {
		if y >= 0 && y < lh {
			if x < 0 || x >= lw {
				return hitResult{}
			}
			if y == 0 || y == lh-1 {
				return hitResult{panel: hitPanelSidebar}
			}
			contentRow := y - 1
			contentHeight := lh - 2
			visible := m.sidebarVisibleEntries(sidebarEntryContentWidth(layout), contentHeight)
			if contentRow < 0 || contentRow >= len(visible) {
				return hitResult{panel: hitPanelSidebar}
			}
			return sidebarEntryHit(visible[contentRow])
		}
		y -= lh
	}
	if ph >= 2 && y >= 0 && y < ph && x >= 0 && x < pw {
		return hitResult{panel: hitPanelPreview}
	}
	return hitResult{}
}

// sidebarEntryHit turns one sidebarEntry (task 028's shared content, also
// used by the renderer) into the matching hitResult.
func sidebarEntryHit(e sidebarEntry) hitResult {
	switch e.kind {
	case sidebarLineHeader:
		return hitResult{panel: hitPanelSidebar, target: hitTargetHeader, groupID: e.groupID}
	case sidebarLineRow:
		return hitResult{panel: hitPanelSidebar, target: hitTargetRow, sessionIndex: e.sessionIndex}
	default:
		return hitResult{panel: hitPanelSidebar}
	}
}

// sidebarContentDims returns the (contentWidth, contentHeight) the sidebar
// panel's own entries are laid out against for the current frame's
// Effective mode, mirroring exactly what renderSideBySideFrame/
// renderStackedFrame pass to sidebarVisibleEntries, so wheel-scroll
// clamping (scrollSidebar below) agrees with what is actually on screen.
// The width half is sidebarEntryContentWidth (tui.go), the single seam
// every caller that lays out sidebar entries shares.
func (m Model) sidebarContentDims(layout LayoutResult) (width, height int) {
	return sidebarEntryContentWidth(layout), max(layout.Sidebar.Height-2, 0)
}

// handleMouse is the tea.MouseMsg branch of Update (SPEC §11.8). Every
// dialog/overlay case is already filtered out by the caller before this is
// reached.
func (m Model) handleMouse(e tea.MouseMsg) (tea.Model, tea.Cmd) {
	if e.Button == tea.MouseButtonWheelUp || e.Button == tea.MouseButtonWheelDown {
		delta := 1
		if e.Button == tea.MouseButtonWheelUp {
			delta = -1
		}
		return m.scrollSidebar(e, delta), nil
	}
	if e.Button != tea.MouseButtonLeft {
		return m, nil
	}
	switch e.Action {
	case tea.MouseActionPress:
		return m.handleMousePress(e)
	case tea.MouseActionMotion:
		return m.handleMouseDrag(e)
	case tea.MouseActionRelease:
		m.draggingSeam = false
		return m, nil
	}
	return m, nil
}

// scrollSidebar is the wheel's own binding (SPEC §11.8: "wheel over the
// sidebar scrolls the list, without changing selection"; duplicates
// ↑/↓/PgUp/PgDn). A wheel event anywhere outside the sidebar panel
// (including over the preview, which must never fall through to the
// sidebar) is a no-op, and the collapsed strip has no list to scroll.
func (m Model) scrollSidebar(e tea.MouseMsg, delta int) Model {
	hit := m.hitTest(e.X, e.Y)
	if hit.panel != hitPanelSidebar || hit.target == hitTargetCollapsedStrip {
		return m
	}
	layout := m.computeLayout()
	width, height := m.sidebarContentDims(layout)
	total := len(m.sidebarEntries(width))
	m.sidebarScroll = clampSidebarScroll(m.sidebarScroll+delta, total, height)
	return m
}

// handleMousePress resolves a left-button press against whatever hitTest
// says is under the pointer.
func (m Model) handleMousePress(e tea.MouseMsg) (tea.Model, tea.Cmd) {
	hit := m.hitTest(e.X, e.Y)
	switch hit.panel {
	case hitPanelSeam:
		// "Drag the seam adjusts sidebar_width live" (duplicates `<`/`>`):
		// the press itself changes nothing until a motion event follows.
		m.draggingSeam = true
		return m, nil
	case hitPanelPreview:
		// "A click ... over the preview does nothing, and that is a
		// binding too": it must not fall through to the sidebar. Steer
		// 017 item 3/task 216's drag-to-copy selection is scoped to
		// interactive mode only (Update's own tea.MouseMsg case routes
		// press/motion/release there directly whenever m.interactive is
		// true, never reaching handleMouse at all -- task 313/R54 hit-tests
		// that same press FIRST, so a press over the sidebar re-targets
		// interactive mode there instead of ever reaching drag-to-copy; a
		// press landing here, over the preview, or over the seam still
		// falls straight through to drag-to-copy exactly as before), so
		// this stays a plain no-op for every gesture over the PASSIVE
		// preview.
		return m, nil
	case hitPanelSidebar:
		switch hit.target {
		case hitTargetCollapsedStrip:
			// "click the collapsed strip restores the previous
			// non-collapsed mode" (SPEC §11.8 requirement 33) -- not the
			// same landing spot as `|`, which always advances to auto
			// from collapsed.
			return m.restoreFromCollapsedStrip()
		case hitTargetHeader:
			// "click a workspace group header toggles collapse"
			// (duplicates the grouping key, `c`), keyed by the header's
			// durable group id (task 013/R129 part 3) and persisted to
			// ui_state's collapsed_groups exactly like the `c` key.
			m.toggleGroupCollapse(hit.groupID)
			return m, m.persistCollapsedGroups()
		case hitTargetRow:
			return m.clickSidebarRow(hit.sessionIndex, e)
		}
	}
	return m, nil
}

// clickSidebarRow implements SPEC §11.8's reversed decision (task
// 311/R55): one press selects the row AND enters interactive mode on it,
// duplicating ↵'s job (task 061's rebind of ↵ to interactive mode /
// II-45) rather than gating entry behind a second, deliberate press. The
// operator's own reasoning (SPEC §11.8) is that a stray click resizing a
// live agent's window (§11.9's fit) is a smaller cost than a click that
// selects but leaves the keyboard in the list; `Ctrl+Q` is the stated
// mitigation, not a confirmation. `a` (full attach) is unchanged and stays
// reachable only via the key or the footer's own "a attach" hint, never
// via a mouse gesture -- unlike interactive entry, full attach hands over
// the WHOLE terminal and Ctrl+Q cannot undo it.
func (m Model) clickSidebarRow(index int, e tea.MouseMsg) (tea.Model, tea.Cmd) {
	m.selected = index
	return m.enterInteractive()
}

// retargetInteractiveSidebarClick is task 313's (R54) job: while interactive
// mode already owns the keyboard, a left press that hit-tests to a sidebar
// row (Update's own tea.MouseMsg case resolves this BEFORE assuming the
// press is task 216's drag-to-copy gesture) re-targets interactive mode
// onto that row instead of being absorbed as a no-op click outside the
// preview's content box -- the same reasoning as clickSidebarRow's own
// list-mode job (task 311), just reachable from inside interactive mode
// too. index is already the clicked row's session index (hitTest's own
// sessionIndex), resolved against the SAME frame the renderer just drew,
// so this never re-derives geometry independently.
//
// A press on the row that is ALREADY the interactive target is a no-op:
// no leave, no re-enter, no resize -- m.selected never changes while
// interactive except by this path or by a sessionsLoaded id-preserving
// re-sort, so comparing it against index is exactly "is this the session
// already showing". Otherwise it leaves the current session first
// (exitInteractive: tear down the transport, restore the window's own
// geometry byte-exact and release ownership -- byte-for-byte the same
// sequence Ctrl+Q runs) before entering the newly clicked one, so a
// concurrent claimant of the OLD window never observes it left resized.
func (m Model) retargetInteractiveSidebarClick(index int) (tea.Model, tea.Cmd) {
	if !m.interactive || index == m.selected {
		return m, nil
	}
	left, _ := m.exitInteractive()
	next := left.(Model)
	next.selected = index
	return next.enterInteractive()
}

// handleMouseDrag adjusts sidebar_width live while draggingSeam is true
// (SPEC §11.8's "drag the seam adjusts sidebar_width live", duplicating
// `<`/`>`), through the identical ClampSidebarWidth bound those keys use.
func (m Model) handleMouseDrag(e tea.MouseMsg) (tea.Model, tea.Cmd) {
	if !m.draggingSeam {
		return m, nil
	}
	width, _ := m.frameSize()
	m.sidebarWidth = ClampSidebarWidth(width, e.X)
	return m, m.persistSidebarWidth()
}
