package tui

import (
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// TestSessionArchivedHidesRowBeforeTheReloadLands is the archive half of
// TestSessionDeletedHidesRowBeforeTheReloadLands (kill_delete_undo.feature:561
// "confirming the archive dialog lands both the kill and the archived flag",
// seen flaky in the sweep at 27aed642e3e). SPEC §9.2's archive row hides the
// row from the default list and, on success, raises a toast saying so. The
// frame rendered for the sessionArchived message itself raised that "Killed
// and archived" toast while still listing the archived row, because the only
// thing that removed it was the m.loadSessions reload landing on a LATER
// Update call; a client sampling that frame saw both together.
//
// This calls Update(sessionArchived{...}) directly -- the very message the
// archive Cmd produces on success -- and asserts, in the RESULT of that same
// Update call and before any sessionsLoaded is delivered, that the row is
// gone from m.sessions and from the rendered frame that carries the toast.
// Deterministic: no timing, no retry, no sleep.
func TestSessionArchivedHidesRowBeforeTheReloadLands(t *testing.T) {
	model := New(nil, config.Settings{}, "")
	archived := store.Session{ID: "archive-submit-id", Name: "archive-confirm-submit", Agent: "shell", Status: "running"}
	survivor := store.Session{ID: "survivor-id", Name: "survivor", Agent: "shell", Status: "running"}
	model.baseSessions = []store.Session{archived, survivor}
	model.sessions = model.filteredSessions()
	model.archiveConfirming = true

	got, _ := model.Update(sessionArchived{session: archived})
	model = got.(Model)

	if indexOfSessionID(model.sessions, archived.ID) >= 0 {
		t.Fatalf("archived session %q is still in m.sessions in the very Update call that raised the archive toast: %+v", archived.Name, model.sessions)
	}
	if indexOfSessionID(model.sessions, survivor.ID) < 0 {
		t.Fatalf("the unrelated survivor session was removed too, want only the archived one gone: %+v", model.sessions)
	}
	if model.archiveUndoSessionID != archived.ID {
		t.Fatalf("archiveUndoSessionID = %q, want %q (u must still unarchive the archived session)", model.archiveUndoSessionID, archived.ID)
	}
	view := model.View()
	if !strings.Contains(view, "Killed and archived") {
		t.Fatalf("the archive success toast is missing from the frame:\n%s", view)
	}
	if strings.Contains(view, archived.Name) {
		t.Fatalf("the frame raising the archive toast still lists the archived row %q:\n%s", archived.Name, view)
	}
	if !strings.Contains(view, survivor.Name) {
		t.Fatalf("the frame lost the unrelated survivor row %q:\n%s", survivor.Name, view)
	}
}
