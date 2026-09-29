package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// TestLockChooserWording is task 009's own acceptance test (SPEC §8/§9.3):
// the conversation lock chooser (task 008's `c` inside detail, formerly the
// top-level `p` pin/start-fresh dialog) is worded throughout as a lock /
// resume-mode operation, never as a "pin" -- in its own title, in
// canPinResume's refusal message, and in the top-level help text's own
// "c inside detail" bullet. The stored value name "pinned"
// (resumeModeOptions) and the store column/struct field resume_pin/
// ResumePin are explicitly OUT of scope here (task 009's own
// successCriteria) and are never touched by this test.
func TestLockChooserWording(t *testing.T) {
	assertLockWording := func(t *testing.T, label, text string) {
		t.Helper()
		if strings.Contains(strings.ToLower(text), "pin") {
			t.Fatalf("%s still contains \"pin\" wording: %q", label, text)
		}
		if !strings.Contains(text, "lock conversation") && !strings.Contains(text, "resume mode") {
			t.Fatalf("%s names neither \"lock conversation\" nor \"resume mode\": %q", label, text)
		}
	}

	t.Run("title", func(t *testing.T) {
		model := NewWithShellCreatorAttacherKillerResumerProfileSwitcherAndResumeModer(
			nil, config.Settings{}, "", nil, nil, nil, nil, nil, nil,
			func(ctx context.Context, id, mode string) (store.Session, error) {
				return store.Session{}, nil
			},
		)
		model.sessions = []store.Session{{ID: "s1", Name: "alpha", Agent: "claude", Status: "running", ConversationID: "conv-1", ResumeState: "auto"}}
		model.selected = rowCursor(0)
		model.pinning = true
		model.pinValue = "auto"

		title := strings.SplitN(model.pinBody(), "\n", 2)[0]
		assertLockWording(t, "the lock chooser's title", title)
	})

	t.Run("canPinResume refusal message", func(t *testing.T) {
		model := NewWithShellCreatorAttacherKillerResumerProfileSwitcherAndResumeModer(
			nil, config.Settings{}, "", nil, nil, nil, nil, nil, nil,
			func(ctx context.Context, id, mode string) (store.Session, error) {
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
			t.Fatal("c opened the lock chooser for a shell session, which canPinResume refuses")
		}
		if model.attachError == "" {
			t.Fatal("canPinResume's refusal produced no message")
		}
		assertLockWording(t, "canPinResume's refusal message", model.attachError)
	})

	t.Run("help text", func(t *testing.T) {
		help := helpText(false)
		idx := strings.Index(help, "c inside detail")
		if idx < 0 {
			t.Fatalf("help text has no \"c inside detail\" bullet:\n%s", help)
		}
		// The bullet is embedded inside the `i` bullet's own continuation
		// lines (task 007/008's shape, unchanged by this task), so its end
		// is found via its own closing clause rather than the next
		// bullet's leading indent -- a bare "\n  " search would also
		// match every continuation line in between, since a 4-space
		// continuation line starts with the same two leading spaces a
		// fresh bullet does.
		const closing = "reachable only from\n    inside detail"
		end := strings.Index(help[idx:], closing)
		if end < 0 {
			t.Fatalf("could not find the end of the \"c inside detail\" bullet:\n%s", help[idx:])
		}
		bullet := help[idx : idx+end+len(closing)]
		assertLockWording(t, "the lock chooser's help text", bullet)
	})
}
