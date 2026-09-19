package tui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// This file is cure-01-02's own regression coverage for R129 ("defined
// but empty groups render with (0), default always exists") and R131's
// "another client's group edit becomes visible on the next reload" --
// review findings TestReviewEmptyGroupsRenderFromDB and its own two
// scratch reproductions above closed the gap groupSessions() left open:
// it only ever bucketed m.sessions, so a group with zero members left no
// trace there at all, and a totally empty store rendered "No sessions
// yet" even when a group HAD been defined. Every test below drives a
// real *store.Store through m.loadSessions()/m.Update (never
// groupHeaderText called directly against a fabricated sidebarGroup), and
// asserts against m.sidebarBodyLines' actual rendered text, per this
// task's own successCriteria.

// reviewReloadForEmptyGroups issues the exact same loadSessions ->
// Update round trip the normal reconcile-driven reload path uses,
// failing outright on a read error rather than silently rendering a
// stale frame.
func reviewReloadForEmptyGroups(t *testing.T, m Model) Model {
	t.Helper()
	msg := m.loadSessions()
	sl, ok := msg.(sessionsLoaded)
	if !ok {
		t.Fatalf("loadSessions() returned %T, want sessionsLoaded", msg)
	}
	if sl.err != nil {
		t.Fatalf("loadSessions() sessions error: %v", sl.err)
	}
	if sl.groupsErr != nil {
		t.Fatalf("loadSessions() groups error: %v", sl.groupsErr)
	}
	next, _ := m.Update(sl)
	return next.(Model)
}

// emptyGroupsTestStore opens a real, on-disk state.db the same way every
// other store-backed test in this package does (openStoreForLastCreateGroup's
// own precedent), so ListGroups/ListSessions exercise the real SQL, not an
// in-memory fixture.
func emptyGroupsTestStore(t *testing.T) *store.Store {
	t.Helper()
	home := t.TempDir()
	db, err := store.OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// TestEmptyGroupsRenderFromDBWithZeroSessions is the review's own
// "populated=false" case: a group is defined ("empty-visible") but the
// store holds NO sessions at all. Before this task's fix,
// sidebarEntries' zero-sessions short-circuit fired unconditionally and
// rendered "No sessions yet." with no group header at all, even though a
// real, persisted group already exists and SPEC \u00a711 promises "a group
// the user defined but has not filled yet still renders". The structural
// default group (also zero members here) must render too, and last.
func TestEmptyGroupsRenderFromDBWithZeroSessions(t *testing.T) {
	db := emptyGroupsTestStore(t)
	g, err := db.CreateGroup(context.Background(), "empty-visible")
	if err != nil {
		t.Fatal(err)
	}
	m := New(db, config.Settings{}, "")
	m.width, m.height = 100, 40
	m = reviewReloadForEmptyGroups(t, m)

	if len(m.sessions) != 0 {
		t.Fatalf("fixture has %d sessions, want 0", len(m.sessions))
	}
	body := strings.Join(m.sidebarBodyLines(60), "\n")
	if strings.Contains(body, "No sessions yet") {
		t.Fatalf("zero-sessions message shown despite a defined group; sidebar=%q", body)
	}
	if !strings.Contains(body, g.Name+"  (0)") {
		t.Errorf("defined empty group %q missing after reload; sidebar=%q", g.Name, body)
	}
	if !strings.Contains(body, "default  (0)") {
		t.Errorf("structural default header missing; sidebar=%q", body)
	}
	// default must render LAST, after the real group, per groupSortsBefore.
	if strings.Index(body, "default  (0)") < strings.Index(body, g.Name+"  (0)") {
		t.Errorf("default did not render last; sidebar=%q", body)
	}
}

// TestEmptyGroupsRenderFromDBWithUnrelatedPopulatedGroup is the review's
// "populated=true" case: a wholly unrelated group ("other") has one real
// member, and a second, still-empty group ("empty-visible") is also
// defined. Before this fix, sidebarEntries rendered only the populated
// group's own header + row and never noticed the second, empty one,
// because groupSessions() only ever iterates m.sessions -- a group with
// no members leaves no trace there to discover.
func TestEmptyGroupsRenderFromDBWithUnrelatedPopulatedGroup(t *testing.T) {
	db := emptyGroupsTestStore(t)
	g, err := db.CreateGroup(context.Background(), "empty-visible")
	if err != nil {
		t.Fatal(err)
	}
	other, err := db.CreateGroup(context.Background(), "other")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateSession(context.Background(), store.CreateSessionInput{
		ID: "00000000-0000-4000-8000-000000000001", Name: "member-1", CWD: "/tmp",
		Agent: "shell", CapturedPath: "/bin", Status: "stopped", StatusAt: 100,
		CreatedAt: 100, GroupID: &other.ID,
	}); err != nil {
		t.Fatal(err)
	}

	m := New(db, config.Settings{}, "")
	m.width, m.height = 100, 40
	m = reviewReloadForEmptyGroups(t, m)

	if len(m.sessions) != 1 {
		t.Fatalf("fixture has %d sessions, want 1", len(m.sessions))
	}
	body := strings.Join(m.sidebarBodyLines(60), "\n")
	if !strings.Contains(body, "other  (1)") {
		t.Errorf("populated group's own header/count missing; sidebar=%q", body)
	}
	if !strings.Contains(body, "member-1") {
		t.Errorf("populated group's own member row missing; sidebar=%q", body)
	}
	if !strings.Contains(body, g.Name+"  (0)") {
		t.Errorf("unrelated empty group %q missing alongside the populated one; sidebar=%q", g.Name, body)
	}
	if !strings.Contains(body, "default  (0)") {
		t.Errorf("structural default header missing; sidebar=%q", body)
	}
}

// TestEmptyGroupsHiddenUnderAnActiveFilter proves the other half of the
// same criterion: seeding every defined group unconditionally would break
// SPEC \u00a711's "under an active filter only groups with a match render"
// -- a defined-but-empty group can never have a filter match, by
// construction, so it must stay hidden the moment a query is in force,
// even though it renders fine unfiltered.
func TestEmptyGroupsHiddenUnderAnActiveFilter(t *testing.T) {
	db := emptyGroupsTestStore(t)
	if _, err := db.CreateGroup(context.Background(), "empty-visible"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateSession(context.Background(), store.CreateSessionInput{
		ID: "00000000-0000-4000-8000-000000000002", Name: "solo-member", CWD: "/tmp",
		Agent: "shell", CapturedPath: "/bin", Status: "stopped", StatusAt: 100, CreatedAt: 100,
	}); err != nil {
		t.Fatal(err)
	}

	m := New(db, config.Settings{}, "")
	m.width, m.height = 100, 40
	m = reviewReloadForEmptyGroups(t, m)
	m.filterQuery = "solo-member"
	m.sessions = m.filteredSessions()

	body := strings.Join(m.sidebarBodyLines(60), "\n")
	if strings.Contains(body, "empty-visible") {
		t.Errorf("defined-but-empty group rendered under an active filter with no match; sidebar=%q", body)
	}
	if !strings.Contains(body, "solo-member") {
		t.Errorf("the actually-matching session is missing; sidebar=%q", body)
	}
}

// TestEmptyGroupCollapsesByItsDurableID proves the other half of this
// task's "empty-group headers use their durable IDs for collapse and
// mouse hit-testing" criterion: toggling collapse for a defined-but-empty
// group (by its real store.Group.ID, the same identity sidebarEntries
// hands hitTest/mouse.go through sidebarEntry.groupID) actually flips
// that header's own marker, exactly as it would for a populated group --
// collapse is keyed by id, never by the fact that a group happens to have
// members loaded right now.
func TestEmptyGroupCollapsesByItsDurableID(t *testing.T) {
	db := emptyGroupsTestStore(t)
	g, err := db.CreateGroup(context.Background(), "empty-visible")
	if err != nil {
		t.Fatal(err)
	}
	m := New(db, config.Settings{}, "")
	m.width, m.height = 100, 40
	m = reviewReloadForEmptyGroups(t, m)

	expanded := strings.Join(m.sidebarBodyLines(60), "\n")
	if !strings.Contains(expanded, "\u25be "+g.Name+"  (0)") {
		t.Fatalf("expanded empty group header missing its expanded marker; sidebar=%q", expanded)
	}

	m.toggleGroupCollapse(g.ID)
	collapsed := strings.Join(m.sidebarBodyLines(60), "\n")
	if !strings.Contains(collapsed, "\u25b8 "+g.Name+"  (0)") {
		t.Errorf("toggling collapse by the group's durable id did not flip its marker; sidebar=%q", collapsed)
	}
}

// TestGroupCreatedByAnotherClientAppearsAfterReload is this task's own
// R131 half: a group another client creates on the SAME state.db file
// must appear in THIS client's sidebar, with (0), the moment its own
// next normal reload runs -- never requiring a restart, and never
// requiring that group to already hold a member this client happens to
// have loaded. Two independent *store.Store handles on one file (the
// same store.OpenPath pattern task 020's shared-state.db test uses)
// stand in for the two clients.
func TestGroupCreatedByAnotherClientAppearsAfterReload(t *testing.T) {
	home := t.TempDir()
	dbPath := filepath.Join(home, "state.db")
	clientA, err := store.OpenPath(home, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { clientA.Close() })
	clientB, err := store.OpenPath(home, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { clientB.Close() })

	m := New(clientB, config.Settings{}, "")
	m.width, m.height = 100, 40
	m = reviewReloadForEmptyGroups(t, m)
	before := strings.Join(m.sidebarBodyLines(60), "\n")
	if strings.Contains(before, "shared-by-a") {
		t.Fatalf("group already visible before client A ever created it; sidebar=%q", before)
	}

	if _, err := clientA.CreateGroup(context.Background(), "shared-by-a"); err != nil {
		t.Fatal(err)
	}

	m = reviewReloadForEmptyGroups(t, m)
	after := strings.Join(m.sidebarBodyLines(60), "\n")
	if !strings.Contains(after, "shared-by-a  (0)") {
		t.Errorf("client A's group did not appear in client B's sidebar on the next reload; sidebar=%q", after)
	}
	if !strings.Contains(after, "default  (0)") {
		t.Errorf("structural default header missing after the reload; sidebar=%q", after)
	}
}
