// Package interactive assembles PRD phase3b Part II's interactive-preview
// transport: one long-lived charmbracelet/x/vt grid fed by a tmux pane's
// live output, armed via internal/tmux's pipe-pane primitive.
package interactive

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	vt "github.com/charmbracelet/x/vt"

	"github.com/n-orlov/deck/internal/tmux"
)

// paneDeadPollInterval is how often the live path polls `#{pane_dead}`
// (PRD II-23). A test may lower this (it is a var, not a const) to keep
// the death-detection assertion fast without changing production
// behaviour.
var paneDeadPollInterval = 200 * time.Millisecond

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

	// deadOnce/deadCh back Dead(): pane_dead polling (below) and Close
	// both call markDead, and exactly one of them may be the one that
	// actually closes deadCh and the pipe.
	deadOnce sync.Once
	deadCh   chan struct{}

	// pollCancel/pollDone stop the pane_dead poll goroutine and let
	// Close wait for it to have actually returned, the same shape
	// drain/done already uses.
	pollCancel context.CancelFunc
	pollDone   chan struct{}

	// pipeGoneOnce/statusMu/status back Status() and the displacement
	// fallback (task 046/II-24): drain calls handlePipeGone exactly once,
	// on the first genuine (non-self-inflicted) EOF, and it alone decides
	// -- by consulting #{pane_pipe} -- whether that EOF meant displaced
	// or disabled.
	pipeGoneOnce sync.Once
	statusMu     sync.Mutex
	status       Status

	// fallbackCh/fallbackDone are the displacement fallback's own
	// trigger/completion pair, always created and always run (grid.go's
	// fallbackLoop below), so Close can unconditionally wait on
	// fallbackDone regardless of whether displacement was ever detected.
	fallbackCh   chan struct{}
	fallbackDone chan struct{}
}

// Status is the live path's own account of why bytes have stopped
// arriving through the pipe, distinct from Dead() (task 045/II-23, which
// answers a different question -- whether the PANE's process has exited).
// Task 046/II-24 requires telling displacement (something else is now
// holding target's only pipe-pane slot; deck falls back to passive
// capture) apart from disablement (nothing is piping target at all right
// now, e.g. deck's own release-on-exit, task 047/II-25).
type Status int

const (
	// StatusLive is the default: the pipe is (as far as this Session
	// knows) still deck's own and still delivering bytes.
	StatusLive Status = iota
	// StatusDisplaced means EOF arrived while #{pane_pipe} was still 1:
	// some other pipe-pane holder has taken target over.
	StatusDisplaced
	// StatusDisabled means EOF arrived while #{pane_pipe} was 0: nothing
	// is piping target right now.
	StatusDisabled
)

func (s *Session) setStatus(st Status) {
	s.statusMu.Lock()
	s.status = st
	s.statusMu.Unlock()
}

// Status reports the live path's current account of the pipe, per the
// Status type's own doc.
func (s *Session) Status() Status {
	s.statusMu.Lock()
	defer s.statusMu.Unlock()
	return s.status
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

	pollCtx, pollCancel := context.WithCancel(context.Background())
	s := &Session{
		pipe:         pipe,
		done:         make(chan struct{}),
		deadCh:       make(chan struct{}),
		pollCancel:   pollCancel,
		pollDone:     make(chan struct{}),
		fallbackCh:   make(chan struct{}),
		fallbackDone: make(chan struct{}),
	}
	s.grid = grid
	go s.drain(client, target)
	go s.pollPaneDead(pollCtx, client, target)
	go s.fallbackLoop(pollCtx, client, target)
	failed = false
	return s, nil
}

// pollPaneDead is the live path's ONLY liveness signal (PRD II-23):
// `pipe-pane`'s stream gives none of its own. Under `remain-on-exit
// failed` (deck's own server default) a dead pane's pipe never closes --
// tmux keeps the pane object, and therefore the still-open write end of
// the FIFO, around for as long as remain-on-exit keeps the pane, which
// under "failed" is forever -- so drain's read(2) blocks indefinitely and
// the grid would otherwise render a stale frame with no way to notice.
// Polling here, independently of drain, is what lets the session notice
// a crashed target at all: on the first observed `pane_dead` (or the
// target vanishing outright, which display-message reports as an error),
// markDead closes the pipe itself, which unblocks drain deterministically
// instead of leaving it parked on a read that would otherwise never
// return, and closes deadCh so a caller selecting on Dead() observes the
// death directly rather than inferring it from drain's side effects.
func (s *Session) pollPaneDead(ctx context.Context, client tmux.Client, target string) {
	defer close(s.pollDone)
	ticker := time.NewTicker(paneDeadPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.done:
			// drain already stopped through some other path (e.g. the
			// pipe was displaced and disarmed, or Close ran); nothing
			// left for this poll to detect or correct.
			return
		case <-ticker.C:
		}
		dead, err := client.PaneDead(ctx, target)
		if err != nil {
			// The target itself is gone (session/pane no longer
			// resolves) -- as terminal as pane_dead==1 for this
			// session's purposes.
			s.markDead()
			return
		}
		if dead {
			s.markDead()
			return
		}
	}
}

// markDead is the one place that closes deadCh and disarms the pipe on a
// detected death; sync.Once makes it safe to call from both the poll
// goroutine and Close without a second close-of-closed-channel panic or a
// second (harmless but redundant) pipe-pane teardown.
func (s *Session) markDead() {
	s.deadOnce.Do(func() {
		close(s.deadCh)
		_ = s.pipe.Close()
	})
}

// Dead returns a channel that is closed once the live path has observed
// (via pane_dead polling, or via Close) that the target pane is gone.
// PRD II-23 requires deck to notice a crashed target itself, since EOF on
// the pipe never arrives for one under remain-on-exit=failed; Dead is
// that notice. Task 046/II-24 builds the displacement-vs-death
// distinction and the panel message on top of this signal.
func (s *Session) Dead() <-chan struct{} { return s.deadCh }

// drain copies every byte the pipe delivers into the session's CURRENT
// grid until the pipe errors (Close makes that happen deterministically).
// It re-reads s.grid under the lock on every iteration rather than
// caching the pointer once, so that a Resize taking effect mid-drain is
// observed by the very next read instead of continuing to feed a grid
// that Resize has already replaced.
//
// A read error is not automatically a shutdown request (task 046/II-24):
// deck's OWN Close (an explicit Session.Close, or markDead's death
// handling) legitimately produces the exact same io.EOF a genuine
// external displacement or disablement does, because Close's disarm
// command makes tmux's job process exit too -- confirmed directly (an
// earlier version of this drain, checking only errors.Is(err, io.EOF),
// misread every ordinary Close as a displacement). It is
// PanePipe.WasClosed(), checked FIRST, not the error's own type, that
// tells "I did this on purpose" apart from "something else happened and
// needs investigating". Only an io.EOF observed while WasClosed() still
// reports false is handed to handlePipeGone.
func (s *Session) drain(client tmux.Client, target string) {
	defer close(s.done)
	buf := make([]byte, 64*1024)
	for {
		n, err := s.pipe.Read(buf)
		if n > 0 {
			g := s.currentGrid()
			_, _ = g.Write(buf[:n])
		}
		if err != nil {
			if !s.pipe.WasClosed() && errors.Is(err, io.EOF) {
				s.handlePipeGone(client, target)
			}
			return
		}
	}
}

// handlePipeGone runs exactly once (pipeGoneOnce), on drain's first
// genuine EOF, and is what actually tells displacement apart from
// disablement (PRD II-24): it re-reads #{pane_pipe} immediately, which
// still reflects whichever of the two actually happened, because neither
// case is reversed by the passage of time on its own. #{pane_pipe} still
// 1 means some other pipe-pane holder has since taken target over
// (displacement); the pipe deck itself armed is unrecoverably gone
// either way, so CloseLocal (never Close -- see its own doc) releases
// deck's own now-orphaned fd/tempdir without touching whatever is
// currently armed. 0 means nothing is piping target right now
// (disablement); Close is fine there since there is nothing live for it
// to disturb.
func (s *Session) handlePipeGone(client tmux.Client, target string) {
	s.pipeGoneOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		stillPiped, err := client.PanePipe(ctx, target)
		if err == nil && stillPiped {
			s.setStatus(StatusDisplaced)
			_ = s.pipe.CloseLocal()
			close(s.fallbackCh)
			return
		}
		s.setStatus(StatusDisabled)
		_ = s.pipe.Close()
	})
}

// pipeDisplacedFallbackInterval is how often the displacement fallback
// (below) re-polls target once its own pipe has been displaced (PRD
// II-24). A test may lower this (it is a var, not a const).
var pipeDisplacedFallbackInterval = 200 * time.Millisecond

// pipeDisplacedNotice is written into the grid -- and therefore into
// whatever eventually renders the grid as the interactive panel -- the
// moment displacement is detected, and again after every fallback
// re-capture (a full reseed replaces the whole grid, notice included).
// PRD II-24 requires deck to SAY that it fell back, in the panel, not
// merely to fall back silently.
const pipeDisplacedNotice = "\r\n[deck: preview pipe displaced by another process -- showing periodic snapshots]\r\n"

// fallbackLoop is always started alongside drain/pollPaneDead, and always
// runs to completion by the time Close returns (Close waits on
// fallbackDone unconditionally) -- but it does nothing at all unless
// handlePipeGone closes fallbackCh, which only happens on a confirmed
// displacement. Once triggered, it is passive preview's own mechanism
// (a periodic capture-pane snapshot, no pipe) substituting for the live
// path that displacement just took away: each tick takes a fresh
// CaptureSeed and writes it into a brand-new grid -- the same
// fresh-parser-per-reseed discipline task 044/II-21-22 established for
// ordinary resizes, for the same reason (a stale parser state has no
// business surviving a full content replacement) -- then re-writes the
// notice, since the fresh grid does not carry it over.
func (s *Session) fallbackLoop(ctx context.Context, client tmux.Client, target string) {
	defer close(s.fallbackDone)
	select {
	case <-ctx.Done():
		return
	case <-s.fallbackCh:
	}

	s.writeNotice(pipeDisplacedNotice)

	ticker := time.NewTicker(pipeDisplacedFallbackInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		data, err := CaptureSeed(ctx, client, target)
		if err != nil {
			// target may be gone entirely; keep trying until ctx is
			// cancelled (Session.Close) rather than giving up on the
			// panel.
			continue
		}
		g := s.currentGrid()
		fresh := newGrid(g.Width(), g.Height())
		if _, err := fresh.Write(data); err != nil {
			continue
		}
		s.mu.Lock()
		s.grid = fresh
		s.mu.Unlock()
		s.writeNotice(pipeDisplacedNotice)
	}
}

// writeNotice writes plain text (no escape sequences of its own) into the
// session's current grid.
func (s *Session) writeNotice(notice string) {
	g := s.currentGrid()
	_, _ = g.Write([]byte(notice))
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
	s.pollCancel()
	err := s.pipe.Close()
	<-s.done
	<-s.pollDone
	<-s.fallbackDone
	return err
}

var _ io.Closer = (*Session)(nil)
