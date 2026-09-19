package tmux

// The pipe-pane FIFO wait (ArmPipePane's waitForFifoWriter) used to carry a
// single 5s wall-clock bound: a writer that had not connected by then was
// reported as "timed out after 5s waiting for pipe-pane's job to open the
// fifo", whether or not anything was actually wrong. On a loaded host that
// bound is simply too short for tmux's own fork+exec of `cat >> fifo`, and
// the resulting failure is indistinguishable from a real breakage --
// docs/reports/phase3j-findings.md records two such failures in
// internal/tui, and TestRaiseLostAttachOnStolenClaimTouchesNothing failed
// the same way during this run's task 017 validation.
//
// The cure splits the one bound into two (fifoWriterProbeGrace, a "this is
// just scheduling latency" window, and fifoWriterConnectTimeout, a
// last-resort pathology bound) with a single `#{pane_pipe}` probe at the
// grace mark deciding between them. These tests pin all three halves of
// that behaviour -- keeps waiting while armed, fails at once (and says
// why) when not armed, and honours the caller's context -- by driving
// waitForFifoWriter directly with small durations and a stub probe, so no
// tmux server and no multi-second sleep is needed. A regression to the old
// single-bound shape fails the first of them.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// newWaitTestFifo makes a fresh FIFO and returns its path plus a reader fd
// opened exactly the way ArmPipePane opens it (O_RDONLY|O_NONBLOCK, so the
// open returns with no writer present).
func newWaitTestFifo(t *testing.T) (string, int) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pane.fifo")
	if err := unix.Mkfifo(path, 0o600); err != nil {
		t.Fatalf("mkfifo %s: %v", path, err)
	}
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NONBLOCK, 0)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	t.Cleanup(func() { _ = unix.Close(fd) })
	return path, fd
}

// TestWaitForFifoWriterWaitsPastGraceWhileStillArmed is the load-tolerance
// half: the writer connects well AFTER the grace period, the probe reports
// the pipe still armed, and the wait must succeed rather than report a
// timeout. Under the old single-bound implementation the grace value here
// WAS the whole budget, so this case returned an error.
func TestWaitForFifoWriterWaitsPastGraceWhileStillArmed(t *testing.T) {
	path, fd := newWaitTestFifo(t)

	const grace = 20 * time.Millisecond
	// The writer appears an order of magnitude past grace -- the "loaded
	// host" shape, scaled down.
	writerAt := 10 * grace
	opened := make(chan struct{})
	go func() {
		time.Sleep(writerAt)
		w, err := os.OpenFile(path, os.O_WRONLY, 0)
		if err != nil {
			close(opened)
			return
		}
		close(opened)
		// Hold the write side open until the test ends: closing it would
		// hand the reader a genuine EOF.
		<-t.Context().Done()
		_ = w.Close()
	}()

	probes := 0
	start := time.Now()
	leftover, err := waitForFifoWriter(context.Background(), fd, func(context.Context) (bool, error) {
		probes++
		return true, nil
	}, grace, 30*time.Second)
	if err != nil {
		t.Fatalf("waitForFifoWriter returned %v, want success: a writer that connects past the grace period while the pipe is still armed must be waited for, not reported as a timeout", err)
	}
	if len(leftover) != 0 {
		t.Fatalf("leftover = %q, want empty (the writer wrote nothing)", leftover)
	}
	if elapsed := time.Since(start); elapsed < writerAt {
		t.Fatalf("waitForFifoWriter returned after %s, before the writer connected at %s -- it cannot have observed a writer at all", elapsed, writerAt)
	}
	if probes != 1 {
		t.Fatalf("stillArmed was consulted %d times, want exactly 1 (once at the grace mark, never per poll)", probes)
	}
	<-opened
}

// TestWaitForFifoWriterFailsAtGraceWhenNoLongerArmed is the fail-fast
// half: nothing ever connects, and the probe reports the pipe gone. The
// wait must return at the grace mark -- nowhere near the long pathology
// bound -- with an error naming the real cause rather than a timeout.
func TestWaitForFifoWriterFailsAtGraceWhenNoLongerArmed(t *testing.T) {
	_, fd := newWaitTestFifo(t)

	const grace = 20 * time.Millisecond
	const timeout = 30 * time.Second

	start := time.Now()
	if _, err := waitForFifoWriter(context.Background(), fd, func(context.Context) (bool, error) {
		return false, nil
	}, grace, timeout); err == nil {
		t.Fatalf("waitForFifoWriter succeeded with no writer and no armed pipe, want an error")
	} else if !strings.Contains(err.Error(), "no longer armed") {
		t.Fatalf("waitForFifoWriter error = %v, want it to name the unarmed pipe as the cause", err)
	}
	if elapsed := time.Since(start); elapsed > timeout/10 {
		t.Fatalf("waitForFifoWriter took %s to notice the pipe was no longer armed, want it to fail at the %s grace mark instead of waiting out the %s bound", elapsed, grace, timeout)
	}
}

// TestWaitForFifoWriterHonoursContextCancellation pins the third half: the
// long pathology bound is not a floor a caller is stuck with. A cancelled
// context ends the wait promptly and the returned error wraps
// context.Canceled, so a caller that wants a tighter bound sets a deadline
// instead of shrinking fifoWriterConnectTimeout for everybody.
func TestWaitForFifoWriterHonoursContextCancellation(t *testing.T) {
	_, fd := newWaitTestFifo(t)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, err := waitForFifoWriter(ctx, fd, func(context.Context) (bool, error) {
		return true, nil
	}, time.Second, 30*time.Second)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("waitForFifoWriter error = %v, want it to wrap context.Canceled", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("waitForFifoWriter took %s to honour a cancelled context", elapsed)
	}
}

// TestFifoWriterBoundsAreSplit pins the two production constants
// ArmPipePane passes: the grace window stays short (a real breakage is
// still diagnosed in seconds) while the pathology bound is far longer than
// the 5s that proved too tight under load. A revert to one 5s bound for
// both fails here.
func TestFifoWriterBoundsAreSplit(t *testing.T) {
	if fifoWriterProbeGrace > 10*time.Second {
		t.Errorf("fifoWriterProbeGrace = %s, want a short window: a genuinely unarmed pipe must still be diagnosed in seconds", fifoWriterProbeGrace)
	}
	if fifoWriterConnectTimeout < 30*time.Second {
		t.Errorf("fifoWriterConnectTimeout = %s, want at least 30s: 5s was measurably too short for tmux's own fork+exec on a loaded host", fifoWriterConnectTimeout)
	}
	if fifoWriterConnectTimeout <= fifoWriterProbeGrace {
		t.Errorf("fifoWriterConnectTimeout (%s) must exceed fifoWriterProbeGrace (%s), otherwise the probe at the grace mark can never buy any extra patience", fifoWriterConnectTimeout, fifoWriterProbeGrace)
	}
}
