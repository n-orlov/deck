// Package interactive assembles PRD phase3b Part II's interactive-preview
// transport: one long-lived charmbracelet/x/vt grid fed by a tmux pane's
// live output, via either of the two mechanisms II-5 names
// (TransportPipe, armed via internal/tmux's pipe-pane primitive; or
// TransportCapture, a periodic capture-pane poll -- see the Transport
// type below).
package interactive

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
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

// Transport selects which mechanism a Session uses to keep its Grid fed
// (PRD II-5, task 070). TransportPipe (the default -- see Start) arms
// tmux's `pipe-pane -IO` and streams every byte it delivers into the
// grid (II-16 onward); TransportCapture never touches pipe-pane at all
// and instead re-polls `capture-pane` on capturePollInterval, replacing
// the grid wholesale each tick the same way an ordinary resize reseeds
// (II-21/22). There is no live byte stream to displace under
// TransportCapture -- Status (below) stays StatusLive for a capture
// Session's whole life, since handlePipeGone is a TransportPipe-only
// mechanism and is never reached when there is no pipe armed to begin
// with.
type Transport int

const (
	// TransportPipe is the default (Start's own transport).
	TransportPipe Transport = iota
	// TransportCapture never arms pipe-pane; see captureLoop.
	TransportCapture
)

// capturePollInterval is how often TransportCapture re-polls
// `capture-pane` (task 070/II-5). A test may lower this (it is a var,
// not a const) to keep a capture-transport assertion fast without
// changing production behaviour.
var capturePollInterval = 200 * time.Millisecond

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

// ScrollbackMaxLines bounds every Grid's own scrollback (PRD II-51): vt's
// library default (vt.DefaultScrollbackSize, 10000 lines) is already a
// bound, but it is the LIBRARY's choice, not a stated, deck-owned one, so
// newGrid overrides it here -- the one place every *Grid this package
// constructs is built (grid_test.go's TestExactlyOneGridConstructorCallSite
// guards that invariant) -- rather than leaving it implicit. 2000 is
// chosen from this repository's own measurement
// (docs/reports/phase3b.md's II-51 section, internal/interactive's own
// TestScrollbackMemoryDoesNotGrowWithoutBound): a 120-column grid whose
// scrollback is fully packed with this many content-filled lines costs
// roughly 28 MiB of resident heap over an empty grid's own baseline in
// this container -- comfortably in the same order of magnitude as the
// PRD's own cited (and, in this environment, unreachable -- see
// tasks.json's discovered.prdCorrections) ~53 MiB/pane spike figure,
// unlike an EMPTY grid's own baseline cost, which this repository
// measures at roughly 6 MiB, nowhere near that figure on its own. 2000
// lines is comfortably more history than a preview panel a few dozen
// rows tall needs for one scroll session, while the bound itself is what
// keeps that cost from growing any further no matter how much MORE is
// written through it (the same test proves feeding 40x more input than
// the bound adds well under a further 2% to that cost, not a further
// 40x).
const ScrollbackMaxLines = 2000

func newGrid(width, height int) *Grid {
	g := vt.NewSafeEmulator(width, height)
	g.SetScrollbackSize(ScrollbackMaxLines)
	return g
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
	// transport records which mechanism Start (or StartWithTransport) was
	// asked to use; informational only today (no method branches on it),
	// but kept so a caller/test can confirm which path a Session is
	// actually running.
	transport Transport
	// pipe is nil for a TransportCapture Session (captureLoop never arms
	// pipe-pane, PRD II-5) -- every use of it elsewhere in this file
	// (markDead, Close) is guarded accordingly.
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

	// renders is II-27's render coalescer: every write that changes grid
	// content (drain's per-read Write, the seed write in Start, the
	// fallback loop's re-capture, writeNotice) calls renders.MarkDirty
	// instead of a caller re-rendering directly off of that write, so
	// that a consumer selecting on Renders() sees at most one
	// notification per renderCoalesceInterval no matter how many writes
	// landed in between.
	renders *RenderCoalescer
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

// Start is StartWithTransport pinned to TransportPipe -- every pre-070
// caller/test keeps working unchanged. It arms the pipe pane BEFORE
// calling seed, so that any bytes the pane emits between the two are
// delivered to the pipe (and, once draining starts, written into the
// SAME grid the seed populates) instead of being lost with no way to
// notice -- the ordering itself, not merely the presence of a pipe, is
// what PRD II-16 requires. seed is invoked only after the pipe is armed
// and its returned bytes are written into the grid before the drain
// goroutine (which delivers whatever the pipe already queued) starts
// running.
func Start(ctx context.Context, client tmux.Client, target string, width, height int, seed func(ctx context.Context) ([]byte, error)) (*Session, error) {
	return StartWithTransport(ctx, client, target, width, height, seed, TransportPipe)
}

// StartWithTransport is Start, generalised over PRD II-5's transport
// selector (task 070). Under TransportPipe its behaviour is exactly
// Start's pre-070 behaviour (see Start's own doc for the arm-before-seed
// ordering this preserves). Under TransportCapture, no pipe-pane is ever
// armed -- seed (still called exactly once, at construction, the same
// as TransportPipe) is this Session's only content until captureLoop's
// first tick lands, and every subsequent update comes from captureLoop
// re-polling capture-pane on capturePollInterval, never from a drained
// byte stream.
func StartWithTransport(ctx context.Context, client tmux.Client, target string, width, height int, seed func(ctx context.Context) ([]byte, error), transport Transport) (*Session, error) {
	var pipe *tmux.PanePipe
	if transport == TransportPipe {
		var err error
		pipe, err = client.ArmPipePane(ctx, target)
		if err != nil {
			return nil, fmt.Errorf("arm pipe-pane before seed capture: %w", err)
		}
	}
	failed := true
	defer func() {
		if failed && pipe != nil {
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
		transport:    transport,
		pipe:         pipe,
		done:         make(chan struct{}),
		deadCh:       make(chan struct{}),
		pollCancel:   pollCancel,
		pollDone:     make(chan struct{}),
		fallbackCh:   make(chan struct{}),
		fallbackDone: make(chan struct{}),
		renders:      NewRenderCoalescer(renderCoalesceInterval),
	}
	s.grid = grid
	// The seed write above already changed grid content before renders
	// existed to be told about it; mark it dirty now so the very first
	// coalesced render (once a consumer starts selecting on Renders())
	// reflects the seed, not an empty grid.
	s.renders.MarkDirty()
	switch transport {
	case TransportCapture:
		// There is no pipe to displace under this transport, so the
		// displacement fallback (fallbackLoop) never runs; Close still
		// unconditionally waits on fallbackDone, so it is closed here,
		// immediately, rather than left to a goroutine with nothing to
		// do.
		close(s.fallbackDone)
		go s.captureLoop(pollCtx, client, target)
	default:
		go s.drain(client, target)
		go s.fallbackLoop(pollCtx, client, target)
	}
	go s.pollPaneDead(pollCtx, client, target)
	failed = false
	return s, nil
}

// captureLoop is TransportCapture's sole update source (task 070/II-5):
// unlike fallbackLoop (which only starts once a live PIPE has already
// been displaced, and announces that in the grid), this runs from the
// moment a capture Session starts, polling CaptureSeed every
// capturePollInterval and replacing the grid wholesale on every tick --
// the same fresh-parser-per-reseed discipline task 044/II-21-22
// established for ordinary resizes, applied here as the ordinary steady
// state rather than an exceptional one. No notice is written (unlike
// fallbackLoop's pipeDisplacedNotice): nothing has been displaced, this
// is simply how TransportCapture always works. It closes s.done when it
// returns (ctx cancelled, i.e. Session.Close), the same channel drain
// closes under TransportPipe, so Close's own wait works unmodified
// regardless of which transport a Session was started with. A
// CaptureSeed error (a transient tmux error, or the target vanishing) is
// not treated as fatal here -- it keeps polling until ctx is cancelled;
// pollPaneDead, running independently, is the mechanism that decides
// whether target has actually died.
func (s *Session) captureLoop(ctx context.Context, client tmux.Client, target string) {
	defer close(s.done)
	ticker := time.NewTicker(capturePollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		data, err := CaptureSeed(ctx, client, target)
		if err != nil {
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
		s.renders.MarkDirty()
	}
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
		if s.pipe != nil {
			_ = s.pipe.Close()
		}
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
			// Takes s.mu (not merely to fetch the grid pointer the way
			// currentGrid does, but held across the Write itself) so that
			// RenderRows -- which composes a scrolled view out of several
			// separate SafeEmulator calls and therefore needs the WHOLE
			// composition to see a consistent grid -- can never observe a
			// Write landing partway through its own read of scrollback/
			// screen state. See RenderRows' own doc for the gotcha this
			// avoids (task 085's CellAt-after-return race, at its first
			// production call site rather than a test helper).
			s.mu.Lock()
			_, _ = s.grid.Write(buf[:n])
			s.mu.Unlock()
			// One MarkDirty per READ, never per byte and never a
			// render itself -- this is exactly the point II-27 makes:
			// a consumer rendering directly here, once per read, is
			// the expensive baseline the coalescer exists to replace.
			s.renders.MarkDirty()
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
		s.renders.MarkDirty()
		s.writeNotice(pipeDisplacedNotice)
	}
}

// writeNotice writes plain text (no escape sequences of its own) into the
// session's current grid. Takes s.mu around the Write itself, the same as
// drain's own per-read Write (see drain's doc for why): a caller of
// RenderRows must never observe this write half-applied.
func (s *Session) writeNotice(notice string) {
	s.mu.Lock()
	_, _ = s.grid.Write([]byte(notice))
	s.mu.Unlock()
	s.renders.MarkDirty()
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

// RenderRows composes exactly `height` rows of the session's current
// view, `offset` lines back from the live bottom (PRD II-51's scrollback:
// offset 0 is byte-for-byte what Grid().Render() split on "\n" would
// give -- the pre-068 behaviour -- and a positive offset pulls rows from
// the grid's own bounded scrollback, oldest at the top). offset is
// clamped against the CURRENT scrollback length (which only grows as
// content actually scrolls off, and is itself bounded at
// ScrollbackMaxLines) and the clamped value actually used is returned as
// usedOffset, so a caller's own stored offset can be kept in bounds from
// this single call rather than a second, separately-locked one racing
// this one.
//
// This takes the session's OWN lock (s.mu) for the WHOLE composition,
// not merely to fetch the grid pointer the way currentGrid does.
// Composing a scrolled view needs MORE than one SafeEmulator call
// (Scrollback(), then that Scrollback's own Len()/Line() methods, plus
// Render() for the current screen), and unlike a single self-contained
// call such as Render() -- whose own brief internal lock covers the
// ENTIRE encode and hands back a plain string with no live pointers --
// a value obtained from Scrollback() is a pointer to the SAME live
// object a concurrent Write can still be mutating (Push-ing a new line,
// evicting the oldest) after Scrollback()'s own lock has already
// released on return. That is the exact CellAt/Scrollback-pointer-
// after-return gotcha task 085 fixed in test code (grid_test.go's
// gridContains); it is fixed here, at its first PRODUCTION call site,
// by making every content-mutating call this package makes (drain's
// per-read Write, writeNotice) take the SAME s.mu this does, so no
// Write can land while a RenderRows call is in progress.
func (s *Session) RenderRows(offset, height int) (rows []string, usedOffset int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	g := s.grid
	width := g.Width()
	screenHeight := g.Height()
	sb := g.Scrollback()
	sbLen := sb.Len()

	if offset < 0 {
		offset = 0
	}
	if offset > sbLen {
		offset = sbLen
	}

	screenLines := strings.Split(g.Render(), "\n")
	total := sbLen + screenHeight
	end := total - offset
	start := end - height

	rows = make([]string, 0, height)
	blank := strings.Repeat(" ", width)
	for i := start; i < end; i++ {
		switch {
		case i < 0:
			rows = append(rows, blank)
		case i < sbLen:
			rows = append(rows, sb.Line(i).Render())
		default:
			y := i - sbLen
			if y >= 0 && y < len(screenLines) {
				rows = append(rows, screenLines[y])
			} else {
				rows = append(rows, blank)
			}
		}
	}
	return rows, offset
}

// Renders delivers one notification per coalesced render (PRD II-27), at
// most once per renderCoalesceInterval, however many writes into the
// grid happened in between. A caller renders by calling Grid().Render()
// each time it receives on this channel -- Renders() itself carries no
// frame data, only the "a repaint is due" signal, so the render always
// reflects whatever the grid holds at the moment the caller acts on it,
// not a snapshot captured when the notification was produced.
func (s *Session) Renders() <-chan struct{} { return s.renders.Renders() }

// Close disarms the pipe (TransportPipe only -- a TransportCapture
// Session has none, s.pipe is nil, see the Session struct's own doc) and
// waits for the update goroutine (drain under TransportPipe, captureLoop
// under TransportCapture -- pollCancel below stops the latter directly,
// since it has no pipe read to unblock it) to observe the resulting
// shutdown, so a caller never observes a Session whose update goroutine
// is still writing into its Grid after Close returns.
func (s *Session) Close() error {
	s.pollCancel()
	var err error
	if s.pipe != nil {
		err = s.pipe.Close()
	}
	<-s.done
	<-s.pollDone
	<-s.fallbackDone
	s.renders.Close()
	return err
}

var _ io.Closer = (*Session)(nil)
