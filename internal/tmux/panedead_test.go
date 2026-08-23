// panedead_test.go proves PRD phase3b II-23's raw material at the tmux
// layer: `#{pane_dead}` reads false against a live pane and true once its
// process has exited under `remain-on-exit failed`, and (the load-bearing
// fact the whole task rests on) `#{pane_pipe}` stays 1 -- a `pipe-pane`
// reader attached before the process died never sees an EOF because of
// it. internal/interactive's own tests build the Session-level polling
// and death-detection proof on top of this.
package tmux

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"
)

func paneDeadSocket(name string) string {
	return fmt.Sprintf("deck-panedead-%s-%d-%d", name, os.Getpid(), time.Now().UnixNano())
}

// killPaneProcessUnderRemainOnExitFailed sets `remain-on-exit failed` on
// target's window and then makes target's own shell exit 1 (rather than
// running a doomed subcommand): that is what actually makes tmux mark the
// PANE itself dead rather than merely reporting a foreground command's
// exit status while the shell is still alive to print another prompt.
func killPaneProcessUnderRemainOnExitFailed(t *testing.T, socket, target string) {
	t.Helper()
	runTmux(t, socket, "set-window-option", "-t", target, "remain-on-exit", "failed")
	runTmux(t, socket, "send-keys", "-t", target, "-l", "--", "exit 1")
	runTmux(t, socket, "send-keys", "-t", target, "Enter")
}

// waitForPaneDeadTest polls PaneDead until it reports dead or the timeout
// passes, failing the test if it never does -- tmux marking a pane dead
// after its process exits is not instantaneous.
func waitForPaneDeadTest(t *testing.T, client Client, target string, timeout time.Duration) {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(timeout)
	for {
		dead, err := client.PaneDead(ctx, target)
		if err != nil {
			t.Fatalf("PaneDead: %v", err)
		}
		if dead {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("pane %s never reported pane_dead within %s", target, timeout)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestPaneDeadReadsFalseThenTrueAfterProcessExits is PaneDead's basic
// correctness proof: false against a live pane, true once the pane's own
// process has exited under remain-on-exit=failed, matching tmux's own
// #{pane_dead} directly (no reliance on internal/interactive's polling,
// which is proved separately).
func TestPaneDeadReadsFalseThenTrueAfterProcessExits(t *testing.T) {
	socket := paneDeadSocket("basic")
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 24)
	defer cleanup()
	client := Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	dead, err := client.PaneDead(ctx, "s0")
	if err != nil {
		t.Fatalf("PaneDead before exit: %v", err)
	}
	if dead {
		t.Fatalf("PaneDead reported true for a freshly created, still-live pane")
	}

	killPaneProcessUnderRemainOnExitFailed(t, socket, "s0")
	waitForPaneDeadTest(t, client, "s0", 5*time.Second)
}

// TestPaneDeadRejectsAnAbsentTarget proves PaneDead surfaces tmux's own
// error rather than silently reporting a nonexistent pane as either dead
// or alive -- interactive.pollPaneDead (task 045/II-23) treats this error
// return itself as terminal for exactly this reason: a target that no
// longer resolves is at least as final as pane_dead==1.
func TestPaneDeadRejectsAnAbsentTarget(t *testing.T) {
	socket := paneDeadSocket("absent")
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 24)
	defer cleanup()
	client := Client{Socket: socket, Timeout: 5 * time.Second}

	if _, err := client.PaneDead(context.Background(), "%999999"); err == nil {
		t.Fatalf("PaneDead succeeded against a pane id that was never created")
	}
}

// TestPanePipeNeverClosesOnADeadPaneUnderRemainOnExitFailed is II-23's raw
// red control, at the tmux layer, independent of anything Go-side: a
// `pipe-pane -IO` reader armed on a live pane observes NO EOF within a
// bounded window after the pane's own process exits under
// `remain-on-exit failed`, even though tmux itself has already marked
// the pane dead (#{pane_dead}==1) and #{pane_pipe} is still 1 -- the pipe
// itself carries no liveness signal of its own. This is the fact
// interactive.pollPaneDead's existence rests on; if tmux ever changed
// this behaviour, this test would go red and the polling mechanism would
// no longer be motivated (it would still be harmless, just unnecessary).
func TestPanePipeNeverClosesOnADeadPaneUnderRemainOnExitFailed(t *testing.T) {
	socket := paneDeadSocket("pipe-red-control")
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 24)
	defer cleanup()
	client := Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	pipe, err := client.ArmPipePane(ctx, "s0")
	if err != nil {
		t.Fatalf("ArmPipePane: %v", err)
	}
	defer pipe.Close()

	killPaneProcessUnderRemainOnExitFailed(t, socket, "s0")
	waitForPaneDeadTest(t, client, "s0", 5*time.Second)

	// PaneDead is true; #{pane_pipe} must still read 1 -- this is what
	// distinguishes "died with the pipe still armed" from displacement
	// (task 046/II-24), which this test does not exercise.
	panePipe := runTmux(t, socket, "display-message", "-p", "-t", "s0", "#{pane_pipe}")
	if pipeFlag, convErr := strconv.Atoi(panePipe); convErr != nil || pipeFlag != 1 {
		t.Fatalf("test assumption violated: #{pane_pipe} = %q after the pane died, want 1 (pipe still armed)", panePipe)
	}

	// Drain whatever tmux had already queued (the echoed "exit 1\r\n"
	// and its own "Pane is dead..." message) in a loop -- a Read
	// returning data with a nil error is a perfectly normal live read,
	// not evidence of anything closing. The proof this control needs is
	// that no read EVER returns a non-nil error (EOF or otherwise)
	// within the bounded window, once there is nothing left queued and
	// the reader is genuinely parked on read(2) with a dead pane on the
	// other end.
	readErrCh := make(chan error, 1)
	go func() {
		buf := make([]byte, 4096)
		for {
			_, err := pipe.Read(buf)
			if err != nil {
				readErrCh <- err
				return
			}
		}
	}()

	select {
	case err := <-readErrCh:
		t.Fatalf("pipe.Read returned a terminal error (err=%v) after a dead pane under remain-on-exit=failed; the whole point of this control is that it must NOT -- EOF is not a substitute for polling pane_dead", err)
	case <-time.After(1500 * time.Millisecond):
		// Never returning an error within this bounded window IS the
		// proof: read(2) is genuinely blocked once the queue drains,
		// exactly as PRD II-23 states.
	}
}
