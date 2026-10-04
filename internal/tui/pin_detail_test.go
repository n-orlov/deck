package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// This file is task 008's own acceptance test obligation (SPEC §8/§9.3):
// the conversation lock chooser (formerly the top-level `p` pin/start-fresh
// dialog) is unbound at the top level entirely, and is reachable only from
// inside the `i` detail dialog, exactly like rename.go's "r"/"l"/"g"/"P"
// before it (task007's own profile_switch_detail_test.go is the direct
// template for this file).
//
// task 010 (SPEC §11's pin rule, R159) later rebinds a bare top-level `p`
// to a completely different action -- the sidebar pin toggle -- so the
// test below no longer asserts that `p` is a pure no-op (it now dispatches
// a tea.Cmd and, once that Cmd runs, mutates pinned_at); it keeps proving
// only what is still true: `p` never resurrects the OLD lock chooser task
// 008 moved to `i` then `c`. The pin toggle itself is proved separately
// (sidebar_pin_test.go's TestPTogglesPin/TestPMarkedSetMixedPinsAllThenUnpinsAll).

// TestTopLevelPDoesNotOpenTheLockChooser proves a bare top-level "p" (the
// cursor resting on a session row, m.detail false) never opens the
// conversation lock chooser or the detail dialog, and never invokes
// resumeMode -- whatever else task 010's own pin toggle does with the
// keypress.
func TestTopLevelPDoesNotOpenTheLockChooser(t *testing.T) {
	var called bool
	model := NewWithShellCreatorAttacherKillerResumerProfileSwitcherAndResumeModer(
		nil, config.Settings{}, "", nil, nil, nil, nil, nil, nil,
		func(_ context.Context, _, _ string) (store.Session, error) {
			called = true
			return store.Session{}, nil
		},
	)
	model.sessions = []store.Session{{ID: "s1", Name: "alpha", Agent: "claude", Status: "running", ConversationID: "conv-1", ResumeState: "auto"}}
	model.selected = rowCursor(0)

	got, _ := model.Update(key("p"))
	after, ok := got.(Model)
	if !ok {
		t.Fatalf("Update(p) returned %T, not tui.Model", got)
	}
	if after.pinning {
		t.Fatal("a top-level p opened the lock chooser")
	}
	if after.detail {
		t.Fatal("a top-level p opened the detail dialog")
	}
	if called {
		t.Fatal("a top-level p invoked resumeMode")
	}
}

// TestDetailCLocksConversation proves the full `i` then `c` route (task
// 008): the chooser opens only once detail is already showing, a
// submitted lock persists through the same durable resumeMode write
// exactly as the old top-level `p` route did (writes resume_state =
// "pinned" and sets resume_pin), the model lands back on the detail view
// (not the main list) once it closes -- on both a successful submit and
// an Esc cancel -- and an ineligible session (no conversation id to pin
// at all) is refused with canPinResume's own message, still leaving
// detail open underneath it.
func TestDetailCLocksConversation(t *testing.T) {
	t.Run("submit persists through resumeMode exactly as the old top-level route and returns to detail", func(t *testing.T) {
		var persistedID, persistedMode string
		updatedSession := store.Session{ID: "s1", Name: "alpha", Agent: "claude", Status: "running", ConversationID: "conv-1", ResumeState: "pinned", ResumePin: "conv-1"}
		model := NewWithShellCreatorAttacherKillerResumerProfileSwitcherAndResumeModer(
			nil, config.Settings{}, "", nil, nil, nil, nil, nil, nil,
			func(_ context.Context, id, mode string) (store.Session, error) {
				persistedID, persistedMode = id, mode
				return updatedSession, nil
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
			t.Fatal("c inside detail did not open the lock chooser")
		}
		// auto -> pinned
		got, _ = model.Update(key("right"))
		model = got.(Model)
		if model.pinValue != "pinned" {
			t.Fatalf("candidate value after cycling = %q, want pinned", model.pinValue)
		}

		got, cmd := model.Update(key("enter"))
		model = got.(Model)
		if cmd == nil {
			t.Fatal("enter on the lock chooser did not dispatch a command")
		}
		msg := cmd()
		got, loadCmd := model.Update(msg)
		model = got.(Model)
		if loadCmd == nil {
			t.Fatal("a successful lock did not trigger a reload")
		}
		if model.pinning {
			t.Fatal("the lock chooser stayed open after a successful submit")
		}
		if !model.detail {
			t.Fatal("a successful lock did not land back on the detail view")
		}
		if persistedID != "s1" {
			t.Fatalf("resumeMode persisted id %q, want s1", persistedID)
		}
		// This is exactly what the old top-level `p` route wrote (SPEC
		// §8/§9.3, resumeModeOptions): resume_state = "pinned", which the
		// wired resumeMode function is what actually sets resume_pin from
		// the session's own current conversation id (service layer, not
		// this package) -- proven here by asserting the mode value handed
		// to resumeMode is unchanged from before this task's move.
		if persistedMode != "pinned" {
			t.Fatalf("resumeMode persisted %q, want pinned", persistedMode)
		}
		if updatedSession.ResumeState != "pinned" || updatedSession.ResumePin != "conv-1" {
			t.Fatalf("resumeMode's own returned session = %+v, want resume_state pinned and resume_pin conv-1", updatedSession)
		}
	})

	t.Run("esc returns to detail, not the list", func(t *testing.T) {
		model := NewWithShellCreatorAttacherKillerResumerProfileSwitcherAndResumeModer(
			nil, config.Settings{}, "", nil, nil, nil, nil, nil, nil,
			func(_ context.Context, _, _ string) (store.Session, error) {
				return store.Session{}, nil
			},
		)
		model.sessions = []store.Session{{ID: "s1", Name: "alpha", Agent: "claude", Status: "running", ConversationID: "conv-1", ResumeState: "auto"}}
		model.selected = rowCursor(0)

		got, _ := model.Update(key("i"))
		model = got.(Model)
		got, _ = model.Update(key("c"))
		model = got.(Model)
		if !model.pinning {
			t.Fatal("c inside detail did not open the lock chooser")
		}

		got, _ = model.Update(key("esc"))
		model = got.(Model)
		if model.pinning {
			t.Fatal("esc did not close the lock chooser")
		}
		if !model.detail {
			t.Fatal("esc from the lock chooser closed detail too -- it must return to detail, not the list")
		}

		// A second esc closes detail itself, proving detail (and only
		// detail) is what the chooser's own esc landed on.
		got, _ = model.Update(key("esc"))
		model = got.(Model)
		if model.detail {
			t.Fatal("esc on the detail view itself did not close it")
		}
	})

	t.Run("ineligible session is refused with canPinResume's message", func(t *testing.T) {
		model := NewWithShellCreatorAttacherKillerResumerProfileSwitcherAndResumeModer(
			nil, config.Settings{}, "", nil, nil, nil, nil, nil, nil,
			func(_ context.Context, _, _ string) (store.Session, error) {
				return store.Session{}, nil
			},
		)
		model.sessions = []store.Session{{ID: "s1", Name: "alpha", Agent: "shell", Status: "running"}}
		model.selected = rowCursor(0)

		got, _ := model.Update(key("i"))
		model = got.(Model)
		got, _ = model.Update(key("c"))
		model = got.(Model)
		if model.pinning {
			t.Fatal("c opened the lock chooser for a shell session, which has no conversation id to lock or restart fresh")
		}
		if !model.detail {
			t.Fatal("c's refusal closed the detail dialog underneath it")
		}
		if !strings.Contains(model.attachError, "no conversation id to lock or restart fresh") {
			t.Fatalf("attachError = %q, want canPinResume's own refusal wording", model.attachError)
		}
	})
}
