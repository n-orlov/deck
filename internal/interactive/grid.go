// Package interactive assembles PRD phase3b Part II's interactive-preview
// transport: one long-lived charmbracelet/x/vt grid fed by a tmux pane's
// live output, armed via internal/tmux's pipe-pane primitive.
package interactive

import (
	"context"
	"fmt"
	"io"
	"sync"

	vt "github.com/charmbracelet/x/vt"

	"github.com/n-orlov/deck/internal/tmux"
)

// Grid is the SAME emulator instance for the whole life of one Session: the
// seed capture (task 042/II-17-18) is written into it before the pipe's
// bytes are drained, and every byte pipe-pane delivers afterward is written
// into it too. There is exactly one vt.NewSafeEmulator call in this
// package's non-test source (grep proves it -- see
// TestExactlyOneGridConstructorCallSite in grid_test.go): a session that
// needed a second emulator for the seed and a third for the live stream
// would silently diverge the moment those two disagreed, which is the
// failure mode task 042 rules out by construction rather than by
// convention.
type Grid = vt.SafeEmulator

func newGrid(width, height int) *Grid {
	return vt.NewSafeEmulator(width, height)
}

// Session streams one selected tmux pane's output into its own Grid via a
// pipe-pane -IO connection armed before any seed capture (PRD II-16).
//
// grid is guarded by mu because Resize (task 044/II-21-22) replaces it
// with a brand-new instance while drain is concurrently writing into
// whatever the CURRENT grid is -- without the lock, a resize racing the
// drain goroutine could write live bytes into the about-to-be-discarded
// old grid, or Grid() could hand a caller a pointer that Resize swaps
// out from under it mid-read.
type Session struct {
	mu   sync.RWMutex
	grid *Grid
	pipe *tmux.PanePipe
	done chan struct{}
}

// Start arms the pipe pane BEFORE calling seed, so that any bytes the pane
// emits between the two are delivered to the pipe (and, once draining
// starts, written into the SAME grid the seed populates) instead of being
// lost with no way to notice -- the ordering itself, not merely the
// presence of a pipe, is what PRD II-16 requires. seed is invoked only
// after the pipe is armed and its returned bytes are written into the
// grid before the drain goroutine (which delivers whatever the pipe
// already queued) starts running.
func Start(ctx context.Context, client tmux.Client, target string, width, height int, seed func(ctx context.Context) ([]byte, error)) (*Session, error) {
	pipe, err := client.ArmPipePane(ctx, target)
	if err != nil {
		return nil, fmt.Errorf("arm pipe-pane before seed capture: %w", err)
	}
	failed := true
	defer func() {
		if failed {
			_ = pipe.Close()
		}
	}()

	grid := newGrid(width, height)

	if seed != nil {
		data, err := seed(ctx)
		if err != nil {
			return nil, fmt.Errorf("seed capture: %w", err)
		}
		if _, err := grid.Write(data); err != nil {
			return nil, fmt.Errorf("write seed capture into grid: %w", err)
		}
	}

	s := &Session{pipe: pipe, done: make(chan struct{})}
	s.grid = grid
	go s.drain()
	failed = false
	return s, nil
}

// drain copies every byte the pipe delivers into the session's CURRENT
// grid until the pipe errors (Close makes that happen deterministically).
// It re-reads s.grid under the lock on every iteration rather than
// caching the pointer once, so that a Resize taking effect mid-drain is
// observed by the very next read instead of continuing to feed a grid
// that Resize has already replaced.
func (s *Session) drain() {
	defer close(s.done)
	buf := make([]byte, 64*1024)
	for {
		n, err := s.pipe.Read(buf)
		if n > 0 {
			g := s.currentGrid()
			_, _ = g.Write(buf[:n])
		}
		if err != nil {
			return
		}
	}
}

// currentGrid returns the session's grid as of right now, under the
// read lock.
func (s *Session) currentGrid() *Grid {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.grid
}

// Grid returns the session's current long-lived emulator. Note "current":
// after a Resize (task 044/II-21-22) this is a DIFFERENT instance than
// whatever a caller may have observed before, by design -- a reseed
// always replaces the grid rather than mutating the old one's canvas.
func (s *Session) Grid() *Grid { return s.currentGrid() }

// Close disarms the pipe and waits for the drain goroutine to observe the
// resulting read error, so a caller never observes a Session whose drain
// goroutine is still writing into its Grid after Close returns.
func (s *Session) Close() error {
	err := s.pipe.Close()
	<-s.done
	return err
}

var _ io.Closer = (*Session)(nil)
