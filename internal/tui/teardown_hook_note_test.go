package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// This file is task 042's own evidence (approach 02, findings §1): the
// shipped binary was discarding runPostDestroy's own hook-failure message
// (cmd/deck/main.go's archiveAdapter/deleteAdapter did `_, err :=
// sessions.Archive/Delete`), so a failing teardown hook never reached the
// screen at all. WithTeardownHookReporters is the message-carrying seam
// that fixes that: when set, it is called INSTEAD of archiveSvc/deleteSvc,
// and a non-empty message becomes teardownHookNote, rendered by
// teardownHookNoteLines exactly like archiveUndoneRebuildNoteLines already
// is. archiveSvc/deleteSvc themselves keep their pre-existing
// (context.Context, store.Session) error signature throughout -- they are
// still required for the "archiving/deleting is unavailable" nil-check
// each submit path performs before ever consulting its reporter.

// TestTeardownHookFailureRendersAsANoteForArchive is the `A` path: a
// confirmed archive whose reporter reports a non-empty hook-failure
// message surfaces that exact text on screen.
func TestTeardownHookFailureRendersAsANoteForArchive(t *testing.T) {
	const hookMsg = "post_destroy (session) failed: exit status 1"
	model := New(nil, config.Settings{Undo: time.Hour}, "")
	model.width, model.height = 100, 40
	model.sessions = []store.Session{{ID: "s1", Name: "alpha", Agent: "shell", Status: "stopped"}}
	model.selected = 0
	// archiveSvc stays wired with its pre-existing signature -- submit's
	// own availability check consults it regardless of the reporter.
	model.archiveSvc = func(context.Context, store.Session) error { return nil }
	model = model.WithTeardownHookReporters(
		func(context.Context, store.Session) (string, error) { return hookMsg, nil },
		nil,
	)

	got, _ := model.Update(key("A"))
	model = got.(Model)
	if !model.archiveConfirming {
		t.Fatal("A did not open the archive confirm dialog")
	}
	got, cmd := model.Update(key("enter"))
	model = got.(Model)
	if cmd == nil {
		t.Fatal("Enter inside the archive confirm issued no command")
	}
	result := cmd()
	got, _ = model.Update(result)
	model = got.(Model)

	if model.teardownHookNote != hookMsg {
		t.Fatalf("teardownHookNote = %q, want %q", model.teardownHookNote, hookMsg)
	}
	lines := model.teardownHookNoteLines(100)
	if len(lines) == 0 {
		t.Fatal("teardownHookNoteLines is empty with a hook-failure message set")
	}
	if view := model.View(); !strings.Contains(view, hookMsg) {
		t.Fatalf("View() does not render the archive path's teardown hook failure message:\n%s", view)
	}
}

// TestTeardownHookFailureRendersAsANoteForDelete is the `dd` path,
// mirroring the archive test above exactly, through deleteHookReporter
// rather than archiveHookReporter.
func TestTeardownHookFailureRendersAsANoteForDelete(t *testing.T) {
	const hookMsg = "post_destroy (global) failed: signal: killed"
	model := New(nil, config.Settings{Undo: time.Hour}, "")
	model.width, model.height = 100, 40
	model.sessions = []store.Session{{ID: "s1", Name: "alpha", Agent: "shell", Status: "stopped"}}
	model.selected = 0
	// deleteSvc stays wired with its pre-existing signature, for the same
	// reason archiveSvc does above.
	model.deleteSvc = func(context.Context, store.Session) error { return nil }
	model = model.WithTeardownHookReporters(
		nil,
		func(context.Context, store.Session) (string, error) { return hookMsg, nil },
	)

	got, _ := model.Update(key("d"))
	model = got.(Model)
	got, _ = model.Update(key("d"))
	model = got.(Model)
	if !model.deleteConfirming {
		t.Fatal("dd did not open the delete confirm dialog")
	}
	got, cmd := model.Update(key("enter"))
	model = got.(Model)
	if cmd == nil {
		t.Fatal("Enter inside the delete confirm issued no command")
	}
	result := cmd()
	got, _ = model.Update(result)
	model = got.(Model)

	if model.teardownHookNote != hookMsg {
		t.Fatalf("teardownHookNote = %q, want %q", model.teardownHookNote, hookMsg)
	}
	lines := model.teardownHookNoteLines(100)
	if len(lines) == 0 {
		t.Fatal("teardownHookNoteLines is empty with a hook-failure message set")
	}
	if view := model.View(); !strings.Contains(view, hookMsg) {
		t.Fatalf("View() does not render the delete path's teardown hook failure message:\n%s", view)
	}
}
