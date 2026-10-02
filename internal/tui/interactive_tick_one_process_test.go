package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// tickShim wraps the real tmux binary in a script that logs one line per
// process spawned, so a test can count what an interactive preview tick
// really costs. It returns the shim's path and its log's path.
func tickShim(t *testing.T) (binary, logPath string) {
	t.Helper()
	realTmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatalf("look up the real tmux binary: %v", err)
	}
	dir := t.TempDir()
	logPath = filepath.Join(dir, "invocations")
	binary = filepath.Join(dir, "tmux")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"" + logPath + "\"\nexec " + realTmux + " \"$@\"\n"
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return binary, logPath
}

func tickShimLines(t *testing.T, logPath string) []string {
	t.Helper()
	data, err := os.ReadFile(logPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	trimmed := strings.TrimRight(string(data), "\n")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}

// interactiveTickModel enters interactive mode (default pipe transport)
// against a quiet pane on a private socket, with every tmux process this
// model spawns going through the counting shim.
func interactiveTickModel(t *testing.T, name string) (Model, tmux.Client, string, string) {
	t.Helper()
	socket := selectionTestSocket(name)
	newQuietSelectionPane(t, socket, "deck_"+name, 80, 24)
	binary, logPath := tickShim(t)
	client := tmux.Client{Socket: socket, Binary: binary}

	m := New(nil, config.Settings{Color: true}, "")
	m.width, m.height = 100, 30
	m.tmuxClient = client
	m.sessions = []store.Session{{ID: "sess-" + name, Name: name, Slug: name, Status: "waiting"}}
	m.selected = rowCursor(0)

	next, _ := m.enterInteractiveBody(false)
	got := next.(Model)
	if !got.interactive {
		t.Fatalf("entry did not enter interactive mode: attachError=%q", got.attachError)
	}
	t.Cleanup(func() { got.exitInteractive() })
	return got, client, socket, logPath
}

// runTick delivers one previewTick and runs every command it returns,
// feeding any interactiveDisplacementChecked back into Update, as the
// bubbletea runtime would. It returns the model after both steps.
func runTick(t *testing.T, m Model) Model {
	t.Helper()
	updated, cmd := m.Update(previewTick(time.Now()))
	m = updated.(Model)
	var msgs []tea.Msg
	var collect func(c tea.Cmd)
	collect = func(c tea.Cmd) {
		if c == nil {
			return
		}
		msg := c()
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, member := range batch {
				collect(member)
			}
			return
		}
		msgs = append(msgs, msg)
	}
	collect(cmd)
	for _, msg := range msgs {
		if checked, ok := msg.(interactiveDisplacementChecked); ok {
			updated, _ := m.Update(checked)
			m = updated.(Model)
		}
	}
	return m
}

// TestInteractivePreviewTickIsOneTmuxProcess pins R185 (GH #49): in
// interactive mode on the pipe transport one preview tick costs exactly one
// tmux process, a display-message, and no capture-pane, and nothing else
// spawns tmux between ticks (the 200 ms pane_dead loop is gone).
func TestInteractivePreviewTickIsOneTmuxProcess(t *testing.T) {
	m, _, _, logPath := interactiveTickModel(t, "tickone")

	// Between ticks nothing of its own spawns tmux: no poll loop.
	before := len(tickShimLines(t, logPath))
	time.Sleep(600 * time.Millisecond)
	if idle := tickShimLines(t, logPath)[before:]; len(idle) != 0 {
		t.Fatalf("tmux spawned %d process(es) with no preview tick running: %q", len(idle), idle)
	}

	before = len(tickShimLines(t, logPath))
	runTick(t, m)
	spawned := tickShimLines(t, logPath)[before:]
	if len(spawned) != 1 {
		t.Fatalf("one interactive preview tick spawned %d tmux processes, want exactly 1: %q", len(spawned), spawned)
	}
	line := spawned[0]
	if !strings.Contains(line, "display-message") {
		t.Fatalf("the tick's one process is %q, want a display-message", line)
	}
	for _, want := range []string{"#{pane_dead}", "#{session_attached}", "#{@deck_isize_owner}"} {
		if !strings.Contains(line, want) {
			t.Fatalf("the tick's display-message %q does not read %s", line, want)
		}
	}
	if strings.Contains(line, "capture-pane") {
		t.Fatalf("the interactive tick ran a capture-pane: %q", line)
	}
}
