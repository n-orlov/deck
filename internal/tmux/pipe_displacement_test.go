// pipe_displacement_test.go proves PRD phase3b II-24's raw material at
// the tmux layer: a PanePipe reader receives a genuine io.EOF -- not a
// silent stall -- both when a second `pipe-pane -IO` displaces deck's
// own on the same target (with `#{pane_pipe}` still 1, because the new
// holder is armed) and when the pipe is disabled outright (with
// `#{pane_pipe}` 0). internal/interactive's own tests build the
// Session-level displaced-vs-disabled distinction and the passive-
// capture fallback on top of this.
package tmux

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"
)

func pipeDisplacementSocket(name string) string {
	return geometrySocket("pipedisp-" + name)
}

// readWithTimeout runs a single Read on a background goroutine so a test
// can bound how long it waits for either bytes or an error -- a genuine
// stall (the very bug this task's redesign of ArmPipePane's reader-open
// order fixes) must be distinguishable from a slow but eventually
// successful read.
func readWithTimeout(t *testing.T, p *PanePipe, timeout time.Duration) (int, error, bool) {
	t.Helper()
	type result struct {
		n   int
		err error
	}
	ch := make(chan result, 1)
	buf := make([]byte, 4096)
	go func() {
		n, err := p.Read(buf)
		ch <- result{n, err}
	}()
	select {
	case r := <-ch:
		return r.n, r.err, true
	case <-time.After(timeout):
		return 0, nil, false
	}
}

// TestPanePipeReceivesGenuineEOFOnDisplacementWithPanePipeStillOne is
// II-24's first half: arming a SECOND `pipe-pane -IO` on the same target
// displaces deck's own pipe silently (no error from either tmux
// invocation) and deck's reader observes a genuine io.EOF -- while
// `#{pane_pipe}` still reads 1, because the new holder's pipe is what is
// now armed.
func TestPanePipeReceivesGenuineEOFOnDisplacementWithPanePipeStillOne(t *testing.T) {
	socket := pipeDisplacementSocket("displaced")
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 24)
	defer cleanup()
	client := Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	pipe, err := client.ArmPipePane(ctx, "s0")
	if err != nil {
		t.Fatalf("ArmPipePane: %v", err)
	}
	defer pipe.CloseLocal()

	// Displace it: a second pipe-pane -IO on the SAME target, writing
	// into a throwaway destination this test never reads from.
	if _, err := client.run(ctx, "pipe-pane", "-IO", "-t", "s0", "cat > /dev/null"); err != nil {
		t.Fatalf("arm the displacing pipe-pane: %v", err)
	}

	n, readErr, done := readWithTimeout(t, pipe, 5*time.Second)
	if !done {
		t.Fatalf("displaced reader never returned from Read within 5s -- it should have observed a clean EOF, not stalled")
	}
	if n != 0 || !errors.Is(readErr, io.EOF) {
		t.Fatalf("displaced reader: got n=%d err=%v, want n=0 err=io.EOF", n, readErr)
	}

	stillPiped, err := client.PanePipe(ctx, "s0")
	if err != nil {
		t.Fatalf("PanePipe after displacement: %v", err)
	}
	if !stillPiped {
		t.Fatalf("#{pane_pipe} reads 0 after displacement, want 1 -- the second holder's pipe should still be armed")
	}
}

// TestPanePipeReceivesGenuineEOFOnDisableWithPanePipeZero is II-24's
// second half, and the discriminating control against the displacement
// case above: a bare `pipe-pane -t target` (no command) disables piping
// outright. The reader still observes a clean io.EOF, but `#{pane_pipe}`
// now reads 0 -- the only observable difference between the two cases,
// and exactly what Session.handlePipeGone (internal/interactive) uses to
// tell them apart.
func TestPanePipeReceivesGenuineEOFOnDisableWithPanePipeZero(t *testing.T) {
	socket := pipeDisplacementSocket("disabled")
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 24)
	defer cleanup()
	client := Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	pipe, err := client.ArmPipePane(ctx, "s0")
	if err != nil {
		t.Fatalf("ArmPipePane: %v", err)
	}
	defer pipe.CloseLocal()

	if _, err := client.run(ctx, "pipe-pane", "-t", "s0"); err != nil {
		t.Fatalf("bare pipe-pane (disable): %v", err)
	}

	n, readErr, done := readWithTimeout(t, pipe, 5*time.Second)
	if !done {
		t.Fatalf("reader never returned from Read within 5s after disable -- it should have observed a clean EOF, not stalled")
	}
	if n != 0 || !errors.Is(readErr, io.EOF) {
		t.Fatalf("reader after disable: got n=%d err=%v, want n=0 err=io.EOF", n, readErr)
	}

	stillPiped, err := client.PanePipe(ctx, "s0")
	if err != nil {
		t.Fatalf("PanePipe after disable: %v", err)
	}
	if stillPiped {
		t.Fatalf("#{pane_pipe} reads 1 after an explicit disable, want 0")
	}
}

// TestPanePipeWasClosedIsTrueBeforeAnyBlockedReadCanObserveOurOwnCloseAsEOF
// is the discriminator drain actually relies on, and the reason io.EOF's
// own type is not enough (task 046/II-24): closing our OWN reader (the
// same path an explicit Session.Close or markDead takes) produces the
// SAME io.EOF a genuine external displacement or disablement does --
// confirmed directly below, and the opposite of what an earlier version
// of this test assumed -- because Close's own disarm command makes
// tmux's job process exit too, closing the FIFO's write side exactly
// the way an external actor closing it would. WasClosed(), not the
// error's type, is what a caller must check first: by the time a
// blocked Read can possibly return as a result of Close, WasClosed()
// already reports true, because Close sets it while still holding
// closeMu, before the disarm command that is what actually unblocks the
// read ever runs.
//
// The pane is a bare shell, not a program that stays silent until Close:
// it can emit its own bytes (a prompt, a motd line) at any point,
// including during the 200ms this test waits for the Read to block. An
// earlier version of this test assumed the very FIRST Read call would be
// the one Close unblocks and failed when the pane won that race instead
// (observed: n=2 err=nil, two bytes of genuine pane output, not our
// Close's EOF). That is real data, not a bug, and asserting on it would
// be exactly the kind of "widen the checkpoint" fix this project
// forbids. So the reader goroutine below drains and discards any number
// of non-error reads -- each one is unrelated pane chatter -- and only
// evaluates WasClosed() against the read that actually returns an
// error, whichever real-numbered read that turns out to be.
func TestPanePipeWasClosedIsTrueBeforeAnyBlockedReadCanObserveOurOwnCloseAsEOF(t *testing.T) {
	socket := pipeDisplacementSocket("selfclose")
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 24)
	defer cleanup()
	client := Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	pipe, err := client.ArmPipePane(ctx, "s0")
	if err != nil {
		t.Fatalf("ArmPipePane: %v", err)
	}

	if pipe.WasClosed() {
		t.Fatalf("WasClosed() is true before Close was ever called")
	}

	type readResult struct {
		n         int
		err       error
		wasClosed bool
	}
	readDone := make(chan readResult, 1)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := pipe.Read(buf)
			if err != nil {
				// Read WasClosed() immediately after Read returns an
				// error, in the same goroutine, so this genuinely
				// reflects what a caller in drain's position would see
				// at the moment it needs to decide.
				readDone <- readResult{n, err, pipe.WasClosed()}
				return
			}
			// n>0, err==nil: genuine pane output (a prompt, a motd
			// line) that arrived before our Close. Not the discriminator
			// this test exists to prove -- drain it and keep waiting for
			// the read that actually observes Close's EOF.
		}
	}()

	time.Sleep(200 * time.Millisecond) // let the Read actually block
	if err := pipe.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	select {
	case r := <-readDone:
		if !errors.Is(r.err, io.EOF) {
			t.Fatalf("Close's own disarm command did not produce io.EOF here (got n=%d err=%v) -- if this changes, the WasClosed-first discriminator this test protects may no longer be necessary, but it would not make it wrong", r.n, r.err)
		}
		if !r.wasClosed {
			t.Fatalf("WasClosed() reported false immediately after Read returned from our own Close -- drain would misread this as an external displacement/disablement and start a passive-capture fallback nothing asked for")
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("blocked Read never returned after Close")
	}
}
