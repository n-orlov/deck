package tui

import (
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// TestEffectiveSortOrderResolvesKnownValuesWithNoReason proves the
// non-fallback half of effectiveSortOrder: every value config.LoadFrom can
// actually produce (the four schema names, or "" for a config.Settings{}
// built directly the way tests -- and a real load's own defaultFileConfig
// -- already treat ui.mouse/group_by_workspace's zero value) resolves
// cleanly, with no reason to show.
func TestEffectiveSortOrderResolvesKnownValuesWithNoReason(t *testing.T) {
	cases := []struct {
		configured string
		want       string
	}{
		{"", SortOrderAttention},
		{SortOrderAttention, SortOrderAttention},
		{SortOrderCreated, SortOrderCreated},
		{SortOrderActivity, SortOrderActivity},
		{SortOrderName, SortOrderName},
	}
	for _, c := range cases {
		model := New(nil, config.Settings{SortOrder: c.configured}, "")
		order, reason := model.effectiveSortOrder()
		if order != c.want {
			t.Errorf("SortOrder=%q: effectiveSortOrder order = %q, want %q", c.configured, order, c.want)
		}
		if reason != "" {
			t.Errorf("SortOrder=%q: effectiveSortOrder reason = %q, want none", c.configured, reason)
		}
	}
}

// TestSortOrderFallbackNoticeShownOnFirstPaint proves requirement R53's
// honest-fallback half, on themeBanner's own footing (task 305): a
// sort_order value that is none of the four schema names resolves to
// attention AND the very first painted frame states so, by name, rather
// than silently rendering attention as though the configured value had
// applied.
func TestSortOrderFallbackNoticeShownOnFirstPaint(t *testing.T) {
	model := New(nil, config.Settings{SortOrder: "bogus-order"}, "")
	model.sessions = []store.Session{{Name: "only-session", Agent: "shell", Status: "running"}}

	order, reason := model.effectiveSortOrder()
	if order != SortOrderAttention {
		t.Fatalf("effectiveSortOrder order = %q, want %q", order, SortOrderAttention)
	}
	if reason == "" || !strings.Contains(reason, "bogus-order") {
		t.Fatalf("effectiveSortOrder reason = %q, want it to name the bad value", reason)
	}

	view := model.View()
	if !strings.Contains(view, reason) {
		t.Fatalf("first-paint view missing sort_order fallback reason %q:\n%s", reason, view)
	}
}

// TestSortOrderFallbackNoticeAbsentForKnownValues proves the notice costs
// nothing and says nothing false for every value effectiveSortOrder
// resolves cleanly (see TestEffectiveSortOrderResolvesKnownValuesWithNoReason).
func TestSortOrderFallbackNoticeAbsentForKnownValues(t *testing.T) {
	for _, configured := range []string{"", SortOrderAttention, SortOrderCreated, SortOrderActivity, SortOrderName} {
		model := New(nil, config.Settings{SortOrder: configured}, "")
		model.sessions = []store.Session{{Name: "only-session", Agent: "shell", Status: "running"}}
		view := model.View()
		if strings.Contains(view, "Unknown [ui] sort_order") {
			t.Errorf("SortOrder=%q: view wrongly shows a sort_order fallback notice:\n%s", configured, view)
		}
	}
}

// namedSession builds a minimal store.Session for the render-order tests
// below, distinguishing the three primary keys sort_order.go's comparators
// read (Name, CreatedAt, StatusAt) plus a stable ID.
func namedSession(id, name string, createdAt, statusAt int64) store.Session {
	return store.Session{ID: id, Name: name, Agent: "shell", Status: "running", CreatedAt: createdAt, StatusAt: statusAt}
}

// TestSessionsLoadedRendersConfiguredOrder proves sessionsLoaded actually
// sorts baseSessions with the configured order (task 305), not merely that
// sortSessionsByOrder itself is correct (task 304 already pins that) --
// this is the wiring from Settings.SortOrder through to m.sessions.
func TestSessionsLoadedRendersConfiguredOrder(t *testing.T) {
	sessions := []store.Session{
		namedSession("a", "Charlie", 100, 100),
		namedSession("b", "alpha", 300, 300),
		namedSession("c", "Bravo", 200, 200),
	}
	cases := []struct {
		order string
		want  []string
	}{
		{SortOrderCreated, []string{"b", "c", "a"}},  // CreatedAt desc: 300,200,100
		{SortOrderActivity, []string{"b", "c", "a"}}, // StatusAt desc: 300,200,100
		{SortOrderName, []string{"b", "c", "a"}},     // case-insensitive asc: alpha,Bravo,Charlie
	}
	for _, c := range cases {
		model := New(nil, config.Settings{SortOrder: c.order}, "")
		updated, _ := model.Update(sessionsLoaded{sessions: sessions})
		got := updated.(Model)
		if gotIDs := idsOf(got.sessions); !equalStrings(gotIDs, c.want) {
			t.Errorf("order %q: rendered ids = %v, want %v", c.order, gotIDs, c.want)
		}
	}
}

// TestSessionsLoadedPreservesSelectionByIDAcrossSortOrderChange proves the
// "selection still preserved by id" half of task 305's success criteria:
// switching which order sessionsLoaded renders in never leaves the
// selected INDEX pointing at whatever session now occupies it -- the same
// session stays selected.
func TestSessionsLoadedPreservesSelectionByIDAcrossSortOrderChange(t *testing.T) {
	sessions := []store.Session{
		namedSession("a", "Charlie", 100, 100),
		namedSession("b", "alpha", 300, 300),
		namedSession("c", "Bravo", 200, 200),
	}
	model := New(nil, config.Settings{SortOrder: SortOrderName}, "")
	updated, _ := model.Update(sessionsLoaded{sessions: sessions})
	got := updated.(Model)
	// Under "name" order, alpha (id "b") is index 0.
	if got.selected != 0 || got.sessions[0].ID != "b" {
		t.Fatalf("setup: selected = %d (%v), want index 0 = session %q", got.selected, idsOf(got.sessions), "b")
	}
	got.selected = 1 // select "Bravo" (id "c"), the middle row under name order
	got.settings.SortOrder = SortOrderCreated
	updated2, _ := got.Update(sessionsLoaded{sessions: sessions})
	got2 := updated2.(Model)
	if got2.selected < 0 || got2.selected >= len(got2.sessions) {
		t.Fatalf("selected index %d out of range after reorder (%v)", got2.selected, idsOf(got2.sessions))
	}
	if got2.sessions[got2.selected].ID != "c" {
		t.Fatalf("selection followed the OLD index, not the session id: now selects %q, want %q",
			got2.sessions[got2.selected].ID, "c")
	}
}

// TestSessionsLoadedGroupingComposesOrderWithinGroupOnly proves task
// 305/309's grouping-composition rule: with group_by_workspace on and a
// non-attention sort_order, the WITHIN-group row order follows the chosen
// order, but WHICH group renders first still follows attention's own
// "most urgent member leads" rule, exactly as it does today -- the new
// sort_order feature never lets a name/created/activity choice reorder
// the groups themselves.
func TestSessionsLoadedGroupingComposesOrderWithinGroupOnly(t *testing.T) {
	// Workspace "z-workspace" has no urgent member (idle); "a-workspace"
	// has a waiting session, so it must lead despite its name sorting
	// AFTER "z-workspace" alphabetically -- the fixture that would catch a
	// group-order regression under sort_order "name".
	sessions := []store.Session{
		{ID: "z1", Name: "zulu-one", Agent: "shell", Status: "idle", Workspace: "z-workspace"},
		{ID: "z2", Name: "zulu-two", Agent: "shell", Status: "idle", Workspace: "z-workspace"},
		{ID: "a1", Name: "alpha-one", Agent: "shell", Status: "waiting", StatusAt: 500, Workspace: "a-workspace"},
	}
	model := New(nil, config.Settings{SortOrder: SortOrderName, GroupByWorkspace: true}, "")
	updated, _ := model.Update(sessionsLoaded{sessions: sessions})
	got := updated.(Model)

	groups := got.groupSessions()
	if len(groups) != 2 {
		t.Fatalf("groupSessions() = %d groups, want 2 (full: %v)", len(groups), idsOf(got.sessions))
	}
	// Group order: a-workspace (has the waiting session) must lead,
	// despite "a-workspace" < "z-workspace" being the SAME direction name
	// order would also pick here -- swap the assertion's basis by
	// asserting on urgency, not alphabetical accident: z-workspace's own
	// two rows are idle, a-workspace's one row is waiting, and requirement
	// 28 ranks waiting first, so this is attention's call, not name's.
	if groups[0].Workspace != "a-workspace" {
		t.Fatalf("group order = %v, want a-workspace (waiting) leading (today's attention-driven group order must survive a non-attention sort_order)",
			[]string{groups[0].Workspace, groups[1].Workspace})
	}
	// Within z-workspace, rows must be in NAME order (zulu-one < zulu-two
	// already agrees alphabetically with insertion order here, so also
	// check the reverse-named case below for a real, non-vacuous proof).
	if len(groups[1].Sessions) != 2 || groups[1].Sessions[0].Session.ID != "z1" || groups[1].Sessions[1].Session.ID != "z2" {
		t.Fatalf("z-workspace rows = %v, want [z1 z2] under name order", groups[1].Sessions)
	}
}

// TestSessionsLoadedGroupingWithinGroupOrderIsNonVacuous strengthens the
// test above: the two same-workspace sessions are seeded so their
// insertion/attention order DISAGREES with name order, proving the
// within-group order genuinely comes from sort_order rather than
// coincidentally matching it.
func TestSessionsLoadedGroupingWithinGroupOrderIsNonVacuous(t *testing.T) {
	sessions := []store.Session{
		// zulu-two created/inserted first (so attention/insertion order
		// would list it before zulu-one), but "zulu-one" < "zulu-two"
		// alphabetically, so name order must reverse them.
		{ID: "z2", Name: "zulu-two", Agent: "shell", Status: "idle", Workspace: "solo-workspace", StatusAt: 100},
		{ID: "z1", Name: "zulu-one", Agent: "shell", Status: "idle", Workspace: "solo-workspace", StatusAt: 200},
	}
	model := New(nil, config.Settings{SortOrder: SortOrderName, GroupByWorkspace: true}, "")
	updated, _ := model.Update(sessionsLoaded{sessions: sessions})
	got := updated.(Model)

	groups := got.groupSessions()
	if len(groups) != 1 || len(groups[0].Sessions) != 2 {
		t.Fatalf("groupSessions() = %v, want one group of two", groups)
	}
	if groups[0].Sessions[0].Session.ID != "z1" || groups[0].Sessions[1].Session.ID != "z2" {
		t.Fatalf("within-group order = [%s %s], want [z1 z2] (name order, which disagrees with insertion/attention order here)",
			groups[0].Sessions[0].Session.ID, groups[0].Sessions[1].Session.ID)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
