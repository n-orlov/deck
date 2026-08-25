package tui

import (
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// TestInteractivePressOnAlreadyTargetRowIsANoOp proves task 313's (R54)
// no-op case: a left press that hit-tests to the sidebar row already
// showing in interactive mode must not leave, not re-enter and not
// resize -- interactiveScrollOffset/previewFitSessionID are the two
// fields exitInteractive always clears (see its own doc comment), so
// seeding them with sentinel values and asserting they SURVIVE the press
// is what actually proves exitInteractive never ran, not merely that the
// final state happens to look unchanged.
func TestInteractivePressOnAlreadyTargetRowIsANoOp(t *testing.T) {
	m := mouseTestModel([]store.Session{
		{ID: "a1", Name: "alpha", Agent: "shell", Status: "running", Slug: "alpha"},
		{ID: "b1", Name: "bravo", Agent: "shell", Status: "running", Slug: "bravo"},
	})
	m.width, m.height = 100, 30
	m.interactive = true
	m.selected = 1
	m.interactiveScrollOffset = 7
	m.previewFitSessionID = "b1"
	m.attachError = "sentinel"

	x, y := findRow(t, m, 1)
	if hit := m.hitTest(x, y); hit.panel != hitPanelSidebar || hit.target != hitTargetRow || hit.sessionIndex != 1 {
		t.Fatalf("test setup: hitTest(%d,%d) = %+v, want sidebar row 1", x, y, hit)
	}

	next, cmd := m.Update(press(x, y))
	got := next.(Model)
	if cmd != nil {
		t.Fatalf("press on the already-interactive row returned a non-nil cmd, want nil")
	}
	if !got.interactive || got.selected != 1 {
		t.Fatalf("press on the already-interactive row changed selection/interactive state: %+v", got)
	}
	if got.interactiveScrollOffset != 7 || got.previewFitSessionID != "b1" {
		t.Fatalf("press on the already-interactive row ran exitInteractive's own clearing (scrollOffset=%d previewFitSessionID=%q), want both untouched -- no leave, no re-enter, no resize", got.interactiveScrollOffset, got.previewFitSessionID)
	}
	if got.attachError != "sentinel" {
		t.Fatalf("press on the already-interactive row touched attachError: %q", got.attachError)
	}
}

// TestInteractivePressOnADifferentSidebarRowRetargets proves task 313's
// re-target case: a left press on a sidebar row that is NOT the current
// interactive target leaves the old session first (exitInteractive's own
// clearing of interactiveScrollOffset/previewFitSessionID, with no real
// tmux call since interactiveWindowTarget/interactiveOwnership are both
// zero here, exactly TestWindowShrinkBelowTheFloorLeavesInteractiveModeAn
// dRestoresTheList's own trick), moves selection to the clicked row, and
// actually calls enterInteractive on it -- proven with
// TestClickSidebarRowEntersInteractiveModeOnOnePress's own floor-refusal
// trick (a non-zero tmux.Client plus a preview squeezed below
// interactiveMinInnerRows), which sets attachError from pure arithmetic
// before any live tmux call, so entry is provably attempted rather than
// merely degrading silently the way a zero tmux.Client would.
func TestInteractivePressOnADifferentSidebarRowRetargets(t *testing.T) {
	m := mouseTestModel([]store.Session{
		{ID: "a1", Name: "alpha", Agent: "shell", Status: "running", Slug: "alpha"},
		{ID: "b1", Name: "bravo", Agent: "shell", Status: "running", Slug: "bravo"},
	})
	m = m.WithTmuxClient(tmux.Client{Socket: "no-such-tmux-server-313"})
	m.width, m.height = 80, 9 // previewContentSize -> 41x6, below interactiveMinInnerRows
	m.interactive = true
	m.selected = 0
	m.interactiveScrollOffset = 7
	m.previewFitSessionID = "a1"

	x, y := findRow(t, m, 1)
	if hit := m.hitTest(x, y); hit.panel != hitPanelSidebar || hit.target != hitTargetRow || hit.sessionIndex != 1 {
		t.Fatalf("test setup: hitTest(%d,%d) = %+v, want sidebar row 1", x, y, hit)
	}

	next, cmd := m.Update(press(x, y))
	got := next.(Model)
	if cmd != nil {
		t.Fatalf("retargeting press returned a non-nil cmd, want nil")
	}
	if got.selected != 1 {
		t.Fatalf("retargeting press left selected = %d, want 1", got.selected)
	}
	if got.interactiveScrollOffset != 0 || got.previewFitSessionID != "" {
		t.Fatalf("retargeting press did not run exitInteractive's own clearing on the OLD session (scrollOffset=%d previewFitSessionID=%q), want both zeroed", got.interactiveScrollOffset, got.previewFitSessionID)
	}
	if got.interactive {
		t.Fatalf("retargeting press left interactive mode on despite the floor refusal on the NEW session")
	}
	if !strings.Contains(got.attachError, "7-row floor") {
		t.Fatalf("attachError %q does not name the 7-row floor -- the retargeting press did not actually reach enterInteractive on the new row", got.attachError)
	}
}

// TestInteractivePressOverPreviewStillFallsThroughToDragToCopy proves
// task 313 left task 216's own routing byte-for-byte: a press that
// hit-tests to the PREVIEW panel (never the sidebar) must still reach
// beginInteractiveSelection, not the new sidebar re-targeting branch --
// with a nil interactiveGrid that call refuses (ok=false), so the press
// stays the same no-op it always was, and in particular does not touch
// selection or leave interactive mode.
func TestInteractivePressOverPreviewStillFallsThroughToDragToCopy(t *testing.T) {
	m := mouseTestModel([]store.Session{
		{ID: "a1", Name: "alpha", Agent: "shell", Status: "running", Slug: "alpha"},
	})
	m.width, m.height = 100, 30
	m.interactive = true
	m.selected = 0

	layout := m.computeLayout()
	px, py := layout.Sidebar.Width+2, 5
	if hit := m.hitTest(px, py); hit.panel != hitPanelPreview {
		t.Fatalf("test setup: hitTest(%d,%d) = %+v, want preview", px, py, hit)
	}

	next, cmd := m.Update(press(px, py))
	got := next.(Model)
	if cmd != nil {
		t.Fatalf("press over the preview returned a non-nil cmd, want nil")
	}
	if got.interactiveSelecting {
		t.Fatalf("beginInteractiveSelection began a selection with a nil interactiveGrid")
	}
	if got.selected != 0 || !got.interactive {
		t.Fatalf("a press over the preview re-targeted interactive mode instead of falling through to drag-to-copy: %+v", got)
	}
}
