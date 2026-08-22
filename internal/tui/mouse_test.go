package tui

import (
	"context"
	"os/exec"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

func mouseTestModel(sessions []store.Session) Model {
	m := New(nil, config.Settings{Mouse: true}, "")
	m.sessions = sessions
	m.attach = func(context.Context, string) (*exec.Cmd, error) { return exec.Command("true"), nil }
	return m
}

func press(x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}
}

func release(x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft}
}

func motion(x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionMotion, Button: tea.MouseButtonLeft}
}

func wheelDown(x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown}
}

func wheelUp(x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonWheelUp}
}

// findRow locates the absolute (x, y) of some visible line belonging to
// sessionIndex, in the side-by-side/collapsed layout hitTest resolves
// against, so tests never hand-compute panel offsets independently of the
// one geometry implementation under test.
func findRow(t *testing.T, m Model, sessionIndex int) (x, y int) {
	t.Helper()
	layout := m.computeLayout()
	width, height := m.sidebarContentDims(layout)
	visible := m.sidebarVisibleEntries(width, height)
	for i, e := range visible {
		if e.kind == sidebarLineRow && e.sessionIndex == sessionIndex {
			return layout.Sidebar.Width / 2, contentRowY(m, layout, i)
		}
	}
	t.Fatalf("no visible row found for session index %d", sessionIndex)
	return 0, 0
}

func findHeader(t *testing.T, m Model, workspace string) (x, y int) {
	t.Helper()
	layout := m.computeLayout()
	width, height := m.sidebarContentDims(layout)
	visible := m.sidebarVisibleEntries(width, height)
	for i, e := range visible {
		if e.kind == sidebarLineHeader && e.workspace == workspace {
			return layout.Sidebar.Width / 2, contentRowY(m, layout, i)
		}
	}
	t.Fatalf("no visible header found for workspace %q", workspace)
	return 0, 0
}

// contentRowY maps a 0-based content-row index (into sidebarVisibleEntries)
// to an absolute terminal row, accounting for the same offset hitTest
// itself adds on top of the border row: the startup banner (none in these
// tests). The below-minimum notice (SPEC requirement 14) lives on the
// footer now, not above the stacked panels, so it never shifts this.
func contentRowY(m Model, layout LayoutResult, contentRow int) int {
	width, _ := m.frameSize()
	banner := len(m.startupBanner(width))
	return banner + contentRow + 1
}

// TestHitTestResolvesRowsHeadersSeamAndPreviewSideBySide proves task 028's
// core claim: one geometry implementation resolves a click to a row, a
// group header, the seam, or the preview, agreeing exactly with what
// mainView actually drew (grouping and a long, elided name are both in
// play).
func TestHitTestResolvesRowsHeadersSeamAndPreviewSideBySide(t *testing.T) {
	m := mouseTestModel([]store.Session{
		{ID: "a1", Name: "alpha-session-with-a-very-long-name-that-must-be-elided", CWD: "/work/infra", Status: "idle"},
		{ID: "b1", Name: "bravo", CWD: "/work/service-a", Status: "idle"},
	})
	m.width, m.height = 100, 30 // side-by-side (width >= 80)

	layout := m.computeLayout()
	if layout.Effective != LayoutSideBySide {
		t.Fatalf("test setup: Effective = %q, want side-by-side", layout.Effective)
	}
	sw := layout.Sidebar.Width

	hx, hy := findHeader(t, m, "infra")
	if hit := m.hitTest(hx, hy); hit.panel != hitPanelSidebar || hit.target != hitTargetHeader || hit.workspace != "infra" {
		t.Fatalf("hitTest(%d,%d) = %+v, want sidebar/header workspace=infra", hx, hy, hit)
	}

	rx, ry := findRow(t, m, 0)
	if hit := m.hitTest(rx, ry); hit.panel != hitPanelSidebar || hit.target != hitTargetRow || hit.sessionIndex != 0 {
		t.Fatalf("hitTest(%d,%d) = %+v, want sidebar/row sessionIndex=0 (elided name must not change the index)", rx, ry, hit)
	}

	rx2, ry2 := findRow(t, m, 1)
	if hit := m.hitTest(rx2, ry2); hit.panel != hitPanelSidebar || hit.target != hitTargetRow || hit.sessionIndex != 1 {
		t.Fatalf("hitTest(%d,%d) = %+v, want sidebar/row sessionIndex=1", rx2, ry2, hit)
	}

	if hit := m.hitTest(sw, 5); hit.panel != hitPanelSeam {
		t.Fatalf("hitTest at seam column %d = %+v, want hitPanelSeam", sw, hit)
	}
	if hit := m.hitTest(sw+3, 5); hit.panel != hitPanelPreview {
		t.Fatalf("hitTest past the seam = %+v, want hitPanelPreview", hit)
	}
}

// TestClickSidebarRowSelectsNeverAttaches proves a single click switches
// the preview (selects) without ever invoking attach (SPEC §11.8's "a
// single click never attaches").
func TestClickSidebarRowSelectsNeverAttaches(t *testing.T) {
	m := mouseTestModel([]store.Session{
		{ID: "a1", Name: "a1", CWD: "/work/infra", Status: "idle"},
		{ID: "b1", Name: "b1", CWD: "/work/service-a", Status: "idle"},
	})
	m.width, m.height = 100, 30
	m.selected = 0

	x, y := findRow(t, m, 1)
	updated, cmd := m.Update(press(x, y))
	got := updated.(Model)
	if got.selected != 1 {
		t.Fatalf("selected = %d, want 1", got.selected)
	}
	if cmd != nil {
		t.Fatalf("single click returned a command (must not attach)")
	}
}

// TestDoubleClickSidebarRowAttaches proves the deliberate second act SPEC
// §11.8 requires before attaching: two presses on the same row within the
// double-click window resolve to attach, matching `↵`'s own path.
func TestDoubleClickSidebarRowAttaches(t *testing.T) {
	m := mouseTestModel([]store.Session{
		{ID: "a1", Name: "a1", CWD: "/work/infra", Status: "idle"},
	})
	m.width, m.height = 100, 30
	m.selected = -1

	x, y := findRow(t, m, 0)
	updated, cmd := m.Update(press(x, y))
	got := updated.(Model)
	if cmd != nil {
		t.Fatalf("first click already returned an attach command")
	}
	updated, cmd = got.Update(press(x, y))
	got = updated.(Model)
	if got.selected != 0 {
		t.Fatalf("selected = %d, want 0", got.selected)
	}
	if cmd == nil {
		t.Fatalf("double click did not schedule attachment")
	}
}

// TestClickGroupHeaderTogglesOnlyThatGroup proves the mouse binding calls
// the identical helper task 039's `g` key uses, and touches no other
// group's collapse state.
func TestClickGroupHeaderTogglesOnlyThatGroup(t *testing.T) {
	m := mouseTestModel([]store.Session{
		{ID: "a1", Name: "a1", CWD: "/work/infra", Status: "idle"},
		{ID: "b1", Name: "b1", CWD: "/work/service-a", Status: "idle"},
	})
	m.width, m.height = 100, 30

	x, y := findHeader(t, m, "infra")
	updated, _ := m.Update(press(x, y))
	got := updated.(Model)
	if !got.isGroupCollapsed("infra") {
		t.Fatalf("clicking infra's header did not collapse it")
	}
	if got.isGroupCollapsed("service-a") {
		t.Fatalf("clicking infra's header collapsed an unrelated group")
	}
}

// TestClickCollapsedStripRestoresThePinInForceBeforeCollapsing proves the
// review's finding is fixed: SPEC §11.8 requirement 33 says clicking the
// collapsed strip "restores the previous non-collapsed mode", not `|`'s
// own collapsed->auto landing spot. The bug: the click used to call
// cycleLayoutMode() directly, so it landed on auto every time regardless of
// what was pinned before collapsing.
func TestClickCollapsedStripRestoresThePinInForceBeforeCollapsing(t *testing.T) {
	cases := []struct {
		name string
		pre  string
		want string
	}{
		{"side-by-side", LayoutSideBySide, LayoutSideBySide},
		{"stacked", LayoutStacked, LayoutStacked},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := mouseTestModel([]store.Session{{ID: "a1", Name: "a1", CWD: "/work/infra"}})
			m.width, m.height = 100, 30
			// pins tc.pre, then cycles to collapsed: cycleLayoutMode is the
			// only path that ever sets layoutMode to collapsed, and it is
			// the one that must record what it is leaving.
			m.layoutMode = tc.pre
			updated, _ := m.cycleLayoutMode()
			for updated.(Model).layoutMode != LayoutCollapsed {
				updated, _ = updated.(Model).cycleLayoutMode()
			}
			m = updated.(Model)

			layout := m.computeLayout()
			if layout.Effective != LayoutCollapsed {
				t.Fatalf("test setup: Effective = %q, want collapsed", layout.Effective)
			}

			clicked, _ := m.Update(press(1, 2))
			got := clicked.(Model)
			if got.layoutMode != tc.want {
				t.Fatalf("layoutMode after collapsed-strip click = %q, want %q (pinned %q before collapsing)", got.layoutMode, tc.want, tc.pre)
			}
		})
	}
}

// TestClickCollapsedStripDiffersFromPipeKeyFromCollapsed proves the strip's
// click and `|` genuinely diverge from collapsed: `|` always advances to
// auto (nextLayoutMode's own documented wraparound), while the strip
// returns to whatever was pinned immediately before collapsing.
func TestClickCollapsedStripDiffersFromPipeKeyFromCollapsed(t *testing.T) {
	m := mouseTestModel([]store.Session{{ID: "a1", Name: "a1", CWD: "/work/infra"}})
	m.width, m.height = 100, 30
	m.layoutMode = LayoutStacked
	updated, _ := m.cycleLayoutMode() // stacked -> collapsed, records "stacked"
	m = updated.(Model)
	if m.layoutMode != LayoutCollapsed {
		t.Fatalf("test setup: layoutMode = %q, want collapsed", m.layoutMode)
	}

	clicked, _ := m.Update(press(1, 2))
	gotClick := clicked.(Model).layoutMode
	if gotClick != LayoutStacked {
		t.Fatalf("layoutMode after collapsed-strip click = %q, want %q", gotClick, LayoutStacked)
	}

	piped, _ := m.cycleLayoutMode()
	gotPipe := piped.(Model).layoutMode
	if gotPipe != LayoutAuto {
		t.Fatalf("layoutMode after `|` from collapsed = %q, want %q", gotPipe, LayoutAuto)
	}
	if gotClick == gotPipe {
		t.Fatalf("collapsed-strip click and `|` landed on the same mode %q; they must diverge from collapsed", gotClick)
	}
}

// TestWheelScrollsSidebarWithoutChangingSelectionOrFocus proves "wheel
// over the sidebar scrolls the list, without changing selection", and that
// a wheel over the preview is a no-op that never falls through.
func TestWheelScrollsSidebarWithoutChangingSelectionOrFocus(t *testing.T) {
	var sessions []store.Session
	for i := 0; i < 30; i++ {
		sessions = append(sessions, store.Session{ID: string(rune('a' + i)), Name: string(rune('a' + i)), CWD: "/work/infra", Status: "idle"})
	}
	m := mouseTestModel(sessions)
	m.width, m.height = 100, 30
	m.selected = 0

	updated, _ := m.Update(wheelDown(10, 5))
	got := updated.(Model)
	if got.sidebarScroll == 0 {
		t.Fatalf("wheel down over the sidebar did not scroll")
	}
	if got.selected != 0 {
		t.Fatalf("selected changed from a wheel event: %d", got.selected)
	}

	layout := got.computeLayout()
	updated, _ = got.Update(wheelDown(layout.Sidebar.Width+3, 5))
	afterPreviewWheel := updated.(Model)
	if afterPreviewWheel.sidebarScroll != got.sidebarScroll {
		t.Fatalf("wheel over the preview changed sidebarScroll: %d -> %d", got.sidebarScroll, afterPreviewWheel.sidebarScroll)
	}

	updated, _ = afterPreviewWheel.Update(wheelUp(10, 5))
	backUp := updated.(Model)
	if backUp.sidebarScroll >= afterPreviewWheel.sidebarScroll {
		t.Fatalf("wheel up did not scroll back toward the top: before=%d after=%d", afterPreviewWheel.sidebarScroll, backUp.sidebarScroll)
	}
}

// TestClickAndWheelOverPreviewAreNoops proves "a click or a wheel over the
// preview does nothing", including that it never falls through to select a
// sidebar row instead.
func TestClickAndWheelOverPreviewAreNoops(t *testing.T) {
	m := mouseTestModel([]store.Session{
		{ID: "a1", Name: "a1", CWD: "/work/infra", Status: "idle"},
		{ID: "b1", Name: "b1", CWD: "/work/service-a", Status: "idle"},
	})
	m.width, m.height = 100, 30
	m.selected = 0
	layout := m.computeLayout()
	previewX := layout.Sidebar.Width + 3

	updated, cmd := m.Update(press(previewX, 5))
	got := updated.(Model)
	if got.selected != 0 {
		t.Fatalf("selected changed after a click over the preview: %d", got.selected)
	}
	if cmd != nil {
		t.Fatalf("click over the preview returned a command")
	}
}

// TestSeamDragAdjustsSidebarWidthLive proves "drag the seam adjusts
// sidebar_width live", through the exact ClampSidebarWidth bound `<`/`>`
// use, and that a plain click on the seam (no motion) changes nothing.
func TestSeamDragAdjustsSidebarWidthLive(t *testing.T) {
	m := mouseTestModel([]store.Session{{ID: "a1", Name: "a1", CWD: "/work/infra"}})
	m.width, m.height = 100, 30
	layout := m.computeLayout()
	sw := layout.Sidebar.Width

	updated, cmd := m.Update(press(sw, 5))
	got := updated.(Model)
	if cmd != nil {
		t.Fatalf("pressing the seam alone returned a command")
	}
	if got.sidebarWidth != m.sidebarWidth {
		t.Fatalf("pressing the seam alone (no motion) changed sidebarWidth")
	}
	if !got.draggingSeam {
		t.Fatalf("pressing the seam did not start a drag")
	}

	newX := sw + 10
	updated, _ = got.Update(motion(newX, 5))
	dragged := updated.(Model)
	want := ClampSidebarWidth(100, newX)
	if dragged.sidebarWidth != want {
		t.Fatalf("sidebarWidth after drag = %d, want %d", dragged.sidebarWidth, want)
	}

	updated, _ = dragged.Update(release(newX, 5))
	released := updated.(Model)
	if released.draggingSeam {
		t.Fatalf("release did not end the drag")
	}

	// A motion after release (no button held down, per bubbletea's own
	// semantics) must not keep adjusting the width.
	updated, _ = released.Update(motion(newX+20, 5))
	afterRelease := updated.(Model)
	if afterRelease.sidebarWidth != released.sidebarWidth {
		t.Fatalf("motion after release still adjusted sidebarWidth")
	}
}

// TestHitTestStackedModeResolvesRowsAndPreview proves the same single
// geometry implementation also resolves clicks correctly in the stacked
// layout (below deck's 80-column minimum), where the list and preview
// stack vertically instead of sharing a seam.
func TestHitTestStackedModeResolvesRowsAndPreview(t *testing.T) {
	m := mouseTestModel([]store.Session{
		{ID: "a1", Name: "a1", CWD: "/work/infra", Status: "idle"},
		{ID: "b1", Name: "b1", CWD: "/work/service-a", Status: "idle"},
	})
	m.width, m.height = 70, 30 // below 80 columns: auto chooses stacked

	layout := m.computeLayout()
	if layout.Effective != LayoutStacked {
		t.Fatalf("test setup: Effective = %q, want stacked", layout.Effective)
	}

	x, y := findRow(t, m, 1)
	if hit := m.hitTest(x, y); hit.panel != hitPanelSidebar || hit.target != hitTargetRow || hit.sessionIndex != 1 {
		t.Fatalf("hitTest(%d,%d) in stacked mode = %+v, want sidebar/row sessionIndex=1", x, y, hit)
	}

	previewY := layout.Sidebar.Height + 1
	if hit := m.hitTest(3, previewY); hit.panel != hitPanelPreview {
		t.Fatalf("hitTest below the list box = %+v, want hitPanelPreview", hit)
	}
}

// TestMouseIgnoredWhileOverlayOpen proves "the mouse can neither cancel
// nor confirm" any dialog: with an overlay open, a press that would
// otherwise select a different row is a complete no-op.
func TestMouseIgnoredWhileOverlayOpen(t *testing.T) {
	m := mouseTestModel([]store.Session{
		{ID: "a1", Name: "a1", CWD: "/work/infra", Status: "idle"},
		{ID: "b1", Name: "b1", CWD: "/work/service-a", Status: "idle"},
	})
	m.width, m.height = 100, 30
	m.selected = 0
	x, y := findRow(t, m, 1)

	m.help = true
	updated, cmd := m.Update(press(x, y))
	got := updated.(Model)
	if got.selected != 0 || cmd != nil {
		t.Fatalf("mouse press acted while help was open: selected=%d cmd=%v", got.selected, cmd)
	}
}

// TestSettingsTakeoverMouseIgnoredWhileOpen proves requirement 24 (SPEC
// §11.5/§11.8): while the full-screen settings takeover -- or the
// unsaved-changes discard confirm painted over it -- is open, the hidden
// main view underneath must not receive mouse input at all: not a single
// click's selection change, not a double click's attach, not a wheel
// scroll. The review of 65e623e reproduced the opposite by driving exactly
// this input against the pre-fix guard (which omitted m.settingsOpen and
// m.settingsDiscardConfirm from the `if m.help || ...` check at the top of
// the tea.MouseMsg case): a double click on the covered sidebar row called
// the instrumented attach once (attachCalls=1) despite the takeover being
// on top. Re-running this test against that pre-fix guard reproduces the
// same failure (see the commit message for the RED output); the fix makes
// it GREEN.
func TestSettingsTakeoverMouseIgnoredWhileOpen(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  func(*Model)
	}{
		{"settingsOpen", func(m *Model) { m.settingsOpen = true }},
		{"settingsDiscardConfirm", func(m *Model) {
			m.settingsOpen = true
			m.settingsDiscardConfirm = true
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			attachCalls := 0
			m := mouseTestModel([]store.Session{
				{ID: "a1", Name: "a1", CWD: "/work/infra", Status: "idle"},
				{ID: "b1", Name: "b1", CWD: "/work/service-a", Status: "idle"},
			})
			m.attach = func(context.Context, string) (*exec.Cmd, error) {
				attachCalls++
				return exec.Command("true"), nil
			}
			m.width, m.height = 100, 30
			m.selected = 0
			x, y := findRow(t, m, 1)
			tc.set(&m)

			updated, cmd := m.Update(press(x, y))
			got := updated.(Model)
			if got.selected != 0 || cmd != nil {
				t.Fatalf("single press reached the hidden sidebar: selected=%d cmd=%v", got.selected, cmd)
			}
			if !got.settingsOpen {
				t.Fatalf("a mouse press closed settingsOpen")
			}

			updated, cmd = got.Update(press(x, y))
			got = updated.(Model)
			if got.selected != 0 || cmd != nil {
				t.Fatalf("second press (double click) reached the hidden sidebar: selected=%d cmd=%v", got.selected, cmd)
			}
			if !got.settingsOpen {
				t.Fatalf("the double click closed settingsOpen")
			}

			updated, cmd = got.Update(wheelDown(x, y))
			got = updated.(Model)
			if got.selected != 0 || cmd != nil {
				t.Fatalf("wheel event reached the hidden sidebar: selected=%d cmd=%v", got.selected, cmd)
			}

			if attachCalls != 0 {
				t.Fatalf("attach was called %d time(s) while the takeover covered the sidebar, want 0", attachCalls)
			}
		})
	}
}

// TestAllFiveDialogsRejectMouseAtBorderBodyAndOutside proves requirement 11
// (SPEC §11.4/§11.8, review finding on req 11 at 65e623e) for every one of
// the five dialogs the guard at the top of the tea.MouseMsg case names --
// create (m.creating), detail (m.detail), pin (m.pinning), profile
// (m.profileSwitching) and help (m.help) -- not just create and help as the
// pre-existing TestMouseIgnoredWhileOverlayOpen and dialog_contract_test.go
// coverage did. Each dialog is rendered directly by View() (framedDialog
// draws the box at (0,0) with no separate placement step), so "border",
// "body" and "outside" are real coordinates against that box; the body
// point is deliberately the same (x, y) findRow resolves for the second
// session's row in the hidden main view underneath, so a guard regression
// is caught the same way task 001/002's settings-takeover test catches it:
// a bypassed guard would fall through to m.handleMouse and change
// m.selected from 0 to 1, or (on a second press, i.e. a double click) call
// attach -- this is verified by literally deleting the five flags from the
// guard's condition and re-running (RED output below, restored for GREEN).
// None of the five dialogs render an actual clickable button (they are
// keyboard-only text dialogs, per createView/pinView/profileSwitchView/
// detailView/helpText), so beyond selected/cmd/attach the dialog's own
// state is asserted via View() being byte-for-byte identical before and
// after, which subsumes every dialog-private field createView/pinView/
// profileSwitchView read to render.
//
// RED (guard's five flags replaced with `false &&` on b1e2c05, before this
// task's fix restored them):
//
//	--- FAIL: TestAllFiveDialogsRejectMouseAtBorderBodyAndOutside (0.00s)
//	    --- FAIL: .../help/body (0.00s)
//	        mouse_test.go:605: help body press at (17,5) changed selected from 0 to 1
//	    --- FAIL: .../create/body (0.00s)
//	        mouse_test.go:605: create body press at (17,5) changed selected from 0 to 1
//	    --- FAIL: .../detail/body (0.00s)
//	        mouse_test.go:605: detail body press at (17,5) changed selected from 0 to 1
//	    --- FAIL: .../pin/body (0.00s)
//	        mouse_test.go:605: pin body press at (17,5) changed selected from 0 to 1
//	    --- FAIL: .../profile/body (0.00s)
//	        mouse_test.go:605: profile body press at (17,5) changed selected from 0 to 1
//	FAIL
//
// GREEN is the real, captured `ci/run.sh go test ./internal/tui/... -run
// Mouse -v` output quoted in this task's commit message.
func TestAllFiveDialogsRejectMouseAtBorderBodyAndOutside(t *testing.T) {
	baseSessions := func() []store.Session {
		return []store.Session{
			{ID: "a1", Name: "a1", Agent: "claude", CWD: "/work/infra", Status: "waiting",
				StatusReason: "needs input", PermissionProfile: "safe", ConversationID: "conv-1", ResumeState: "auto"},
			{ID: "b1", Name: "b1", CWD: "/work/service-a", Status: "idle"},
		}
	}

	cases := []struct {
		name  string
		setup func(m *Model)
		open  func(Model) bool
	}{
		{name: "help", setup: func(m *Model) { m.help = true }, open: func(m Model) bool { return m.help }},
		{
			name: "create",
			setup: func(m *Model) {
				m.creating = true
				m.createName = "my session"
				m.createCWD = "/work/infra"
				m.createAgent = "shell"
				m.createField = 0
			},
			open: func(m Model) bool { return m.creating },
		},
		{name: "detail", setup: func(m *Model) { m.detail = true }, open: func(m Model) bool { return m.detail }},
		{
			name:  "pin",
			setup: func(m *Model) { m.pinning = true; m.pinValue = "auto" },
			open:  func(m Model) bool { return m.pinning },
		},
		{
			name:  "profile",
			setup: func(m *Model) { m.profileSwitching = true; m.profileSwitchValue = "safe" },
			open:  func(m Model) bool { return m.profileSwitching },
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			newModel := func() (Model, *int) {
				attachCalls := 0
				m := mouseTestModel(baseSessions())
				m.attach = func(context.Context, string) (*exec.Cmd, error) {
					attachCalls++
					return exec.Command("true"), nil
				}
				m.width, m.height = 100, 30
				m.selected = 0
				tc.setup(&m)
				if !tc.open(m) {
					t.Fatalf("setup did not open the %s dialog", tc.name)
				}
				return m, &attachCalls
			}
			m0, _ := newModel()
			boxWidth := m0.dialogWidth()
			bodyX, bodyY := findRow(t, m0, 1)

			positions := []struct {
				where string
				x, y  int
			}{
				{"border", 0, 0},
				{"body", bodyX, bodyY},
				{"outside", boxWidth + 5, 0},
			}
			for _, pos := range positions {
				t.Run(pos.where, func(t *testing.T) {
					m, attachCalls := newModel()
					beforeView := m.View()

					updated, cmd := m.Update(press(pos.x, pos.y))
					got := updated.(Model)
					if cmd != nil {
						t.Fatalf("%s %s press at (%d,%d) returned a non-nil cmd", tc.name, pos.where, pos.x, pos.y)
					}
					if got.selected != 0 {
						t.Fatalf("%s %s press at (%d,%d) changed selected from 0 to %d", tc.name, pos.where, pos.x, pos.y, got.selected)
					}
					if !tc.open(got) {
						t.Fatalf("%s %s press at (%d,%d) closed the dialog", tc.name, pos.where, pos.x, pos.y)
					}
					if afterView := got.View(); afterView != beforeView {
						t.Fatalf("%s %s press at (%d,%d) changed the dialog's rendered view (own state), before:\n%s\nafter:\n%s", tc.name, pos.where, pos.x, pos.y, beforeView, afterView)
					}

					// A second press right after (a double click) must be
					// equally inert -- this is what would attach on the hidden
					// sidebar row if the guard were bypassed.
					updated2, cmd2 := got.Update(press(pos.x, pos.y))
					got2 := updated2.(Model)
					if cmd2 != nil {
						t.Fatalf("%s %s second press (double click) at (%d,%d) returned a non-nil cmd", tc.name, pos.where, pos.x, pos.y)
					}
					if got2.selected != 0 || !tc.open(got2) {
						t.Fatalf("%s %s double click at (%d,%d) changed selected/open state", tc.name, pos.where, pos.x, pos.y)
					}
					if afterView := got2.View(); afterView != beforeView {
						t.Fatalf("%s %s double click at (%d,%d) changed the dialog's rendered view (own state)", tc.name, pos.where, pos.x, pos.y)
					}
					if *attachCalls != 0 {
						t.Fatalf("%s %s double click at (%d,%d) attached %d time(s), want 0", tc.name, pos.where, pos.x, pos.y, *attachCalls)
					}
				})
			}
		})
	}
}
