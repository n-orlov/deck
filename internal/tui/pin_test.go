package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// Task 008 moved every one of these tests off a bare top-level `p` onto
// `i` then `c`: the conversation lock chooser is now reachable only from
// inside the `i` detail dialog (there is no top-level "p" case in
// Model.Update's main switch that opens IT any more -- see rename.go's
// own case "c"; task 010 later gives a bare top-level "p" a completely
// different job, the sidebar pin toggle, which never opens this chooser).
// Each test below opens detail first, asserting it actually opened,
// exactly the same way profile_switch_test.go's own tests do for `P`.

// TestPinDialogPersistsPinnedMode proves `i` then `c` opens the
// resume-mode dialog, cycles to "pinned", persists that mode through the
// wired resumeMode function without ever touching a live pane, and
// returns to the detail view once the change succeeds (task 008/021,
// SPEC §8/§9.3).
func TestPinDialogPersistsPinnedMode(t *testing.T) {
	var persistedID, persistedMode string
	updated := store.Session{ID: "s1", Name: "alpha", Agent: "claude", Status: "running", ConversationID: "conv-1", ResumeState: "pinned", ResumePin: "conv-1"}
	model := NewWithShellCreatorAttacherKillerResumerProfileSwitcherAndResumeModer(
		nil, config.Settings{}, "", nil, nil, nil, nil, nil, nil,
		func(_ context.Context, id, mode string) (store.Session, error) {
			persistedID, persistedMode = id, mode
			return updated, nil
		},
	)
	model.sessions = []store.Session{{ID: "s1", Name: "alpha", Agent: "claude", Status: "running", ConversationID: "conv-1", ResumeState: "auto"}}
	model.selected = rowCursor(0)

	got, _ := model.Update(key("i"))
	model = got.(Model)
	if !model.detail {
		t.Fatal("i did not open the detail dialog")
	}

	got, _ = model.Update(key("c"))
	model = got.(Model)
	if !model.pinning {
		t.Fatal("c did not open the pin/fresh dialog from inside detail")
	}
	view := model.View()
	if !strings.Contains(view, "sticky") {
		t.Fatalf("pin dialog did not explain pinned's sticky-across-restart behavior:\n%s", view)
	}

	// Cycle right once so the candidate value moves from auto to pinned.
	got, _ = model.Update(key("right"))
	model = got.(Model)

	got, cmd := model.Update(key("enter"))
	model = got.(Model)
	if cmd == nil {
		t.Fatal("enter on the pin dialog did not dispatch a command")
	}
	msg := cmd()
	got, loadCmd := model.Update(msg)
	model = got.(Model)
	if loadCmd == nil {
		t.Fatal("a successful resume-mode change did not trigger a reload")
	}
	if model.pinning {
		t.Fatal("pin dialog remained open after a successful change")
	}
	if !model.detail {
		t.Fatal("a successful resume-mode change did not return to the detail view")
	}
	if persistedID != "s1" {
		t.Fatalf("resumeMode called with unexpected id %q", persistedID)
	}
	if persistedMode != "pinned" {
		t.Fatalf("resumeMode persisted %q, want pinned", persistedMode)
	}
}

// TestPinDialogEscCancelsWithoutPersisting proves Esc closes the pin
// dialog without calling resumeMode at all, and returns to the detail
// view underneath it (task 008), not all the way out to the main list.
func TestPinDialogEscCancelsWithoutPersisting(t *testing.T) {
	called := false
	model := NewWithShellCreatorAttacherKillerResumerProfileSwitcherAndResumeModer(
		nil, config.Settings{}, "", nil, nil, nil, nil, nil, nil,
		func(_ context.Context, _, _ string) (store.Session, error) {
			called = true
			return store.Session{}, nil
		},
	)
	model.sessions = []store.Session{{ID: "s1", Name: "alpha", Agent: "claude", Status: "running", ConversationID: "conv-1"}}
	model.selected = rowCursor(0)

	got, _ := model.Update(key("i"))
	model = got.(Model)
	got, _ = model.Update(key("c"))
	model = got.(Model)
	got, _ = model.Update(key("esc"))
	model = got.(Model)

	if model.pinning {
		t.Fatal("Esc did not close the pin dialog")
	}
	if !model.detail {
		t.Fatal("Esc from the pin dialog did not return to the detail view")
	}
	if called {
		t.Fatal("Esc invoked resumeMode")
	}
}

// TestPinDialogNotOfferedForShell proves `c` refuses to open the dialog
// for a shell session, which has no conversation id to lock or restart
// fresh, staying on the detail view underneath it (canPinResume's
// refusal message) rather than closing anything.
func TestPinDialogNotOfferedForShell(t *testing.T) {
	model := NewWithShellCreatorAttacherKillerResumerProfileSwitcherAndResumeModer(
		nil, config.Settings{}, "", nil, nil, nil, nil, nil, nil,
		func(_ context.Context, _, _ string) (store.Session, error) {
			return store.Session{}, nil
		},
	)
	model.sessions = []store.Session{{ID: "s1", Name: "term", Agent: "shell", Status: "running"}}
	model.selected = rowCursor(0)

	got, _ := model.Update(key("i"))
	model = got.(Model)
	if !model.detail {
		t.Fatal("i did not open the detail dialog")
	}
	got, _ = model.Update(key("c"))
	model = got.(Model)
	if model.pinning {
		t.Fatal("c opened the pin dialog for a shell session, which has no conversation id")
	}
	if !model.detail {
		t.Fatal("c's refusal for a shell session closed the detail dialog underneath it")
	}
	if model.attachError == "" {
		t.Fatal("c on a shell session produced no explanation")
	}
}
