package tui

import (
	"path/filepath"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// TestInteractiveHeaderPressTogglesCollapseWithoutResize proves task 005's
// (B.1/#33, R138) own claim: a left press on a group header while
// interactive mode owns the keyboard reaches the SAME shared resolver list
// mode's handleMousePress uses (resolveSidebarPress, mouse.go), not the
// drag-to-copy path -- it toggles that group's collapse, returns the
// persistCollapsedGroups command (proven live, with a real store attached,
// exactly like TestCollapsedGroupsSurviveModelRebuildFromUIState's own
// trick), leaves m.interactive true, and makes no geometry/resize call at
// all: interactiveScrollOffset/previewFitSessionID are the two fields
// exitInteractive always clears (its own doc comment), so seeding them
// with sentinel values and asserting they SURVIVE the press is what
// actually proves neither exitInteractive nor enterInteractive ran, not
// merely that m.interactive happens to still read true.
func TestInteractiveHeaderPressTogglesCollapseWithoutResize(t *testing.T) {
	home := t.TempDir()
	db, err := store.OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	infraID, serviceID := int64(1), int64(2)
	m := New(db, config.Settings{Mouse: true}, "")
	m.sessions = []store.Session{
		{ID: "a1", Name: "a1", CWD: "/work/infra", Status: "idle", GroupName: "infra", GroupID: &infraID},
		{ID: "b1", Name: "b1", CWD: "/work/service-a", Status: "idle", GroupName: "service-a", GroupID: &serviceID},
	}
	m.width, m.height = 100, 30
	m.interactive = true
	m.selected = 0
	m.setInteractiveScrollOffset(7)
	m.previewFitSessionID = "a1"

	hx, hy := findHeader(t, m, "infra")
	if hit := m.hitTest(hx, hy); hit.panel != hitPanelSidebar || hit.target != hitTargetHeader || hit.groupID != infraID {
		t.Fatalf("test setup: hitTest(%d,%d) = %+v, want sidebar/header groupID=%d", hx, hy, hit, infraID)
	}

	next, cmd := m.Update(press(hx, hy))
	got := next.(Model)

	if !got.isGroupCollapsed(infraID) {
		t.Fatalf("header press while interactive did not collapse the infra group")
	}
	if got.isGroupCollapsed(serviceID) {
		t.Fatalf("header press while interactive collapsed an unrelated group")
	}
	if !got.interactive {
		t.Fatalf("header press while interactive left m.interactive false, want it to stay true")
	}
	if got.interactiveScrollOffset() != 7 || got.previewFitSessionID != "a1" {
		t.Fatalf("header press while interactive ran exitInteractive/enterInteractive's own clearing (scrollOffset=%d previewFitSessionID=%q), want both untouched -- no geometry/resize call at all", got.interactiveScrollOffset(), got.previewFitSessionID)
	}
	if cmd == nil {
		t.Fatal("header press while interactive returned a nil cmd, want persistCollapsedGroups' command (a store is attached)")
	}
	msg := cmd()
	persisted, ok := msg.(uiStatePersisted)
	if !ok || persisted.err != nil {
		t.Fatalf("header press while interactive returned cmd = %+v, want a successful uiStatePersisted from persistCollapsedGroups", msg)
	}
}

// TestInteractiveCollapsedStripPressRestoresLayout proves the other half of
// the same shared resolver: a press on the collapsed strip while
// interactive mode owns the keyboard reaches restoreFromCollapsedStrip,
// byte-identically to list mode's own TestClickCollapsedStripRestoresThePi
// nInForceBeforeCollapsing, rather than falling through to drag-to-copy
// (which a nil interactiveGrid would otherwise silently no-op).
func TestInteractiveCollapsedStripPressRestoresLayout(t *testing.T) {
	m := mouseTestModel([]store.Session{{ID: "a1", Name: "a1", CWD: "/work/infra"}})
	m.width, m.height = 100, 30
	m.layoutMode = LayoutStacked
	updated, _ := m.cycleLayoutMode() // stacked -> collapsed, records "stacked"
	m = updated.(Model)
	if m.layoutMode != LayoutCollapsed {
		t.Fatalf("test setup: layoutMode = %q, want collapsed", m.layoutMode)
	}
	m.interactive = true
	m.selected = 0
	m.setInteractiveScrollOffset(3)

	if hit := m.hitTest(1, 2); hit.panel != hitPanelSidebar || hit.target != hitTargetCollapsedStrip {
		t.Fatalf("test setup: hitTest(1,2) = %+v, want sidebar/collapsed-strip", hit)
	}

	clicked, _ := m.Update(press(1, 2))
	got := clicked.(Model)
	if got.layoutMode != LayoutStacked {
		t.Fatalf("collapsed-strip press while interactive left layoutMode = %q, want %q (the pinned mode before collapsing)", got.layoutMode, LayoutStacked)
	}
	if !got.interactive {
		t.Fatalf("collapsed-strip press while interactive left m.interactive false, want it to stay true")
	}
	if got.interactiveScrollOffset() != 3 {
		t.Fatalf("collapsed-strip press while interactive touched interactiveScrollOffset (%d), want unchanged 3 -- restoreFromCollapsedStrip only changes layoutMode", got.interactiveScrollOffset())
	}
}

// TestHeaderPressParityBetweenListAndInteractiveMode proves the shared
// resolver behaves identically whether or not interactive mode owns the
// keyboard: the same header press, against the same starting model modulo
// m.interactive, collapses the same group by the same amount in both
// modes.
func TestHeaderPressParityBetweenListAndInteractiveMode(t *testing.T) {
	infraID, serviceID := int64(1), int64(2)
	base := func() Model {
		m := mouseTestModel([]store.Session{
			{ID: "a1", Name: "a1", CWD: "/work/infra", Status: "idle", GroupName: "infra", GroupID: &infraID},
			{ID: "b1", Name: "b1", CWD: "/work/service-a", Status: "idle", GroupName: "service-a", GroupID: &serviceID},
		})
		m.width, m.height = 100, 30
		m.selected = 0
		return m
	}

	listMode := base()
	x, y := findHeader(t, listMode, "infra")

	listNext, _ := listMode.Update(press(x, y))
	listGot := listNext.(Model)

	interactiveMode := base()
	interactiveMode.interactive = true
	interactiveNext, _ := interactiveMode.Update(press(x, y))
	interactiveGot := interactiveNext.(Model)

	if listGot.isGroupCollapsed(infraID) != interactiveGot.isGroupCollapsed(infraID) {
		t.Fatalf("header press parity broke: list mode collapsed=%v, interactive mode collapsed=%v", listGot.isGroupCollapsed(infraID), interactiveGot.isGroupCollapsed(infraID))
	}
	if !listGot.isGroupCollapsed(infraID) {
		t.Fatalf("header press collapsed nothing in either mode")
	}
	if !interactiveGot.interactive {
		t.Fatalf("header press dropped m.interactive in interactive mode")
	}
}
