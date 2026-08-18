package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// TestPinDialogPersistsPinnedMode proves `p` opens the resume-mode dialog,
// cycles to "pinned", and persists that mode through the wired resumeMode
// function without ever touching a live pane (task 021, SPEC §8/§9.3).
func TestPinDialogPersistsPinnedMode(t *testing.T) {
	var persistedID, persistedMode string
	updated := store.Session{ID: "s1", Name: "alpha", Agent: "claude", Status: "running", ConversationID: "conv-1", ResumeState: "pinned", ResumePin: "conv-1"}
	model := NewWithShellCreatorAttacherKillerResumerProfileSwitcherAndResumeModer(
		nil, config.Settings{}, "", nil, nil, nil, nil, nil, nil,
		func(ctx context.Context, id, mode string) (store.Session, error) {
			persistedID, persistedMode = id, mode
			return updated, nil
		},
	)
	model.sessions = []store.Session{{ID: "s1", Name: "alpha", Agent: "claude", Status: "running", ConversationID: "conv-1", ResumeState: "auto"}}
	model.selected = 0

	got, _ := model.Update(key("p"))
	model = got.(Model)
	if !model.pinning {
		t.Fatal("p did not open the pin/fresh dialog")
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
	if persistedID != "s1" {
		t.Fatalf("resumeMode called with unexpected id %q", persistedID)
	}
	if persistedMode != "pinned" {
		t.Fatalf("resumeMode persisted %q, want pinned", persistedMode)
	}
}

// TestPinDialogEscCancelsWithoutPersisting proves Esc closes the pin dialog
// without calling resumeMode at all.
func TestPinDialogEscCancelsWithoutPersisting(t *testing.T) {
	called := false
	model := NewWithShellCreatorAttacherKillerResumerProfileSwitcherAndResumeModer(
		nil, config.Settings{}, "", nil, nil, nil, nil, nil, nil,
		func(ctx context.Context, id, mode string) (store.Session, error) {
			called = true
			return store.Session{}, nil
		},
	)
	model.sessions = []store.Session{{ID: "s1", Name: "alpha", Agent: "claude", Status: "running", ConversationID: "conv-1"}}
	model.selected = 0

	got, _ := model.Update(key("p"))
	model = got.(Model)
	got, _ = model.Update(key("esc"))
	model = got.(Model)

	if model.pinning {
		t.Fatal("Esc did not close the pin dialog")
	}
	if called {
		t.Fatal("Esc invoked resumeMode")
	}
}

// TestPinDialogNotOfferedForShell proves `p` refuses to open the dialog for
// a shell session, which has no conversation id to pin or restart fresh.
func TestPinDialogNotOfferedForShell(t *testing.T) {
	model := NewWithShellCreatorAttacherKillerResumerProfileSwitcherAndResumeModer(
		nil, config.Settings{}, "", nil, nil, nil, nil, nil, nil,
		func(ctx context.Context, id, mode string) (store.Session, error) {
			return store.Session{}, nil
		},
	)
	model.sessions = []store.Session{{ID: "s1", Name: "term", Agent: "shell", Status: "running"}}
	model.selected = 0

	got, _ := model.Update(key("p"))
	model = got.(Model)
	if model.pinning {
		t.Fatal("p opened the pin dialog for a shell session, which has no conversation id")
	}
	if model.attachError == "" {
		t.Fatal("p on a shell session produced no explanation")
	}
}
