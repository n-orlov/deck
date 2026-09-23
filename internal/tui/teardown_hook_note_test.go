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
	model.selected = rowCursor(0)
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
	model.selected = rowCursor(0)
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

// TestTeardownHookFailureRendersAsANoteForBulkDelete is task 048: the
// marked-set `dd` path had no equivalent of the two tests above. It ran
// each row's post_destroy through the plain deleteSvc adapter, whose
// signature cannot carry a message, so a bulk teardown-hook failure was
// written to the durable `note` events and then dropped before the toast.
// Two marked rows, each with its own failure, assert the note names both
// rows (the single-row toasts stay bare -- there the row is the selected
// one -- but a batch note is ambiguous without the prefix) and that the
// reporter, not deleteSvc, is what the batch actually calls.
func TestTeardownHookFailureRendersAsANoteForBulkDelete(t *testing.T) {
	const alphaMsg = "post_destroy (session) failed: exit status 1"
	const betaMsg = "post_destroy (global) failed: signal: killed"

	var reported []store.Session
	deleteSvcCalls := 0

	model := New(nil, config.Settings{Undo: time.Hour, DeleteGrace: time.Hour}, "")
	// Wider than the single-row tests on purpose: the batch note carries
	// both rows, and at width 100 wrapText would split the second message
	// mid-phrase, so a Contains check on the rendered view would fail for
	// the wrapping rather than for the behaviour under test.
	model.width, model.height = 200, 40
	model.sessions = []store.Session{
		{ID: "s1", Name: "alpha", Agent: "shell", Status: "stopped"},
		{ID: "s2", Name: "beta", Agent: "shell", Status: "stopped"},
	}
	model.deleteSvc = func(context.Context, store.Session) error {
		deleteSvcCalls++
		return nil
	}
	model = model.WithTeardownHookReporters(
		nil,
		func(_ context.Context, s store.Session) (string, error) {
			reported = append(reported, s)
			switch s.ID {
			case "s1":
				return alphaMsg, nil
			case "s2":
				return betaMsg, nil
			}
			return "", nil
		},
	)

	for _, idx := range []int{0, 1} {
		model.selected = rowCursor(idx)
		got, _ := model.Update(key("m"))
		model = got.(Model)
	}
	if len(model.marked) != 2 {
		t.Fatalf("expected both sessions marked before dd, got %#v", model.marked)
	}

	got, _ := model.Update(key("d"))
	model = got.(Model)
	got, _ = model.Update(key("d"))
	model = got.(Model)
	if !model.deleteConfirming {
		t.Fatal("second d did not open the bulk delete confirm dialog")
	}
	got, cmd := model.Update(key("enter"))
	model = got.(Model)
	if cmd == nil {
		t.Fatal("Enter inside the bulk delete confirm issued no command")
	}
	got, _ = model.Update(cmd())
	model = got.(Model)

	if len(reported) != 2 {
		t.Fatalf("deleteHookReporter invoked %d time(s) for a 2-session bulk dd, want exactly 2 (one per marked session): %#v", len(reported), reported)
	}
	if deleteSvcCalls != 0 {
		t.Fatalf("deleteSvc invoked %d time(s) while a reporter was wired; the batch must prefer the reporter exactly as the single-row submit does", deleteSvcCalls)
	}
	for _, want := range []string{"alpha: " + alphaMsg, "beta: " + betaMsg} {
		if !strings.Contains(model.teardownHookNote, want) {
			t.Fatalf("teardownHookNote = %q, want it to contain %q", model.teardownHookNote, want)
		}
	}
	if len(model.teardownHookNoteLines(200)) == 0 {
		t.Fatal("teardownHookNoteLines is empty with a bulk hook-failure message set")
	}
	view := model.View()
	for _, want := range []string{alphaMsg, betaMsg} {
		if !strings.Contains(view, want) {
			t.Fatalf("View() does not render the bulk delete path's teardown hook failure %q:\n%s", want, view)
		}
	}
}

// TestBulkDeleteWithoutHookFailuresRaisesNoNote pins the other half of the
// contract: the batch must not invent a note when every row's hooks
// succeeded (or when no post_destroy is configured at all), which is the
// overwhelmingly common bulk dd.
func TestBulkDeleteWithoutHookFailuresRaisesNoNote(t *testing.T) {
	model := New(nil, config.Settings{Undo: time.Hour, DeleteGrace: time.Hour}, "")
	model.width, model.height = 100, 40
	model.sessions = []store.Session{
		{ID: "s1", Name: "alpha", Agent: "shell", Status: "stopped"},
		{ID: "s2", Name: "beta", Agent: "shell", Status: "stopped"},
	}
	model.deleteSvc = func(context.Context, store.Session) error { return nil }
	model = model.WithTeardownHookReporters(
		nil,
		func(context.Context, store.Session) (string, error) { return "", nil },
	)

	for _, idx := range []int{0, 1} {
		model.selected = rowCursor(idx)
		got, _ := model.Update(key("m"))
		model = got.(Model)
	}
	got, _ := model.Update(key("d"))
	model = got.(Model)
	got, _ = model.Update(key("d"))
	model = got.(Model)
	got, cmd := model.Update(key("enter"))
	model = got.(Model)
	if cmd == nil {
		t.Fatal("Enter inside the bulk delete confirm issued no command")
	}
	got, _ = model.Update(cmd())
	model = got.(Model)

	if model.teardownHookNote != "" {
		t.Fatalf("teardownHookNote = %q, want empty when no row reported a hook failure", model.teardownHookNote)
	}
	if lines := model.teardownHookNoteLines(100); len(lines) != 0 {
		t.Fatalf("teardownHookNoteLines = %#v, want none when no row reported a hook failure", lines)
	}
}
