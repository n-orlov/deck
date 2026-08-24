// release_test.go proves PRD phase3b II-25 at the Session layer: every
// exit route out of a Session -- the clean explicit Close, and Start
// itself failing partway through after the pipe has already been armed
// -- must actually release pipe-pane, not merely stop reading from it.
// internal/tmux's own pipe_release_test.go proves the underlying
// PanePipe.Close mechanism does what it claims (#{pane_pipe} returns to
// 0); this file proves every route this package has out of holding a
// PanePipe actually calls it.
package interactive

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/tmux"
)

// TestSessionCloseReleasesThePipeAndPanePipeReturnsToZero is II-25's
// clean-exit case: after Start succeeds and the caller later calls
// Close, #{pane_pipe} must read 0 -- checked against tmux itself, not
// against any in-process state Session/PanePipe keep.
func TestSessionCloseReleasesThePipeAndPanePipeReturnsToZero(t *testing.T) {
	socket := interactiveSocket("release-clean")
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

	armed, err := client.PanePipe(ctx, "s0")
	if err != nil {
		t.Fatalf("PanePipe after Start: %v", err)
	}
	if !armed {
		t.Fatalf("test assumption violated: #{pane_pipe} reads 0 right after Start, want 1")
	}

	if err := session.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	stillPiped, err := client.PanePipe(ctx, "s0")
	if err != nil {
		t.Fatalf("PanePipe after Close: %v", err)
	}
	if stillPiped {
		t.Fatalf("#{pane_pipe} still reads 1 after Session.Close -- the clean-exit path did not release the pipe")
	}
}

// TestSessionStartFailureAfterArmingReleasesThePipeOnErrorExit is II-25's
// error-exit case, distinct from the clean exit above per the task's own
// wording ("a test covers exit-by-error, not only the clean exit"): the
// pipe is armed successfully (so #{pane_pipe} really does flip to 1),
// but the seed callback that runs immediately afterward, still inside
// Start, fails. Start's own deferred cleanup must release the pipe
// before returning the error, not merely on an explicit, later
// Session.Close the caller never gets to make because Start itself
// never returned a *Session.
func TestSessionStartFailureAfterArmingReleasesThePipeOnErrorExit(t *testing.T) {
	socket := interactiveSocket("release-error")
	cleanup := newBareInteractiveSession(t, socket, "s0", 40, 10)
	defer cleanup()
	client := tmux.Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	seedErr := errors.New("injected seed failure")
	var observedArmed bool
	session, err := Start(ctx, client, "s0", 40, 10, func(ctx context.Context) ([]byte, error) {
		// The pipe is armed by the time this callback runs (Start's own
		// ordering guarantee, task 040/II-16) -- confirm that directly
		// here so a bug that skipped arming couldn't make this test
		// pass for the wrong reason (it would "succeed" at 0 either
		// way).
		armed, armErr := client.PanePipe(ctx, "s0")
		if armErr == nil {
			observedArmed = armed
		}
		return nil, seedErr
	})
	if err == nil {
		t.Fatalf("Start succeeded despite the seed callback returning an error")
	}
	if !errors.Is(err, seedErr) {
		t.Fatalf("Start's error does not wrap the injected seed failure: %v", err)
	}
	if session != nil {
		t.Fatalf("Start returned a non-nil *Session alongside an error")
	}
	if !observedArmed {
		t.Fatalf("test assumption violated: #{pane_pipe} was not 1 when the seed callback ran, so this test cannot distinguish a released pipe from one that was never armed")
	}

	stillPiped, err := client.PanePipe(ctx, "s0")
	if err != nil {
		t.Fatalf("PanePipe after Start's error return: %v", err)
	}
	if stillPiped {
		t.Fatalf("#{pane_pipe} still reads 1 after Start returned an error -- the error-exit path did not release the pipe")
	}
}
