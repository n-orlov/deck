package tui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// This file is finding B1's own regression: internal/tui/filter.go:46-59
// matched sessionGroupKey(session) (internal/tui/group.go:47-49), which
// returns store.Session.GroupName verbatim -- empty both for a NULL
// group_id and for a dangling one -- so typing "/default" matched nothing
// even though the sidebar header both cases fall under literally reads
// "default  (n)" (groupHeaderText, internal/tui/group.go:409-441). The cure
// switches filterMatches' group leg to sessionGroupLabel (group.go:85-103),
// the same seam the `i`/`g` dialogs already resolve through, which degrades
// that same empty key to the literal string "default".
//
// filterDefaultLabelTestStore opens a real, on-disk state.db (the same
// store.OpenPath pattern every other store-backed test in this package
// uses, e.g. empty_groups_render_test.go's emptyGroupsTestStore) so the
// NULL and dangling group_id cases below are the real SQL LEFT JOIN
// degrade (store.go:641-643), never an in-memory GroupName="" fixture that
// only coincidentally looks the same.
func filterDefaultLabelTestStore(t *testing.T) *store.Store {
	t.Helper()
	home := t.TempDir()
	db, err := store.OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// filterDefaultLabelReload issues the same loadSessions -> Update round
// trip every other store-backed test in this package uses (e.g.
// empty_groups_render_test.go's reviewReloadForEmptyGroups), failing
// outright on a read error rather than silently rendering a stale frame.
func filterDefaultLabelReload(t *testing.T, m Model) Model {
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

// TestFilterByDefaultMatchesTheSidebarsOwnDefaultLabel is finding B1's
// regression: a store holding one NULL-group_id session, one
// dangling-group_id session (its group was created, then deleted, so
// group_id still names an id no groups row resolves any more) and one
// named-group control must, once "/default" is typed, keep both default
// members on screen, show that group's header as "default  (2)" (its
// MATCHING count, per TestFilterHidesTheHeaderOfAGroupWithNoMatch's own
// precedent -- there are only two default members total here, so the
// matching and unfiltered counts happen to coincide, but the count comes
// off the already-filtered bucket either way) and exclude the
// named-group control entirely, since "crew-tools" contains no substring
// "default" in its own name, group or cwd.
func TestFilterByDefaultMatchesTheSidebarsOwnDefaultLabel(t *testing.T) {
	db := filterDefaultLabelTestStore(t)
	ctx := context.Background()

	dangling, err := db.CreateGroup(ctx, "gone-group")
	if err != nil {
		t.Fatal(err)
	}
	named, err := db.CreateGroup(ctx, "crew-tools")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := db.CreateSession(ctx, store.CreateSessionInput{
		ID: "00000000-0000-4000-8000-000000000001", Name: "null-member", CWD: "/repos/null",
		Agent: "shell", CapturedPath: "/bin/sh", Status: "idle", StatusAt: 100, CreatedAt: 100,
		// GroupID left nil: the true NULL-group_id case.
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateSession(ctx, store.CreateSessionInput{
		ID: "00000000-0000-4000-8000-000000000002", Name: "dangling-member", CWD: "/repos/dangling",
		Agent: "shell", CapturedPath: "/bin/sh", Status: "idle", StatusAt: 100, CreatedAt: 100,
		GroupID: &dangling.ID,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateSession(ctx, store.CreateSessionInput{
		ID: "00000000-0000-4000-8000-000000000003", Name: "control-member", CWD: "/repos/control",
		Agent: "shell", CapturedPath: "/bin/sh", Status: "idle", StatusAt: 100, CreatedAt: 100,
		GroupID: &named.ID,
	}); err != nil {
		t.Fatal(err)
	}
	// Delete the group the second session was assigned to AFTER assigning
	// it, so that session's group_id now dangles (SPEC §11: "a group_id
	// that no longer resolves ... renders under default"), rather than
	// ever being nil the way the first session's always was.
	if err := db.DeleteGroup(ctx, dangling.ID); err != nil {
		t.Fatal(err)
	}

	m := New(db, config.Settings{}, "")
	m.width, m.height = 100, 40
	m = filterDefaultLabelReload(t, m)

	if len(m.sessions) != 3 {
		t.Fatalf("fixture has %d sessions, want 3", len(m.sessions))
	}

	// Sanity: unfiltered, both default members render under a "default"
	// header alongside the named control.
	unfiltered := m.View()
	if rowLine(unfiltered, "null-member") == "" || rowLine(unfiltered, "dangling-member") == "" || rowLine(unfiltered, "control-member") == "" {
		t.Fatalf("fixture setup failed: not all three sessions render unfiltered:\n%s", unfiltered)
	}

	got, _ := m.Update(key("/"))
	m = got.(Model)
	if !m.filtering {
		t.Fatal("/ did not open the filter")
	}
	for _, r := range "default" {
		got, _ = m.Update(key(string(r)))
		m = got.(Model)
	}

	view := m.View()
	if rowLine(view, "null-member") == "" {
		t.Fatalf("NULL-group_id session missing from the /default filter:\n%s", view)
	}
	if rowLine(view, "dangling-member") == "" {
		t.Fatalf("dangling-group_id session missing from the /default filter:\n%s", view)
	}
	if rowLine(view, "control-member") != "" {
		t.Fatalf("named-group control shown under a /default filter:\n%s", view)
	}
	header := rowLine(view, "default")
	if header == "" {
		t.Fatalf("no \"default\" header rendered under a /default filter:\n%s", view)
	}
	if !strings.Contains(header, "default  (2)") {
		t.Fatalf("default header = %q, want it to read \"default  (2)\" (both default members matched)", header)
	}
}
