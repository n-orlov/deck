// displacement_test.go proves PRD phase3b/phase3c II-24 at the
// Session/live-path level: an EOF that arrives while `#{pane_pipe}` is
// still 1 means something else has displaced deck's own pipe-pane, and a
// Session must fall back to periodic passive-capture snapshots and say
// so in the panel; an EOF that arrives while `#{pane_pipe}` is 0 means
// the pipe was simply disabled, and nothing gets a fallback. The raw
// EOF/#{pane_pipe} mechanics this depends on are proved directly, at the
// tmux layer, by internal/tmux's own
// TestPanePipeReceivesGenuineEOFOnDisplacementWithPanePipeStillOne and
// TestPanePipeReceivesGenuineEOFOnDisableWithPanePipeZero.
package interactive

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/tmux"
)

// TestSessionStatusStaysLiveWithoutAnyDisplacementOrDisable is the
// sanity control for the two positive cases below: against a pipe that
// is neither displaced nor disabled, Status() must stay StatusLive even
// after several fallback-poll-interval-sized waits, so a bug that flipped
// status unconditionally could not make either positive test below pass
// for the wrong reason.
func TestSessionStatusStaysLiveWithoutAnyDisplacementOrDisable(t *testing.T) {
	socket := interactiveSocket("status-live")
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

	time.Sleep(300 * time.Millisecond)
	if got := session.Status(); got != StatusLive {
		t.Fatalf("Status() reports %v against an undisturbed pipe, want StatusLive", got)
	}
	if gridContains(session.Grid(), "displaced") {
		t.Fatalf("grid contains the displacement notice against a pipe that was never displaced")
	}
}

// TestSessionFallsBackToPassiveCaptureWhenPipeIsDisplaced is II-24's
// first positive case: a second `pipe-pane -IO` armed on the SAME target
// displaces deck's own pipe (tmux's pipe-pane is single-holder-per-pane).
// The Session must notice (Status() reports StatusDisplaced), say so in
// the panel (the grid carries the notice text), and keep the panel
// USEFUL afterward via periodic passive-capture snapshots rather than
// merely freezing on the last live frame.
func TestSessionFallsBackToPassiveCaptureWhenPipeIsDisplaced(t *testing.T) {
	originalInterval := pipeDisplacedFallbackInterval
	pipeDisplacedFallbackInterval = 30 * time.Millisecond
	defer func() { pipeDisplacedFallbackInterval = originalInterval }()

	socket := interactiveSocket("displaced")
	cleanup := newBareInteractiveSession(t, socket, "s0", 40, 10)
	defer cleanup()
	client := tmux.Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	// The fallback loop's own CaptureSeed (grid.go) requires a real
	// "%N" pane id target, unlike ArmPipePane/PanePipe/pipe-pane, which
	// all accept a bare session name just as well -- so, unlike this
	// package's other Session tests, target must be resolved to the
	// pane id here.
	pane := firstPaneID(t, socket, "s0")

	session, err := Start(ctx, client, pane, 40, 10, func(ctx context.Context) ([]byte, error) {
		return rawCapturePane(t, socket, "s0"), nil
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer session.Close()

	// Displace deck's own pipe with a second `pipe-pane -IO` on the same
	// target, writing into a destination this test never reads from --
	// deck's own reader observes the resulting EOF exactly as it would
	// from any other process that happened to do the same thing.
	if out, err := exec.Command("tmux", "-L", socket, "pipe-pane", "-IO", "-t", "s0", "cat > /dev/null").CombinedOutput(); err != nil {
		t.Fatalf("arm the displacing pipe-pane: %v: %s", err, out)
	}

	if !waitFor(t, 2*time.Second, func() bool { return session.Status() == StatusDisplaced }) {
		t.Fatalf("Session never reported StatusDisplaced within 2s of its pipe being displaced")
	}

	if !waitFor(t, 2*time.Second, func() bool { return gridContains(session.Grid(), "displaced") }) {
		t.Fatalf("grid never shows the displacement notice after StatusDisplaced was reported")
	}

	// Prove the fallback is actually alive, not a one-shot notice that
	// then freezes: type something new into the pane and confirm a
	// periodic passive-capture snapshot eventually picks it up, with
	// nothing left listening on the (now displaced) pipe to deliver it
	// any other way.
	sendLiteralLine(t, socket, "s0", "FALLBACK-CAPTURED")
	if !waitFor(t, 2*time.Second, func() bool { return gridContains(session.Grid(), "FALLBACK-CAPTURED") }) {
		t.Fatalf("fallback passive-capture snapshots never picked up pane output written after displacement")
	}

	// Close must still return promptly: CloseLocal releases deck's own
	// already-orphaned fd/tempdir without touching the displacing
	// holder's still-live pipe, and the fallback loop must unwind on
	// ctx cancellation like everything else Close waits on.
	closeDone := make(chan struct{})
	go func() {
		_ = session.Close()
		close(closeDone)
	}()
	select {
	case <-closeDone:
	case <-time.After(2 * time.Second):
		t.Fatalf("Session.Close() did not return within 2s after displacement; the fallback loop or CloseLocal appears stuck")
	}
}

// TestSessionReportsDisabledAndNeverStartsAFallbackWhenPipeIsDisabled is
// II-24's second positive case, and the discriminating control against
// the displacement case above: a bare `pipe-pane -t target` (no command)
// disables piping outright, with `#{pane_pipe}` reading 0 afterward. The
// Session must report StatusDisabled, and -- unlike displacement -- must
// NOT start the passive-capture fallback, since there is nothing to fall
// back FROM losing (nobody asked deck to keep the panel live against an
// explicit disable; task 047/II-25 covers deck's own release-on-exit,
// which takes this same disabled path deliberately).
func TestSessionReportsDisabledAndNeverStartsAFallbackWhenPipeIsDisabled(t *testing.T) {
	socket := interactiveSocket("disabled")
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

	if out, err := exec.Command("tmux", "-L", socket, "pipe-pane", "-t", "s0").CombinedOutput(); err != nil {
		t.Fatalf("bare pipe-pane (disable): %v: %s", err, out)
	}

	if !waitFor(t, 2*time.Second, func() bool { return session.Status() == StatusDisabled }) {
		t.Fatalf("Session never reported StatusDisabled within 2s of its pipe being disabled")
	}

	// Give the fallback loop every chance to misfire before concluding
	// it did not.
	time.Sleep(300 * time.Millisecond)
	if gridContains(session.Grid(), "displaced") {
		t.Fatalf("an explicit disable incorrectly started the displacement fallback (notice text present in the grid)")
	}

	closeDone := make(chan struct{})
	go func() {
		_ = session.Close()
		close(closeDone)
	}()
	select {
	case <-closeDone:
	case <-time.After(2 * time.Second):
		t.Fatalf("Session.Close() did not return within 2s after disablement")
	}
}
