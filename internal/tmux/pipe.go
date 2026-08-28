package tmux

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

// interactivePipeTempDirPrefix names the prefix every interactive-pipe temp
// dir carries (PRD R89, `/tmp/deck-interactive-pipe-*`) -- both the prefix
// ArmPipePane's own os.MkdirTemp call uses, and the one
// ReclaimLeakedInteractivePipes (reclaim.go) matches directory names
// against, so the two ends of one leak/reclaim pair can never drift apart
// by one of them being edited alone.
const interactivePipeTempDirPrefix = "deck-interactive-pipe-"

// interactivePipeTempRoot is the parent directory ArmPipePane's own
// os.MkdirTemp call creates each pipe's temp dir under, and the directory
// ReclaimLeakedInteractivePipes scans. It is a var, not a hard-coded ""
// (which os.MkdirTemp/os.TempDir would otherwise resolve to the real OS
// temp directory), purely so a test can point both ends at an isolated
// t.TempDir() instead of racing every other process on the machine that
// also happens to share the real /tmp (task 030). Production code never
// sets it; the zero value ("") is os.MkdirTemp's own "use os.TempDir()"
// sentinel.
var interactivePipeTempRoot = ""

// PanePipe is a long-lived stream of one pane's raw bytes, produced by
// `pipe-pane -IO` (PRD phase3b II-16). It is the ONLY primitive that reads
// a pane's live output; nothing else in this package taps pipe-pane, and
// a pane may only be connected to one command at a time (task 046/II-24
// covers what happens when something else displaces it).
//
// Close disarms the pipe and removes the temporary FIFO. It is idempotent
// and safe to call after a Read has already failed. closeMu makes it also
// safe to call CONCURRENTLY from two goroutines (task 045/II-23: the
// pane_dead poll goroutine and an explicit Session.Close can both race to
// call Close on the same PanePipe once a death is detected) -- without
// it, two overlapping calls could each read/mutate disarmed and fifo
// unsynchronized, which is a real data race, not merely a hypothetical
// one (caught by go test -race while building task 045).
type PanePipe struct {
	client   Client
	target   string
	fifo     *os.File
	tempDir  string
	closeMu  sync.Mutex
	disarmed bool

	// leftover holds any bytes waitForFifoWriter (below) consumed off the
	// raw fd while probing for the real writer's connection, before Read
	// ever ran for real -- Read drains this first so those bytes are
	// delivered to the caller exactly once, in order, rather than lost.
	leftover []byte
}

// ArmPipePane starts `pipe-pane -IO` on target, writing into a fresh named
// pipe that this call opens and returns (as the embedded reader). Call
// this BEFORE taking any seed capture of the pane (PRD II-16): pipe-pane
// only streams bytes emitted from the moment it is armed onward, so a
// snapshot taken first and armed second loses everything the pane emits
// in between, permanently and undetectably -- the snapshot already ran,
// and the pipe was not listening yet, so neither side ever sees those
// bytes. Arming first means any such interstitial bytes are queued by the
// pipe and delivered to the reader once draining starts, even though they
// arrived before the caller ever asked for a seed.
//
// The reader's own descriptor must be a genuine one-way O_RDONLY handle,
// not O_RDWR (task 046/II-24). O_RDWR was tried first because a plain
// O_RDONLY open blocks until *something* opens the write side, which does
// not exist yet at this point in the call -- but holding our own fd open
// for writing too means the kernel always sees at least one writer on
// the FIFO for as long as OUR fd is open, so a real Read(2) on it can
// never observe EOF even once tmux's own pipe-pane job (the actual writer)
// goes away: displacement and disablement (both of which end that job
// cleanly) would then look identical to a live, merely-quiet pipe --
// confirmed directly, not assumed (a throwaway O_RDWR reader against a
// displaced pipe timed out waiting for an EOF that never came). The fix
// used here instead is the standard one-open trick: open O_RDONLY with
// O_NONBLOCK set, which POSIX guarantees returns immediately even with
// no writer yet present, rather than blocking.
//
// The fd is deliberately left O_NONBLOCK for the rest of this type's
// life, rather than cleared via fcntl once a writer is confirmed. Two
// unrelated things both depend on that: (1) waitForFifoWriter below
// polls the raw fd with non-blocking reads to positively confirm tmux's
// job has actually connected before this call returns (#{pane_pipe}
// flips true as soon as tmux accepts the arm command, well before the
// job it just forked has gotten around to its own open() of this fifo --
// confirmed directly, not assumed), and (2) os.NewFile only registers a
// descriptor with the Go runtime's poller if it is ALREADY non-blocking
// at the moment NewFile is called -- clearing it first produces a File
// whose Read the runtime cannot interrupt, so Close from another
// goroutine no longer unblocks a concurrently blocked Read at all
// (confirmed directly: this hung TestSessionNoticesAndClosesDownOnADead
// PaneUnderRemainOnExitFailed, in internal/interactive, instead of
// failing it, until found and fixed). Poller registration does not
// change Read's blocking behaviour as this type's own callers see it --
// it still blocks the calling goroutine exactly as a plain blocking
// O_RDONLY open would; it only changes HOW that block is implemented
// (parked in the runtime, interruptible by Close, rather than stuck in
// the read(2) syscall itself), and it still delivers a genuine io.EOF
// the moment the writer side actually closes, with zero bytes lost for
// output already queued ahead of that close -- confirmed directly.
func (c Client) ArmPipePane(ctx context.Context, target string) (*PanePipe, error) {
	tempDir, err := os.MkdirTemp(interactivePipeTempRoot, interactivePipeTempDirPrefix)
	if err != nil {
		return nil, fmt.Errorf("create pipe temp dir: %w", err)
	}
	p := &PanePipe{client: c, target: target, tempDir: tempDir}
	failed := true
	defer func() {
		if failed {
			_ = os.RemoveAll(tempDir)
		}
	}()

	fifoPath := filepath.Join(tempDir, "pane.fifo")
	if output, err := exec.CommandContext(ctx, "mkfifo", fifoPath).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("mkfifo %s: %w: %s", fifoPath, err, output)
	}

	fd, err := unix.Open(fifoPath, unix.O_RDONLY|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, fmt.Errorf("open pane fifo %s: %w", fifoPath, err)
	}
	failedFd := true
	defer func() {
		if failedFd {
			_ = unix.Close(fd)
		}
	}()

	if _, err := c.run(ctx, "pipe-pane", "-IO", "-t", target, "cat >> "+shellQuote(fifoPath)); err != nil {
		return nil, fmt.Errorf("arm pipe-pane -IO on %s: %w", target, err)
	}

	// tmux's own bookkeeping (#{pane_pipe}) flips the instant it accepts
	// the command above, well before the job it just forked has actually
	// reached ITS OWN open() of fifoPath (fork+exec+the shell's own ">>"
	// redirection are not instantaneous). Reading from our still-O_NONBLOCK
	// fd right now, before that job's writer has actually connected, would
	// see a fifo with zero writers -- indistinguishable from a genuine EOF
	// (confirmed directly: both return n=0, err=nil) even though nothing
	// has gone wrong at all. waitForFifoWriter blocks (via short polling,
	// not by leaving O_NONBLOCK set on the fd this type hands callers)
	// until it can positively confirm a writer -- EAGAIN on a non-blocking
	// read means "empty, but a writer IS attached", not "no writer yet",
	// confirmed as the discriminator directly.
	leftover, err := waitForFifoWriter(fd, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("wait for pipe-pane's job to connect to %s: %w", fifoPath, err)
	}
	p.leftover = leftover

	// The fd stays O_NONBLOCK. This is deliberate, not an oversight left
	// over from probing above: os.NewFile only registers a descriptor
	// with the Go runtime's poller if it is ALREADY non-blocking at the
	// moment NewFile is called (confirmed directly -- clearing it first,
	// the way an earlier version of this function did, produced a File
	// whose Read the runtime cannot interrupt: Close() from another
	// goroutine no longer unblocks a concurrently blocked Read at all,
	// which made TestSessionNoticesAndClosesDownOnADeadPaneUnderRemainOnExitFailed
	// hang instead of failing loudly, until this was found and fixed).
	// Poller registration does not change Read's blocking behaviour as
	// this type's own callers see it -- os.File.Read still blocks the
	// calling goroutine exactly as before; it only changes HOW that
	// block is implemented (parked in the runtime, interruptible by
	// Close, rather than stuck in the read(2) syscall itself).
	p.fifo = os.NewFile(uintptr(fd), fifoPath)
	failedFd = false

	failed = false
	return p, nil
}

// waitForFifoWriter polls fd (opened O_RDONLY|O_NONBLOCK) with raw
// non-blocking reads until it can tell, unambiguously, that a writer is
// now attached: EAGAIN means the fifo is empty but a writer exists (a
// real read would block); n>0 means a writer not only exists but has
// already written something, which is returned so the caller can
// deliver it rather than discard it (fifos have no "peek"). n==0/err==nil
// means no writer has connected YET (indistinguishable, by return value
// alone, from a writer having connected and already gone again -- but
// this early, right after arming, that second reading is not yet
// possible) and is retried until timeout.
func waitForFifoWriter(fd int, timeout time.Duration) ([]byte, error) {
	deadline := time.Now().Add(timeout)
	buf := make([]byte, 64*1024)
	for {
		n, err := unix.Read(fd, buf)
		if n > 0 {
			return buf[:n], nil
		}
		if err == nil {
			// n == 0, err == nil: no writer connected yet.
		} else if err == unix.EAGAIN {
			return nil, nil
		} else {
			return nil, err
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timed out after %s waiting for pipe-pane's job to open the fifo", timeout)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// Read satisfies io.Reader by reading raw pane bytes off the FIFO. It
// blocks like any other stream read; it returns an error once Close has
// closed the underlying descriptor. Any bytes ArmPipePane's own
// writer-connection probe already consumed off the raw fd (leftover) are
// delivered first, in order, exactly once.
func (p *PanePipe) Read(b []byte) (int, error) {
	if len(p.leftover) > 0 {
		n := copy(b, p.leftover)
		p.leftover = p.leftover[n:]
		return n, nil
	}
	return p.fifo.Read(b)
}

// TempDir returns the temp directory this PanePipe created its FIFO under
// (task 030/R89: the caller that just armed this pipe records enough
// metadata inside it -- via SaveInteractiveClaimRecord -- for a LATER
// process's ReclaimLeakedInteractivePipes to find and reclaim it if this
// one never gets to call Close itself, e.g. because it was SIGKILLed).
// Valid for the whole life of a PanePipe, including after Close/CloseLocal
// has already removed the directory on disk.
func (p *PanePipe) TempDir() string { return p.tempDir }

// WasClosed reports whether Close or CloseLocal has already run on this
// PanePipe -- the discriminator a caller's Read-error handling needs
// (task 046/II-24): Close's own disarm command legitimately produces the
// exact same io.EOF a genuine external displacement or disablement does
// (confirmed directly: disarming pipe-pane makes tmux's job process exit,
// closing the FIFO's write side, which is indistinguishable in the raw
// Read(2) error from someone ELSE having done the same thing), so io.EOF
// alone cannot tell an intentional shutdown apart from one that needs
// investigating. Callers must check this FIRST, before inspecting the
// error's type at all: an EOF observed after WasClosed reports true
// means "I did this on purpose"; one observed while it is still false
// means the caller needs to find out why on its own (re-reading
// #{pane_pipe}, per PanePipe below).
func (p *PanePipe) WasClosed() bool {
	p.closeMu.Lock()
	defer p.closeMu.Unlock()
	return p.disarmed
}

// Close disarms `pipe-pane` on the target (a bare `pipe-pane -t target`
// with no command disables it, per tmux(1)) and removes the FIFO. It is
// idempotent: calling it twice, or after the target session/pane has
// already gone away, never returns an error the caller has to handle
// specially -- disarming a pipe that is not this one's anymore (task
// 046/II-24's displacement case) is not this type's problem to detect.
//
// Close must NEVER be called once displacement has already been detected
// (that is exactly what CloseLocal is for): by then target's pipe-pane
// belongs to whoever displaced deck, and a bare `pipe-pane -t target`
// addresses whatever is CURRENTLY armed on target, not specifically
// deck's own former one -- calling it here would disarm the new holder's
// pipe as a side effect, not merely deck's already-gone one.
func (p *PanePipe) Close() error {
	p.closeMu.Lock()
	defer p.closeMu.Unlock()
	if p.disarmed {
		return nil
	}
	p.disarmed = true

	ctx, cancel := context.WithTimeout(context.Background(), p.client.timeout())
	defer cancel()
	_, _ = p.client.run(ctx, "pipe-pane", "-t", p.target)

	return p.closeLocal()
}

// CloseLocal releases only this process's own resources (the reader fd
// and the temp dir/FIFO) without issuing tmux's disarm command. Use this
// once a genuine EOF has already shown target's pipe-pane was displaced
// by a second holder (task 046/II-24): the pipe deck armed is already
// gone by construction (tmux's single-holder-per-pane rule is exactly why
// the EOF happened at all), and the live `#{pane_pipe}` at that point
// belongs to whoever displaced deck -- disarming it would take down THAT
// holder's pipe, not deck's already-severed one. It is idempotent for
// the same reason Close is.
func (p *PanePipe) CloseLocal() error {
	p.closeMu.Lock()
	defer p.closeMu.Unlock()
	if p.disarmed {
		return nil
	}
	p.disarmed = true
	return p.closeLocal()
}

// closeLocal is the shared fd/tempdir teardown behind both Close and
// CloseLocal. Callers must hold closeMu and must have already set
// disarmed = true.
func (p *PanePipe) closeLocal() error {
	var err error
	if p.fifo != nil {
		err = p.fifo.Close()
		p.fifo = nil
	}
	if removeErr := os.RemoveAll(p.tempDir); err == nil {
		err = removeErr
	}
	return err
}

// shellQuote produces a single-quoted shell word safe to embed in the
// shell-command argument tmux hands to sh -c for pipe-pane/run-shell; the
// only character that needs escaping inside single quotes is a literal
// single quote itself.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
