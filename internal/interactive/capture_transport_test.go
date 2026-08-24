// capture_transport_test.go proves PRD phase3b II-5's second transport
// (task 070): a Session started with TransportCapture never arms
// pipe-pane at all, keeps its grid fed purely by polling capture-pane on
// capturePollInterval, still notices genuine pane content the same way a
// TransportPipe Session does, still reports pane death via Dead(), and
// still tears down cleanly through Close() -- with no pipe left armed
// behind it (confirmed directly against #{pane_pipe}).
package interactive

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/tmux"
)

// capturePaneID resolves target's real `#{pane_id}` (e.g. "%0"):
// captureLoop drives CaptureSeed (grid.go), and CaptureSeed's own
// CapturePaneSeedAtomic (internal/tmux/paneseed_atomic.go) rejects
// anything that is not that exact `%<digits>` shape, unlike ArmPipePane/
// capture-pane -p, which accept an ordinary session/window target string
// like "s0" just fine -- exactly the same pane-id-only contract
// Dispatcher enforces (PRD II-30) and exactly what production always
// passes (client.PreviewPane's own pane.ID), so this is the target every
// TransportCapture test in this file must use, not "s0" itself.
func capturePaneID(t *testing.T, socket, target string) string {
	t.Helper()
	out, err := exec.Command("tmux", "-L", socket, "display-message", "-p", "-t", target, "#{pane_id}").Output()
	if err != nil {
		t.Fatalf("display-message #{pane_id} -t %s: %v", target, err)
	}
	return strings.TrimSpace(string(out))
}

// startCaptureSession is this file's own StartWithTransport(...,
// TransportCapture) convenience wrapper, mirroring the seed closure every
// other test in this package builds inline. Unlike Start's own tests
// elsewhere in this package, target here must already be a resolved pane
// id (see capturePaneID) -- captureLoop's CaptureSeed calls require it.
func startCaptureSession(t *testing.T, client tmux.Client, socket, target string, width, height int) *Session {
	t.Helper()
	ctx := context.Background()
	session, err := StartWithTransport(ctx, client, target, width, height, func(ctx context.Context) ([]byte, error) {
		return rawCapturePane(t, socket, target), nil
	}, TransportCapture)
	if err != nil {
		t.Fatalf("StartWithTransport(..., TransportCapture): %v", err)
	}
	return session
}

// TestCaptureTransportNeverArmsPipePane is the raw, non-vacuous
// distinguishing property between the two transports: #{pane_pipe} stays
// 0 for the whole life of a TransportCapture Session, on the exact same
// pane a TransportPipe Session would have flipped it to 1 for (confirmed
// by the sibling assertion below).
func TestCaptureTransportNeverArmsPipePane(t *testing.T) {
	original := capturePollInterval
	capturePollInterval = 20 * time.Millisecond
	defer func() { capturePollInterval = original }()

	socket := interactiveSocket("capture-no-pipe")
	cleanup := newBareInteractiveSession(t, socket, "s0", 40, 10)
	defer cleanup()
	client := tmux.Client{Socket: socket, Timeout: 5 * time.Second}

	paneID := capturePaneID(t, socket, "s0")
	session := startCaptureSession(t, client, socket, paneID, 40, 10)
	defer session.Close()

	// Give captureLoop a few ticks to have actually run at least once,
	// so this is not merely "immediately after Start", before asserting
	// the negative.
	time.Sleep(5 * capturePollInterval)

	out, err := exec.Command("tmux", "-L", socket, "display-message", "-p", "-t", "s0", "#{pane_pipe}").Output()
	if err != nil {
		t.Fatalf("display-message #{pane_pipe}: %v", err)
	}
	if got := string(out); got != "0\n" {
		t.Fatalf("#{pane_pipe} = %q after a TransportCapture Session ran, want %q -- capture transport must never arm pipe-pane", got, "0\n")
	}
}

// TestCaptureTransportPicksUpNewContentByPolling is TransportCapture's
// own positive control: content typed into the pane AFTER Start (so it
// could not possibly be in the initial seed) is picked up by captureLoop's
// next poll, with no pipe involved at all.
func TestCaptureTransportPicksUpNewContentByPolling(t *testing.T) {
	original := capturePollInterval
	capturePollInterval = 20 * time.Millisecond
	defer func() { capturePollInterval = original }()

	socket := interactiveSocket("capture-polls")
	cleanup := newBareInteractiveSession(t, socket, "s0", 40, 10)
	defer cleanup()
	client := tmux.Client{Socket: socket, Timeout: 5 * time.Second}

	paneID := capturePaneID(t, socket, "s0")
	session := startCaptureSession(t, client, socket, paneID, 40, 10)
	defer session.Close()

	sendLiteralLine(t, socket, "s0", "CAPTURE-POLLED-CONTENT")

	if !waitFor(t, 3*time.Second, func() bool {
		return gridContains(session.Grid(), "CAPTURE-POLLED-CONTENT")
	}) {
		t.Fatalf("grid never showed content typed after Start -- captureLoop is not polling")
	}
}

// TestCaptureTransportStatusStaysLive confirms the Status doc's own
// claim: handlePipeGone (the only thing that ever moves Status off
// StatusLive) is a TransportPipe-only mechanism, so a capture Session's
// Status never moves, for its whole life, regardless of what happens to
// the target pane.
func TestCaptureTransportStatusStaysLive(t *testing.T) {
	original := capturePollInterval
	capturePollInterval = 20 * time.Millisecond
	defer func() { capturePollInterval = original }()

	socket := interactiveSocket("capture-status")
	cleanup := newBareInteractiveSession(t, socket, "s0", 40, 10)
	defer cleanup()
	client := tmux.Client{Socket: socket, Timeout: 5 * time.Second}

	paneID := capturePaneID(t, socket, "s0")
	session := startCaptureSession(t, client, socket, paneID, 40, 10)
	defer session.Close()

	time.Sleep(5 * capturePollInterval)
	if got := session.Status(); got != StatusLive {
		t.Fatalf("Status() = %v after ordinary capture polling, want StatusLive", got)
	}
}

// TestCaptureTransportNoticesDeadPaneAndClosesCleanly proves pollPaneDead
// (PRD II-23) is fully transport-agnostic: it runs identically under
// TransportCapture, and Close() tears the session down without blocking
// on the pipe-specific fields captureLoop never touches (s.pipe is nil
// for the whole life of this Session -- markDead's and Close's own nil
// guards are what this test would deadlock or panic without).
func TestCaptureTransportNoticesDeadPaneAndClosesCleanly(t *testing.T) {
	originalCapture := capturePollInterval
	capturePollInterval = 20 * time.Millisecond
	originalDead := paneDeadPollInterval
	paneDeadPollInterval = 30 * time.Millisecond
	defer func() {
		capturePollInterval = originalCapture
		paneDeadPollInterval = originalDead
	}()

	socket := interactiveSocket("capture-dead")
	cleanup := newBareInteractiveSession(t, socket, "s0", 40, 10)
	defer cleanup()
	client := tmux.Client{Socket: socket, Timeout: 5 * time.Second}

	paneID := capturePaneID(t, socket, "s0")
	session := startCaptureSession(t, client, socket, paneID, 40, 10)

	killPaneProcessUnderRemainOnExitFailed(t, socket, "s0")

	select {
	case <-session.Dead():
	case <-time.After(3 * time.Second):
		t.Fatal("Dead() never closed for a killed pane under TransportCapture")
	}

	closeDone := make(chan error, 1)
	go func() { closeDone <- session.Close() }()
	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("Close() after a dead pane under TransportCapture: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Close() blocked -- s.pipe is nil under TransportCapture and Close/markDead must guard against it")
	}
}

// TestStartStillDefaultsToTransportPipe is the regression control for
// this task's whole change: Start (every pre-070 call site, in
// production and in every other test in this package) must still behave
// exactly as it did before StartWithTransport existed -- a real pipe
// gets armed.
func TestStartStillDefaultsToTransportPipe(t *testing.T) {
	socket := interactiveSocket("start-still-pipe")
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

	if !waitFor(t, 3*time.Second, func() bool {
		out, err := exec.Command("tmux", "-L", socket, "display-message", "-p", "-t", "s0", "#{pane_pipe}").Output()
		return err == nil && string(out) == "1\n"
	}) {
		t.Fatalf("#{pane_pipe} never reported 1 -- Start must still default to TransportPipe and arm a real pipe")
	}
}
