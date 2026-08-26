package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/n-orlov/deck/internal/store"
)

// This file is R71's TUI half (SPEC.md:323-332, issue #8): `A` is
// reversible, so `U` clears archived_at on the selected row -- and the row
// it has to be able to act on is one that is ONLY ever displayed inside
// requirement 33's `/` filter results, since an archived row is absent from
// store.ListSessions' default view entirely.

// unarchiveRecorder is a fake unarchiveSvc that records every session id it
// is handed (never merely a count: the whole point of the filter test below
// is WHICH row `U` acted on) and returns that row with archived_at cleared,
// exactly as service.Unarchive does.
type unarchiveRecorder struct {
	calls []string
	row   store.Session
}

func (u *unarchiveRecorder) unarchive(_ context.Context, sessionID string) (store.Session, error) {
	u.calls = append(u.calls, sessionID)
	cleared := u.row
	cleared.ArchivedAt = 0
	return cleared, nil
}

// TestUnarchiveKeyActsOnAnArchivedRowFoundThroughTheFilter is the reachability
// proof this requirement turns on: the archived row exists only in
// m.archivedSessions (the pool loadArchivedSessions fills when `/` opens),
// so it can be selected at all only after a query surfaces it and Enter
// hands the keymap back to the narrowed list. `U` there must unarchive
// exactly that row -- not the row that would have been selected in the
// unfiltered list, which is a different session entirely here.
func TestUnarchiveKeyActsOnAnArchivedRowFoundThroughTheFilter(t *testing.T) {
	archived := store.Session{ID: "s-old", Name: "retired-agent", Workspace: "ws-old", CWD: "/repos/old-project", Agent: "shell", Status: "stopped", ArchivedAt: 999}
	recorder := &unarchiveRecorder{row: archived}
	model := newFilterTestModel(filterTestSessions())
	model.archivedSessions = []store.Session{archived}
	model.unarchiveSvc = recorder.unarchive

	got, _ := model.Update(key("/"))
	model = got.(Model)
	for _, r := range "retired-agent" {
		got, _ = model.Update(key(string(r)))
		model = got.(Model)
	}
	got, _ = model.Update(key("enter"))
	model = got.(Model)
	if rowLine(model.View(), "retired-agent") == "" {
		t.Fatalf("setup failed: the archived row is not on screen inside the filter results:\n%s", model.View())
	}

	got, cmd := model.Update(key("U"))
	model = got.(Model)
	if cmd == nil {
		t.Fatal("U inside the filter results issued no command at all")
	}
	msg := cmd()
	if len(recorder.calls) != 1 || recorder.calls[0] != "s-old" {
		t.Fatalf("unarchive called with %v, want exactly one call for the archived row %q", recorder.calls, "s-old")
	}
	result, ok := msg.(sessionUnarchived)
	if !ok {
		t.Fatalf("U produced %T, want sessionUnarchived", msg)
	}
	if result.err != nil {
		t.Fatalf("unarchive reported an error: %v", result.err)
	}
	if result.session.ArchivedAt != 0 {
		t.Fatalf("unarchived session still carries ArchivedAt=%d", result.session.ArchivedAt)
	}
	if model.attachError != "" {
		t.Fatalf("U left an error line on screen: %q", model.attachError)
	}
}

// TestUnarchiveResultReloadsBothTheDefaultListAndTheArchivedPool proves the
// sessionUnarchived handler refreshes the archived-side pool too, not only
// the default list: the row is in m.sessions solely because
// m.archivedSessions still holds it, so a handler that reloaded one list
// would leave a stale archived copy of the row behind. With no store
// attached both loads degrade to empty results (loadSessions/
// loadArchivedSessions' own no-store branch), which is exactly what makes
// the pool's emptying observable here.
func TestUnarchiveResultReloadsBothTheDefaultListAndTheArchivedPool(t *testing.T) {
	archived := store.Session{ID: "s-old", Name: "retired-agent", Agent: "shell", Status: "stopped", ArchivedAt: 999}
	model := newFilterTestModel(filterTestSessions())
	model.archivedSessions = []store.Session{archived}
	model.filterQuery = "retired-agent"
	model.sessions = model.filteredSessions()

	got, cmd := model.Update(sessionUnarchived{session: archived})
	model = got.(Model)
	if cmd == nil {
		t.Fatal("a successful unarchive issued no reload at all")
	}
	// tea.Batch's own message carries both commands; running them the way
	// bubbletea would and feeding each result back through Update is what
	// proves BOTH lists were refreshed rather than only the default one.
	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatalf("a successful unarchive returned %T, want a tea.Batch of both reloads", cmd())
	}
	if len(batch) != 2 {
		t.Fatalf("unarchive reload batch has %d commands, want 2 (sessions and archived sessions)", len(batch))
	}
	for _, c := range batch {
		got, _ = model.Update(c())
		model = got.(Model)
	}
	if len(model.archivedSessions) != 0 {
		t.Fatalf("archived pool still holds %+v after the unarchive reload", model.archivedSessions)
	}
	// Not asserted through the rendered view: with no store attached both
	// reloads land empty, and the empty-list placeholder itself echoes the
	// query ("No sessions match \"retired-agent\"."), so a view-level
	// substring check here would report the row as still displayed no
	// matter what. The displayed list is checked by id instead.
	if idx := indexOfSessionID(model.sessions, "s-old"); idx >= 0 {
		t.Fatalf("the stale archived copy of the row is still in the displayed list at index %d: %+v", idx, model.sessions)
	}
}

// TestUnarchiveKeyRefusesARowThatIsNotArchived keeps `U` from recording an
// "unarchived" event for a row that was never archived: the store would
// happily clear an already-zero archived_at (mutateSessionWithEvent has no
// opinion on the current value, exactly like ArchiveSession), so the
// refusal has to live at the keypress.
func TestUnarchiveKeyRefusesARowThatIsNotArchived(t *testing.T) {
	recorder := &unarchiveRecorder{}
	model := newFilterTestModel(filterTestSessions())
	model.unarchiveSvc = recorder.unarchive

	got, cmd := model.Update(key("U"))
	model = got.(Model)
	if cmd != nil {
		t.Fatal("U on a row that is not archived issued a command")
	}
	if len(recorder.calls) != 0 {
		t.Fatalf("U reached the store for a row that is not archived: %v", recorder.calls)
	}
	if !strings.Contains(model.attachError, "not archived") {
		t.Fatalf("U on a row that is not archived said %q, want it to state the row is not archived", model.attachError)
	}
}

// TestUnarchiveKeyStatesItIsUnavailableWhenUnwired mirrors every other
// unwired-dependency path in this package (archiveSvc, deleteSvc, ...): a
// Model built without an unarchiver says so instead of silently swallowing
// the keypress.
func TestUnarchiveKeyStatesItIsUnavailableWhenUnwired(t *testing.T) {
	model := newFilterTestModel(filterTestSessions())
	model.sessions[0].ArchivedAt = 999

	got, cmd := model.Update(key("U"))
	model = got.(Model)
	if cmd != nil {
		t.Fatal("U with no unarchiver wired issued a command")
	}
	if !strings.Contains(model.attachError, "unavailable") {
		t.Fatalf("U with no unarchiver wired said %q, want it to state unarchiving is unavailable", model.attachError)
	}
}
