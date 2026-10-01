package tui

import (
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// TestSessionDeletedHidesRowBeforeTheReloadLands pins kill_delete_undo
// mechanism M10 (task 012 inventory, from CI run 36828007039,
// kill_delete_undo.feature:75 "submitting the confirm dialog kills the live
// pane, tombstones the row, and it disappears from the sidebar
// immediately"). SPEC.md:1085 says delete is "Hidden immediately", and
// tui.go's own sessionDeleted doc comment claims the same ("A successful
// delete reloads the session list so the now-tombstoned row disappears
// from the sidebar immediately rather than waiting for the next reconcile
// tick"), but the ONLY mechanism it used to make that happen was scheduling
// m.loadSessions as its own tea.Cmd -- a goroutine that reports back with a
// LATER sessionsLoaded message, on a LATER Update call. The frame rendered
// for the sessionDeleted message itself (closing the confirm dialog and
// raising the "Deleted — press u to undo" toast) still carried the stale,
// pre-delete m.sessions, so a client that captures a frame in that window
// sees the undo toast and the deleted row's own (still "starting") sidebar
// line together -- exactly the failure CI observed once.
//
// This test calls Update(sessionDeleted{...}) directly -- the very message
// deleteSvc's own tea.Cmd produces on a successful delete -- and asserts
// the deleted session is already gone from m.sessions in the RESULT of
// that same Update call, before any sessionsLoaded message is ever
// delivered. Deterministic: no timing, no retry, no sleep, and it does not
// depend on the reload Cmd ever running.
func TestSessionDeletedHidesRowBeforeTheReloadLands(t *testing.T) {
	model := New(nil, config.Settings{}, "")
	deleted := store.Session{ID: "dd-submit-id", Name: "dd-submit", Agent: "shell", Status: "starting"}
	survivor := store.Session{ID: "survivor-id", Name: "survivor", Agent: "shell", Status: "running"}
	model.baseSessions = []store.Session{deleted, survivor}
	model.sessions = model.filteredSessions()

	got, _ := model.Update(sessionDeleted{session: deleted})
	model = got.(Model)

	for _, s := range model.sessions {
		if s.ID == deleted.ID {
			t.Fatalf("deleted session %q is still present in m.sessions in the very Update call that raised the undo toast, before any reload landed: %+v", deleted.Name, model.sessions)
		}
	}
	foundSurvivor := false
	for _, s := range model.sessions {
		if s.ID == survivor.ID {
			foundSurvivor = true
		}
	}
	if !foundSurvivor {
		t.Fatalf("the unrelated survivor session was removed too, want only the deleted one gone: %+v", model.sessions)
	}
	if model.deleteUndoSessionID != deleted.ID {
		t.Fatalf("deleteUndoSessionID = %q, want %q (the undo toast must still name the deleted session)", model.deleteUndoSessionID, deleted.ID)
	}
}
