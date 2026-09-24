package tui

import (
	"testing"

	"github.com/n-orlov/deck/internal/store"
)

// TestCure010103ExplicitHeaderOnEmptySidebarSurvivesBackgroundArrival is
// cure-01-01-3's own regression (review's own
// TestReview132ExplicitHeaderOnEmptySidebarSurvivesBackgroundArrival):
// header-only does not mean nobody has deliberately navigated. cure-01-01-2's
// hadNoSessionsAtAll heuristic (tui.go's sessionsLoaded case) could not tell
// an explicitly navigated header on a header-only sidebar apart from the
// automatic zero-value-cursor promotion it exists for
// (TestCure0105FirstSessionUnderHeaderOnlyLoadFollowsSelection), so a
// background arrival (another client's first session landing in the
// selected, still-empty group, with no local creation intent) stole the
// deliberately selected header. Fail-before (HEAD 10c5021): "background
// arrival stole explicitly navigated header on header-only sidebar:
// {1 0 2} -> {0 0 0}; pending=\"\"" (artifacts/review132/transitions.log).
func TestCure010103ExplicitHeaderOnEmptySidebarSurvivesBackgroundArrival(t *testing.T) {
	m := groupTestModel(nil)
	m.width, m.height = 80, 24
	groups := []store.Group{{ID: 1, Name: "aaa"}, {ID: 2, Name: "bbb"}}
	n, _ := m.Update(sessionsLoaded{groups: groups})
	m = n.(Model)
	n, _ = m.Update(key("g"))
	m = n.(Model)
	if m.selected != headerCursor(1) {
		t.Fatalf("fixture: initial header=%v, want header 1 (g jumps to the first stop)", m.selected)
	}
	n, _ = m.Update(key("down"))
	m = n.(Model)
	if m.selected != headerCursor(2) {
		t.Fatalf("fixture: Down did not deliberately select header 2: %v", m.selected)
	}
	if m.pendingSelectSessionID != "" {
		t.Fatal("fixture unexpectedly has local creation intent")
	}
	gid := int64(2)
	n, _ = m.Update(sessionsLoaded{
		groups:   groups,
		sessions: []store.Session{{ID: "external", Name: "external", GroupID: &gid, GroupName: "bbb", Status: "idle"}},
	})
	got := n.(Model)
	if got.selected != headerCursor(2) {
		t.Fatalf("background arrival stole explicitly navigated header on header-only sidebar: %v -> %v; pending=%q", m.selected, got.selected, got.pendingSelectSessionID)
	}
	assertSelectionInView(t, got, "preserved explicit header on header-only sidebar")
}
