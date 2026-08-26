// writestall_test.go is R68's second half of regression cover for issue
// #5 (replydrain_test.go is the first). Fix 1 removed the one KNOWN way a
// grid.Write can park forever (vt's undrained reply pipe). Fix 2 removes
// the ESCALATION: a write that stalls for any reason at all -- a future
// undrained writer inside vt, or simply a very large burst taking real
// time to parse -- must not be able to stall the frame path, because
// RenderRows is what View() calls and View() is bubbletea's event loop.
//
// Every test here stalls a REAL grid.Write (a DA1 query written into a
// grid with no reply drain: vt answers it into an unbuffered io.Pipe, so
// with nobody reading, the writer parks permanently) and then asserts,
// against its OWN deadline, that the rest of the Session still works.
// Bounding each assertion is deliberate and is the difference between a
// regression test and a trap: against unfixed code the work does not
// fail, it PARKS, so an unbounded assertion would hang until `go test`'s
// package timeout fired and dumped every unrelated goroutine with no
// attribution.
package interactive

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	// writeStallDeadline bounds every "did this call return at all?" wait
	// in this file. Generous against a loaded host, tight enough that a
	// full red run reports in seconds rather than by package timeout.
	writeStallDeadline = 3 * time.Second
	// renderProbeWindow is how long the render loop keeps calling
	// RenderRows once a write is known to be in flight. It is not a
	// settle: the loop's own returns are the assertion, and the test
	// separately proves the write was STILL stalled when the window
	// closed (so the window provably overlapped the stall).
	renderProbeWindow = 250 * time.Millisecond
	// renderProbeMinCalls is how many RenderRows calls must complete
	// inside that window. Against unfixed code (s.mu held for the
	// duration of the Write) at most the handful that raced ahead of the
	// park can complete and the loop then blocks forever, so any count
	// well above "a handful" separates fixed from unfixed; the real
	// separator is the deadline the loop is bounded by.
	renderProbeMinCalls = 20
)

// stalledWriteSession returns a Session whose current grid has NO reply
// drain, plus that grid, plus a func that stalls a write into it for real
// and a func that unparks it again. The grid deliberately comes from
// newGrid rather than newDrainedGrid: with no reader on vt's reply pipe,
// writing a DA1 query is exactly the production stall issue #5 reported,
// reproduced without tmux and without a race.
func stalledWriteSession(t *testing.T, width, height int) (s *Session, g *Grid, stall func() (started, returned <-chan struct{}), unpark func()) {
	t.Helper()
	g = newGrid(width, height)
	s = &Session{grid: g, renders: NewRenderCoalescer(renderCoalesceInterval)}
	t.Cleanup(s.renders.Close)

	stall = func() (<-chan struct{}, <-chan struct{}) {
		startedCh, returnedCh := make(chan struct{}), make(chan struct{})
		go func() {
			defer close(returnedCh)
			close(startedCh)
			// The production write path, not a bare g.Write: whatever
			// locking drain's own per-read write takes, this takes too.
			s.writeNotice("\x1b[c")
		}()
		return startedCh, returnedCh
	}
	unpark = func() {
		// Closing the reply pipe's write end makes the parked write
		// return io.ErrClosedPipe instead of leaking a goroutine for the
		// rest of the package's run (retireGrid's own doc).
		retireGrid(g)
	}
	return s, g, stall, unpark
}

// TestStalledGridWriteNeverBlocksRenderRows is fix 2's regression test:
// with a write parked inside grid.Write, RenderRows must keep returning
// (serving the last composed frame) instead of waiting for it. Before the
// fix the write held s.mu for its whole duration and RenderRows could not
// take the read lock at all -- the wedge issue #5 reported.
func TestStalledGridWriteNeverBlocksRenderRows(t *testing.T) {
	const width, height = 40, 8
	s, _, stall, unpark := stalledWriteSession(t, width, height)

	// Content that must still be visible while the next write is stalled,
	// and one composed frame so there is a last-known frame to serve.
	const marker = "BEFORE-THE-STALL"
	s.writeNotice(marker + "\r\n")
	if rows, _ := s.RenderRows(0, height); !rowsContain(rows, marker) {
		t.Fatalf("setup: %q is not in the grid before the stall; frame:\n%s", marker, strings.Join(rows, "\n"))
	}

	started, returned := stall()
	<-started
	defer unpark()

	type result struct {
		calls int
		rows  []string
	}
	done := make(chan result, 1)
	go func() {
		var r result
		deadline := time.Now().Add(renderProbeWindow)
		for {
			// The exact call View() makes via interactiveBodyLines.
			r.rows, _ = s.RenderRows(0, height)
			r.calls++
			if time.Now().After(deadline) {
				break
			}
		}
		done <- r
	}()

	var got result
	select {
	case got = <-done:
	case <-time.After(writeStallDeadline):
		t.Fatalf("DEADLOCK: with one grid.Write stalled, RenderRows stopped returning -- the render loop did not finish %s of calls within %s. This is issue #5's escalation: the write path holds the lock RenderRows needs, so View() and with it bubbletea's event loop park with no keyboard escape",
			renderProbeWindow, writeStallDeadline)
	}

	// Proof the window above really did overlap a stalled write: a DA1
	// written into a grid with no reply drain can never complete on its
	// own, so this write is still inside grid.Write right now. Without
	// this check a green result could mean "the write finished first".
	select {
	case <-returned:
		t.Fatalf("inconclusive: the stalled write returned on its own -- vt must have stopped answering DA1 into its reply pipe, so this test no longer stalls anything; re-derive the stall against the current vt version")
	default:
	}

	if got.calls < renderProbeMinCalls {
		t.Errorf("RenderRows completed only %d calls in %s while a write was stalled, want at least %d -- it is being slowed by the stalled write even if it is no longer blocked by it",
			got.calls, renderProbeWindow, renderProbeMinCalls)
	}
	if !rowsContain(got.rows, marker) {
		t.Errorf("RenderRows kept returning while a write was stalled, but the frame it served does not contain %q -- it is serving blanks rather than the last composed frame:\n%s",
			marker, strings.Join(got.rows, "\n"))
	}
}

// TestStalledGridWriteNeverBlocksAReseed covers the other half of "the
// session lock is not held across a Write": a resize is a full reseed
// (Resize -> installGrid, which takes s.mu for WRITING), so before fix 2
// a stalled write blocked every reseed too -- including the fallback
// loop's and captureLoop's, i.e. the very mechanisms that would otherwise
// replace the stalled grid.
func TestStalledGridWriteNeverBlocksAReseed(t *testing.T) {
	const width, height = 40, 8
	s, old, stall, unpark := stalledWriteSession(t, width, height)
	defer unpark()

	started, returned := stall()
	<-started

	resized := make(chan error, 1)
	go func() {
		resized <- s.Resize(context.Background(), width, height+2, func(context.Context) ([]byte, error) {
			return []byte("AFTER-THE-RESEED\r\n"), nil
		})
	}()

	select {
	case err := <-resized:
		if err != nil {
			t.Fatalf("Resize while a write was stalled: %v", err)
		}
	case <-time.After(writeStallDeadline):
		t.Fatalf("DEADLOCK: Resize did not return within %s while one grid.Write was stalled -- the write is holding s.mu, which installGrid needs to publish the fresh grid, so no reseed (resize, capture tick or displacement fallback) can ever replace the stalled grid",
			writeStallDeadline)
	}

	if s.Grid() == old {
		t.Fatalf("Resize returned but the session's grid is still the stalled one -- the reseed did not install its fresh grid")
	}
	// installGrid retires the grid it displaced, which is also what
	// unparks the stalled write; join it so this test leaves no parked
	// writer behind for the rest of the package.
	select {
	case <-returned:
	case <-time.After(writeStallDeadline):
		t.Errorf("the stalled write did not return within %s after the reseed retired its grid -- retireGrid no longer unparks a writer (see retireGrid's doc)", writeStallDeadline)
	}
}

// TestConcurrentWritesAndReadsStayConsistent exercises what fix 2 keeps:
// with s.mu no longer held across a Write, s.writes is what makes the
// multi-call read paths (RenderRows, AbsoluteRow, SelectedText) atomic
// against a concurrent write. Its value is realised under `go test
// -race`, where dropping that lock reports the Scrollback/CellAt
// pointer-after-return race RenderRows' doc describes -- vt hands those
// calls pointers into live buffers, so a concurrent Write mutating a cell
// this reader is copying IS a data race, not merely a torn frame. Without
// -race it still covers the lock ordering: a deadlock between the write
// path and either read path hangs here.
func TestConcurrentWritesAndReadsStayConsistent(t *testing.T) {
	const width, height = 40, 8
	// A drained grid this time: the point is real, completing writes.
	s := &Session{renders: NewRenderCoalescer(renderCoalesceInterval)}
	defer s.renders.Close()
	s.grid = s.newDrainedGrid(width, height)
	defer func() {
		retireGrid(s.currentGrid())
		s.replyDrains.Wait()
	}()

	const rounds = 300
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		for i := 0; i < rounds; i++ {
			s.writeNotice("line of live output\r\n")
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < rounds; i++ {
			rows, used := s.RenderRows(i%3, height)
			if len(rows) != height {
				t.Errorf("RenderRows returned %d rows, want %d", len(rows), height)
				return
			}
			if used < 0 {
				t.Errorf("RenderRows returned usedOffset %d", used)
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < rounds; i++ {
			_ = s.AbsoluteRow(0, height, i%height)
			_ = s.SelectedText(0, 0, width-1, height-1)
		}
	}()

	finished := make(chan struct{})
	go func() {
		wg.Wait()
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(writeStallDeadline):
		t.Fatalf("DEADLOCK: %d rounds of concurrent writes, RenderRows, AbsoluteRow and SelectedText did not finish within %s -- the write path and a read path are deadlocking (see the Session struct's lock-ordering rule: never acquire s.mu while holding s.writes)",
			rounds, writeStallDeadline)
	}
}
