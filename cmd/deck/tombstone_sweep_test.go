package main

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// TestAbandonedDDIsReapedAtNextStoreOpen is task 011's proof that task
// 010's store-open sweep (cmd/deck/main.go, wired right beside R62's own
// EnforceEventRetention call) is the ONLY thing that ever reaps a row `dd`
// tombstoned and then abandoned -- the owning deck process quit (or was
// killed, or crashed) before its own tea.Tick deleteGraceExpired could
// fire, so nothing in-process is left to call sessions.Reap for it.
//
// Run 1 creates a session, dd's it, and quits well inside
// DECK_DELETE_GRACE_MS's real wall-clock window -- long enough that the
// handful of PTY round trips this needs can never approach it, so the
// grace tick never has a chance to fire. That leaves the row exactly as an
// abandoned dd leaves it: tombstoned, with its own create/status events
// and its name still on record, checked directly against the store before
// run 2 ever starts (belt-and-suspenders: proves the premise, not merely
// asserted).
//
// Run 2 opens the SAME store with its DECK_CLOCK moved more than an hour
// past run 1 -- past both DeleteGrace and the sweep's own hourly throttle
// (which run 1's own store-open call already stamped). Task 010's call
// site must reap the row before the model even exists, well before this
// run's first frame: there is no keypress, no tea.Tick and no reconcile
// tick on this run's own critical path that could otherwise explain a
// reap, so this checks the row, its events and its name directly against
// the store rather than inferring anything from the sidebar (an
// already-tombstoned row is invisible there either way). Removing task
// 010's call site from main.go leaves the row, its events and the name
// reservation exactly as run 1 left them, so this test goes red without
// it.
//
// Run 3 then spends the freed name through the ordinary create route,
// proving it is genuinely reusable and not merely absent from the
// tombstone list.
func TestAbandonedDDIsReapedAtNextStoreOpen(t *testing.T) {
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
	socket := "deck-sweep-pty-" + strings.ReplaceAll(filepath.Base(home), "_", "")
	defer exec.Command("tmux", "-L", socket, "kill-server").Run()

	const sessionName = "abandoned-dd"
	const firstClock = "2025-06-01T00:00:00Z"
	// Two hours later clears both DeleteGrace below and the sweep's own
	// hourly throttle, which run 1's own store-open call already stamped
	// at firstClock.
	const secondClock = "2025-06-01T02:00:00Z"
	// Real wall-clock milliseconds: long enough that run 1's own handful of
	// PTY round trips (create, dd, dd, submit, quit) can never take this
	// long, so deleteGraceExpired's tea.Tick never fires before it exits.
	const deleteGraceMS = "60000"

	runEnv := func(clock string) []string {
		return append(os.Environ(),
			"DECK_HOME="+home, "DECK_TMUX_SOCKET="+socket, "DECK_RECONCILE_MS=5000",
			"NO_COLOR=1", "DECK_ASCII=1", "DECK_ANIM=0", "TERM=xterm-256color", "SHELL=/bin/sh",
			"DECK_DELETE_GRACE_MS="+deleteGraceMS, "DECK_CLOCK="+clock)
	}
	paths := config.Paths{Home: home, LogDir: filepath.Join(home, "log"), StateDB: filepath.Join(home, "state.db")}

	// --- run 1: create the session, dd it, quit before any grace-window
	// tick could possibly fire. ---
	ctx1, cancel1 := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel1()
	cmd1 := exec.CommandContext(ctx1, binary)
	cmd1.Env = runEnv(firstClock)
	terminal1, err := pty.StartWithSize(cmd1, &pty.Winsize{Rows: 24, Cols: 100})
	if err != nil {
		t.Fatal(err)
	}
	defer terminal1.Close()
	output1 := newPTYOutput()
	go io.Copy(output1, terminal1)
	done1 := make(chan error, 1)
	go func() { done1 <- cmd1.Wait() }()

	waitForScreen(t, output1, done1, "\x1b[6n")
	if _, err := terminal1.Write([]byte("\x1b]11;rgb:0000/0000/0000\x1b\\\x1b[1;1R")); err != nil {
		t.Fatal(err)
	}
	waitForScreen(t, output1, done1, "No sessions yet")
	if _, err := terminal1.Write([]byte("n")); err != nil {
		t.Fatal(err)
	}
	waitForScreen(t, output1, done1, "Create shell session")
	if _, err := terminal1.Write([]byte(sessionName + "\t" + cwd + "\r")); err != nil {
		t.Fatal(err)
	}
	waitForScreen(t, output1, done1, "starting")
	if _, err := terminal1.Write([]byte("d")); err != nil {
		t.Fatal(err)
	}
	waitForScreen(t, output1, done1, "press d again to confirm")
	if _, err := terminal1.Write([]byte("d")); err != nil {
		t.Fatal(err)
	}
	waitForScreen(t, output1, done1, "Enter deletes")
	if _, err := terminal1.Write([]byte("\r")); err != nil {
		t.Fatal(err)
	}
	waitForScreen(t, output1, done1, "No sessions yet")
	// A short settle delay before quitting: the confirm dialog's own submit
	// ("\r") and the very next keypress can otherwise land in the same PTY
	// read as one burst (see clientPressesDD's own comment on this driver's
	// ANSI decoder), which has been observed to swallow the quit keypress
	// entirely -- unlike the render wait above, only real elapsed time
	// guarantees the two reads are separate.
	time.Sleep(300 * time.Millisecond)
	if _, err := terminal1.Write([]byte("q")); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done1:
		if err != nil {
			t.Fatalf("run 1 did not quit cleanly: %v\noutput: %q", err, output1.String())
		}
	case <-ctx1.Done():
		t.Fatalf("run 1 did not quit: %v\noutput: %q", ctx1.Err(), output1.String())
	}

	verify1, err := store.Open(paths)
	if err != nil {
		t.Fatal(err)
	}
	deleted, err := verify1.ListDeletedSessions(context.Background())
	if err != nil {
		verify1.Close()
		t.Fatal(err)
	}
	if len(deleted) != 1 || deleted[0].Name != sessionName || deleted[0].DeletedAt == 0 {
		verify1.Close()
		t.Fatalf("tombstoned rows after run 1 = %#v, want exactly one tombstoned %q", deleted, sessionName)
	}
	sessionID := deleted[0].ID
	var eventsAfterRun1 int
	if err := verify1.DB().QueryRow(`SELECT COUNT(*) FROM events WHERE session_id = ?`, sessionID).Scan(&eventsAfterRun1); err != nil {
		verify1.Close()
		t.Fatal(err)
	}
	if eventsAfterRun1 == 0 {
		verify1.Close()
		t.Fatal("tombstoned session has no events of its own after run 1, want its create/status history still on record before any sweep has a chance to run")
	}
	verify1.Close()

	// --- run 2: open the SAME store more than an hour later, past
	// DeleteGrace and the sweep's own hourly throttle. ---
	ctx2, cancel2 := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel2()
	cmd2 := exec.CommandContext(ctx2, binary)
	cmd2.Env = runEnv(secondClock)
	terminal2, err := pty.StartWithSize(cmd2, &pty.Winsize{Rows: 24, Cols: 100})
	if err != nil {
		t.Fatal(err)
	}
	defer terminal2.Close()
	output2 := newPTYOutput()
	go io.Copy(output2, terminal2)
	done2 := make(chan error, 1)
	go func() { done2 <- cmd2.Wait() }()

	waitForScreen(t, output2, done2, "\x1b[6n")
	if _, err := terminal2.Write([]byte("\x1b]11;rgb:0000/0000/0000\x1b\\\x1b[1;1R")); err != nil {
		t.Fatal(err)
	}
	waitForScreen(t, output2, done2, "No sessions yet")
	if _, err := terminal2.Write([]byte("q")); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done2:
		if err != nil {
			t.Fatalf("run 2 did not quit cleanly: %v\noutput: %q", err, output2.String())
		}
	case <-ctx2.Done():
		t.Fatalf("run 2 did not quit: %v\noutput: %q", ctx2.Err(), output2.String())
	}

	verify2, err := store.Open(paths)
	if err != nil {
		t.Fatal(err)
	}
	exists, err := verify2.SessionRowExists(context.Background(), sessionID)
	if err != nil {
		verify2.Close()
		t.Fatal(err)
	}
	if exists {
		verify2.Close()
		t.Fatalf("session %q row still exists after run 2's store open, want the abandoned tombstone reaped by task 010's call site", sessionID)
	}
	var eventsAfterRun2 int
	if err := verify2.DB().QueryRow(`SELECT COUNT(*) FROM events WHERE session_id = ?`, sessionID).Scan(&eventsAfterRun2); err != nil {
		verify2.Close()
		t.Fatal(err)
	}
	if eventsAfterRun2 != 0 {
		verify2.Close()
		t.Fatalf("events for reaped session %q = %d, want 0 (reapSessionTx's cascade delete)", sessionID, eventsAfterRun2)
	}
	holders, err := verify2.TombstonedNameHolders(context.Background(), sessionName)
	if err != nil {
		verify2.Close()
		t.Fatal(err)
	}
	if len(holders) != 0 {
		verify2.Close()
		t.Fatalf("tombstoned holders of %q after run 2 = %v, want none -- the name must be free", sessionName, holders)
	}
	verify2.Close()

	// --- run 3: the name is not merely absent from the tombstone list --
	// it must be genuinely reusable, through the same real deck binary and
	// the same create route a user would take, with no "collides" refusal.
	ctx3, cancel3 := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel3()
	cmd3 := exec.CommandContext(ctx3, binary)
	cmd3.Env = runEnv(secondClock)
	terminal3, err := pty.StartWithSize(cmd3, &pty.Winsize{Rows: 24, Cols: 100})
	if err != nil {
		t.Fatal(err)
	}
	defer terminal3.Close()
	output3 := newPTYOutput()
	go io.Copy(output3, terminal3)
	done3 := make(chan error, 1)
	go func() { done3 <- cmd3.Wait() }()

	waitForScreen(t, output3, done3, "\x1b[6n")
	if _, err := terminal3.Write([]byte("\x1b]11;rgb:0000/0000/0000\x1b\\\x1b[1;1R")); err != nil {
		t.Fatal(err)
	}
	waitForScreen(t, output3, done3, "No sessions yet")
	if _, err := terminal3.Write([]byte("n")); err != nil {
		t.Fatal(err)
	}
	waitForScreen(t, output3, done3, "Create shell session")
	if _, err := terminal3.Write([]byte(sessionName + "\t" + cwd + "\r")); err != nil {
		t.Fatal(err)
	}
	waitForScreen(t, output3, done3, "starting")
	if strings.Contains(output3.String(), "collides") {
		t.Fatalf("reusing the reaped name %q was refused: %q", sessionName, output3.String())
	}
	if _, err := terminal3.Write([]byte("q")); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done3:
		if err != nil {
			t.Fatalf("run 3 did not quit cleanly: %v\noutput: %q", err, output3.String())
		}
	case <-ctx3.Done():
		t.Fatalf("run 3 did not quit: %v\noutput: %q", ctx3.Err(), output3.String())
	}
}
