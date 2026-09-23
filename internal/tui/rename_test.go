package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

var errRenameCollisionForTest = errors.New(`session name "b" already exists`)

// TestRenameOnlyReachableInsideDetailNotAsTopLevelKey proves task 013's
// central placement rule (SPEC §11.4, PRD requirement 31, I-8): a
// top-level "r" (outside detail) does NOT open the rename dialog -- it
// keeps meaning resume, exactly as before this task -- and rename only
// opens once the `i` detail dialog is already showing.
func TestRenameOnlyReachableInsideDetailNotAsTopLevelKey(t *testing.T) {
	model := New(nil, config.Settings{}, "")
	model.sessions = []store.Session{{ID: "s1", Name: "alpha", Agent: "shell", Status: "running"}}
	model.selected = rowCursor(0)

	got, _ := model.Update(key("r"))
	model = got.(Model)
	if model.renaming {
		t.Fatal("a top-level r opened the rename dialog; it must stay reachable only from inside detail")
	}
	if model.detail {
		t.Fatal("a top-level r opened detail")
	}

	got, _ = model.Update(key("i"))
	model = got.(Model)
	if !model.detail {
		t.Fatal("i did not open detail")
	}
	got, _ = model.Update(key("r"))
	model = got.(Model)
	if !model.renaming {
		t.Fatal("r inside detail did not open the rename dialog")
	}
	if !model.detail {
		t.Fatal("opening rename closed the detail dialog underneath it")
	}
}

// TestRenameDialogPrefillsSubmitsAndClosesBackToDetail proves the rename
// dialog prefills the session's current name, a first keystroke replaces
// it wholesale, Enter submits through the wired renamer, and a successful
// rename closes the rename sub-dialog while leaving m.detail (and
// therefore detailView, not the main list) showing underneath it.
func TestRenameDialogPrefillsSubmitsAndClosesBackToDetail(t *testing.T) {
	var renamedID, renamedTo string
	updated := store.Session{ID: "s1", Name: "new-name", Agent: "shell", Status: "running", Slug: "alpha"}
	model := NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAgentCreatorRegistryPreviewCapturerEnvSetterRestarterInjectorDeleterRestorerReaperPurgerArchiverAndRenamer(
		nil, config.Settings{}, "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		func(ctx context.Context, id, newName string) (store.Session, error) {
			renamedID, renamedTo = id, newName
			return updated, nil
		},
	)
	model.sessions = []store.Session{{ID: "s1", Name: "alpha", Agent: "shell", Status: "running", Slug: "alpha"}}
	model.selected = rowCursor(0)
	model.detail = true

	got, _ := model.Update(key("r"))
	model = got.(Model)
	if model.renameValue != "alpha" || !model.renamePrefilled {
		t.Fatalf("rename dialog did not prefill the current name: value=%q prefilled=%v", model.renameValue, model.renamePrefilled)
	}
	view := model.View()
	if !strings.Contains(view, "deck_alpha") {
		t.Fatalf("rename dialog did not state the fixed tmux session name:\n%s", view)
	}

	// First keystroke replaces the prefilled value wholesale.
	got, _ = model.Update(key("b"))
	model = got.(Model)
	if model.renameValue != "b" || model.renamePrefilled {
		t.Fatalf("first keystroke did not replace the prefill: value=%q prefilled=%v", model.renameValue, model.renamePrefilled)
	}
	got, _ = model.Update(key("2"))
	model = got.(Model)
	if model.renameValue != "b2" {
		t.Fatalf("value = %q, want b2", model.renameValue)
	}

	got, cmd := model.Update(key("enter"))
	model = got.(Model)
	if cmd == nil {
		t.Fatal("enter on the rename dialog did not dispatch a command")
	}
	msg := cmd()
	got, loadCmd := model.Update(msg)
	model = got.(Model)
	if loadCmd == nil {
		t.Fatal("a successful rename did not trigger a reload")
	}
	if model.renaming {
		t.Fatal("rename dialog remained open after a successful rename")
	}
	if !model.detail {
		t.Fatal("a successful rename closed detail underneath it; it must return to detailView")
	}
	if renamedID != "s1" || renamedTo != "b2" {
		t.Fatalf("renamer called with (%q, %q), want (s1, b2)", renamedID, renamedTo)
	}
}

// TestRenameDialogEscCancelsWithoutPersistingAndKeepsDetailOpen proves Esc
// on the rename dialog discards the candidate value without calling the
// renamer, and leaves the underlying detail dialog open (esc closes only
// the innermost dialog, one at a time).
func TestRenameDialogEscCancelsWithoutPersistingAndKeepsDetailOpen(t *testing.T) {
	called := false
	model := NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAgentCreatorRegistryPreviewCapturerEnvSetterRestarterInjectorDeleterRestorerReaperPurgerArchiverAndRenamer(
		nil, config.Settings{}, "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		func(ctx context.Context, id, newName string) (store.Session, error) {
			called = true
			return store.Session{}, nil
		},
	)
	model.sessions = []store.Session{{ID: "s1", Name: "alpha", Agent: "shell", Status: "running"}}
	model.selected = rowCursor(0)
	model.detail = true

	got, _ := model.Update(key("r"))
	model = got.(Model)
	got, _ = model.Update(key("z"))
	model = got.(Model)
	got, _ = model.Update(key("esc"))
	model = got.(Model)

	if model.renaming {
		t.Fatal("Esc did not close the rename dialog")
	}
	if !model.detail {
		t.Fatal("Esc on the rename dialog closed detail underneath it too")
	}
	if called {
		t.Fatal("Esc invoked the renamer")
	}
	if model.renameValue != "" {
		t.Fatalf("Esc left a stale candidate value %q behind", model.renameValue)
	}
}

// TestRenameDialogRejectionRetainsCandidateForCorrection proves a rejected
// rename (e.g. a uniqueness collision) reports the error and keeps the
// user's candidate value in the field rather than reverting it, so it can
// be corrected rather than retyped from scratch.
func TestRenameDialogRejectionRetainsCandidateForCorrection(t *testing.T) {
	model := NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAgentCreatorRegistryPreviewCapturerEnvSetterRestarterInjectorDeleterRestorerReaperPurgerArchiverAndRenamer(
		nil, config.Settings{}, "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		func(ctx context.Context, id, newName string) (store.Session, error) {
			return store.Session{}, errRenameCollisionForTest
		},
	)
	model.sessions = []store.Session{{ID: "s1", Name: "alpha", Agent: "shell", Status: "running"}}
	model.selected = rowCursor(0)
	model.detail = true

	got, _ := model.Update(key("r"))
	model = got.(Model)
	got, _ = model.Update(key("b"))
	model = got.(Model)

	got, cmd := model.Update(key("enter"))
	model = got.(Model)
	msg := cmd()
	got, loadCmd := model.Update(msg)
	model = got.(Model)

	if loadCmd != nil {
		t.Fatal("a rejected rename triggered a reload")
	}
	if !model.renaming {
		t.Fatal("a rejected rename closed the dialog; it must stay open so the value can be corrected")
	}
	if model.renameValue != "b" {
		t.Fatalf("a rejected rename discarded the candidate value: got %q, want %q", model.renameValue, "b")
	}
	if model.renameNote == "" {
		t.Fatal("a rejected rename produced no explanation")
	}
}

// TestRenameDialogUnavailableWithoutRenamerWired proves the "unavailable"
// message (rather than silently doing nothing) when no renamer is wired at
// all, mirroring pin/profile/env's own nil-callback convention.
func TestRenameDialogUnavailableWithoutRenamerWired(t *testing.T) {
	model := New(nil, config.Settings{}, "")
	model.sessions = []store.Session{{ID: "s1", Name: "alpha", Agent: "shell", Status: "running"}}
	model.selected = rowCursor(0)
	model.detail = true

	got, _ := model.Update(key("r"))
	model = got.(Model)
	if !model.renaming {
		t.Fatal("r did not open the rename dialog")
	}
	got, cmd := model.Update(key("enter"))
	model = got.(Model)
	if cmd != nil {
		t.Fatal("enter with no renamer wired returned a non-nil command")
	}
	if model.renameNote == "" {
		t.Fatal("no renamer wired produced no explanation")
	}
}
