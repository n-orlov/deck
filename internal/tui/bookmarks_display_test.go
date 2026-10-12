package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func sidebarText(m Model) string {
	return stripANSI(strings.Join(m.sidebarBodyLines(), "\n"))
}

// TestBookmarkSlotDigitsRenderOnRowsAndFooterAndHelpListTheKeys pins R277's
// on state: a bookmarked row draws its slot digit(s), and the footer and the
// `?` help list the bookmark keys.
func TestBookmarkSlotDigitsRenderOnRowsAndFooterAndHelpListTheKeys(t *testing.T) {
	m, db := bookmarkKeyModel(t, true)
	m.width, m.height = 160, 30
	for slot, id := range map[int]string{3: "s2", 1: "s1", 7: "s1"} {
		if err := db.AssignBookmark(context.Background(), slot, id); err != nil {
			t.Fatal(err)
		}
	}
	side := sidebarText(m)
	for _, want := range []string{"[3] b2", "[1,7] b1"} {
		if !strings.Contains(side, want) {
			t.Fatalf("sidebar lacks %q:\n%s", want, side)
		}
	}
	if strings.Contains(side, "] b0") || strings.Contains(side, "] b3") {
		t.Fatalf("an unbookmarked row drew a slot:\n%s", side)
	}
	footer := stripANSI(m.footerLine())
	for _, want := range []string{"alt+0-9/m", "bookmarks"} {
		if !strings.Contains(footer, want) {
			t.Fatalf("footer lacks %q: %q", want, footer)
		}
	}
	help := stripANSI(m.styledHelpText())
	for _, want := range []string{"alt+0", "alt+m", "bookmark"} {
		if !strings.Contains(help, want) {
			t.Fatalf("help lacks %q", want)
		}
	}
}

// TestBookmarkDisplayIsAbsentWhileTheSettingIsOff pins the off state: with a
// filled slot table, no slot digit is drawn and no footer or help line
// mentions bookmarks.
func TestBookmarkDisplayIsAbsentWhileTheSettingIsOff(t *testing.T) {
	m, db := bookmarkKeyModel(t, false)
	m.width, m.height = 160, 30
	if err := db.AssignBookmark(context.Background(), 3, "s2"); err != nil {
		t.Fatal(err)
	}
	if side := sidebarText(m); strings.Contains(side, "[3]") {
		t.Fatalf("a slot digit was drawn with bookmarks off:\n%s", side)
	}
	footer := strings.ToLower(stripANSI(m.footerLine()))
	if strings.Contains(footer, "bookmark") || strings.Contains(footer, "alt+") {
		t.Fatalf("footer mentions bookmarks: %q", footer)
	}
	help := strings.ToLower(stripANSI(m.styledHelpText()))
	if strings.Contains(help, "bookmark") || strings.Contains(help, "alt+m") {
		t.Fatal("help mentions bookmarks")
	}
}

// TestBookmarkJumpRevealsAHiddenTarget pins R278: whatever hides the target,
// the jump leaves it selected and visible.
func TestBookmarkJumpRevealsAHiddenTarget(t *testing.T) {
	groupID := int64(7)
	cases := []struct {
		name   string
		target string
		setup  func(m *Model)
		check  func(t *testing.T, m Model)
	}{
		{"a filter hides the target", "s3", func(m *Model) {
			m.filterQuery = "b0"
			m.sessions = m.filteredSessions()
		}, func(t *testing.T, m Model) {
			if m.filterQuery != "" || m.filtering {
				t.Fatalf("the filter was not cleared: %q", m.filterQuery)
			}
		}},
		{"a collapsed group holds the target", "s3", func(m *Model) {
			for _, i := range []int{2, 3} {
				m.baseSessions[i].GroupID, m.baseSessions[i].GroupName = &groupID, "grp"
			}
			m.sessions = m.filteredSessions()
			m.setGroupCollapsed(groupID, true)
		}, func(t *testing.T, m Model) {
			if m.isGroupCollapsed(groupID) {
				t.Fatal("the group was not expanded")
			}
		}},
		{"the target is in the pin section", "s3", func(m *Model) {
			m.baseSessions[3].PinnedAt = 5
			m.baseSessions = sortSessionsByOrder(m.baseSessions, m.baseSessions, SortOrderCreated)
			m.sessions = m.filteredSessions()
		}, nil},
		{"the target is already visible", "s1", nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, db := bookmarkKeyModel(t, true)
			if tc.setup != nil {
				tc.setup(&m)
			}
			m.selected = rowCursor(0)
			if err := db.AssignBookmark(context.Background(), 4, tc.target); err != nil {
				t.Fatal(err)
			}
			got, _ := sendKey(t, m, altRune('4'))
			if id := selectedID(t, got); id != tc.target {
				t.Fatalf("selected %s, want %s (note %q)", id, tc.target, got.bookmarkNote)
			}
			idx, _ := got.selected.SessionIndex()
			if !got.isSessionVisible(idx) {
				t.Fatal("the selected row is hidden")
			}
			name := fmt.Sprintf("b%s", strings.TrimPrefix(tc.target, "s"))
			if side := sidebarText(got); !strings.Contains(side, name) {
				t.Fatalf("the sidebar does not show %s:\n%s", name, side)
			}
			if tc.check != nil {
				tc.check(t, got)
			}
		})
	}
}
