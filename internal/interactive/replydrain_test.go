// replydrain_test.go is R68's regression cover for issue #5: a terminal
// capability query arriving in a previewed pane's output used to park
// deck's drain goroutine inside grid.Write -- holding s.mu, which wedged
// RenderRows, View() and therefore bubbletea's whole event loop, with no
// keyboard escape and no response to SIGTERM.
//
// Every test in this file bounds its own work against its own deadline and
// fails with a message naming the deadlock. That is deliberate and it is
// the difference between a regression test and a trap: against unfixed
// code the work does not fail, it PARKS -- so a naive assertion would hang
// until `go test`'s global package timeout fired and dumped 50 unrelated
// goroutines with no attribution to the test that caused it.
package interactive

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/tmux"
)

// replyQueryCases are the sequences fed through a live pane. DA1 is the
// one the issue was root-caused on (Claude Code emits it at startup), but
// the defect is the undrained reply pipe, not that sequence: every
// replying handler in charmbracelet/x/vt is its own trigger, so a fix
// that special-cased CSI c would pass a DA1-only test and leave the rest
// live. DSR and an OSC colour query are two of those others, reaching the
// pipe through two different handler families (CSI and OSC).
var replyQueryCases = []struct {
	name string
	// seq is the query as a shell printf FORMAT (single-quoted in the
	// command below, so printf itself expands the octal escapes).
	seq string
	// what the sequence is, and where vt replies to it.
	what string
}{
	{
		name: "da1",
		seq:  `\033[c`,
		what: "primary device attributes (CSI c), which vt answers at handlers.go:695 -- the query issue #5 was root-caused on",
	},
	{
		name: "dsr",
		seq:  `\033[6n`,
		what: "device status report / cursor position (CSI 6n), which vt answers at handlers.go:806",
	},
	{
		name: "osc11",
		seq:  `\033]11;?\007`,
		what: "the OSC 11 background-colour query, which vt answers at osc.go:92",
	},
}

const (
	// replyQueryPollTimeout is how long the assertion goroutine waits for
	// the post-query marker to reach the grid. On fixed code the pane
	// emits it within a few tens of milliseconds of the query (measured:
	// each subtest passes in ~40ms end to end); this is slack for a loaded
	// host, not a settle.
	replyQueryPollTimeout = 3 * time.Second
	// replyQueryDeadline is the test's OWN deadline for that goroutine
	// having returned at all. It has to exceed replyQueryPollTimeout,
	// because a goroutine that merely failed to see the marker still
	// returns; only a goroutine parked inside RenderRows (or inside the
	// drain that holds its lock) fails to. Kept tight on purpose: against
	// unfixed code all three cases plus their bounded Closes have to
	// report within about half a minute, not eventually.
	replyQueryDeadline = 5 * time.Second
	// replyDrainCloseTimeout bounds every Session.Close and every
	// wait-for-a-parked-writer in this file, for the same reason.
	replyDrainCloseTimeout = 3 * time.Second
)

// TestTerminalQueryInPaneOutputNeverStallsSession is R68's regression
// test. For each query it drives a REAL tmux pane -- the actual defect
// path, pane output -> pipe-pane -> Session.drain -> grid.Write -- and
// then asserts the two things the deadlock took away: the drain kept
// going (bytes emitted immediately AFTER the query still reach the grid)
// and RenderRows still returns (the lock the wedged writer used to hold
// is available to a reader again).
func TestTerminalQueryInPaneOutputNeverStallsSession(t *testing.T) {
	for _, tc := range replyQueryCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			const width, height = 80, 12
			socket := interactiveSocket("reply-" + tc.name)
			cleanup := newBareInteractiveSession(t, socket, "s0", width, height)
			defer cleanup()
			client := tmux.Client{Socket: socket, Timeout: 5 * time.Second}

			session, err := Start(context.Background(), client, "s0", width, height, func(ctx context.Context) ([]byte, error) {
				return rawCapturePane(t, socket, "s0"), nil
			})
			if err != nil {
				t.Fatalf("Start: %v", err)
			}
			// NOT a plain `defer session.Close()`: Close waits for the
			// drain goroutine, which is precisely what is parked when
			// this regression is present, so an unbounded Close would
			// convert a clean failure into a package-timeout hang.
			defer closeSessionWithin(t, session, replyDrainCloseTimeout)

			// The marker is printed by the SAME printf that emits the
			// query, immediately after it, so it can only reach the grid
			// by way of a drain that survived the query. It is passed as
			// two printf ARGUMENTS rather than spelled out in the format
			// string on purpose: the shell echoes the typed command line
			// into the pane before running it, and a marker that appeared
			// literally in that line would be visible in the grid even
			// with the drain parked -- the assertion would pass against
			// unfixed code. Echoed, the two arguments are separated by a
			// space ("DRAINED PASTDA1"); printed, they are contiguous.
			head, tail := "DRAINED", "PAST"+strings.ToUpper(tc.name)
			marker := head + tail
			command := fmt.Sprintf(`printf '%s%%s%%s\n' %s %s`, tc.seq, head, tail)
			sendLiteralLine(t, socket, "s0", command)

			var (
				mu     sync.Mutex
				found  bool
				frame  []string
				polled = make(chan struct{})
			)
			go func() {
				defer close(polled)
				deadline := time.Now().Add(replyQueryPollTimeout)
				for {
					// RenderRows is the call View() makes (via
					// interactiveBodyLines); on unfixed code it never
					// returns here, because the drain goroutine is
					// parked inside grid.Write holding s.mu.
					rows, _ := session.RenderRows(0, height)
					mu.Lock()
					frame = rows
					if rowsContain(rows, marker) {
						found = true
					}
					done := found
					mu.Unlock()
					if done || time.Now().After(deadline) {
						return
					}
					time.Sleep(20 * time.Millisecond)
				}
			}()

			select {
			case <-polled:
			case <-time.After(replyQueryDeadline):
				t.Fatalf("DEADLOCK: after the pane emitted %s, the assertion goroutine never returned within %s -- Session.drain is parked inside grid.Write (vt writes the reply into an unbuffered io.Pipe nobody reads) while holding s.mu, so RenderRows can never take the read lock. This is issue #5: in the real client that parked View() and with it bubbletea's event loop, killing the whole TUI",
					tc.what, replyQueryDeadline)
			}

			mu.Lock()
			ok, last := found, append([]string(nil), frame...)
			mu.Unlock()
			if !ok {
				t.Fatalf("the pane emitted %s followed immediately by %q, and RenderRows kept returning, but %q never reached the grid within %s -- the drain stopped making progress past the query. Last frame:\n%s",
					tc.what, marker, marker, replyQueryPollTimeout, strings.Join(last, "\n"))
			}
		})
	}
}

// TestRetireGridUnblocksItsReplyDrain pins the mechanism the fix uses to
// retire a grid's reply drain when a reseed replaces that grid, without
// needing tmux: closing the emulator's reply-pipe write end hands the
// parked reader io.EOF, so it exits instead of leaking. captureLoop
// reseeds five times a second, so an unretirable drain would be a
// goroutine leak at that rate.
func TestRetireGridUnblocksItsReplyDrain(t *testing.T) {
	s := &Session{}
	g := s.newDrainedGrid(40, 6)

	// Writing a query is what parks a writer with no reader (issue #5);
	// bounded here for the same reason as above -- with the drain missing
	// this Write never returns.
	wrote := make(chan struct{})
	go func() {
		defer close(wrote)
		_, _ = g.Write([]byte("\x1b[c"))
	}()
	select {
	case <-wrote:
	case <-time.After(replyDrainCloseTimeout):
		t.Fatalf("DEADLOCK: grid.Write(\"\\x1b[c\") never returned within %s -- the grid's reply drain is not reading, so vt's DA1 reply parked its writer permanently (issue #5)", replyDrainCloseTimeout)
	}

	retireGrid(g)

	drained := make(chan struct{})
	go func() {
		defer close(drained)
		s.replyDrains.Wait()
	}()
	select {
	case <-drained:
	case <-time.After(replyDrainCloseTimeout):
		t.Fatalf("retireGrid did not stop the grid's reply drain within %s -- a reseed would leak one goroutine per replaced grid (captureLoop replaces one every capturePollInterval)", replyDrainCloseTimeout)
	}
}

// TestEmulatorInputPipeIsAPipeWriter pins the one library detail
// retireGrid depends on: vt's Emulator.InputPipe() is the *io.PipeWriter
// half of the io.Pipe its replying handlers write to, so closing it is
// what retires a drain (and, unlike Emulator.Close, it leaves vt's
// unsynchronised `closed` field alone -- see retireGrid's doc). A go.mod
// bump that changes the type must fail here, loudly, rather than silently
// turning every retirement into a leaked goroutine.
func TestEmulatorInputPipeIsAPipeWriter(t *testing.T) {
	g := newGrid(10, 3)
	if _, ok := g.InputPipe().(*io.PipeWriter); !ok {
		t.Fatalf("vt Emulator.InputPipe() is %T, want *io.PipeWriter -- retireGrid can no longer retire a reply drain by closing it; re-derive the mechanism against the current vt version", g.InputPipe())
	}
}

// closeSessionWithin closes a Session but never hangs the package doing
// it: Session.Close waits for the drain goroutine, which is exactly what
// is parked when R68's regression is present.
func closeSessionWithin(t *testing.T, s *Session, timeout time.Duration) {
	t.Helper()
	closed := make(chan error, 1)
	go func() { closed <- s.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Logf("Session.Close: %v", err)
		}
	case <-time.After(timeout):
		t.Errorf("Session.Close did not return within %s -- its drain goroutine is still parked; leaking this Session rather than hanging the package", timeout)
	}
}

// rowsContain searches a RenderRows frame for needle, one row at a time
// and with each row's styling escapes stripped, for exactly the reasons
// gridContains documents (Render() emits an SGR/OSC sequence wherever a
// style changes, and a whole-frame Contains could match across a row
// boundary that was never contiguous on screen).
func rowsContain(rows []string, needle string) bool {
	for _, row := range rows {
		if strings.Contains(ansiEscapeRe.ReplaceAllString(row, ""), needle) {
			return true
		}
	}
	return false
}
