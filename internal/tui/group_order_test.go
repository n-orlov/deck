package tui

import (
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/theme"
)

// TestGroupOrderAaaLeadsAlphabetically proves R129/task 011's group order:
// alphabetical, case-insensitive -- a group named "aaa" (mixed case, to
// prove the comparison folds case) leads every other real group
// regardless of insertion order or each group's own first-appearance
// position in m.sessions (the removed reorderPreservingGrouping's own
// rule, which this replaces entirely).
func TestGroupOrderAaaLeadsAlphabetically(t *testing.T) {
	m := groupTestModel([]store.Session{
		{ID: "z1", GroupName: "zzz"},
		{ID: "m1", GroupName: "Mmm"},
		{ID: "a1", GroupName: "AAA"}, // uppercase, still must sort as "aaa"
	})
	groups := m.groupSessions()
	if len(groups) != 3 {
		t.Fatalf("len(groups) = %d, want 3 (%v)", len(groups), groups)
	}
	var got []string
	for _, g := range groups {
		got = append(got, g.Workspace)
	}
	want := []string{"AAA", "Mmm", "zzz"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("group order = %v, want %v (case-insensitive alphabetical, insertion order zzz/Mmm/AAA must not survive)", got, want)
		}
	}
}

// TestGroupOrderZzzSortsBeforeDefaultDespiteName proves a group named
// "zzz" -- the alphabetically LAST-sorting real name possible in this
// fixture -- still renders before the implicit default group, because
// default's position is a structural rule ("always last"), never a
// consequence of comparing the literal string "default" against "zzz".
func TestGroupOrderZzzSortsBeforeDefaultDespiteName(t *testing.T) {
	m := groupTestModel([]store.Session{
		{ID: "d1", GroupName: ""}, // implicit default
		{ID: "z1", GroupName: "zzz"},
	})
	groups := m.groupSessions()
	if len(groups) != 2 {
		t.Fatalf("len(groups) = %d, want 2 (%v)", len(groups), groups)
	}
	if groups[0].Workspace != "zzz" || groups[1].Workspace != "" {
		t.Fatalf("group order = [%q, %q], want [zzz, \"\"] (default always last, even after the alphabetically-latest real group)", groups[0].Workspace, groups[1].Workspace)
	}
}

// TestGroupOrderDefaultAlwaysLastRegardlessOfEmptyStringSortingFirst is
// the other half of "default always last": default's group key is the
// EMPTY string, which a naive lexicographic comparison would place FIRST,
// not last, against any non-empty name (including "aaa", the
// alphabetically-earliest name a real group could have). groupSortsBefore
// must special-case this rather than compare "" against "aaa" as plain
// strings.
func TestGroupOrderDefaultAlwaysLastRegardlessOfEmptyStringSortingFirst(t *testing.T) {
	m := groupTestModel([]store.Session{
		{ID: "d1", GroupName: ""}, // implicit default, inserted FIRST
		{ID: "a1", GroupName: "aaa"},
	})
	groups := m.groupSessions()
	if len(groups) != 2 {
		t.Fatalf("len(groups) = %d, want 2 (%v)", len(groups), groups)
	}
	if groups[0].Workspace != "aaa" || groups[1].Workspace != "" {
		t.Fatalf("group order = [%q, %q], want [aaa, \"\"] (default must not win a naive \"\" < \"aaa\" string comparison)", groups[0].Workspace, groups[1].Workspace)
	}
}

// TestGroupHeaderTextCountsPopulatedAndEmptyGroups proves SPEC §11's
// "every header carries its member count, including (0)": a
// defined-but-empty group (zero Sessions) renders "(0)" exactly like a
// populated group renders its own real count, both via the same
// groupHeaderText call -- neither ever omits the count or renders a
// header that looks unfinished.
func TestGroupHeaderTextCountsPopulatedAndEmptyGroups(t *testing.T) {
	m := groupTestModel(nil)
	populated := sidebarGroup{
		Workspace: "tooling maintenance",
		Sessions: []indexedSession{
			{Index: 0, Session: store.Session{ID: "a"}},
			{Index: 1, Session: store.Session{ID: "b"}},
			{Index: 2, Session: store.Session{ID: "c"}},
		},
	}
	empty := sidebarGroup{Workspace: "sprint work"} // no Sessions: defined but empty

	gotPopulated := m.groupHeaderText(populated, 60)
	if !strings.Contains(gotPopulated, "(3)") {
		t.Fatalf("groupHeaderText(populated) = %q, want it to contain %q", gotPopulated, "(3)")
	}
	gotEmpty := m.groupHeaderText(empty, 60)
	if !strings.Contains(gotEmpty, "(0)") {
		t.Fatalf("groupHeaderText(empty) = %q, want it to contain %q (a defined-but-empty group still renders)", gotEmpty, "(0)")
	}
	if !strings.Contains(gotEmpty, "sprint work") {
		t.Fatalf("groupHeaderText(empty) = %q, want the group's own name still present at this width", gotEmpty)
	}
}

// TestGroupHeaderTextElidesNameAtSidebarFloorKeepingChevronAndCount proves
// SPEC §11's "The name elides; the count and the chevron never do" ON THE
// REAL RENDER PATH at the 24-column sidebar floor (§11.2 clamps
// sidebar_width to [24, width-40], so 24 is the narrowest legal sidebar).
//
// Driving the real pipeline is the whole point of this test, and the
// reason it does not simply call groupHeaderText with a hand-picked
// budget: the header text groupHeaderText returns is not what reaches the
// screen -- sidebarEntries hands it to sidebarContentLine (side-by-side)
// or fullBoxContentLine (stacked), whose padTrunc crops any overflow off
// the RIGHT edge, which is exactly where the count lives. A header that
// fits its own stated budget but not the panel's real one loses "(2)" and
// keeps a dangling "(…" instead, so the assertions below run against the
// composed panel line and against the whole rendered frame, never against
// groupHeaderText's own return value alone.
func TestGroupHeaderTextElidesNameAtSidebarFloorKeepingChevronAndCount(t *testing.T) {
	const longName = "a much longer group name than the 24-column sidebar floor can ever show in full"
	const longNameGroupID = int64(7)
	sessions := []store.Session{
		{ID: "a", Name: "alpha", GroupName: longName, GroupID: groupIDPtr(longNameGroupID), Status: "running"},
		{ID: "b", Name: "bravo", GroupName: longName, GroupID: groupIDPtr(longNameGroupID), Status: "running"},
	}

	for _, tc := range []struct {
		name      string
		collapsed bool
		chevron   string
	}{
		{"expanded", false, "\u25be"},
		{"collapsed", true, "\u25b8"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := groupTestModel(sessions)
			m.width, m.height = AutoSideBySideWidth, MinRows
			m.sidebarWidth = SidebarWidthFloor
			if tc.collapsed {
				m.collapsedGroups = map[int64]bool{longNameGroupID: true}
			}

			layout := m.computeLayout()
			if layout.Effective != LayoutSideBySide {
				t.Fatalf("test setup: Effective = %q, want side-by-side", layout.Effective)
			}
			if layout.Sidebar.Width != SidebarWidthFloor {
				t.Fatalf("test setup: sidebar width = %d, want the %d-column floor", layout.Sidebar.Width, SidebarWidthFloor)
			}

			// The panel line, composed exactly the way the renderer does:
			// the shared content-width seam, then sidebarContentLine's own
			// padTrunc. No hand-picked budget anywhere.
			var header sidebarEntry
			found := false
			for _, e := range m.sidebarEntries(sidebarEntryContentWidth(layout)) {
				if e.kind == sidebarLineHeader {
					header, found = e, true
					break
				}
			}
			if !found {
				t.Fatalf("no group header entry rendered at the sidebar floor")
			}
			line := stripANSI(m.sidebarContentLine(layout.Sidebar.Width, header.gutter, header.text, theme.Token("")))

			if !strings.Contains(line, tc.chevron+" ") {
				t.Fatalf("sidebar floor header line = %q, want the %s chevron %q to survive", line, tc.name, tc.chevron)
			}
			if !strings.Contains(line, "(2)") {
				t.Fatalf("sidebar floor header line = %q, want the count %q to survive the panel's own padTrunc (a cropped %q means the content-width seam overstates the real budget)", line, "(2)", "(\u2026")
			}
			if !strings.Contains(line, "\u2026") {
				t.Fatalf("sidebar floor header line = %q, want the name elided with an ellipsis", line)
			}
			if strings.Contains(line, longName) {
				t.Fatalf("sidebar floor header line = %q, want the full name elided rather than rendered whole", line)
			}
			if got := stringWidth(line); got != layout.Sidebar.Width {
				t.Fatalf("sidebar floor header line = %q (width %d), want exactly the panel's %d columns", line, got, layout.Sidebar.Width)
			}

			// ...and once more through the whole frame, so nothing between
			// sidebarEntries and the painted row can drop the count either.
			frame, _ := m.renderSideBySideFrame(layout)
			frameHeader := ""
			for _, raw := range frame {
				if s := stripANSI(raw); strings.Contains(s, tc.chevron+" ") {
					frameHeader = s
					break
				}
			}
			if frameHeader == "" {
				t.Fatalf("rendered frame carries no group header row:\n%s", strings.Join(frame, "\n"))
			}
			sidebarSpan := string([]rune(frameHeader)[:layout.Sidebar.Width])
			if !strings.Contains(sidebarSpan, "(2)") || !strings.Contains(sidebarSpan, "\u2026") {
				t.Fatalf("rendered frame's sidebar span = %q, want the elided name AND the surviving count %q", sidebarSpan, "(2)")
			}
		})
	}
}

// TestSidebarEntryContentWidthMatchesEachModesRealTextBudget pins the seam
// the test above leans on: sidebarEntryContentWidth must equal the number
// of text columns the mode's own line builder (sidebarContentLine or
// fullBoxContentLine) actually gives a gutter-less line. This is the
// invariant whose breach cost task 011 its first validation attempt --
// the render path passed Sidebar.Width-2 while sidebarContentLine's
// padTrunc budget was Sidebar.Width-3, so every header was composed one
// column too wide and had its rightmost cell (the count's closing paren)
// cropped.
func TestSidebarEntryContentWidthMatchesEachModesRealTextBudget(t *testing.T) {
	m := groupTestModel(nil)
	const probe = "\u2588" // a full block: padTrunc keeps or drops it visibly

	for _, tc := range []struct {
		name      string
		effective string
		width     int
		compose   func(width int, text string) string
	}{
		{"side-by-side", LayoutSideBySide, SidebarWidthFloor, func(width int, text string) string {
			return m.sidebarContentLine(width, "", text, theme.Token(""))
		}},
		{"stacked", LayoutStacked, 60, func(width int, text string) string {
			return m.fullBoxContentLine(width, "", text, false, theme.Token(""))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			layout := LayoutResult{Effective: tc.effective, Sidebar: Rect{Width: tc.width}}
			budget := sidebarEntryContentWidth(layout)
			if budget <= 0 {
				t.Fatalf("sidebarEntryContentWidth = %d, want a positive budget at width %d", budget, tc.width)
			}
			// A text exactly budget wide must survive whole...
			full := strings.Repeat(probe, budget)
			if got := stripANSI(tc.compose(tc.width, full)); !strings.Contains(got, full) {
				t.Fatalf("%s: a %d-column text was cropped by the panel line %q -- sidebarEntryContentWidth overstates the real budget", tc.name, budget, got)
			}
			// ...and one column wider must NOT, or the budget understates it.
			over := strings.Repeat(probe, budget+1)
			if got := stripANSI(tc.compose(tc.width, over)); strings.Contains(got, over) {
				t.Fatalf("%s: a %d-column text survived the panel line %q -- sidebarEntryContentWidth understates the real budget", tc.name, budget+1, got)
			}
		})
	}
}
