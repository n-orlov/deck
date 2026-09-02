package tui

import (
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// attachForceEnterPTY attaches a real tmux client directly to target
// through a pty, exactly like internal/tmux/restore_test.go's own
// attachThroughPTY, so SessionAttachedCount reads a genuine attached
// client rather than a hand-set option -- this is the one part of task
// 105's own refusal-skip (SessionAttachedCount > 0) that cannot be
// produced any other way.
func attachForceEnterPTY(t *testing.T, socket, target string) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, "tmux", "-L", socket, "attach-session", "-t", target)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	terminal, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 24, Cols: 80})
	if err != nil {
		t.Fatalf("attach %q through pty: %v", target, err)
	}
	go func() { _, _ = io.CopyBuffer(io.Discard, terminal, make([]byte, 4096)) }()
	t.Cleanup(func() { _ = terminal.Close() })
}

// waitForSessionAttachedCountForce polls target's #{session_attached}
// until it reads want or a deadline expires -- attaching a client through
// a pty is asynchronous from this goroutine's point of view. This mirrors
// internal/tmux/restore_test.go's own waitForSessionAttachedCount, which
// is unexported from a different package.
func waitForSessionAttachedCountForce(t *testing.T, client tmux.Client, target string, want int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	var last int
	var lastErr error
	for time.Now().Before(deadline) {
		last, lastErr = client.SessionAttachedCount(context.Background(), target)
		if lastErr == nil && last == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("session_attached on %q did not reach %d within the deadline (last=%d err=%v)", target, want, last, lastErr)
}

// TestForceEntersDespiteAnAttachedClient proves task 105's whole point:
// `F` (enterInteractiveBody(true)) enters interactive mode over a window
// SessionAttachedCount already reports > 0 for -- the one refusal `↵`
// itself still respects -- and does so by taking a REAL
// ForceClaimWindowOwnership steal (task 101), not merely skipping the
// count check ahead of the ordinary ClaimWindowOwnership underneath:
// WindowOwnership.Probe (task 103) on the claim enterInteractiveBody
// leaves in m.interactiveOwnership reads ClaimStillMine only if the
// option on the window is exactly the value that claim wrote, which is
// what pins the force path down to the force claim itself.
func TestForceEntersDespiteAnAttachedClient(t *testing.T) {
	m := New(nil, config.Settings{Color: true}, "")
	m.width, m.height = 100, 30
	if _, height := m.previewContentSize(); height < interactiveMinInnerRows {
		t.Fatalf("test assumption violated: preview content height %d is below the %d-row floor", height, interactiveMinInnerRows)
	}

	socket := selectionTestSocket("forceattached")
	newQuietSelectionPane(t, socket, "deck_forceattached", 80, 24)
	client := tmux.Client{Socket: socket}
	m.tmuxClient = client
	m.sessions = []store.Session{{ID: "sess-force-1", Name: "forceattached", Slug: "forceattached", Status: "waiting"}}
	m.selected = 0

	windowTarget, err := tmux.SessionName("forceattached")
	if err != nil {
		t.Fatalf("SessionName: %v", err)
	}
	attachForceEnterPTY(t, socket, windowTarget)
	waitForSessionAttachedCountForce(t, client, windowTarget, 1)

	// Prove the ordinary path still refuses while attached, so this test
	// is not vacuous about what force is actually skipping.
	plainNext, _ := m.enterInteractive()
	plainGot := plainNext.(Model)
	if plainGot.interactive {
		t.Fatalf("enterInteractive (force=false) entered while a real client was attached")
	}
	if !strings.Contains(plainGot.attachError, "another client is attached") {
		t.Fatalf("enterInteractive (force=false) attachError = %q, want the attached-client refusal", plainGot.attachError)
	}

	next, _ := m.enterInteractiveBody(true)
	got := next.(Model)
	if got.attachError != "" {
		t.Fatalf("F (force=true) refused despite an attached client: %q", got.attachError)
	}
	if !got.interactive {
		t.Fatalf("F (force=true) did not enter interactive mode")
	}
	if got.interactiveOwnership == nil {
		t.Fatalf("F entered interactive mode without recording a WindowOwnership claim")
	}
	state, err := got.interactiveOwnership.Probe(context.Background())
	if err != nil {
		t.Fatalf("probe the force claim after entry: %v", err)
	}
	if state != tmux.ClaimStillMine {
		t.Fatalf("force claim probe = %v, want %v (proves the force claim itself ran, not merely the count-check skip)", state, tmux.ClaimStillMine)
	}

	got.exitInteractive()
}

// TestForceStillRefusesAStoppedRow proves force does not touch the
// stopped-session refusal: it is checked before force is ever consulted
// (canReachPane, ahead of any tmux call), and is not the refusal force
// exists to skip.
func TestForceStillRefusesAStoppedRow(t *testing.T) {
	m := New(nil, config.Settings{Color: true}, "")
	m.width, m.height = 100, 30
	m.tmuxClient = tmux.Client{Socket: "deck-tui-force-no-server"}
	m.sessions = []store.Session{{ID: "sess-force-2", Name: "forcestopped", Slug: "forcestopped", Status: "stopped"}}
	m.selected = 0

	next, cmd := m.enterInteractiveBody(true)
	got := next.(Model)
	if got.interactive {
		t.Fatalf("F entered interactive mode against a stopped session")
	}
	if cmd != nil {
		t.Fatalf("F returned a non-nil cmd on the stopped-session refusal, want nil")
	}
	if !strings.Contains(got.attachError, stoppedSessionRefusalTail) {
		t.Fatalf("attachError = %q, want the stopped-session refusal %q", got.attachError, stoppedSessionRefusalTail)
	}
}

// TestForceStillRefusesBelowTheFloor proves force does not touch the
// 7-inner-row floor refusal either: it runs first in the ladder, before
// any tmux call and before force is consulted at all.
func TestForceStillRefusesBelowTheFloor(t *testing.T) {
	m := New(nil, config.Settings{Color: true}, "")
	m.width, m.height = 100, 8
	if _, height := m.previewContentSize(); height >= interactiveMinInnerRows {
		t.Fatalf("test assumption violated: preview content height %d is not below the %d-row floor", height, interactiveMinInnerRows)
	}
	m.tmuxClient = tmux.Client{Socket: "deck-tui-force-no-server"}
	m.sessions = []store.Session{{ID: "sess-force-3", Name: "forcefloor", Slug: "forcefloor", Status: "running"}}
	m.selected = 0

	next, cmd := m.enterInteractiveBody(true)
	got := next.(Model)
	if got.interactive {
		t.Fatalf("F entered interactive mode below the row floor")
	}
	if cmd != nil {
		t.Fatalf("F returned a non-nil cmd on the floor refusal, want nil")
	}
	if !strings.Contains(got.attachError, "7-row floor") {
		t.Fatalf("attachError = %q, want the 7-row floor refusal", got.attachError)
	}
}

// TestFKeyRoutesToForceEnterInteractiveBody proves the actual keymap
// rebind (task 105): pressing `F` in list mode calls
// enterInteractiveBody(true), not enterInteractive/attachSelected, and
// degrades to a no-op exactly like `\u21b5`/`a` do with a zero tmux.Client.
func TestFKeyRoutesToForceEnterInteractiveBody(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	m.sessions = []store.Session{{ID: "s1", Name: "one", Agent: "shell", Status: "running", Slug: "one"}}
	m.selected = 0
	next, cmd := m.Update(tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune("F")}))
	got := next.(Model)
	if got.interactive {
		t.Fatalf("F entered interactive mode with a zero tmux.Client")
	}
	if cmd != nil {
		t.Fatalf("F returned a non-nil cmd with a zero tmux.Client")
	}
}
