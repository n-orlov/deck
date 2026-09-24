package tui

import (
	"testing"

	"github.com/n-orlov/deck/internal/store"
)

// TestReview113ExplicitHeaderSurvivesBackgroundArrival is cure-01-01-2's
// own R136/SPEC §11 regression: a header the user DELIBERATELY navigated
// to (via g, not the zero-value default cursor an empty startup ever
// promotes automatically) must keep its identity across a background
// reload that gives its own, still-empty bucket its first row -- unless a
// local creation intent (pendingSelectSessionID) says otherwise. This is
// the mirror image of cure-01-05's own
// TestCure0105FirstSessionUnderHeaderOnlyLoadFollowsSelection (task 022
// sweep), which is the OTHER case: nothing existed anywhere yet, so the
// header cursor there was never a deliberate stop, only the zero-value
// cursor's own automatic promotion. Fail-before (pre-cure-01-01-2, HEAD
// 46b4073): "background arrival stole explicit header cursor {1 0 2} ->
// {0 3 0} without a creation intent or selection key"
// (artifacts/review113/hidden-stop-probes.log).
func TestReview113ExplicitHeaderSurvivesBackgroundArrival(t *testing.T) {
	m := viewportFollowTestModel(3, 24)
	m.allGroups = []store.Group{{ID: 2, Name: "aaa-empty"}}
	// A deliberate navigation stop, not automatic initial empty-list
	// selection: other sessions already exist elsewhere.
	n, _ := m.Update(key("g"))
	m = n.(Model)
	if m.selected != headerCursor(2) {
		t.Fatalf("fixture cursor=%v, want explicitly selected empty header 2", m.selected)
	}
	if m.pendingSelectSessionID != "" {
		t.Fatal("fixture unexpectedly has local create intent")
	}
	incoming := append([]store.Session(nil), m.sessions...)
	gid := int64(2)
	incoming = append(incoming, store.Session{ID: "from-another-client", Name: "new", GroupName: "aaa-empty", GroupID: &gid, Status: "idle"})
	n, _ = m.Update(sessionsLoaded{sessions: incoming, groups: m.allGroups})
	got := n.(Model)
	if got.selected != headerCursor(2) {
		t.Fatalf("background arrival stole explicit header cursor %v -> %v without a creation intent or selection key", m.selected, got.selected)
	}
}

// TestReview113NewSessionIntentCannotSelectFoldedRow is cure-01-01-2's own
// R137 regression: pendingSelectSessionID's one-shot creation override
// (the ordinary create path, not a raw cursor the test hands in) must
// never select a row hidden by its own folded group -- the new session's
// group must be unfolded so its complete row is actually exposed.
// Fail-before (HEAD 46b4073): "new-session selection intent chose a
// hidden stop: cursor={kind:0 index:3 groupID:0} collapsed=map[1:true]
// pending=\"\"" (artifacts/review113/hidden-stop-probes.log).
func TestReview113NewSessionIntentCannotSelectFoldedRow(t *testing.T) {
	m := viewportFollowTestModel(3, 24)
	m.setSelection(rowCursor(0))
	n, _ := m.Update(key("c"))
	m = n.(Model)
	if !m.isGroupCollapsed(1) || m.selected != headerCursor(1) {
		t.Fatal("fixture did not fold")
	}
	incoming := append([]store.Session(nil), m.sessions...)
	s := incoming[0]
	s.ID = "new"
	s.Name = "new"
	incoming = append(incoming, s)
	n, _ = m.Update(shellCreated{session: s})
	m = n.(Model)
	if m.pendingSelectSessionID != s.ID {
		t.Fatal("shellCreated did not establish selection intent")
	}
	n, _ = m.Update(sessionsLoaded{sessions: incoming})
	got := n.(Model)
	if !got.cursorNamesVisibleStop(got.selected) {
		t.Fatalf("new-session selection intent chose a hidden stop: cursor=%+v collapsed=%v pending=%q", got.selected, got.collapsedGroups, got.pendingSelectSessionID)
	}
	if got.isGroupCollapsed(1) {
		t.Fatalf("new session landed in a group still collapsed=%v; its row is not actually exposed", got.collapsedGroups)
	}
	assertSelectionInView(t, got, "new session in collapsed group")
}

// TestReview113ArchivedReloadCannotLeaveAbsentHeader is cure-01-01-2's own
// R136/SPEC §11 regression: archivedSessionsLoaded used to only clamp a
// raw row index, never normalize a header cursor whose bucket the
// archive removal just emptied. Fail-before (HEAD 46b4073): "archived
// reload left absent header cursor={kind:1 index:0 groupID:1} while
// remaining row is live" (artifacts/review113/hidden-stop-probes.log).
func TestReview113ArchivedReloadCannotLeaveAbsentHeader(t *testing.T) {
	a, b := int64(1), int64(2)
	live := store.Session{ID: "live", Name: "keep-live", GroupName: "bbb", GroupID: &b}
	archived := store.Session{ID: "archived", Name: "keep-archived", GroupName: "aaa", GroupID: &a, ArchivedAt: 1}
	m := groupTestModel([]store.Session{live})
	m.width, m.height = 80, 24
	m.baseSessions = []store.Session{live}
	m.archivedSessions = []store.Session{archived}
	m.filterQuery = "keep"
	m.sessions = m.filteredSessions()
	m.setSelection(headerCursor(a))
	n, _ := m.Update(archivedSessionsLoaded{sessions: nil})
	got := n.(Model)
	if len(got.sessions) != 1 {
		t.Fatal("fixture did not remove archive")
	}
	if !got.cursorNamesVisibleStop(got.selected) {
		t.Fatalf("archived reload left absent header cursor=%+v while remaining row is %s", got.selected, got.sessions[0].ID)
	}
	assertSelectionInView(t, got, "archived reload")
}

// TestReview113ArchivedReloadCannotSelectHiddenRow is
// TestReview113ArchivedReloadCannotLeaveAbsentHeader's row-cursor
// counterpart: the surviving row belongs to a collapsed group, so the
// clamp must not land the cursor on a row its own fold hides.
// Fail-before (HEAD 46b4073): "archive removal clamped onto hidden
// cursor={kind:0 index:0 groupID:0} in collapsed=map[2:true]"
// (artifacts/review113/hidden-stop-probes.log).
func TestReview113ArchivedReloadCannotSelectHiddenRow(t *testing.T) {
	a, b := int64(1), int64(2)
	live := store.Session{ID: "live", Name: "keep-live", GroupName: "bbb", GroupID: &b}
	archived := store.Session{ID: "archived", Name: "keep-archived", GroupName: "aaa", GroupID: &a, ArchivedAt: 1}
	m := groupTestModel([]store.Session{live})
	m.width, m.height = 80, 24
	m.baseSessions = []store.Session{live}
	m.archivedSessions = []store.Session{archived}
	m.filterQuery = "keep"
	m.sessions = m.filteredSessions()
	m.setGroupCollapsed(b, true)
	m.setSelection(rowCursor(1))
	n, _ := m.Update(archivedSessionsLoaded{sessions: nil})
	got := n.(Model)
	if !got.cursorNamesVisibleStop(got.selected) {
		t.Fatalf("archive removal clamped onto hidden cursor=%+v in collapsed=%v", got.selected, got.collapsedGroups)
	}
	assertSelectionInView(t, got, "archived reload hidden-row clamp")
}
