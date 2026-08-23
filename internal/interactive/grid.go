// Package interactive assembles PRD phase3b Part II's interactive-preview
// transport: one long-lived charmbracelet/x/vt grid fed by a tmux pane's
// live output, armed via internal/tmux's pipe-pane primitive.
package interactive

import (
	"context"
	"fmt"
	"io"

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
type Session struct {
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

	s := &Session{grid: grid, pipe: pipe, done: make(chan struct{})}
	go s.drain()
	failed = false
	return s, nil
}

// drain copies every byte the pipe delivers into the session's single
// grid until the pipe errors (Close makes that happen deterministically).
func (s *Session) drain() {
	defer close(s.done)
	buf := make([]byte, 64*1024)
	for {
		n, err := s.pipe.Read(buf)
		if n > 0 {
			_, _ = s.grid.Write(buf[:n])
		}
		if err != nil {
			return
		}
	}
}

// Grid returns the session's single long-lived emulator.
func (s *Session) Grid() *Grid { return s.grid }

// Close disarms the pipe and waits for the drain goroutine to observe the
// resulting read error, so a caller never observes a Session whose drain
// goroutine is still writing into its Grid after Close returns.
func (s *Session) Close() error {
	err := s.pipe.Close()
	<-s.done
	return err
}

var _ io.Closer = (*Session)(nil)
