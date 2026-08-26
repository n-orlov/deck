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
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	uv "github.com/charmbracelet/ultraviolet"
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

// replyDrainBufSize bounds one reply-drain read (see startReplyDrain).
// A capability reply is a handful of bytes; this is simply comfortably
// more than the longest one vt can emit, so a reply is always taken in
// a single read.
const replyDrainBufSize = 4096

// startReplyDrain starts g's reply drain: the per-Session (strictly,
// per-*Grid) reader of Grid.Read() that R68 / issue #5 requires, and
// without which ANY terminal query arriving in a previewed pane's
// output stalls the goroutine that wrote it, forever.
//
// The mechanism, read off charmbracelet/x/vt
// v0.0.0-20260816001655-68d539dca504: an Emulator answers a capability
// query by writing the reply into an UNBUFFERED io.Pipe (emulator.go:102,
// `t.pr, t.pw = io.Pipe()`), whose only reader is Emulator.Read
// (emulator.go:251). An io.Pipe write completes only once a reader takes
// the bytes, so with nobody reading, the very first reply byte parks its
// writer permanently -- and that writer is deck's own drain goroutine,
// inside grid.Write, holding s.mu, which is what escalated one parked
// write into a wedged event loop (RenderRows -> View -> bubbletea's
// eventLoop). Every replying handler is its own trigger: DA1
// (handlers.go:695), DA2 (:712), DSR (:806, :809, :826), DECRQM
// (csi.go:30), the OSC 10/11/12 colour queries (osc.go:87, :92, :97) and
// in-band resize (csi_mode.go:95) -- which is why the fix is a reader,
// not a per-sequence special case.
//
// The reply is DISCARDED, deliberately. deck is a viewer: the pane's
// program already has a real terminal on the other side of tmux, and a
// reply vt synthesised here is not what that terminal would have
// answered. The pipe is armed -IO, so forwarding these bytes into the
// pane IS possible -- but it would feed a live agent a fabricated DA1 it
// never asked deck for, a behaviour change with its own blast radius.
// Dropping them restores exactly the pre-preview situation: the program
// gets no answer from deck's copy of the terminal, the same as when no
// preview is open at all.
//
// SafeEmulator.Read (safe_emulator.go:33) deliberately does NOT take
// se.mu -- it calls straight through to the embedded Emulator -- so a
// reader parked in Read blocks no Write, and this goroutine can sit on a
// quiet grid for the whole life of a Session at zero cost.
func (s *Session) startReplyDrain(g *Grid) {
	s.replyDrains.Add(1)
	go func() {
		defer s.replyDrains.Done()
		buf := make([]byte, replyDrainBufSize)
		for {
			// The bytes are read purely to unpark whoever wrote them;
			// see this function's own doc for why they are dropped.
			if _, err := g.Read(buf); err != nil {
				return
			}
		}
	}()
}

// newDrainedGrid is newGrid plus its reply drain, started BEFORE the
// caller writes a single byte into the returned grid -- which is the
// whole point: a query sitting in whatever is about to be written (a
// seed capture taken with -e, a live stream's next read) would otherwise
// park that very first write. Every grid this constructor hands out must
// end up either passed to installGrid (which retires the grid it
// displaces) or retired directly by retireGrid, or its drain goroutine
// outlives the grid it was reading.
func (s *Session) newDrainedGrid(width, height int) *Grid {
	g := newGrid(width, height)
	s.startReplyDrain(g)
	return g
}

// retireGrid releases a grid this package is finished with (a reseed has
// replaced it, or the Session is closing) and, by doing so, makes its
// reply drain return.
//
// It closes the emulator's reply-pipe WRITE end rather than calling
// Emulator.Close, and the difference matters twice. Closing the write end
// hands the parked reader an io.EOF, so the drain goroutine exits
// deterministically instead of leaking -- captureLoop reseeds five times
// a second, so a drain that could not be retired would be a goroutine
// leak at that rate, not a curiosity. And it leaves vt's own
// Emulator.closed flag alone: that field is written by Close and read
// (unsynchronised) by every Read and Write, so calling Close while a
// reader is live is a data race inside the library -- recorded as a
// finding, not worked around by giving up the reader. After retirement
// the grid still renders exactly as before (only its reply pipe is
// gone), which is what makes it safe for a caller still holding a
// pointer handed out by Grid(); any further reply write now fails
// immediately with io.ErrClosedPipe instead of blocking, which is the
// same non-stalling outcome the drain provides.
func retireGrid(g *Grid) {
	// vt's InputPipe() is the io.Pipe write end the replying handlers
	// write to (emulator.go:297). The type assertion is pinned by
	// TestEmulatorInputPipeIsAPipeWriter, so a go.mod bump that changes
	// it fails loudly here rather than silently leaking drains.
	if pw, ok := g.InputPipe().(*io.PipeWriter); ok {
		_ = pw.CloseWithError(io.EOF)
	}
}

// renderedFrame is one composed RenderRows result, kept so that a
// render arriving while a grid MUTATION is in flight can be served
// without touching the emulator at all -- see RenderRows, which is the
// only producer and the only consumer. Every field is treated as
// immutable once stored (rows is a private copy nobody mutates
// afterwards), so publishing it through an atomic pointer needs no lock
// of its own and can never make a reader wait on a writer.
type renderedFrame struct {
	rows []string
	// usedOffset/scrollbackLen are what RenderRows returned and clamped
	// against when this frame was composed, so a stale hit can still
	// answer the usedOffset half of RenderRows' contract instead of
	// silently resetting a caller's scroll position.
	usedOffset    int
	scrollbackLen int
	// width is the grid width this frame was composed at, so padding a
	// stale frame out to a taller request uses the same blank row
	// RenderRows itself would have produced.
	width int
}

// installGrid publishes fresh as the session's current grid and retires
// the one it replaces, so that exactly one reply drain is live per
// Session in steady state. The swap itself keeps Resize's own contract
// (see Resize's doc): it happens under the write lock, so a concurrent
// RenderRows/Grid() sees the old grid in full or the new one in full,
// never a grid mid-swap.
func (s *Session) installGrid(fresh *Grid) {
	s.mu.Lock()
	old := s.grid
	s.grid = fresh
	s.mu.Unlock()
	if old != nil && old != fresh {
		retireGrid(old)
	}
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
//
// mu guards the grid POINTER only, and is deliberately never held across
// a grid.Write (R68 / issue #5 fix 2): grid MUTATION is serialised by
// the separate `writes` lock below. The one lock-ordering rule that
// keeps the two safe together: a goroutine may take writes while holding
// nothing, and may take mu and then writes, but must NEVER acquire mu
// while holding writes -- which is why writeGrid resolves the grid
// pointer (currentGrid, i.e. mu) BEFORE it takes writes, and why nothing
// that holds writes ever installs a grid.
type Session struct {
	mu   sync.RWMutex
	grid *Grid

	// writes serialises every MUTATION of whatever grid is currently
	// installed (drain's per-read Write and writeNotice, both via
	// writeGrid) against the readers that need several emulator calls to
	// agree with one another (RenderRows, AbsoluteRow, SelectedText).
	// This is the lock that used to be s.mu itself; splitting it out is
	// R68's second fix, and RenderRows' own doc states exactly what the
	// split does and does not guarantee.
	writes sync.RWMutex

	// lastFrame is the most recent successful RenderRows composition, the
	// value RenderRows serves when a write is in flight rather than
	// waiting for it (see RenderRows). atomic, not mutex-guarded, because
	// the whole point of that path is that a repaint never blocks on
	// anything the transport is doing.
	lastFrame atomic.Pointer[renderedFrame]
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

	// replyDrains joins every reply-drain goroutine this Session has
	// started (one per *Grid instance -- see startReplyDrain), so Close
	// never returns while one is still reading an emulator this Session
	// created. Retiring a grid (retireGrid) is what makes each of them
	// return.
	replyDrains sync.WaitGroup

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
	// The grid's reply drain starts before the seed is written into it
	// (see newDrainedGrid): a seed capture is written by THIS goroutine,
	// so a query inside it would park Start itself.
	s.grid = s.newDrainedGrid(width, height)

	failed := true
	defer func() {
		if !failed {
			return
		}
		if pipe != nil {
			_ = pipe.Close()
		}
		// No update goroutine has started yet on this path, so retiring
		// the grid here cannot race an install.
		pollCancel()
		retireGrid(s.currentGrid())
		s.replyDrains.Wait()
	}()

	if seed != nil {
		data, err := seed(ctx)
		if err != nil {
			return nil, fmt.Errorf("seed capture: %w", err)
		}
		if _, err := s.currentGrid().Write(data); err != nil {
			return nil, fmt.Errorf("write seed capture into grid: %w", err)
		}
	}

	// The seed write above changed grid content without going through any
	// of the paths that mark the grid dirty themselves (drain's per-read
	// Write, writeNotice, a reseed); mark it dirty now so the very first
	// coalesced render (once a consumer starts selecting on Renders())
	// reflects the seed, not an empty grid.
	s.renders.MarkDirty()
	// Compose one frame here, before any goroutine that could be writing
	// exists, purely to prime lastFrame: RenderRows serves the previous
	// frame while a write is in flight, and priming means "the previous
	// frame" is the seed rather than nothing at all even if the very
	// first repaint happens to coincide with the first pipe read.
	s.RenderRows(0, height)
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
		fresh := s.newDrainedGrid(g.Width(), g.Height())
		if _, err := fresh.Write(data); err != nil {
			retireGrid(fresh)
			continue
		}
		s.installGrid(fresh)
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
			// writeGrid, never a bare s.grid.Write under s.mu: the
			// session lock must not be held for the duration of a Write
			// (R68 / issue #5 fix 2 -- see writeGrid and RenderRows).
			// It marks the grid dirty itself, once per READ, never per
			// byte and never a render -- exactly the point II-27 makes:
			// a consumer rendering directly here, once per read, is the
			// expensive baseline the coalescer exists to replace.
			s.writeGrid(buf[:n])
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
		fresh := s.newDrainedGrid(g.Width(), g.Height())
		if _, err := fresh.Write(data); err != nil {
			retireGrid(fresh)
			continue
		}
		s.installGrid(fresh)
		s.renders.MarkDirty()
		s.writeNotice(pipeDisplacedNotice)
	}
}

// writeNotice writes plain text (no escape sequences of its own) into the
// session's current grid, through the same writeGrid path drain's
// per-read Write uses -- so a caller of RenderRows never observes this
// write half-applied, and so this write can never hold s.mu.
func (s *Session) writeNotice(notice string) {
	s.writeGrid([]byte(notice))
}

// writeGrid is the ONE place this package mutates a grid that is already
// installed (drain's per-read Write and writeNotice; a reseed's writes go
// into a private grid nobody can render yet, which is why they need none
// of this). It is R68 / issue #5's second fix in three lines:
//
//  1. the grid POINTER is resolved first, under s.mu, and s.mu is
//     released again before a single byte is written -- so no Write,
//     however long it takes, can stall Grid()/currentGrid()/installGrid,
//     or a Resize, or (crucially) RenderRows' fallback path. Before this
//     fix s.mu was held for the whole Write, which is what turned one
//     parked writer into a wedged bubbletea event loop.
//  2. the Write itself happens under s.writes, so a reader that needs
//     several emulator calls to agree with each other still sees a grid
//     no write is landing in the middle of (see RenderRows' doc).
//  3. this order -- s.mu, released, THEN s.writes -- is the lock
//     ordering the Session struct's own doc pins: a holder of s.writes
//     must never go on to acquire s.mu, or a Resize waiting for s.mu
//     could deadlock against a reader waiting for s.writes.
//
// The write is deliberately made against the pointer captured in step 1
// even if a reseed installs a different grid meanwhile: bytes that were
// already in flight belong to the state the reseed superseded, and a
// reseed replaces the whole content anyway (Resize's doc). drain
// re-resolves the pointer on its very next read, which is the behaviour
// its own doc has always promised.
func (s *Session) writeGrid(data []byte) {
	g := s.currentGrid()
	s.writes.Lock()
	_, _ = g.Write(data)
	s.writes.Unlock()
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
// This composition needs MORE than one SafeEmulator call (Scrollback(),
// then that Scrollback's own Len()/Line() methods, plus Render() for the
// current screen), and unlike a single self-contained call such as
// Render() -- whose own brief internal lock covers the ENTIRE encode and
// hands back a plain string with no live pointers -- a value obtained
// from Scrollback() is a pointer to the SAME live object a concurrent
// Write can still be mutating (Push-ing a new line, evicting the oldest)
// after Scrollback()'s own lock has already released on return. That is
// the exact CellAt/Scrollback-pointer-after-return gotcha task 085 fixed
// in test code (grid_test.go's gridContains); at this, its first
// PRODUCTION call site, THREE things -- and no lock held across a Write
// anywhere in the package -- are what keep the composition consistent:
//
//  1. s.mu (read) is held for the whole composition, so every call below
//     runs against ONE grid instance: a reseed's swap (installGrid, which
//     takes s.mu for writing) happens either entirely before this call or
//     entirely after it. Since R68 fix 2 nothing holds s.mu across a
//     Write, so holding it here costs a repaint nothing.
//  2. s.writes (read) excludes grid MUTATION for that same span, and THAT
//     is what actually makes the multi-call read atomic: every mutation
//     of an installed grid goes through writeGrid, which takes s.writes
//     for writing.
//  3. that second lock is taken with TryRLock, NOT RLock -- the
//     behavioural difference R68's second fix delivers. If a write is in
//     flight this call does not wait for it and does not touch the
//     emulator at all (a parked writer also holds vt's own se.mu, so even
//     g.Width() would block behind it): it returns the previously
//     composed frame, immediately. A repaint is therefore never coupled
//     to the transport's progress, which is exactly how issue #5
//     escalated one stalled Write into a wedged bubbletea event loop.
//
// Serving the previous frame can only ever be ONE frame stale, never a
// permanently stale panel: writeGrid marks the grid dirty AFTER it
// releases s.writes, so the coalesced render that write triggers (PRD
// II-27) finds the lock free unless yet another write is already in
// flight -- and that write will mark it dirty in turn. The last write
// always ends in a fresh composition.
func (s *Session) RenderRows(offset, height int) (rows []string, usedOffset int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.writes.TryRLock() {
		return s.staleRows(offset, height)
	}
	defer s.writes.RUnlock()
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
	s.lastFrame.Store(&renderedFrame{
		// A private copy: the caller owns what it is handed (internal/tui
		// prepends the II-49 notice to it), and a cached frame a caller
		// could mutate would corrupt every later stale hit.
		rows:          slices.Clone(rows),
		usedOffset:    offset,
		scrollbackLen: sbLen,
		width:         width,
	})
	return rows, offset
}

// staleRows is RenderRows' answer while a grid mutation is in flight: the
// previously composed frame, adapted to the requested height, with no
// emulator call of any kind (RenderRows' point 3 says why touching the
// emulator would defeat the purpose). Padding goes at the TOP and
// cropping keeps the LAST rows, both matching what RenderRows itself does
// when the view is taller than the content -- the live bottom edge is the
// part a viewer is looking at.
func (s *Session) staleRows(offset, height int) (rows []string, usedOffset int) {
	if height < 0 {
		height = 0
	}
	if offset < 0 {
		offset = 0
	}
	last := s.lastFrame.Load()
	if last == nil {
		// No frame has ever been composed for this Session. Start primes
		// one (see its seed handling), so in production this is only
		// reachable on a directly-constructed Session in a test: empty
		// rows, and the caller's own offset back unchanged, since there
		// is no scrollback length to clamp it against and inventing 0
		// would silently reset a scroll position.
		return make([]string, height), offset
	}
	if offset > last.scrollbackLen {
		offset = last.scrollbackLen
	}
	out := make([]string, 0, height)
	blank := strings.Repeat(" ", last.width)
	for i := len(last.rows); i < height; i++ {
		out = append(out, blank)
	}
	start := 0
	if len(last.rows) > height {
		start = len(last.rows) - height
	}
	out = append(out, last.rows[start:]...)
	return out, offset
}

// AbsoluteRow converts a view-relative row -- 0 at the top of whatever
// RenderRows(offset, height) most recently rendered onto the screen a
// user's drag was actually resolved against, the only coordinate space a
// mouse event's hit-tested cell can be expressed in -- into the SAME
// absolute row space RenderRows/SelectedText both use. It re-derives the
// identical start/end arithmetic RenderRows itself uses (clamping offset
// against the CURRENT scrollback length exactly like RenderRows does),
// rather than have a caller duplicate that math independently and risk
// the two silently drifting apart the moment either changes.
func (s *Session) AbsoluteRow(offset, height, viewRow int) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	// Blocking RLock, not RenderRows' TryRLock: this resolves ONE mouse
	// gesture rather than a per-frame repaint, and a wrong row number is a
	// wrong selection (not a one-frame-stale picture), so exactness is
	// worth waiting out an in-flight write. That wait is bounded by the
	// Write itself, which R68's FIRST fix (the reply drain -- see
	// startReplyDrain) is what keeps finite.
	s.writes.RLock()
	defer s.writes.RUnlock()
	g := s.grid
	sbLen := g.ScrollbackLen()
	if offset < 0 {
		offset = 0
	}
	if offset > sbLen {
		offset = sbLen
	}
	total := sbLen + g.Height()
	end := total - offset
	start := end - height
	return start + viewRow
}

// SelectedText answers steer 017 item 3 / SPEC §11.8's drag-to-copy
// selection (task 216): the PLAIN (unstyled) text of the linear,
// reading-order run from (fromCol, fromRow) to (toCol, toRow), in the
// exact SAME absolute row space RenderRows uses -- row 0 is the oldest
// scrollback line, row sbLen+screenHeight-1 is the live bottom row -- so
// a caller that resolved both endpoints against one RenderRows call (or
// the same scroll offset) can hand them here unchanged. "Linear" (not
// rectangular/block) selection is what an ordinary terminal's own
// click-drag does and what tmux's own default copy-mode selection does
// without a modifier: a multi-row run includes every column of every
// row strictly between the two endpoints, only the first selected row's
// tail (from fromCol onward) and the last selected row's head (up to
// toCol) are partial. The two endpoints are swapped first if the drag
// ran backwards (a release above its own press, or leftward on the same
// row), so a drag performed in either direction over the same two cells
// selects identical text -- exactly what a real terminal's own
// click-drag selection guarantees. Trailing spaces (padding, never real
// content -- newGrid's blank cells are always " ") are trimmed from
// every selected row, matching what a user reading the screen actually
// perceives as the row's content.
//
// This takes the SAME two locks AbsoluteRow does, in the same order,
// over the SAME CellAt/ScrollbackCellAt pointer-after-return hazard
// RenderRows's own doc comment covers: s.mu (read) pins one grid instance
// for the whole extraction, s.writes (read) excludes grid mutation for
// it, so no concurrent Write can land mid-read. The s.writes acquisition
// blocks here for exactly AbsoluteRow's reason -- a gesture needs the
// real cells, not a stale approximation of them.
func (s *Session) SelectedText(fromCol, fromRow, toCol, toRow int) string {
	if fromRow > toRow || (fromRow == toRow && fromCol > toCol) {
		fromCol, fromRow, toCol, toRow = toCol, toRow, fromCol, fromRow
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	s.writes.RLock()
	defer s.writes.RUnlock()
	g := s.grid
	width := g.Width()
	screenHeight := g.Height()
	sbLen := g.ScrollbackLen()
	total := sbLen + screenHeight
	if total <= 0 || width <= 0 {
		return ""
	}
	clampRow := func(r int) int {
		if r < 0 {
			return 0
		}
		if r >= total {
			return total - 1
		}
		return r
	}
	clampCol := func(c int) int {
		if c < 0 {
			return 0
		}
		if c >= width {
			return width - 1
		}
		return c
	}
	fromRow, toRow = clampRow(fromRow), clampRow(toRow)
	fromCol, toCol = clampCol(fromCol), clampCol(toCol)

	cellAt := func(x, i int) *uv.Cell {
		if i < sbLen {
			return g.ScrollbackCellAt(x, i)
		}
		return g.CellAt(x, i-sbLen)
	}

	lines := make([]string, 0, toRow-fromRow+1)
	for i := fromRow; i <= toRow; i++ {
		startCol, endCol := 0, width-1
		if i == fromRow {
			startCol = fromCol
		}
		if i == toRow {
			endCol = toCol
		}
		var b strings.Builder
		for x := startCol; x <= endCol; x++ {
			if c := cellAt(x, i); c != nil {
				b.WriteString(c.Content)
			}
		}
		lines = append(lines, strings.TrimRight(b.String(), " "))
	}
	return strings.Join(lines, "\n")
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
	// Every goroutine that could install a grid has returned by now, so
	// the current grid is the last one: retire it and join its reply
	// drain (every earlier grid's drain was already joined when
	// installGrid retired it), so no goroutine this Session started is
	// still reading an emulator after Close returns.
	retireGrid(s.currentGrid())
	s.replyDrains.Wait()
	s.renders.Close()
	return err
}

var _ io.Closer = (*Session)(nil)
