// panedead_test.go proves PRD phase3b II-23 at the Session/live-path
// level: a pane that dies under `remain-on-exit failed` never closes its
// pipe (internal/tmux's TestPanePipeNeverClosesOnADeadPaneUnderRemainOnExitFailed
// proves that raw fact directly against tmux), so without pane_dead
// polling drain would block on read(2) forever and the grid would render
// a stale frame with no way to notice. Session's pollPaneDead (grid.go)
// is what makes it notice instead.
package interactive

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/tmux"
)

// killPaneProcessUnderRemainOnExitFailed sets `remain-on-exit failed` on
// target's window, then makes target's own shell process exit 1 --
// exactly what makes tmux mark the PANE dead (not merely a foreground
// subcommand), mirroring internal/tmux's own
// killPaneProcessUnderRemainOnExitFailed helper (unexported there too, so
// duplicated rather than shared across package boundaries).
func killPaneProcessUnderRemainOnExitFailed(t *testing.T, socket, target string) {
	t.Helper()
	if out, err := exec.Command("tmux", "-L", socket, "set-window-option", "-t", target, "remain-on-exit", "failed").CombinedOutput(); err != nil {
		t.Fatalf("set-window-option remain-on-exit failed: %v: %s", err, out)
	}
	sendLiteralLine(t, socket, target, "exit 1")
}

// TestSessionNoticesAndClosesDownOnADeadPaneUnderRemainOnExitFailed is
// II-23's positive case: with pollPaneDead running, a Session notices a
// dead target well within a bounded window (nowhere near "forever") and
// reports it through Dead(), and Close() returns promptly afterward --
// proving drain is not left stuck on the read(2) that would otherwise
// never return (per the sibling red control in internal/tmux).
func TestSessionNoticesAndClosesDownOnADeadPaneUnderRemainOnExitFailed(t *testing.T) {
	original := paneDeadPollInterval
	paneDeadPollInterval = 30 * time.Millisecond
	defer func() { paneDeadPollInterval = original }()

	socket := interactiveSocket("dead-pane")
	cleanup := newBareInteractiveSession(t, socket, "s0", 40, 10)
	defer cleanup()
	client := tmux.Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	session, err := Start(ctx, client, "s0", 40, 10, func(ctx context.Context) ([]byte, error) {
		return rawCapturePane(t, socket, "s0"), nil
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer session.Close()

	killPaneProcessUnderRemainOnExitFailed(t, socket, "s0")

	select {
	case <-session.Dead():
		// noticed -- this is the whole point of the poll.
	case <-time.After(2 * time.Second):
		t.Fatalf("Session never reported Dead() within 2s of the pane dying under remain-on-exit=failed; pane_dead polling did not notice")
	}

	// Close must return promptly: markDead already closed the pipe, so
	// drain (and pollPaneDead itself) must already have unwound. If
	// pane_dead polling were removed, this Close would instead block on
	// drain's own read(2), which the sibling tmux-layer red control
	// proves never returns on its own for a pane in this state.
	closeDone := make(chan struct{})
	go func() {
		_ = session.Close()
		close(closeDone)
	}()
	select {
	case <-closeDone:
	case <-time.After(2 * time.Second):
		t.Fatalf("Session.Close() did not return within 2s after Dead() fired; drain appears stuck on read(2) despite markDead having run")
	}
}

// TestSessionDeadChannelNeverClosesWithoutAnyDeathAndAPollTick is a sanity
// control for the positive case above: against a pane that stays alive,
// Dead() must NOT fire merely because time -- and several poll ticks --
// passed. Without this, a bug that closed deadCh unconditionally on
// every tick would make the positive test above pass for the wrong
// reason.
func TestSessionDeadChannelNeverClosesWithoutAnyDeathAndAPollTick(t *testing.T) {
	original := paneDeadPollInterval
	paneDeadPollInterval = 30 * time.Millisecond
	defer func() { paneDeadPollInterval = original }()

	socket := interactiveSocket("alive-pane")
	cleanup := newBareInteractiveSession(t, socket, "s0", 40, 10)
	defer cleanup()
	client := tmux.Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	session, err := Start(ctx, client, "s0", 40, 10, func(ctx context.Context) ([]byte, error) {
		return rawCapturePane(t, socket, "s0"), nil
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer session.Close()

	select {
	case <-session.Dead():
		t.Fatalf("Dead() fired against a pane that never died")
	case <-time.After(300 * time.Millisecond):
		// several poll ticks have elapsed (interval is 30ms); no death
		// reported, as expected.
	}
}
