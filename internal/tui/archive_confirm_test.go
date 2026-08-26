package tui

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// This file is R72's TUI half (issue #10, SPEC.md:752): `A` -- one Shift away
// from `a`, attach -- used to kill a live agent and hide its row on a single
// unconfirmed keystroke, which is how the operator lost a working session.
// The keypress now only opens a confirm dialog; the dialog's own Enter is the
// first thing that reaches archiveSvc.

// archiveRecorder is a fake archiveSvc that records every session it is
// handed. It records the whole row, not a count: "the keypress wrote nothing"
// and "the confirm wrote exactly the row the dialog named" are different
// claims, and only the row proves the second.
type archiveRecorder struct {
	calls []store.Session
}

func (a *archiveRecorder) archive(_ context.Context, session store.Session) error {
	a.calls = append(a.calls, session)
	return nil
}

// archiveConfirmTestModel is a two-row model whose selected row is LIVE
// (status "idle", the state `A` used to kill without asking), with an
// instrumented archiver and killer so any write either key performs is
// observable rather than inferred.
func archiveConfirmTestModel(t *testing.T) (Model, *archiveRecorder, *int) {
	t.Helper()
	archiver := &archiveRecorder{}
	kills := 0
	model := New(nil, config.Settings{}, "")
	model.sessions = []store.Session{
		{ID: "s-live", Name: "live-agent", Agent: "claude", Status: "idle", CWD: "/repos/project", ConversationID: "conv-live"},
		{ID: "s-other", Name: "bystander", Agent: "shell", Status: "idle", CWD: "/repos/other"},
	}
	model.selected = 0
	model.archiveSvc = archiver.archive
	model.kill = func(context.Context, store.Session) error {
		kills++
		return nil
	}
	model.attach = func(context.Context, string) (*exec.Cmd, error) { return exec.Command("true"), nil }
	return model, archiver, &kills
}

// TestArchiveKeyOpensAConfirmAndWritesNothing is the discriminating test for
// this requirement: `A` on a running row must issue NO command at all (the
// only route to archiveSvc, hence to the kill and to archived_at) and must
// instead put a dialog on screen that names the session and says, in as many
// words, that a live agent will be killed.
func TestArchiveKeyOpensAConfirmAndWritesNothing(t *testing.T) {
	model, archiver, kills := archiveConfirmTestModel(t)

	got, cmd := model.Update(key("A"))
	model = got.(Model)

	if cmd != nil {
		t.Fatalf("A issued a command on the keypress itself (%T from running it) -- it must write nothing until confirmed", cmd())
	}
	if len(archiver.calls) != 0 {
		t.Fatalf("A reached the archive service on the keypress: %+v", archiver.calls)
	}
	if *kills != 0 {
		t.Fatalf("A killed %d session(s) on the keypress", *kills)
	}
	if !model.archiveConfirming {
		t.Fatal("A did not open the archive confirm dialog")
	}
	body := model.archiveConfirmBody()
	if !strings.Contains(body, "Archive live-agent") {
		t.Fatalf("the archive confirm does not name the session:\n%s", body)
	}
	if !strings.Contains(body, "kills the live agent") {
		t.Fatalf("the archive confirm on a non-stopped row does not state that a live agent will be killed:\n%s", body)
	}
	if !strings.Contains(body, "Enter archives") || !strings.Contains(body, "Esc cancels") {
		t.Fatalf("the archive confirm does not state its own keys:\n%s", body)
	}
	if view := model.View(); !strings.Contains(view, "Archive live-agent") {
		t.Fatalf("View() does not render the archive confirm dialog:\n%s", view)
	}
}

// TestArchiveConfirmSubmitArchivesExactlyTheRowItNamed proves the other half:
// Enter inside the dialog does perform the archive, for the row the dialog
// named, and closing is driven by the sessionArchived result rather than
// optimistically at submit time.
func TestArchiveConfirmSubmitArchivesExactlyTheRowItNamed(t *testing.T) {
	model, archiver, _ := archiveConfirmTestModel(t)

	got, _ := model.Update(key("A"))
	model = got.(Model)
	got, cmd := model.Update(key("enter"))
	model = got.(Model)
	if cmd == nil {
		t.Fatal("Enter inside the archive confirm issued no command")
	}
	msg := cmd()
	if len(archiver.calls) != 1 || archiver.calls[0].ID != "s-live" {
		t.Fatalf("archive called with %+v, want exactly one call for the row the dialog named (s-live)", archiver.calls)
	}
	result, ok := msg.(sessionArchived)
	if !ok {
		t.Fatalf("the archive confirm's submit produced %T, want sessionArchived", msg)
	}
	if result.err != nil {
		t.Fatalf("archive reported an error: %v", result.err)
	}
	if !model.archiveConfirming {
		t.Fatal("the dialog closed at submit time, before the archive result landed")
	}

	got, _ = model.Update(result)
	model = got.(Model)
	if model.archiveConfirming {
		t.Fatal("a successful archive left the confirm dialog open")
	}
	if model.archiveNote != "" || model.attachError != "" {
		t.Fatalf("a successful archive left a note on screen: note=%q attachError=%q", model.archiveNote, model.attachError)
	}
}

// TestArchiveConfirmEscCancelsWithoutWritingAnything covers the §11.4 contract
// half that matters most here: the escape hatch really is an escape hatch.
func TestArchiveConfirmEscCancelsWithoutWritingAnything(t *testing.T) {
	model, archiver, kills := archiveConfirmTestModel(t)

	got, _ := model.Update(key("A"))
	model = got.(Model)
	got, cmd := model.Update(key("esc"))
	model = got.(Model)
	if cmd != nil {
		t.Fatalf("esc inside the archive confirm issued a command (%T)", cmd())
	}
	if model.archiveConfirming {
		t.Fatal("esc did not close the archive confirm")
	}
	if len(archiver.calls) != 0 || *kills != 0 {
		t.Fatalf("esc wrote something anyway: archive=%+v kills=%d", archiver.calls, *kills)
	}
}

// TestArchiveConfirmSuppressesTheBareLetterKeymap is the assertion the
// requirement asks for by name: while the dialog is open the bare-letter
// keymap is suppressed, so a second `A` does nothing surprising -- and
// neither does `x` (kill), `d` (the dd chord's first half), `j` (move the
// selection out from under the dialog's own question) or `u` (undo). Every
// one of those keys is swallowed by updateArchiveConfirm, leaving the dialog
// exactly as it was.
func TestArchiveConfirmSuppressesTheBareLetterKeymap(t *testing.T) {
	model, archiver, kills := archiveConfirmTestModel(t)

	got, _ := model.Update(key("A"))
	model = got.(Model)
	before := model.View()

	for _, k := range []string{"A", "x", "d", "j", "u", "m", "r"} {
		got, cmd := model.Update(key(k))
		model = got.(Model)
		if cmd != nil {
			t.Fatalf("%q inside the archive confirm issued a command (%T)", k, cmd())
		}
		if !model.archiveConfirming {
			t.Fatalf("%q inside the archive confirm closed the dialog", k)
		}
		if model.pendingDelete {
			t.Fatalf("%q inside the archive confirm armed the dd chord", k)
		}
		if model.selected != 0 {
			t.Fatalf("%q inside the archive confirm moved the selection to %d", k, model.selected)
		}
		if len(model.marked) != 0 {
			t.Fatalf("%q inside the archive confirm marked %d row(s)", k, len(model.marked))
		}
		if view := model.View(); view != before {
			t.Fatalf("%q inside the archive confirm changed the rendered dialog:\n%s", k, view)
		}
	}
	if len(archiver.calls) != 0 || *kills != 0 {
		t.Fatalf("the suppressed keys wrote something: archive=%+v kills=%d", archiver.calls, *kills)
	}
}

// TestArchiveConfirmOnAStoppedRowPromisesNoKill keeps the live-agent sentence
// honest in the other direction: `A` on an already-stopped row sets
// archived_at alone, so a dialog claiming it would kill something would be a
// false statement about what confirming does.
func TestArchiveConfirmOnAStoppedRowPromisesNoKill(t *testing.T) {
	model, _, _ := archiveConfirmTestModel(t)
	model.sessions[0].Status = "stopped"

	got, _ := model.Update(key("A"))
	model = got.(Model)
	body := model.archiveConfirmBody()
	if !strings.Contains(body, "Archive live-agent") {
		t.Fatalf("the archive confirm does not name the stopped session:\n%s", body)
	}
	if strings.Contains(body, "kills the live agent") {
		t.Fatalf("the archive confirm threatens a kill on an already-stopped row:\n%s", body)
	}
}

// TestArchiveConfirmFailedSubmitStaysOpenAndSaysWhy mirrors sessionDeleted's
// own failure handling: a refused archive keeps the dialog up with the reason
// on it rather than vanishing, which would leave the operator unable to tell
// a refusal from a success.
func TestArchiveConfirmFailedSubmitStaysOpenAndSaysWhy(t *testing.T) {
	model, _, _ := archiveConfirmTestModel(t)

	got, _ := model.Update(key("A"))
	model = got.(Model)
	got, _ = model.Update(sessionArchived{session: model.sessions[0], err: context.DeadlineExceeded})
	model = got.(Model)
	if !model.archiveConfirming {
		t.Fatal("a failed archive closed the confirm dialog")
	}
	if !strings.Contains(model.archiveConfirmBody(), "Cannot archive") {
		t.Fatalf("a failed archive did not state why on the dialog:\n%s", model.archiveConfirmBody())
	}
}

// TestArchiveKeyStatesItIsUnavailableWhenUnwired keeps the pre-existing
// unwired-dependency behaviour (task 111): a Model built without an archiver
// says so on the keypress instead of opening a dialog whose Enter can only
// fail.
func TestArchiveKeyStatesItIsUnavailableWhenUnwired(t *testing.T) {
	model := New(nil, config.Settings{}, "")
	model.sessions = []store.Session{{ID: "s1", Name: "alpha", Agent: "shell", Status: "idle"}}

	got, cmd := model.Update(key("A"))
	model = got.(Model)
	if cmd != nil {
		t.Fatal("A with no archiver wired issued a command")
	}
	if model.archiveConfirming {
		t.Fatal("A with no archiver wired opened a confirm dialog that could not archive anything")
	}
	if !strings.Contains(model.attachError, "unavailable") {
		t.Fatalf("A with no archiver wired said %q, want it to state archiving is unavailable", model.attachError)
	}
}
