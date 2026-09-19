package tui

import (
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// filterTestSessions returns three active sessions distinguished so a
// filter on exactly one field never accidentally matches another: each
// name, workspace and cwd is unique across all three, and none of the
// three fields shares a substring with any other session's same field.
func filterTestSessions() []store.Session {
	return []store.Session{
		{ID: "s-alpha", Name: "alpha-agent", GroupName: "ws-north", CWD: "/repos/north-project", Agent: "shell", Status: "running"},
		{ID: "s-beta", Name: "beta-agent", GroupName: "ws-south", CWD: "/repos/south-project", Agent: "shell", Status: "running"},
		{ID: "s-gamma", Name: "gamma-agent", GroupName: "ws-east", CWD: "/repos/east-project", Agent: "shell", Status: "running"},
	}
}

// newFilterTestModel builds a Model with baseSessions/sessions seeded the
// way the real bootstrap (sessionsLoaded) would: both fields equal, since
// filteredSessions() is the only thing ever allowed to diverge them.
func newFilterTestModel(sessions []store.Session) Model {
	model := New(nil, config.Settings{}, "")
	model.baseSessions = sessions
	model.sessions = sessions
	model.selected = 0
	return model
}

// rowLine returns the first screen line containing needle, or "" if none
// does.
func rowLine(view, needle string) string {
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, needle) {
			return line
		}
	}
	return ""
}

// TestFilterByNameShowsOnlyTheMatchingRow proves SPEC requirement 33's
// "filters ... by name": typing a query that matches exactly one session's
// name (and no other session's name, workspace or cwd) leaves only that
// row on screen.
func TestFilterByNameShowsOnlyTheMatchingRow(t *testing.T) {
	model := newFilterTestModel(filterTestSessions())
	got, _ := model.Update(key("/"))
	model = got.(Model)
	if !model.filtering {
		t.Fatal("/ did not open the filter")
	}
	for _, r := range "alpha-agent" {
		got, _ = model.Update(key(string(r)))
		model = got.(Model)
	}
	view := model.View()
	if rowLine(view, "alpha-agent") == "" {
		t.Fatalf("matching row not shown while filtering by name:\n%s", view)
	}
	if rowLine(view, "beta-agent") != "" || rowLine(view, "gamma-agent") != "" {
		t.Fatalf("non-matching rows still shown while filtering by name:\n%s", view)
	}
}

// TestFilterByGroupShowsOnlyTheMatchingRow proves the "group" leg
// of requirement 33's three named fields, using a query that appears in NO
// session's name or cwd -- only its workspace.
func TestFilterByGroupShowsOnlyTheMatchingRow(t *testing.T) {
	model := newFilterTestModel(filterTestSessions())
	got, _ := model.Update(key("/"))
	model = got.(Model)
	for _, r := range "ws-south" {
		got, _ = model.Update(key(string(r)))
		model = got.(Model)
	}
	view := model.View()
	if rowLine(view, "beta-agent") == "" {
		t.Fatalf("row whose workspace matches was not shown:\n%s", view)
	}
	if rowLine(view, "alpha-agent") != "" || rowLine(view, "gamma-agent") != "" {
		t.Fatalf("rows whose workspace does not match were still shown:\n%s", view)
	}
}

// TestFilterByCWDShowsOnlyTheMatchingRow is requirement 33's third named
// field, using a query that appears in no session's name or workspace --
// only its cwd.
func TestFilterByCWDShowsOnlyTheMatchingRow(t *testing.T) {
	model := newFilterTestModel(filterTestSessions())
	got, _ := model.Update(key("/"))
	model = got.(Model)
	for _, r := range "east-project" {
		got, _ = model.Update(key(string(r)))
		model = got.(Model)
	}
	view := model.View()
	if rowLine(view, "gamma-agent") == "" {
		t.Fatalf("row whose cwd matches was not shown:\n%s", view)
	}
	if rowLine(view, "alpha-agent") != "" || rowLine(view, "beta-agent") != "" {
		t.Fatalf("rows whose cwd does not match were still shown:\n%s", view)
	}
}

// TestFilterIsIncrementalAsYouType proves SPEC §11.10's "incrementally":
// each keystroke narrows or widens the visible set immediately, not only
// once the query is complete or Enter is pressed.
func TestFilterIsIncrementalAsYouType(t *testing.T) {
	model := newFilterTestModel(filterTestSessions())
	got, _ := model.Update(key("/"))
	model = got.(Model)

	got, _ = model.Update(key("a"))
	model = got.(Model)
	view := model.View()
	// "a" appears in alpha-agent's own name AND in every "*-agent" name, so
	// this step alone should still show all three -- only the fuller query
	// below narrows it. This step instead asserts the narrowing happens
	// before Enter/Esc: typing further changes what's on screen right away.
	if rowLine(view, "beta-agent") == "" {
		t.Fatalf("single-letter query unexpectedly hid a row it should still match:\n%s", view)
	}

	for _, r := range "lpha-agent" {
		got, _ = model.Update(key(string(r)))
		model = got.(Model)
	}
	view = model.View()
	if rowLine(view, "beta-agent") != "" {
		t.Fatalf("completing the query to an alpha-only match did not narrow the list live:\n%s", view)
	}
	if rowLine(view, "alpha-agent") == "" {
		t.Fatalf("the matching row disappeared while still typing its own full name:\n%s", view)
	}
}

// TestFilterEscClearsBackToTheFullList proves SPEC §11.10's "esc clears the
// query and returns to the unfiltered list": Esc while filtering discards
// the query and restores every row, not just closing the input with the
// narrowed list still applied.
func TestFilterEscClearsBackToTheFullList(t *testing.T) {
	model := newFilterTestModel(filterTestSessions())
	got, _ := model.Update(key("/"))
	model = got.(Model)
	for _, r := range "alpha-agent" {
		got, _ = model.Update(key(string(r)))
		model = got.(Model)
	}
	if rowLine(model.View(), "beta-agent") != "" {
		t.Fatalf("setup failed: filter did not narrow the list before Esc")
	}
	got, _ = model.Update(key("esc"))
	model = got.(Model)
	if model.filtering {
		t.Fatal("esc left the filter input open")
	}
	if model.filterQuery != "" {
		t.Fatalf("esc left a query in force: %q", model.filterQuery)
	}
	view := model.View()
	for _, name := range []string{"alpha-agent", "beta-agent", "gamma-agent"} {
		if rowLine(view, name) == "" {
			t.Fatalf("esc did not restore row %q to the full list:\n%s", name, view)
		}
	}
}

// TestFilterEnterKeepsQueryAppliedAndReturnsKeymap proves Enter's contrast
// with Esc: the narrowed list and the query both survive, and the freed
// keymap (here, ↑/↓) now navigates the filtered set.
func TestFilterEnterKeepsQueryAppliedAndReturnsKeymap(t *testing.T) {
	model := newFilterTestModel(filterTestSessions())
	got, _ := model.Update(key("/"))
	model = got.(Model)
	for _, r := range "alpha-agent" {
		got, _ = model.Update(key(string(r)))
		model = got.(Model)
	}
	got, _ = model.Update(key("enter"))
	model = got.(Model)
	if model.filtering {
		t.Fatal("enter left the filter input open")
	}
	if model.filterQuery != "alpha-agent" {
		t.Fatalf("enter discarded the query: %q", model.filterQuery)
	}
	if rowLine(model.View(), "beta-agent") != "" {
		t.Fatal("enter dropped the narrowed filter back to the full list")
	}
	// The keymap is free again: a bare "n" (top-level "new session") must
	// no longer be swallowed as filter text.
	got, _ = model.Update(key("n"))
	model = got.(Model)
	if !model.creating {
		t.Fatal("after enter closed the filter input, a top-level key was still captured as filter text")
	}
}

// TestFilterReachesAnArchivedRowHiddenFromTheDefaultList proves I-10's own
// success criterion: an archived session -- excluded from
// store.ListSessions' default view (SPEC requirement 27) and therefore
// never in m.baseSessions -- becomes visible once a query matches it,
// which the default (unfiltered) list can never do since it never even
// loads archived rows. This is deliberately NOT satisfied by filtering
// m.baseSessions alone: archivedSessions starts populated here exactly as
// loadArchivedSessions (issued when `/` opens) would leave it.
func TestFilterReachesAnArchivedRowHiddenFromTheDefaultList(t *testing.T) {
	model := newFilterTestModel(filterTestSessions())
	model.archivedSessions = []store.Session{
		{ID: "s-old", Name: "retired-agent", GroupName: "ws-old", CWD: "/repos/old-project", Agent: "shell", Status: "stopped", ArchivedAt: 999},
	}

	// The default, unfiltered list never shows it.
	if rowLine(model.View(), "retired-agent") != "" {
		t.Fatal("setup failed: archived row visible without any filter in force")
	}

	got, _ := model.Update(key("/"))
	model = got.(Model)
	for _, r := range "retired-agent" {
		got, _ = model.Update(key(string(r)))
		model = got.(Model)
	}
	view := model.View()
	if rowLine(view, "retired-agent") == "" {
		t.Fatalf("filter query matching the archived row's own name did not surface it:\n%s", view)
	}
	// Nothing else leaked in from the archived pool by accident.
	if rowLine(view, "alpha-agent") != "" || rowLine(view, "beta-agent") != "" || rowLine(view, "gamma-agent") != "" {
		t.Fatalf("filter query specific to the archived row unexpectedly also matched an active one:\n%s", view)
	}
}

// TestFilterStatesItIsInForceOnScreen proves SPEC.md:319-320: "the sidebar
// states the filter is in force so a hidden row is never mistaken for a
// deleted one" -- both while actively typing and after Enter has closed
// the input with the query still applied.
func TestFilterStatesItIsInForceOnScreen(t *testing.T) {
	model := newFilterTestModel(filterTestSessions())
	got, _ := model.Update(key("/"))
	model = got.(Model)
	for _, r := range "alpha" {
		got, _ = model.Update(key(string(r)))
		model = got.(Model)
	}
	if !strings.Contains(model.View(), "Filter") {
		t.Fatalf("no on-screen statement that a filter is in force while typing:\n%s", model.View())
	}
	got, _ = model.Update(key("enter"))
	model = got.(Model)
	if !strings.Contains(model.View(), "Filter") {
		t.Fatalf("no on-screen statement that a filter is in force after enter closed the input:\n%s", model.View())
	}
}

// TestEscClearsAFilterHeldInForceWithoutReopeningTheField proves task
// 027: once `enter` has closed the `/` text field with a query still
// applied (m.filtering == false, m.filterQuery != ""), a plain top-level
// Esc -- pressed directly, with the field never reopened -- discards that
// held query and restores the unfiltered list, the same outcome Esc gives
// while the field itself still has focus (TestFilterEscClearsBackToThe
// FullList above), reached through the top-level switch in tui.go instead
// of updateFilter's own esc case.
func TestEscClearsAFilterHeldInForceWithoutReopeningTheField(t *testing.T) {
	model := newFilterTestModel(filterTestSessions())
	got, _ := model.Update(key("/"))
	model = got.(Model)
	for _, r := range "alpha-agent" {
		got, _ = model.Update(key(string(r)))
		model = got.(Model)
	}
	got, _ = model.Update(key("enter"))
	model = got.(Model)
	if model.filtering {
		t.Fatal("setup failed: enter left the filter field open")
	}
	if model.filterQuery != "alpha-agent" {
		t.Fatalf("setup failed: enter did not keep the query in force: %q", model.filterQuery)
	}
	if rowLine(model.View(), "beta-agent") != "" {
		t.Fatal("setup failed: filter did not narrow the list before the top-level Esc")
	}

	// The field is NOT reopened here -- this is the whole point of the
	// scenario: Esc reaches tui.go's top-level switch, not updateFilter's.
	got, _ = model.Update(key("esc"))
	model = got.(Model)

	if model.filterQuery != "" {
		t.Fatalf("top-level esc left a held query in force: %q", model.filterQuery)
	}
	view := model.View()
	for _, name := range []string{"alpha-agent", "beta-agent", "gamma-agent"} {
		if rowLine(view, name) == "" {
			t.Fatalf("top-level esc did not restore row %q to the full list:\n%s", name, view)
		}
	}
}

// TestEscOnAMarkedSetClearsOnlyTheMarksNotAHeldFilter proves the "one press
// never clears two things at once" half of task 027: with BOTH a mark set
// and a held filter query in force, a plain top-level Esc clears the
// mark set (task 112's own contract) and leaves the filter untouched --
// never both in the same press.
func TestEscOnAMarkedSetClearsOnlyTheMarksNotAHeldFilter(t *testing.T) {
	model := newFilterTestModel(filterTestSessions())
	model.filterQuery = "alpha-agent"
	model.sessions = model.filteredSessions()
	model.selected = 0

	got, _ := model.Update(key("m"))
	model = got.(Model)
	if len(model.marked) != 1 {
		t.Fatalf("setup failed: m did not mark the selected row: %#v", model.marked)
	}

	got, _ = model.Update(key("esc"))
	model = got.(Model)

	if len(model.marked) != 0 {
		t.Fatalf("esc did not clear the mark set: %#v", model.marked)
	}
	if model.filterQuery != "alpha-agent" {
		t.Fatalf("esc on a marked set also cleared the held filter query, clearing two things at once: %q", model.filterQuery)
	}
}

// TestEscOnTheHelpOverlayClosesItWithoutTouchingAHeldFilter is the
// "overlay" half of the same "never clears two things at once" rule:
// updateHelpView intercepts Esc before tui.go's top-level switch is ever
// reached, so closing the help overlay must never also discard a filter
// query already held in force.
func TestEscOnTheHelpOverlayClosesItWithoutTouchingAHeldFilter(t *testing.T) {
	model := newFilterTestModel(filterTestSessions())
	model.filterQuery = "alpha-agent"
	model.sessions = model.filteredSessions()
	model.help = true

	got, _ := model.Update(key("esc"))
	model = got.(Model)

	if model.help {
		t.Fatal("esc did not close the help overlay")
	}
	if model.filterQuery != "alpha-agent" {
		t.Fatalf("esc closing the help overlay also cleared a held filter query, clearing two things at once: %q", model.filterQuery)
	}
}

// TestEscOnTheRenameDialogClosesItWithoutTouchingAHeldFilter is the
// "dialog" half of the same rule: updateRenameDialog intercepts Esc
// before the top-level switch too, so cancelling it must never also
// discard a filter query already held in force.
func TestEscOnTheRenameDialogClosesItWithoutTouchingAHeldFilter(t *testing.T) {
	model := newFilterTestModel(filterTestSessions())
	model.filterQuery = "alpha-agent"
	model.sessions = model.filteredSessions()
	model.renaming = true
	model.renameValue = "new-name"

	got, _ := model.Update(key("esc"))
	model = got.(Model)

	if model.renaming {
		t.Fatal("esc did not close the rename dialog")
	}
	if model.filterQuery != "alpha-agent" {
		t.Fatalf("esc closing the rename dialog also cleared a held filter query, clearing two things at once: %q", model.filterQuery)
	}
}

// TestFilterFrameBudgetAccountsForTheStatusLine is requirement 30's own
// rule (every transient message counted in computeLayout's reserved rows)
// applied to this task's new status line: with the filter in force at
// every layout mode and exactly deck's 80x24 supported minimum, the whole
// frame -- panels, status line and footer together -- still fits inside
// height lines, the same shape TestUndoToastStaysWithinFrameBudgetAtEvery
// LayoutMode already proves for the undo toast.
func TestFilterFrameBudgetAccountsForTheStatusLine(t *testing.T) {
	modes := []string{LayoutAuto, LayoutSideBySide, LayoutStacked, LayoutCollapsed}
	width, height := 80, 24
	for _, mode := range modes {
		t.Run(mode, func(t *testing.T) {
			model := newFilterTestModel(filterTestSessions())
			model.width, model.height = width, height
			model.layoutMode = mode
			model.filtering = true
			model.filterQuery = "alpha"
			model.sessions = model.filteredSessions()

			if model.filterStatusLine(width) == nil {
				t.Fatal("test setup: filterStatusLine returned nil while filtering")
			}

			view := model.View()
			lines := strings.Split(view, "\n")
			if len(lines) > height {
				t.Fatalf("view has %d lines with the filter status line on screen, exceeding height %d:\n%s", len(lines), height, view)
			}
			if !strings.Contains(view, "Filter") {
				t.Fatalf("view is missing the filter status line with a query in force:\n%s", view)
			}
		})
	}
}
