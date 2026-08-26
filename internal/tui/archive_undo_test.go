package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/service"
	"github.com/n-orlov/deck/internal/store"
)

// This file is R72's second half (issue #10, SPEC.md:752: "On success a toast
// says what happened, with `u` to undo"). The confirm dialog (see
// archive_confirm_test.go) stopped `A` from archiving on one unconfirmed
// keystroke; this is the way back out of a CONFIRMED one, so a deliberate
// archive of the wrong row is still recoverable without hunting for it inside
// the `/` filter. It is the THIRD undo trio in the model, behind x's
// (undoSessionID) and dd's (deleteUndoSessionID), and it must behave like
// them: same generation-tied expiry, same "u acts on the last action, not on
// whatever row is selected now" contract.

// archiveUndoTestModel is a one-live-row model wired with an archiver, an
// unarchiver and a killer, all instrumented, so every write either `A` or `u`
// performs is observable rather than inferred. Undo is an hour so no tick can
// race the assertions; expiry is driven by feeding archiveUndoExpired
// directly, exactly as its siblings' behaviour is reasoned about.
func archiveUndoTestModel(t *testing.T) (Model, *archiveRecorder, *[]string) {
	t.Helper()
	archiver := &archiveRecorder{}
	unarchived := []string{}
	model := New(nil, config.Settings{Undo: time.Hour}, "")
	model.sessions = []store.Session{
		{ID: "s-live", Name: "live-agent", Agent: "claude", Status: "idle", CWD: "/repos/project"},
	}
	model.selected = 0
	model.archiveSvc = archiver.archive
	model.kill = func(context.Context, store.Session) error { return nil }
	model.unarchiveSvc = func(_ context.Context, id string) (store.Session, error) {
		unarchived = append(unarchived, id)
		return store.Session{ID: id, Name: "live-agent", Status: "stopped"}, nil
	}
	return model, archiver, &unarchived
}

// TestSuccessfulArchiveStartsAnUndoWindowSayingWhatHappened pins the toast
// itself: a confirmed archive records the third trio and renders a line that
// says what the archive did and that `u` reverses it. The two wordings are
// asserted separately because they are two different factual claims -- an
// already-stopped row was only hidden, while a live one was killed first, and
// a toast that reported the wrong one would be a false statement about what
// just happened to the operator's agent.
func TestSuccessfulArchiveStartsAnUndoWindowSayingWhatHappened(t *testing.T) {
	cases := []struct {
		name     string
		status   string
		want     string
		unwanted string
	}{
		{name: "live-row-was-killed", status: "idle", want: "Killed and archived \u2014 press u to unarchive", unwanted: "Archived \u2014 press u"},
		{name: "stopped-row-only-hidden", status: "stopped", want: "Archived \u2014 press u to unarchive", unwanted: "Killed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			model, _, _ := archiveUndoTestModel(t)
			model.sessions[0].Status = tc.status
			model.width, model.height = 100, 40

			got, _ := model.Update(key("A"))
			model = got.(Model)
			got, cmd := model.Update(key("enter"))
			model = got.(Model)
			if cmd == nil {
				t.Fatal("Enter inside the archive confirm issued no command")
			}
			result := cmd()
			got, cmd = model.Update(result)
			model = got.(Model)

			if model.archiveUndoSessionID != "s-live" {
				t.Fatalf("archiveUndoSessionID = %q, want the archived row's id -- a successful archive must open its own undo window", model.archiveUndoSessionID)
			}
			if model.archiveUndoSessionName != "live-agent" {
				t.Fatalf("archiveUndoSessionName = %q, want %q", model.archiveUndoSessionName, "live-agent")
			}
			if model.archiveUndoGeneration == 0 {
				t.Fatal("archiveUndoGeneration was not bumped, so no expiry tick can be tied to this window")
			}
			if cmd == nil {
				t.Fatal("a successful archive issued no command, so neither the reload nor the DECK_UNDO_MS expiry tick was scheduled")
			}
			lines := model.archiveUndoNoteLines(100)
			if len(lines) == 0 {
				t.Fatal("archiveUndoNoteLines is empty with the archive undo window open")
			}
			toast := strings.Join(lines, " ")
			if !strings.Contains(toast, tc.want) {
				t.Fatalf("archive toast = %q, want it to contain %q", toast, tc.want)
			}
			if strings.Contains(toast, tc.unwanted) {
				t.Fatalf("archive toast = %q, must not claim %q for a %q row", toast, tc.unwanted, tc.status)
			}
			if view := model.View(); !strings.Contains(view, tc.want) {
				t.Fatalf("View() does not render the archive toast:\n%s", view)
			}
			// The toast must not name the row: `A` hides it from the default
			// list and the submit scenario asserts the name is gone from the
			// whole screen (see archiveUndoNoteLines' doc comment).
			if strings.Contains(toast, "live-agent") {
				t.Fatalf("archive toast names the archived session (%q), which the submit scenario's whole-screen assertion forbids", toast)
			}
		})
	}
}

// TestArchiveUndoPressingUUnarchivesTheRowAndClearsTheToast is the paired
// half: the toast's promise is real. `u` calls the same UnarchiveSession
// service `U` uses (R71), for the id the archive recorded rather than for
// whatever row is selected now -- the archived row is not even in the default
// list any more -- and the toast goes away so a second `u` cannot unarchive
// twice.
func TestArchiveUndoPressingUUnarchivesTheRowAndClearsTheToast(t *testing.T) {
	model, _, unarchived := archiveUndoTestModel(t)

	got, _ := model.Update(key("A"))
	model = got.(Model)
	got, cmd := model.Update(key("enter"))
	model = got.(Model)
	got, _ = model.Update(cmd())
	model = got.(Model)
	// The archived row leaves the default list, exactly as it does in the
	// real reload: `u` must still know what to unarchive.
	model.sessions = nil
	model.selected = 0

	got, cmd = model.Update(key("u"))
	model = got.(Model)
	if cmd == nil {
		t.Fatal("u issued no command with an archive undo window open")
	}
	msg := cmd()
	if len(*unarchived) != 1 || (*unarchived)[0] != "s-live" {
		t.Fatalf("unarchive called with %v, want exactly one call for the archived row (s-live)", *unarchived)
	}
	if _, ok := msg.(sessionUnarchived); !ok {
		t.Fatalf("u produced %T, want sessionUnarchived", msg)
	}
	if model.archiveUndoSessionID != "" || model.archiveUndoSessionName != "" || model.archiveUndoKilled {
		t.Fatalf("u left the archive undo trio populated: id=%q name=%q killed=%v", model.archiveUndoSessionID, model.archiveUndoSessionName, model.archiveUndoKilled)
	}
	if lines := model.archiveUndoNoteLines(100); lines != nil {
		t.Fatalf("the archive toast survived its own undo: %q", lines)
	}

	got, cmd = model.Update(key("u"))
	model = got.(Model)
	if cmd != nil {
		t.Fatalf("a second u after the window was consumed issued a command (%T)", cmd())
	}
	if len(*unarchived) != 1 {
		t.Fatalf("a second u unarchived again: %v", *unarchived)
	}
}

// TestArchiveUndoWindowExpiresLikeItsSiblings is the expiry assertion the
// requirement asks for by name: archiveUndoExpired is generation-tied exactly
// as undoExpired and deleteGraceExpired are, so a stale tick from an earlier
// archive/undo cycle cannot clear a newer window, the matching tick clears the
// trio and the toast, and `u` after expiry is a no-op rather than unarchiving
// a row the operator has since stopped thinking about. Unlike
// deleteGraceExpired, expiry must reap NOTHING -- an expired archive window
// leaves the archived row exactly where it is.
func TestArchiveUndoWindowExpiresLikeItsSiblings(t *testing.T) {
	model, _, unarchived := archiveUndoTestModel(t)

	got, _ := model.Update(key("A"))
	model = got.(Model)
	got, cmd := model.Update(key("enter"))
	model = got.(Model)
	got, _ = model.Update(cmd())
	model = got.(Model)
	generation := model.archiveUndoGeneration

	got, cmd = model.Update(archiveUndoExpired(generation - 1))
	model = got.(Model)
	if cmd != nil {
		t.Fatalf("a stale archiveUndoExpired tick issued a command (%T)", cmd())
	}
	if model.archiveUndoSessionID != "s-live" {
		t.Fatalf("a stale archiveUndoExpired tick cleared the current window (id=%q)", model.archiveUndoSessionID)
	}

	got, cmd = model.Update(archiveUndoExpired(generation))
	model = got.(Model)
	if cmd != nil {
		t.Fatalf("the archive window's expiry issued a command (%T) -- expiry must reap nothing, only drop the toast", cmd())
	}
	if model.archiveUndoSessionID != "" || model.archiveUndoSessionName != "" || model.archiveUndoKilled {
		t.Fatalf("the archive undo window survived its own expiry: id=%q name=%q killed=%v", model.archiveUndoSessionID, model.archiveUndoSessionName, model.archiveUndoKilled)
	}
	if lines := model.archiveUndoNoteLines(100); lines != nil {
		t.Fatalf("the archive toast survived its own expiry: %q", lines)
	}

	got, cmd = model.Update(key("u"))
	model = got.(Model)
	if cmd != nil {
		t.Fatalf("u after the archive window expired issued a command (%T)", cmd())
	}
	if len(*unarchived) != 0 {
		t.Fatalf("u after expiry unarchived anyway: %v", *unarchived)
	}
}

// TestArchiveUndoIsCheckedBehindTheKillAndDeleteUndoTrios proves the new trio
// changed nothing about what `u` already did: with a kill window and a delete
// window outstanding at the same time as an archive window, `u` resumes first,
// restores second, and only unarchives once both of the pre-existing trios are
// empty. Ordering is the whole risk of adding a third window to one key.
func TestArchiveUndoIsCheckedBehindTheKillAndDeleteUndoTrios(t *testing.T) {
	model, _, unarchived := archiveUndoTestModel(t)
	resumed := []string{}
	restored := []string{}
	model.resume = func(_ context.Context, id string) (store.Session, service.ResumeOutcome, error) {
		resumed = append(resumed, id)
		return store.Session{ID: id}, service.ResumeStarted, nil
	}
	model.restoreSvc = func(_ context.Context, id string) (store.Session, error) {
		restored = append(restored, id)
		return store.Session{ID: id}, nil
	}
	model.undoSessionID, model.undoSessionName = "killed-row", "killed-row"
	model.deleteUndoSessionID, model.deleteUndoSessionName = "deleted-row", "deleted-row"
	model.archiveUndoSessionID, model.archiveUndoSessionName = "archived-row", "archived-row"

	// 1st u: the kill window wins.
	got, cmd := model.Update(key("u"))
	model = got.(Model)
	cmd()
	if len(resumed) != 1 || resumed[0] != "killed-row" {
		t.Fatalf("the first u resumed %v, want the killed row -- the archive window must not jump the queue", resumed)
	}
	if model.archiveUndoSessionID != "archived-row" || model.deleteUndoSessionID != "deleted-row" {
		t.Fatalf("the first u disturbed the other windows: archive=%q delete=%q", model.archiveUndoSessionID, model.deleteUndoSessionID)
	}

	// 2nd u: the delete window wins.
	got, cmd = model.Update(key("u"))
	model = got.(Model)
	cmd()
	if len(restored) != 1 || restored[0] != "deleted-row" {
		t.Fatalf("the second u restored %v, want the deleted row", restored)
	}
	if model.archiveUndoSessionID != "archived-row" {
		t.Fatalf("the second u consumed the archive window (id=%q)", model.archiveUndoSessionID)
	}

	// 3rd u: only now the archive window.
	got, cmd = model.Update(key("u"))
	model = got.(Model)
	cmd()
	if len(*unarchived) != 1 || (*unarchived)[0] != "archived-row" {
		t.Fatalf("the third u unarchived %v, want the archived row", *unarchived)
	}
	if len(resumed) != 1 || len(restored) != 1 {
		t.Fatalf("the third u also touched a pre-existing window: resumed=%v restored=%v", resumed, restored)
	}
}

// TestArchiveUndoWithNoUnarchiverWiredIsANoOp keeps the unwired-dependency
// shape the other trios already have (u with no resume/restoreSvc wired):
// nothing panics and nothing is written.
func TestArchiveUndoWithNoUnarchiverWiredIsANoOp(t *testing.T) {
	model := New(nil, config.Settings{Undo: time.Hour}, "")
	model.archiveUndoSessionID, model.archiveUndoSessionName = "archived-row", "archived-row"

	got, cmd := model.Update(key("u"))
	model = got.(Model)
	if cmd != nil {
		t.Fatalf("u with no unarchiver wired issued a command (%T)", cmd())
	}
	if model.archiveUndoSessionID != "archived-row" {
		t.Fatalf("u with no unarchiver wired consumed the window anyway (id=%q)", model.archiveUndoSessionID)
	}
}
