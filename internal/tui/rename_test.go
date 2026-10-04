package tui

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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
		func(_ context.Context, id, newName string) (store.Session, error) {
			renamedID, renamedTo = id, newName
			return updated, nil
		},
	)
	model.sessions = []store.Session{{ID: "s1", Name: "alpha", Agent: "shell", Status: "running", Slug: "alpha"}}
	model.selected = rowCursor(0)
	model.detail = true

	got, _ := model.Update(key("r"))
	model = got.(Model)
	if model.renameEdit.Value() != "alpha" || !model.renameEdit.Offered() {
		t.Fatalf("rename dialog did not offer the current name: value=%q offered=%v", model.renameEdit.Value(), model.renameEdit.Offered())
	}
	view := model.View()
	if !strings.Contains(view, "deck_alpha") {
		t.Fatalf("rename dialog did not state the fixed tmux session name:\n%s", view)
	}

	// A printable first keystroke replaces the offered value wholesale.
	got, _ = model.Update(key("b"))
	model = got.(Model)
	if model.renameEdit.Value() != "b" || model.renameEdit.Offered() {
		t.Fatalf("first keystroke did not replace the offer: value=%q offered=%v", model.renameEdit.Value(), model.renameEdit.Offered())
	}
	got, _ = model.Update(key("2"))
	model = got.(Model)
	if model.renameEdit.Value() != "b2" {
		t.Fatalf("value = %q, want b2", model.renameEdit.Value())
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
		func(_ context.Context, _, _ string) (store.Session, error) {
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
	if model.renameEdit.Value() != "" {
		t.Fatalf("Esc left a stale candidate value %q behind", model.renameEdit.Value())
	}
}

// TestRenameDialogRejectionRetainsCandidateForCorrection proves a rejected
// rename (e.g. a uniqueness collision) reports the error and keeps the
// user's candidate value in the field rather than reverting it, so it can
// be corrected rather than retyped from scratch.
func TestRenameDialogRejectionRetainsCandidateForCorrection(t *testing.T) {
	model := NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAgentCreatorRegistryPreviewCapturerEnvSetterRestarterInjectorDeleterRestorerReaperPurgerArchiverAndRenamer(
		nil, config.Settings{}, "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		func(_ context.Context, _, _ string) (store.Session, error) {
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
	if model.renameEdit.Value() != "b" {
		t.Fatalf("a rejected rename discarded the candidate value: got %q, want %q", model.renameEdit.Value(), "b")
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

// renameEditModel opens the rename dialog on a session named name through the
// real `r` key inside detail, and returns the model plus a pointer to what the
// wired renamer was last asked to store.
func renameEditModel(t *testing.T, name string) (Model, *string) {
	t.Helper()
	var renamedTo string
	model := NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAgentCreatorRegistryPreviewCapturerEnvSetterRestarterInjectorDeleterRestorerReaperPurgerArchiverAndRenamer(
		nil, config.Settings{}, "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		func(_ context.Context, id, newName string) (store.Session, error) {
			renamedTo = newName
			return store.Session{ID: id, Name: newName}, nil
		},
	)
	model.width, model.height = 80, 24
	model.sessions = []store.Session{{ID: "s1", Name: name, Agent: "shell", Status: "running", Slug: "alpha"}}
	model.selected = rowCursor(0)
	model.detail = true
	got, _ := model.Update(key("r"))
	return got.(Model), &renamedTo
}

func renameSend(t *testing.T, m Model, keys ...tea.KeyMsg) Model {
	t.Helper()
	for _, k := range keys {
		got, _ := m.Update(k)
		m = got.(Model)
	}
	return m
}

// TestRenameDialogEditsInTheMiddleOfTheName is R178's rename leg: `left`
// accepts the offered name and steps into it, a typed character lands at the
// caret, and Enter stores the edited name -- not an append and not a retype.
func TestRenameDialogEditsInTheMiddleOfTheName(t *testing.T) {
	model, renamedTo := renameEditModel(t, "alpha")
	model = renameSend(t, model, key("left"), key("left"), key("X"))
	if got := model.renameEdit.Value(); got != "alpXha" {
		t.Fatalf("value after left, left, X = %q, want alpXha (the name was %q)", got, "alpha")
	}
	if model.renameEdit.Offered() {
		t.Fatal("a caret key left the name offered")
	}
	got, cmd := model.Update(key("enter"))
	model = got.(Model)
	if cmd == nil {
		t.Fatal("enter did not dispatch the rename")
	}
	cmd()
	if *renamedTo != "alpXha" {
		t.Fatalf("renamer was asked for %q, want alpXha", *renamedTo)
	}
}

// TestRenameDialogBackspaceOnTheOfferedNameDeletesOneCharacter is the old
// wholesale-clear expectation rewritten to §11.11: backspace accepts the
// offer and deletes the character before the caret, it no longer empties it.
func TestRenameDialogBackspaceOnTheOfferedNameDeletesOneCharacter(t *testing.T) {
	model, _ := renameEditModel(t, "alpha")
	model = renameSend(t, model, key("backspace"))
	if got := model.renameEdit.Value(); got != "alph" {
		t.Fatalf("backspace on the offered name left %q, want alph", got)
	}
	model = renameSend(t, model, key("ctrl+h"))
	if got := model.renameEdit.Value(); got != "alp" {
		t.Fatalf("ctrl+h left %q, want alp", got)
	}
}

// TestRenameDialogPasteReplacesTheOfferedName: a bracketed paste is one
// insertion with control characters dropped, and it replaces an offer.
func TestRenameDialogPasteReplacesTheOfferedName(t *testing.T) {
	model, _ := renameEditModel(t, "alpha")
	paste := tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune("pasted name\n"), Paste: true})
	model = renameSend(t, model, paste)
	if got := model.renameEdit.Value(); got != "pasted name" {
		t.Fatalf("paste left %q, want %q", got, "pasted name")
	}
}

// TestRenameDialogSpaceEditsAndTabIsUnbound: space is typed text, and tab does
// nothing in a dialog with no path field (§11.4).
func TestRenameDialogSpaceEditsAndTabIsUnbound(t *testing.T) {
	model, _ := renameEditModel(t, "alpha")
	model = renameSend(t, model, tea.KeyMsg{Type: tea.KeyEnd}, key("tab"), tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}, key("b"))
	if got := model.renameEdit.Value(); got != "alpha b" {
		t.Fatalf("value = %q, want %q", got, "alpha b")
	}
}

// TestRenameDialogDrawsTheCaretInTheNameAndNowhereElse: the field draws a
// reverse-video caret over the character the caret is on, and it moves.
func TestRenameDialogDrawsTheCaretInTheNameAndNowhereElse(t *testing.T) {
	model, _ := renameEditModel(t, "alpha")
	model = renameSend(t, model, key("left"), key("left"))
	if body := model.renameBody(); !strings.Contains(body, "alp\x1b[7mh\x1b[27ma") {
		t.Fatalf("caret is not drawn as a reversed cell on the h of alpha:\n%q", body)
	}
	if strings.Contains(model.renameBody(), "_") && !strings.Contains(model.renameBody(), "deck_alpha") {
		t.Fatal("rename field draws a stand-in caret")
	}
}

// TestRenameHandlerHasNoOwnBackspaceOrAppendCase is R178's source scan: the
// handler edits through the shared editor and keeps no byte-trimming backspace
// or rune-append of its own.
func TestRenameHandlerHasNoOwnBackspaceOrAppendCase(t *testing.T) {
	src, err := os.ReadFile("rename.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	start := strings.Index(text, "func (m Model) updateRenameDialog(")
	end := strings.Index(text, "func (m *Model) submitRename(")
	if start < 0 || end < start {
		t.Fatal("could not locate updateRenameDialog in rename.go")
	}
	body := text[start:end]
	for _, banned := range []string{`case "backspace"`, `"ctrl+h"`, `+= string(`, `msg.Runes`} {
		if strings.Contains(body, banned) {
			t.Fatalf("updateRenameDialog still carries its own text editing (%s):\n%s", banned, body)
		}
	}
	if !strings.Contains(body, "renameEdit.Update(") {
		t.Fatal("updateRenameDialog does not edit through the shared line editor")
	}
}

// TestRenameCaretSurvivesNoColor: with colour off the caret is still an SGR 7
// cell (§11.11), the one escape the field draws, and the offered name carries
// no selection background.
func TestRenameCaretSurvivesNoColor(t *testing.T) {
	model, _ := renameEditModel(t, "alpha")
	model.settings.Color = false
	body := model.styledRenameBody()
	if !strings.Contains(body, "alpha\x1b[7m \x1b[27m") {
		t.Fatalf("NO_COLOR rename field lost its reversed caret cell:\n%q", body)
	}
	if strings.Contains(body, "\x1b[48") || strings.Contains(body, "\x1b[38") {
		t.Fatalf("NO_COLOR rename field carries colour:\n%q", body)
	}
}
