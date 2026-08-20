package tui

import "testing"

// TestComputeLayoutAutoBoundary covers 79/80/81 columns: auto must pick
// stacked strictly below deck's 80-column minimum and side-by-side at and
// above it (§11.2).
func TestComputeLayoutAutoBoundary(t *testing.T) {
	cases := []struct {
		width int
		want  string
	}{
		{79, LayoutStacked},
		{80, LayoutSideBySide},
		{81, LayoutSideBySide},
	}
	for _, c := range cases {
		got := ComputeLayout(c.width, 24, "", 35)
		if got.Effective != c.want {
			t.Errorf("width=%d: Effective = %q, want %q", c.width, got.Effective, c.want)
		}
		if got.Requested != LayoutAuto {
			t.Errorf("width=%d: Requested = %q, want %q", c.width, got.Requested, LayoutAuto)
		}
		if got.Fallback {
			t.Errorf("width=%d: unexpected Fallback for auto", c.width)
		}
	}
}

// TestComputeLayoutGoldenMinimum is §11.2's golden minimum-size frame: at
// exactly 80×24 in auto, side-by-side with sidebar 35 total and preview 45
// total.
func TestComputeLayoutGoldenMinimum(t *testing.T) {
	got := ComputeLayout(80, 24, "", 35)
	if got.Effective != LayoutSideBySide {
		t.Fatalf("Effective = %q, want %q", got.Effective, LayoutSideBySide)
	}
	if got.Sidebar.Width != 35 {
		t.Errorf("Sidebar.Width = %d, want 35", got.Sidebar.Width)
	}
	if got.Preview.Width != 45 {
		t.Errorf("Preview.Width = %d, want 45", got.Preview.Width)
	}
	if got.Preview.Width < PreviewWidthFloor {
		t.Errorf("Preview.Width = %d below its floor %d", got.Preview.Width, PreviewWidthFloor)
	}
	if got.BelowMinimum {
		t.Errorf("80x24 must not report BelowMinimum")
	}
}

// TestComputeLayoutPinnedFallsBackWithoutOverwritingPin: a pinned
// side-by-side mode that cannot hold its floors (sidebar 24 + preview 40 =
// 64) at the current width renders as auto (stacked, since width<80) while
// Requested still reports the pin unchanged.
func TestComputeLayoutPinnedFallsBackWithoutOverwritingPin(t *testing.T) {
	got := ComputeLayout(60, 24, LayoutSideBySide, 35)
	if got.Requested != LayoutSideBySide {
		t.Fatalf("Requested = %q, want %q (pin must not be overwritten)", got.Requested, LayoutSideBySide)
	}
	if got.Effective != LayoutStacked {
		t.Fatalf("Effective = %q, want %q", got.Effective, LayoutStacked)
	}
	if !got.Fallback {
		t.Errorf("Fallback = false, want true")
	}

	// Just above the floor sum, the pin holds and is rendered as pinned.
	held := ComputeLayout(SidebarWidthFloor+PreviewWidthFloor, 24, LayoutSideBySide, 35)
	if held.Effective != LayoutSideBySide || held.Fallback {
		t.Fatalf("at width %d the pin should hold: Effective=%q Fallback=%v",
			SidebarWidthFloor+PreviewWidthFloor, held.Effective, held.Fallback)
	}
}

// TestComputeLayoutStackedNeverFallsBack: stacked is itself the degrade
// mode, so pinning it never falls back even far below its own floors.
func TestComputeLayoutStackedNeverFallsBack(t *testing.T) {
	got := ComputeLayout(80, 10, LayoutStacked, 35)
	if got.Effective != LayoutStacked || got.Fallback {
		t.Fatalf("Effective=%q Fallback=%v, want stacked with no fallback", got.Effective, got.Fallback)
	}
}

// TestComputeLayoutCollapsedNeverAutomatic: auto never chooses collapsed at
// any width, only an explicit pin does.
func TestComputeLayoutCollapsedNeverAutomatic(t *testing.T) {
	for _, width := range []int{3, 24, 80, 200} {
		got := ComputeLayout(width, 24, "", 35)
		if got.Effective == LayoutCollapsed {
			t.Errorf("auto chose collapsed at width=%d", width)
		}
	}
	pinned := ComputeLayout(80, 24, LayoutCollapsed, 35)
	if pinned.Effective != LayoutCollapsed {
		t.Fatalf("pinned collapsed rendered as %q", pinned.Effective)
	}
	if pinned.Sidebar.Width != CollapsedStripWidth {
		t.Errorf("collapsed strip width = %d, want %d", pinned.Sidebar.Width, CollapsedStripWidth)
	}
}

// TestComputeLayoutBelowMinimum covers the below-80×24 case: auto renders
// stacked as far as it fits and BelowMinimum is reported.
func TestComputeLayoutBelowMinimum(t *testing.T) {
	got := ComputeLayout(70, 20, "", 35)
	if !got.BelowMinimum {
		t.Fatalf("BelowMinimum = false at 70x20, want true")
	}
	if got.Effective != LayoutStacked {
		t.Fatalf("Effective = %q, want %q below the minimum", got.Effective, LayoutStacked)
	}
	if got.Sidebar.Height+got.Preview.Height != 20 {
		t.Errorf("Sidebar.Height+Preview.Height = %d, want 20 (rows fully accounted for)",
			got.Sidebar.Height+got.Preview.Height)
	}
	if got.Sidebar.Width != 70 || got.Preview.Width != 70 {
		t.Errorf("stacked panels must be full width: sidebar=%d preview=%d", got.Sidebar.Width, got.Preview.Width)
	}
}

// TestStackedListHeightBounds covers §11.2's min(max(rows/3,5),12) and that
// the preview never gets a negative height even in a very short stacked
// terminal.
func TestStackedListHeightBounds(t *testing.T) {
	cases := []struct {
		rows int
		want int
	}{
		{15, 5},  // 15/3=5 -> max(5,5)=5
		{24, 8},  // 24/3=8 -> max(8,5)=8, min(8,12)=8
		{60, 12}, // 60/3=20 -> min(20,12)=12
		{3, 5},   // tiny terminal: list floor still 5, clamped to rows below
	}
	for _, c := range cases {
		got := stackedListHeight(c.rows)
		if got != c.want {
			t.Errorf("stackedListHeight(%d) = %d, want %d", c.rows, got, c.want)
		}
	}

	// A terminal shorter than the unclamped list height still yields a
	// non-negative preview height and a sidebar clamped to the rows
	// available.
	tiny := ComputeLayout(80, 3, LayoutStacked, 35)
	if tiny.Sidebar.Height > 3 {
		t.Errorf("Sidebar.Height = %d, must not exceed rows=3", tiny.Sidebar.Height)
	}
	if tiny.Preview.Height < 0 {
		t.Errorf("Preview.Height = %d, must not be negative", tiny.Preview.Height)
	}
	if tiny.Sidebar.Height+tiny.Preview.Height != 3 {
		t.Errorf("rows not fully accounted for: %d+%d != 3", tiny.Sidebar.Height, tiny.Preview.Height)
	}
}

// TestClampSidebarWidth covers both clamp ends used by `<`/`>` (task 015).
func TestClampSidebarWidth(t *testing.T) {
	if got := ClampSidebarWidth(100, 10); got != SidebarWidthFloor {
		t.Errorf("ClampSidebarWidth(100,10) = %d, want floor %d", got, SidebarWidthFloor)
	}
	if got := ClampSidebarWidth(100, 1000); got != 60 { // 100-40
		t.Errorf("ClampSidebarWidth(100,1000) = %d, want 60", got)
	}
	if got := ClampSidebarWidth(100, 50); got != 50 {
		t.Errorf("ClampSidebarWidth(100,50) = %d, want 50 (unclamped)", got)
	}
}

// TestComputeLayoutSidebarWidthDefault: a non-positive sidebar_width means
// "use the default", never a zero-width or negative-width sidebar.
func TestComputeLayoutSidebarWidthDefault(t *testing.T) {
	for _, sw := range []int{0, -1, -100} {
		got := ComputeLayout(80, 24, LayoutSideBySide, sw)
		if got.Sidebar.Width != 35 {
			t.Errorf("sidebarWidth=%d: Sidebar.Width = %d, want default 35", sw, got.Sidebar.Width)
		}
	}
}
