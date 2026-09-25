package tui

import (
	"fmt"
	"testing"

	"github.com/n-orlov/deck/internal/store"
)

// cure-01-01-4 (R136/SPEC §11, binding ruling 002): review 154 re-ran the
// accepted R152-2 counterexamples and found the list-mode Esc branch (the
// one that clears a query held past Enter, tui.go's own top-level "esc"
// case) still only normalized the selection onto SOME visible stop by
// position, never followed the viewport to it, and -- for a row cursor --
// never re-anchored on the session's own ID once unfiltering could shift
// its numeric index. All three tests below fail on 610be00.
//
// Esc changes the query in two places: inside the filter editor (handled
// by filter.go's own updateFilter, not exercised here) or after Enter has
// returned keyboard focus to the list (exercised here). Both must follow
// the cursor; this file only exercises the list-mode path review 154
// found broken.
func TestReview152ClosedFilterEscapeKeepsSelectionVisible(t *testing.T) {
	m := groupTestModel(nil)
	m.width, m.height = 80, 24
	gid := int64(99)
	rows := []store.Session{{ID: "kept", Name: "keep", GroupID: &gid, GroupName: "zzz", Status: "idle"}}
	groups := []store.Group{{ID: gid, Name: "zzz"}}
	for i := 1; i <= 30; i++ {
		groups = append(groups, store.Group{ID: int64(i), Name: fmt.Sprintf("aaa%02d", i)})
	}
	next, _ := m.Update(sessionsLoaded{sessions: rows, groups: groups})
	m = next.(Model)
	for _, k := range []string{"/", "k", "e", "e", "p", "enter"} {
		next, _ = m.Update(key(k))
		m = next.(Model)
	}
	if m.filtering || m.filterQuery != "keep" {
		t.Fatalf("fixture: filter state=%v %q", m.filtering, m.filterQuery)
	}
	assertSelectionInView(t, m, "closed filter before Esc")

	next, _ = m.Update(key("esc"))
	got := next.(Model)
	if got.filterQuery != "" {
		t.Fatal("Esc did not clear held filter")
	}
	idx, ok := got.selected.SessionIndex()
	if !ok || got.sessions[idx].ID != "kept" {
		t.Fatal("fixture did not keep the same row")
	}
	assertSelectionInView(t, got, "list Esc clearing held filter")
}

// TestReview152HeldFilterEscapeKeepsHeaderVisible mirrors the row case
// above for a header cursor: `g` narrows to the one matching header while
// filtered, and Esc must retain that same header AND scroll it into view,
// not merely leave it "some" visible stop at whatever scroll offset the
// filtered render happened to compute.
func TestReview152HeldFilterEscapeKeepsHeaderVisible(t *testing.T) {
	m := groupTestModel(nil)
	m.width, m.height = 80, 24
	gid := int64(99)
	rows := []store.Session{{ID: "kept", Name: "keep", GroupID: &gid, GroupName: "zzz", Status: "idle"}}
	groups := []store.Group{{ID: gid, Name: "zzz"}}
	for i := 1; i <= 30; i++ {
		groups = append(groups, store.Group{ID: int64(i), Name: fmt.Sprintf("aaa%02d", i)})
	}
	next, _ := m.Update(sessionsLoaded{sessions: rows, groups: groups})
	m = next.(Model)
	for _, k := range []string{"/", "k", "e", "e", "p", "enter"} {
		next, _ = m.Update(key(k))
		m = next.(Model)
	}
	if m.filtering || m.filterQuery != "keep" {
		t.Fatalf("fixture: filter state=%v %q", m.filtering, m.filterQuery)
	}
	next, _ = m.Update(key("g"))
	m = next.(Model)
	if m.selected != headerCursor(gid) {
		t.Fatalf("g did not choose only matching header: %v", m.selected)
	}
	assertSelectionInView(t, m, "closed filter header before Esc")

	next, _ = m.Update(key("esc"))
	got := next.(Model)
	if got.filterQuery != "" {
		t.Fatal("Esc did not clear held filter")
	}
	if got.selected != headerCursor(gid) {
		t.Fatal("Esc did not retain the same header")
	}
	assertSelectionInView(t, got, "list Esc clearing held filter with header cursor")
}

// TestReview154HeldFilterEscapePreservesSessionID pins ruling 002's own
// binding: the SAME session ID, not merely a visible row at the same
// numeric position -- unfiltering inserts the nonmatch back BEFORE the
// kept session (by name sort), so a fix that only re-clamped the old raw
// index into the larger, unfiltered list would land on the wrong session.
func TestReview154HeldFilterEscapePreservesSessionID(t *testing.T) {
	m := groupTestModel(nil)
	m.width, m.height = 80, 24
	m.settings.SortOrder = SortOrderName
	rows := []store.Session{
		{ID: "other-id", Name: "aaa-other", Status: "idle"},
		{ID: "kept-id", Name: "keep", Status: "idle"},
	}
	next, _ := m.Update(sessionsLoaded{sessions: rows})
	m = next.(Model)
	if len(m.sessions) != 2 || m.sessions[0].ID != "other-id" || m.sessions[1].ID != "kept-id" {
		t.Fatalf("fixture must put the nonmatch before the match: %v", m.sessions)
	}
	for _, k := range []string{"/", "k", "e", "e", "p", "enter"} {
		next, _ = m.Update(key(k))
		m = next.(Model)
	}
	if m.filtering || m.filterQuery != "keep" || len(m.sessions) != 1 {
		t.Fatalf("invalid held-filter fixture: filtering=%v query=%q rows=%v", m.filtering, m.filterQuery, m.sessions)
	}
	index, ok := m.selected.SessionIndex()
	if !ok || m.sessions[index].ID != "kept-id" {
		t.Fatalf("fixture did not select matching session: cursor=%v rows=%v", m.selected, m.sessions)
	}

	next, _ = m.Update(key("esc"))
	got := next.(Model)
	if got.filterQuery != "" || len(got.sessions) != 2 {
		t.Fatal("Esc did not restore unfiltered rows")
	}
	index, ok = got.selected.SessionIndex()
	if !ok {
		t.Fatalf("Esc replaced selected session with header: %v", got.selected)
	}
	if id := got.sessions[index].ID; id != "kept-id" {
		t.Fatalf("held-filter Esc changed selected session ID: kept-id -> %s (raw index=%d)", id, index)
	}
}
