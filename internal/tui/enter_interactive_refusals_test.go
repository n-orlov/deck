package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// failingTmuxBinary writes an executable stand-in for tmux that fails every
// call with a recognisable message, so the ladder's tmux-error refusals can
// be driven deterministically without a tmux server.
func failingTmuxBinary(t *testing.T, message string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tmux")
	script := "#!/bin/sh\necho '" + message + "' >&2\nexit 1\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func refusalTestModel(slug string, client tmux.Client) Model {
	m := New(nil, config.Settings{}, "")
	m.width, m.height = 100, 30
	m.tmuxClient = client
	m.sessions = []store.Session{{ID: "sess-ladder", Name: "ladder", Slug: slug, Status: "waiting"}}
	m.selected = rowCursor(0)
	return m
}

// TestEnterInteractiveRefusesAnInvalidSlugBeforeTouchingTmux: the window
// target is derived from the slug first, so a slug tmux cannot name is
// refused with the slug error and no tmux call is ever made (the binary
// does not even exist).
func TestEnterInteractiveRefusesAnInvalidSlugBeforeTouchingTmux(t *testing.T) {
	m := refusalTestModel("Bad Slug!", tmux.Client{Binary: filepath.Join(t.TempDir(), "missing-tmux"), Socket: "ladder-sock"})
	next, cmd := m.enterInteractiveBody(false)
	got := next.(Model)
	if cmd != nil || got.interactive {
		t.Fatalf("an invalid slug entered interactive mode (cmd=%v interactive=%v)", cmd != nil, got.interactive)
	}
	if !got.entryRefusal.active || got.entryRefusal.kind != entryRefusalOther || got.entryRefusal.sessionID != "sess-ladder" {
		t.Fatalf("refusal = %+v, want an active entryRefusalOther for sess-ladder", got.entryRefusal)
	}
	if !strings.Contains(got.entryRefusal.reason, `invalid session slug "Bad Slug!"`) {
		t.Fatalf("refusal reason = %q, want the slug error", got.entryRefusal.reason)
	}
}

// TestForcedEntryRefusesWhenTmuxCannotListPanes: F skips the
// attached-client check, so the first tmux call is the pane lookup; its
// failure is surfaced as the refusal reason rather than swallowed.
func TestForcedEntryRefusesWhenTmuxCannotListPanes(t *testing.T) {
	bin := failingTmuxBinary(t, "fake tmux: server unreachable")
	m := refusalTestModel("ladder", tmux.Client{Binary: bin, Socket: "ladder-sock"})
	next, cmd := m.enterInteractiveBody(true)
	got := next.(Model)
	if cmd != nil || got.interactive {
		t.Fatalf("entry succeeded against a tmux that fails every call (cmd=%v interactive=%v)", cmd != nil, got.interactive)
	}
	if !got.entryRefusal.active || got.entryRefusal.kind != entryRefusalOther {
		t.Fatalf("refusal = %+v, want an active entryRefusalOther", got.entryRefusal)
	}
	if !strings.Contains(got.entryRefusal.reason, "fake tmux: server unreachable") {
		t.Fatalf("refusal reason = %q, want tmux's own failure text", got.entryRefusal.reason)
	}
}

// TestPlainEntryRefusesWhenTmuxCannotCountAttachedClients: without force the
// attached-count call comes first, and its failure refuses the entry the
// same way.
func TestPlainEntryRefusesWhenTmuxCannotCountAttachedClients(t *testing.T) {
	bin := failingTmuxBinary(t, "fake tmux: no such session")
	m := refusalTestModel("ladder", tmux.Client{Binary: bin, Socket: "ladder-sock"})
	next, _ := m.enterInteractiveBody(false)
	got := next.(Model)
	if got.interactive || !got.entryRefusal.active || got.entryRefusal.kind != entryRefusalOther {
		t.Fatalf("state = interactive %v, refusal %+v; want an active entryRefusalOther", got.interactive, got.entryRefusal)
	}
	if !strings.Contains(got.entryRefusal.reason, "fake tmux: no such session") {
		t.Fatalf("refusal reason = %q, want tmux's own failure text", got.entryRefusal.reason)
	}
}
