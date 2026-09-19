package tui

import (
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/store"
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
// SPEC §11's "The name elides; the count and the chevron never do": at
// the sidebar's narrowest legal content budget (store.GroupNameMaxLength's
// own comment derives this as the 20-cell content floor: §11.2 clamps
// sidebar_width to [24, width-40], the narrowest legal sidebar has a
// 20-cell content floor), a long group name is elided down, but the
// leading chevron marker and the trailing "(<n>)" count are always
// emitted in full and never touched by that elision.
func TestGroupHeaderTextElidesNameAtSidebarFloorKeepingChevronAndCount(t *testing.T) {
	m := groupTestModel(nil)
	const sidebarFloorContentWidth = 20 // store.GroupNameMaxLength's own derivation
	group := sidebarGroup{
		Workspace: "a much longer group name than the 24-column sidebar floor can ever show in full",
		Sessions: []indexedSession{
			{Index: 0, Session: store.Session{ID: "a"}},
			{Index: 1, Session: store.Session{ID: "b"}},
		},
	}
	got := m.groupHeaderText(group, sidebarFloorContentWidth)

	if !strings.HasPrefix(got, "\u25be ") {
		t.Fatalf("groupHeaderText at the sidebar floor = %q, want it to still start with the expanded chevron %q", got, "\u25be ")
	}
	if !strings.Contains(got, "(2)") {
		t.Fatalf("groupHeaderText at the sidebar floor = %q, want the count %q to survive", got, "(2)")
	}
	if strings.Contains(got, group.Workspace) {
		t.Fatalf("groupHeaderText at the sidebar floor = %q, want the full name elided rather than rendered whole", got)
	}
	if stringWidth(got) > sidebarFloorContentWidth {
		t.Fatalf("groupHeaderText at the sidebar floor = %q (width %d), want it to fit within %d", got, stringWidth(got), sidebarFloorContentWidth)
	}

	// Collapsed marker must survive the same way.
	m.collapsedGroups = map[string]bool{group.Workspace: true}
	gotCollapsed := m.groupHeaderText(group, sidebarFloorContentWidth)
	if !strings.HasPrefix(gotCollapsed, "\u25b8 ") {
		t.Fatalf("collapsed groupHeaderText at the sidebar floor = %q, want it to start with the collapsed chevron %q", gotCollapsed, "\u25b8 ")
	}
	if !strings.Contains(gotCollapsed, "(2)") {
		t.Fatalf("collapsed groupHeaderText at the sidebar floor = %q, want the count %q to survive", gotCollapsed, "(2)")
	}
}
