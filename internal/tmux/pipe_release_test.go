// pipe_release_test.go proves PRD phase3b II-25 at the tmux layer:
// deck's OWN Close call -- the mechanism every exit path from
// ArmPipePane routes through, clean or not -- actually disarms
// pipe-pane, so `#{pane_pipe}` reads 0 afterward rather than staying
// armed against a reader nobody is draining anymore. Task 046/II-24
// already proved Close's disarm command produces a genuine io.EOF
// indistinguishable in TYPE from an external displacement/disablement
// (TestPanePipeWasClosedIsTrueBeforeAnyBlockedReadCanObserveOurOwnCloseAsEOF);
// this file proves the tmux-visible SIDE EFFECT of that same disarm
// command, which task 046 did not itself assert.
package tmux

import (
	"context"
	"testing"
	"time"
)

func pipeReleaseSocket(name string) string {
	return geometrySocket("piperelease-" + name)
}

// TestPanePipeCloseDisarmsAndPanePipeReturnsToZero is the clean-exit
// half of II-25: arm, then Close, then read `#{pane_pipe}` back from
// tmux itself (not from any in-process bookkeeping) and require 0.
func TestPanePipeCloseDisarmsAndPanePipeReturnsToZero(t *testing.T) {
	socket := pipeReleaseSocket("clean")
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 24)
	defer cleanup()
	client := Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	pipe, err := client.ArmPipePane(ctx, "s0")
	if err != nil {
		t.Fatalf("ArmPipePane: %v", err)
	}

	armed, err := client.PanePipe(ctx, "s0")
	if err != nil {
		t.Fatalf("PanePipe after arm: %v", err)
	}
	if !armed {
		t.Fatalf("test assumption violated: #{pane_pipe} reads 0 immediately after ArmPipePane, want 1")
	}

	if err := pipe.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	stillPiped, err := client.PanePipe(ctx, "s0")
	if err != nil {
		t.Fatalf("PanePipe after Close: %v", err)
	}
	if stillPiped {
		t.Fatalf("#{pane_pipe} still reads 1 after our own Close -- the pipe was not actually released")
	}
}

// TestPanePipeCloseIsIdempotentAndPanePipeStaysZero is the negative
// control that would catch a Close whose disarm command only appears to
// work the first time (e.g. a bug that re-armed the pipe as a side
// effect of a second disarm attempt): calling Close twice must leave
// `#{pane_pipe}` at 0, not flip it back.
func TestPanePipeCloseIsIdempotentAndPanePipeStaysZero(t *testing.T) {
	socket := pipeReleaseSocket("idempotent")
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 24)
	defer cleanup()
	client := Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	pipe, err := client.ArmPipePane(ctx, "s0")
	if err != nil {
		t.Fatalf("ArmPipePane: %v", err)
	}
	if err := pipe.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := pipe.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}

	stillPiped, err := client.PanePipe(ctx, "s0")
	if err != nil {
		t.Fatalf("PanePipe after two Close calls: %v", err)
	}
	if stillPiped {
		t.Fatalf("#{pane_pipe} reads 1 after two Close calls, want 0")
	}
}
