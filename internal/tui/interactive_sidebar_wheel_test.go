// interactive_sidebar_wheel_test.go covers R149/GH #47: a wheel over the
// sidebar while interactive does exactly what list mode's own scrollSidebar
// (mouse.go:208) does -- move the sidebar viewport and arm
// m.sidebarScrollDrifted -- and never touches the interactive target, the
// selection, or the interactive grid's own content/scroll offset. A wheel
// over the preview is unchanged (interactive_page_scroll_test.go and
// mouse_test.go already cover that half; this file adds only the sidebar
// half those never asserted).
package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/interactive"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// interactiveSidebarWheelTestModel builds an interactive Model whose
// sidebar list is longer than the sidebar panel itself (so a wheel notch
// has somewhere to scroll to) and whose interactive target is a real
// session drawn from that same list, resolved through interactiveTargetSession
// exactly as the product does (SPEC §11.6/R138), not a hand-set field the
// product itself never sets this way.
func interactiveSidebarWheelTestModel(t *testing.T) Model {
	t.Helper()
	var sessions []store.Session
	for i := 0; i < 30; i++ {
		slug := string(rune('a' + i))
		sessions = append(sessions, store.Session{ID: slug, Name: slug, Slug: slug, CWD: "/work/infra", Status: "idle"})
	}
	m := mouseTestModel(sessions)
	m.width, m.height = 100, 20
	m.selected = rowCursor(3)

	target, err := tmux.SessionName(sessions[3].Slug)
	if err != nil {
		t.Fatalf("tmux.SessionName: %v", err)
	}
	m.interactive = true
	m.interactiveWindowTarget = target
	m.interactiveGrid = &interactive.Session{}
	return m
}

// TestInteractiveWheelOverSidebarScrollsListViewportOnly proves the R149
// contract directly: a wheel notch over the sidebar while interactive
// changes sidebarScroll (and only that), leaving m.interactive, the
// resolved interactive target, the selection, the interactive grid pointer
// and its scroll offset all exactly as they were, and issuing no command
// (in particular, no resize).
func TestInteractiveWheelOverSidebarScrollsListViewportOnly(t *testing.T) {
	m := interactiveSidebarWheelTestModel(t)

	beforeTarget, beforeOK := m.interactiveTargetSession()
	if !beforeOK {
		t.Fatalf("test setup: interactiveTargetSession did not resolve before the wheel")
	}
	beforeSelected := m.selected
	beforeGrid := m.interactiveGrid
	beforeGridOffset := m.interactiveScrollOffset()
	beforeSidebarScroll := m.sidebarScroll

	updatedModel, cmd := m.Update(wheelDown(10, 5))
	got := updatedModel.(Model)

	if got.sidebarScroll == beforeSidebarScroll {
		t.Fatalf("a wheel over the sidebar while interactive did not move sidebarScroll: still %d", got.sidebarScroll)
	}
	if !got.sidebarScrollDrifted {
		t.Fatalf("a wheel over the sidebar while interactive did not arm sidebarScrollDrifted")
	}
	if !got.interactive {
		t.Fatalf("a wheel over the sidebar while interactive left interactive mode: m.interactive = false")
	}

	afterTarget, afterOK := got.interactiveTargetSession()
	if !afterOK || afterTarget.ID != beforeTarget.ID {
		t.Fatalf("interactive target changed: before=%+v(ok=%v) after=%+v(ok=%v)", beforeTarget, beforeOK, afterTarget, afterOK)
	}
	if got.selected != beforeSelected {
		t.Fatalf("selection changed from a sidebar wheel while interactive: before=%v after=%v", beforeSelected, got.selected)
	}
	if got.interactiveGrid != beforeGrid {
		t.Fatalf("interactive grid pointer changed from a sidebar wheel: before=%p after=%p", beforeGrid, got.interactiveGrid)
	}
	if got.interactiveScrollOffset() != beforeGridOffset {
		t.Fatalf("interactive grid scroll offset changed from a sidebar wheel: before=%d after=%d", beforeGridOffset, got.interactiveScrollOffset())
	}
	if cmd != nil {
		t.Fatalf("a sidebar wheel while interactive issued a command (want none, in particular no resize): %#v", cmd)
	}

	// A wheel over the preview is unchanged: it still scrolls the
	// interactive scrollback, never the sidebar.
	layout := got.computeLayout()
	previewX := layout.Sidebar.Width + 3
	updatedModel2, _ := got.Update(tea.MouseMsg{X: previewX, Y: 5, Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
	afterPreviewWheel := updatedModel2.(Model)
	if afterPreviewWheel.sidebarScroll != got.sidebarScroll {
		t.Fatalf("wheel over the preview while interactive changed sidebarScroll: %d -> %d", got.sidebarScroll, afterPreviewWheel.sidebarScroll)
	}
}
