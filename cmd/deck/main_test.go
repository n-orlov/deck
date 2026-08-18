package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

type ptyOutput struct {
	mu sync.Mutex
	b  bytes.Buffer
	ch chan struct{}
}

func newPTYOutput() *ptyOutput { return &ptyOutput{ch: make(chan struct{}, 1)} }
func (o *ptyOutput) Write(p []byte) (int, error) {
	o.mu.Lock()
	n, err := o.b.Write(p)
	o.mu.Unlock()
	select {
	case o.ch <- struct{}{}:
	default:
	}
	return n, err
}
func (o *ptyOutput) String() string { o.mu.Lock(); defer o.mu.Unlock(); return o.b.String() }

func waitForScreen(t *testing.T, output *ptyOutput, done <-chan error, want string) {
	waitForScreenWithin(t, output, done, want, 8*time.Second)
}

func waitForScreenWithin(t *testing.T, output *ptyOutput, done <-chan error, want string, timeout time.Duration) {
	t.Helper()
	waitForScreensWithin(t, []screenWait{{output: output, done: done, want: want}}, timeout)
}

type screenWait struct {
	output *ptyOutput
	done   <-chan error
	want   string
}

// waitForScreensWithin uses one deadline for every client. In particular, it
// must not give each client a full interval in sequence: that would allow the
// final client to be multiple configured refresh intervals behind.
func waitForScreensWithin(t *testing.T, waits []screenWait, timeout time.Duration) {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	poll := time.NewTicker(5 * time.Millisecond)
	defer poll.Stop()
	for {
		allFound := true
		for _, wait := range waits {
			if strings.Contains(wait.output.String(), wait.want) {
				continue
			}
			allFound = false
			select {
			case err := <-wait.done:
				t.Fatalf("deck exited before rendering %q: %v\noutput: %q", wait.want, err, wait.output.String())
			default:
			}
		}
		if allFound {
			return
		}
		select {
		case <-poll.C:
		case <-deadline.C:
			for _, wait := range waits {
				if !strings.Contains(wait.output.String(), wait.want) {
					t.Fatalf("timed out waiting for %q within %s\noutput: %q", wait.want, timeout, wait.output.String())
				}
			}
		}
	}
}

func TestDeckBinaryShellCreateAndSlugCollisionThroughPTY(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "deck")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build deck binary: %v: %s", err, output)
	}

	home, cwd := t.TempDir(), t.TempDir()
	sentinel := filepath.Join(cwd, "deck-kill-sentinel")
	sentinelContents := []byte("cwd remains user-owned\n")
	if err := os.WriteFile(sentinel, sentinelContents, 0o600); err != nil {
		t.Fatal(err)
	}
	socket := "deck-create-pty-" + strings.ReplaceAll(filepath.Base(home), "_", "")
	defer exec.Command("tmux", "-L", socket, "kill-server").Run()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary)
	cmd.Env = append(os.Environ(), "DECK_HOME="+home, "DECK_TMUX_SOCKET="+socket,
		"DECK_RECONCILE_MS=100", "NO_COLOR=1", "DECK_ASCII=1", "DECK_ANIM=0", "TERM=xterm-256color", "SHELL=/bin/sh",
		"TMUX=/nested,123,0") // successful private attachment proves the subprocess clears TMUX
	terminal, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 24, Cols: 100})
	if err != nil {
		t.Fatal(err)
	}
	defer terminal.Close()
	output := newPTYOutput()
	go io.Copy(output, terminal)
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	waitForScreen(t, output, done, "\x1b[6n")
	if _, err := terminal.Write([]byte("\x1b]11;rgb:0000/0000/0000\x1b\\\x1b[1;1R")); err != nil {
		t.Fatal(err)
	}
	waitForScreen(t, output, done, "No sessions yet")
	if _, err := terminal.Write([]byte("n")); err != nil {
		t.Fatal(err)
	}
	waitForScreen(t, output, done, "Create shell session")
	if _, err := terminal.Write([]byte("first session")); err != nil {
		t.Fatal(err)
	}
	if _, err := terminal.Write([]byte("\t" + cwd + "\r")); err != nil {
		t.Fatal(err)
	}
	waitForScreen(t, output, done, "starting")
	// Enter hands the actual terminal to the selected private tmux pane. The
	// pane command proves that this is a real attachment, not merely a UI state
	// change; Ctrl-B d returns control to Bubble Tea so it can redraw and accept
	// the following new-session command.
	if _, err := terminal.Write([]byte("\r")); err != nil {
		t.Fatal(err)
	}
	waitForScreen(t, output, done, "$ ")
	if _, err := terminal.Write([]byte("echo attached-private-pane\r")); err != nil {
		t.Fatal(err)
	}
	waitForScreen(t, output, done, "attached-private-pane")
	// Let tmux return to input mode after rendering the command output before
	// delivering its prefix sequence.
	time.Sleep(150 * time.Millisecond)
	if _, err := terminal.Write([]byte("\x02d")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(250 * time.Millisecond)
	if _, err := terminal.Write([]byte("n")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	if _, err := terminal.Write([]byte("first-session\t" + cwd + "\r")); err != nil {
		t.Fatal(err)
	}
	waitForScreen(t, output, done, "collides with existing slug")
	if _, err := terminal.Write([]byte("\x1b")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	if _, err := terminal.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	waitForScreen(t, output, done, "resumable")
	if _, err := terminal.Write([]byte("q")); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("deck did not quit cleanly: %v\\noutput: %q", err, output.String())
		}
	case <-ctx.Done():
		t.Fatalf("deck did not quit: %v\\noutput: %q", ctx.Err(), output.String())
	}
	if err := exec.Command("tmux", "-L", socket, "has-session", "-t", "deck_first-session").Run(); err == nil {
		t.Fatal("private tmux session survived x")
	}
	db, err := store.Open(config.Paths{Home: home, LogDir: filepath.Join(home, "log"), StateDB: filepath.Join(home, "state.db")})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := db.ListSessions(context.Background())
	db.Close()
	if err != nil || len(rows) != 1 || rows[0].Status != "stopped" {
		t.Fatalf("rows after x = %#v, %v", rows, err)
	}
	gotSentinel, err := os.ReadFile(sentinel)
	if err != nil || !bytes.Equal(gotSentinel, sentinelContents) {
		t.Fatalf("sentinel after x = %q, %v", gotSentinel, err)
	}
	log, err := os.ReadFile(filepath.Join(home, "log", "deck.jsonl"))
	if err != nil || !strings.Contains(string(log), `"event":"killed"`) {
		t.Fatalf("kill transition audit = %q, %v", log, err)
	}
}

type deckPTYClient struct {
	t        *testing.T
	terminal *os.File
	output   *ptyOutput
	done     <-chan error
}

func startDeckPTYClient(t *testing.T, binary, home, socket string, reconcile time.Duration) *deckPTYClient {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	cmd := exec.CommandContext(ctx, binary)
	cmd.Env = append(os.Environ(), "DECK_HOME="+home, "DECK_TMUX_SOCKET="+socket,
		"DECK_RECONCILE_MS="+fmt.Sprint(reconcile.Milliseconds()), "NO_COLOR=1", "DECK_ASCII=1", "DECK_ANIM=0", "TERM=xterm-256color", "SHELL=/bin/sh")
	terminal, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 24, Cols: 100})
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	output := newPTYOutput()
	go io.Copy(output, terminal)
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
		cancel()
	}()
	client := &deckPTYClient{t: t, terminal: terminal, output: output, done: done}
	waitForScreen(t, output, done, "\x1b[6n")
	if _, err := terminal.Write([]byte("\x1b]11;rgb:0000/0000/0000\x1b\\\x1b[1;1R")); err != nil {
		t.Fatal(err)
	}
	waitForScreen(t, output, done, "No sessions yet")
	return client
}

func (c *deckPTYClient) send(input string) {
	c.t.Helper()
	if _, err := c.terminal.Write([]byte(input)); err != nil {
		c.t.Fatal(err)
	}
}

func (c *deckPTYClient) close() {
	c.t.Helper()
	c.send("q")
	select {
	case err := <-c.done:
		if err != nil {
			c.t.Errorf("deck did not quit cleanly: %v\\noutput: %q", err, c.output.String())
		}
	case <-time.After(5 * time.Second):
		c.t.Errorf("deck did not quit\\noutput: %q", c.output.String())
	}
	_ = c.terminal.Close()
}

func TestDeckBinaryRefreshesAllConcurrentClients(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "deck")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build deck binary: %v: %s", err, output)
	}

	home, cwd := t.TempDir(), t.TempDir()
	socket := "deck-multiclient-" + strings.ReplaceAll(filepath.Base(home), "_", "")
	defer exec.Command("tmux", "-L", socket, "kill-server").Run()
	// A full second leaves enough room for a real tmux/SQLite integration run
	// while remaining the literal configured deadline asserted below.
	interval := time.Second
	first := startDeckPTYClient(t, binary, home, socket, interval)
	second := startDeckPTYClient(t, binary, home, socket, interval)
	third := startDeckPTYClient(t, binary, home, socket, interval)
	defer third.close()
	defer second.close()
	defer first.close()

	// Create through one real TUI. The other processes have independent SQLite
	// connections and must learn about the committed row from their refresh tick.
	first.send("n")
	waitForScreen(t, first.output, first.done, "Create shell session")
	// Begin the shared deadline at the mutation itself, then require every
	// surviving client—including the creating client—to render it before that
	// one configured refresh interval expires.
	first.send("shared session\t" + cwd + "\r")
	waitForScreensWithin(t, []screenWait{
		{output: first.output, done: first.done, want: "shared session"},
		{output: second.output, done: second.done, want: "shared session"},
		{output: third.output, done: third.done, want: "shared session"},
	}, interval)

	// Kill through a different TUI and apply the same single deadline to all
	// surviving clients, including the client that performed the kill.
	second.send("x")
	waitForScreensWithin(t, []screenWait{
		{output: first.output, done: first.done, want: "resumable"},
		{output: second.output, done: second.done, want: "resumable"},
		{output: third.output, done: third.done, want: "resumable"},
	}, interval)
}

func TestDeckBinaryEmptyHelpAndQuitThroughPTY(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "deck")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build deck binary: %v: %s", err, output)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary)
	cmd.Env = append(os.Environ(), "DECK_HOME="+t.TempDir(), "DECK_TMUX_SOCKET=deck-tui-pty", "DECK_RECONCILE_MS=100", "NO_COLOR=1", "DECK_ASCII=1", "DECK_ANIM=0", "TERM=xterm-256color")
	terminal, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 24, Cols: 100})
	if err != nil {
		t.Fatal(err)
	}
	defer terminal.Close()
	output := newPTYOutput()
	go io.Copy(output, terminal)
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	// Bubble Tea asks a real terminal for OSC 11 and cursor position before
	// rendering. A PTY transports those bytes but does not answer them.
	waitForScreen(t, output, done, "\x1b[6n")
	if _, err := terminal.Write([]byte("\x1b]11;rgb:0000/0000/0000\x1b\\\x1b[1;1R")); err != nil {
		t.Fatal(err)
	}
	waitForScreen(t, output, done, "No sessions yet")
	if _, err := terminal.Write([]byte("?")); err != nil {
		t.Fatal(err)
	}
	waitForScreen(t, output, done, "DECK_TMUX_SOCKET")
	// The released PTY shows the actionable footer before help opens; the
	// companion lifecycle PTY test exercises n, Enter, attachment, and x.
	// Help itself must never advertise a later-phase command.
	help := output.String()
	if !strings.Contains(help, "up/down select - Enter attach - n new - x kill - ? help - q quit") {
		t.Errorf("released footer does not list the implemented action map:\n%s", help)
	}
	for _, unavailable := range []string{"resume/start", "restart preserving", "send message", "env editor", "permission profile", "event log", "filter list", "snooze", "archive", "undo"} {
		if strings.Contains(help, unavailable) {
			t.Errorf("released help advertises unavailable action %q:\n%s", unavailable, help)
		}
	}
	if _, err := terminal.Write([]byte("?")); err != nil {
		t.Fatal(err)
	}
	// The previous frame still contains the empty-state text, so allow the
	// program to process the key before opening help again for the Esc path.
	time.Sleep(100 * time.Millisecond)
	if _, err := terminal.Write([]byte("?")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	if _, err := terminal.Write([]byte("\x1b")); err != nil {
		t.Fatal(err)
	}
	// Escape is deliberately parsed after a short ambiguity timeout so it is
	// not confused with the start of an Alt key sequence.
	time.Sleep(100 * time.Millisecond)
	if _, err := terminal.Write([]byte("q")); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("deck did not quit cleanly: %v\noutput: %q", err, output.String())
		}
	case <-ctx.Done():
		t.Fatalf("deck did not quit: %v\noutput: %q", ctx.Err(), output.String())
	}
}
