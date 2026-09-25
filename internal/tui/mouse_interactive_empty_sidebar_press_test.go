package tui

import (
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/store"
)

// findEmptySidebarSpace locates an absolute (x, y) that hitTest resolves
// to hitPanelSidebar/hitTargetNone -- the blank padding below the last
// visible entry -- for m's current side-by-side layout, mirroring
// TestClickSidebarPaddingBelowLastRowIsANoOp's own setup (mouse_test.go)
// rather than hand-deriving the padding row a second time.
func findEmptySidebarSpace(t *testing.T, m Model) (x, y int) {
	t.Helper()
	layout := m.computeLayout()
	width, height := m.sidebarContentDims(layout)
	entries := m.sidebarEntries(width)
	if len(entries) >= height {
		t.Fatalf("test setup: no padding row available below the last entry (entries=%d, contentHeight=%d)", len(entries), height)
	}
	x = layout.Sidebar.Width / 2
	y = contentRowY(m, layout, len(entries))
	if hit := m.hitTest(x, y); hit.panel != hitPanelSidebar || hit.target != hitTargetNone {
		t.Fatalf("test setup: hitTest(%d,%d) = %+v, want sidebar/hitTargetNone (padding row)", x, y, hit)
	}
	return x, y
}

// emptySidebarTestModel builds a two-session model sized so the sidebar's
// content box has at least one blank padding row below the last entry.
func emptySidebarTestModel() Model {
	m := mouseTestModel([]store.Session{
		{ID: "a1", Name: "alpha", Agent: "shell", Status: "running", Slug: "alpha"},
		{ID: "b1", Name: "bravo", Agent: "shell", Status: "running", Slug: "bravo"},
	})
	m.width, m.height = 100, 30
	return m
}

// TestInteractiveEmptySidebarPressLeavesInteractiveModeLikeCtrlQ is R144/GH
// #37's core claim: a left press that hit-tests to hitTargetNone -- blank
// sidebar space, never a row/header/strip -- while interactive mode owns
// the keyboard runs the SAME exitInteractive Ctrl+Q itself calls (tui.go's
// updateInteractive), not a second, divergent teardown: the selection is
// untouched and every interactive* geometry field the two paths clear ends
// up identical, proven by running both from byte-identical starting models
// rather than merely asserting m.interactive == false (which a mutant that
// only flipped the bool, without clearing anything else, would still
// pass).
func TestInteractiveEmptySidebarPressLeavesInteractiveModeLikeCtrlQ(t *testing.T) {
	base := func() Model {
		m := emptySidebarTestModel()
		m.interactive = true
		m.selected = rowCursor(1)
		m.setInteractiveScrollOffset(7)
		m.previewFitSessionID = "b1"
		m.attachError = "sentinel"
		return m
	}

	viaPress := base()
	x, y := findEmptySidebarSpace(t, viaPress)
	pressed, cmd := viaPress.Update(press(x, y))
	gotPress := pressed.(Model)
	if cmd != nil {
		t.Fatalf("empty-sidebar press while interactive returned a non-nil cmd, want nil (exitInteractive's own success path returns nil)")
	}

	viaCtrlQ := base()
	next, _ := viaCtrlQ.exitInteractive()
	gotCtrlQ := next.(Model)

	if gotPress.interactive {
		t.Fatalf("empty-sidebar press while interactive did not leave interactive mode")
	}
	if gotPress.selected != rowCursor(1) {
		t.Fatalf("empty-sidebar press while interactive changed the selection to %v, want it unchanged at rowCursor(1)", gotPress.selected)
	}
	if gotPress.attachError != "sentinel" {
		t.Fatalf("empty-sidebar press while interactive touched attachError: %q", gotPress.attachError)
	}

	// The geometry/teardown fields must agree exactly with what Ctrl+Q's
	// own exitInteractive produced from the identical starting model --
	// not merely "both are zero" by coincidence, but the same fields
	// exitInteractive's own doc comment names as what it clears.
	if gotPress.interactive != gotCtrlQ.interactive {
		t.Fatalf("interactive = %v, want %v (Ctrl+Q's own result)", gotPress.interactive, gotCtrlQ.interactive)
	}
	if gotPress.interactiveWindowTarget != gotCtrlQ.interactiveWindowTarget {
		t.Fatalf("interactiveWindowTarget = %q, want %q (Ctrl+Q's own result)", gotPress.interactiveWindowTarget, gotCtrlQ.interactiveWindowTarget)
	}
	if gotPress.interactiveGeometry != gotCtrlQ.interactiveGeometry {
		t.Fatalf("interactiveGeometry = %+v, want %+v (Ctrl+Q's own result)", gotPress.interactiveGeometry, gotCtrlQ.interactiveGeometry)
	}
	if gotPress.interactiveOwnership != gotCtrlQ.interactiveOwnership {
		t.Fatalf("interactiveOwnership = %+v, want %+v (Ctrl+Q's own result)", gotPress.interactiveOwnership, gotCtrlQ.interactiveOwnership)
	}
	if gotPress.interactiveGrid != gotCtrlQ.interactiveGrid {
		t.Fatalf("interactiveGrid = %+v, want %+v (Ctrl+Q's own result)", gotPress.interactiveGrid, gotCtrlQ.interactiveGrid)
	}
	if gotPress.interactiveDispatcher != gotCtrlQ.interactiveDispatcher {
		t.Fatalf("interactiveDispatcher = %+v, want %+v (Ctrl+Q's own result)", gotPress.interactiveDispatcher, gotCtrlQ.interactiveDispatcher)
	}
	if gotPress.interactiveScrollOffset() != gotCtrlQ.interactiveScrollOffset() {
		t.Fatalf("interactiveScrollOffset = %d, want %d (Ctrl+Q's own result)", gotPress.interactiveScrollOffset(), gotCtrlQ.interactiveScrollOffset())
	}
	if gotPress.previewFitSessionID != gotCtrlQ.previewFitSessionID {
		t.Fatalf("previewFitSessionID = %q, want %q (Ctrl+Q's own result)", gotPress.previewFitSessionID, gotCtrlQ.previewFitSessionID)
	}
}

// TestEmptySidebarPressInListModeIsStillANoOp re-asserts, against this
// task's own setup (emptySidebarTestModel/findEmptySidebarSpace), the
// invariant TestClickSidebarPaddingBelowLastRowIsANoOp already pins for
// list mode: hitTargetNone has no case (and no `default`) in
// handleMousePress's own switch, so nothing about R144/GH #37's new
// interactive-mode exit path leaks into list mode, where an empty-sidebar
// press must remain a plain no-op -- no selection change, no interactive
// entry, no command.
func TestEmptySidebarPressInListModeIsStillANoOp(t *testing.T) {
	m := emptySidebarTestModel()
	m.selected = rowCursor(1)

	x, y := findEmptySidebarSpace(t, m)
	updated, cmd := m.Update(press(x, y))
	got := updated.(Model)

	if got.selected != rowCursor(1) {
		t.Fatalf("empty-sidebar press in list mode changed the selection to %v, want unchanged rowCursor(1)", got.selected)
	}
	if got.interactive {
		t.Fatalf("empty-sidebar press in list mode entered interactive mode")
	}
	if cmd != nil {
		t.Fatalf("empty-sidebar press in list mode returned a non-nil command, want nil")
	}
}

// TestHelpNamesThePreviewClickAndEmptySidebarClickGestures is R144/GH
// #37's `?` overlay half: the Mouse table names the preview-click gesture
// beside the enter key (↵) it duplicates, and the empty-sidebar-click
// gesture beside Ctrl+Q, the same cross-reference style every other mouse
// row already uses ("(like c)", "(like </>)", ...).
func TestHelpNamesThePreviewClickAndEmptySidebarClickGestures(t *testing.T) {
	for _, ascii := range []bool{false, true} {
		help := helpText(ascii)

		previewBullet := helpBulletFor(t, help, "  click over the preview")
		enterGlyph := "\u21b5"
		if ascii {
			enterGlyph = "Enter"
		}
		if !strings.Contains(previewBullet, enterGlyph) {
			t.Errorf("helpText(ascii=%v)'s preview-click bullet does not name the enter key %q:\n%s", ascii, enterGlyph, previewBullet)
		}
		if !strings.Contains(previewBullet, "interactive") {
			t.Errorf("helpText(ascii=%v)'s preview-click bullet does not say it enters interactive mode:\n%s", ascii, previewBullet)
		}

		emptyBullet := helpBulletFor(t, help, "  click empty sidebar space")
		if !strings.Contains(emptyBullet, "Ctrl+Q") {
			t.Errorf("helpText(ascii=%v)'s empty-sidebar-click bullet does not name Ctrl+Q:\n%s", ascii, emptyBullet)
		}
		if !strings.Contains(emptyBullet, "leaves interactive mode") {
			t.Errorf("helpText(ascii=%v)'s empty-sidebar-click bullet does not say it leaves interactive mode:\n%s", ascii, emptyBullet)
		}
	}
}
