package features

import (
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/x/vt"
)

// TestScreenDriverReadNeverSignalsAPartialMergeOfAnOversizedWrite is task
// cure-01-01's regression guard (review finding B1) for the "hint chrome"
// flake (/run/ralphd/artifacts/stability/analysis/hint_chrome-flake.md and
// themes.feature:96's "a built-in theme colours border_focus/border,
// selection/selection_idle, title, group and key/hint chrome, read per
// cell from a real client" scenario failing its `text "attach" has
// foreground token "hint"` step), fixed by 1c57a3ebb8 (task 017, R173).
//
// This drives the real, unmodified ScreenDriver.read() -- the exact seam
// task 017 changed, present under that same name and signature on both
// trees -- rather than drainPTYChunk, the pure-function merge step
// 1c57a3ebb8 itself introduced (which does not exist before the fix, and
// so cannot be the seam: a test built around it cannot even compile
// against a192accf7d). No part of read()'s own logic is reimplemented
// here; this test only ever reads driver.raw.Len() through driver.mu,
// exactly as ScreenDriver.Raw() itself does.
//
// Forcing the real race by placing a SINGLE external waiter behind
// read()'s own first critical section (hold d.mu, start read(), queue one
// more Lock() attempt, release) was tried first and found NOT to work:
// empirically, in this environment, a goroutine's own immediate
// re-Lock() right after its own Unlock() always wins over an
// already-queued separate goroutine (confirmed by a standalone
// experiment: 20/20 runs), because the second goroutine must first be
// woken and rescheduled while the first simply continues running --
// sync.Mutex's starvation mode never actually engages here, since the
// mutex is uncontended at the moment each waiter wakes. See this task's
// own investigation notes.
//
// This test instead uses a POOL of goroutines continuously polling
// mu.TryLock() (added in Go 1.18) across GOMAXPROCS cores, started BEFORE
// read() does, so at least one is always actively contending rather than
// parked. Confirmed empirically (standalone experiment, this task): with
// 16 such pollers, every one of 2 independent runs caught the unfixed
// tree's torn intermediate state (raw.Len() == 4096, i.e. only the first
// physical Read()'s own 4096-byte buffer, with the remaining 777 bytes
// already sitting in the pipe but not yet merged in) during the real
// read() goroutine's own window between its first Unlock() and its
// second Lock() -- a window that exists on the unfixed tree (one Lock
// per physical Read()) but can never exist on the fixed tree, because
// 1c57a3ebb8 moved the whole merge (drainPTYChunk, polling ptyReadable)
// BEFORE the first d.mu.Lock() of a logical flush, so there is only ever
// ONE critical section, carrying every byte already queued, and the
// pollers can only ever observe raw.Len() as 0 (before) or the full
// merged length (after) -- never anything strictly in between.
//
// Unmodified in a git-archive export of a192accf7d, this file compiles
// (every identifier it names -- ScreenDriver, its mu/raw/terminal/screen/
// budget fields, drainScreenInput, drainBudgetInput, closeInputPipe, and
// read() itself -- already exists there) and this test FAILS on a real
// assertion (observing a partial length), not a compile error. It passes
// at the cure sha.
func TestScreenDriverReadNeverSignalsAPartialMergeOfAnOversizedWrite(t *testing.T) {
	runtime.GOMAXPROCS(16)

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("open pipe: %v", err)
	}

	const cols, rows = int(terminalColumns), int(terminalRows)
	driver := &ScreenDriver{
		terminal: r,
		screen:   vt.NewEmulator(cols, rows),
		budget:   vt.NewEmulator(cols+frameBudgetMargin, rows+frameBudgetMargin),
		updated:  make(chan struct{}, 1),
		done:     make(chan struct{}),
		readDone: make(chan struct{}),
	}
	go driver.drainScreenInput()
	go driver.drainBudgetInput()
	t.Cleanup(func() { closeInputPipe(driver.screen) })
	t.Cleanup(func() { closeInputPipe(driver.budget) })
	t.Cleanup(func() { _ = r.Close() })

	// A payload bigger than read()'s own 4096-byte buffer, written in full
	// BEFORE read() ever starts, so the remainder is already sitting in
	// the pipe when read()'s first physical Read() call happens -- no
	// write-side timing is involved in whether more is already available.
	first := strings.Repeat("A", 4096)
	rest := strings.Repeat("B", 777)
	full := first + rest
	if _, err := w.Write([]byte(full)); err != nil {
		t.Fatalf("stage payload: %v", err)
	}
	// Close the write end now, explicitly: read() returns (and closes
	// readDone) only on EOF, and EOF on a pipe arrives only once every
	// writer fd is closed. Leaving w open made read()'s exit depend on the
	// garbage collector finalizing w -- a 5 s timeout under -race whenever
	// GC did not run in time (reproduced deterministically with GOGC=off).
	// The staged bytes stay queued in the pipe buffer either way, so the
	// race this test forces is unchanged.
	if err := w.Close(); err != nil {
		t.Fatalf("close pipe writer: %v", err)
	}

	var mu sync.Mutex // guards observed, not driver.mu
	var observed []int
	stop := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				if driver.mu.TryLock() {
					n := driver.raw.Len()
					driver.mu.Unlock()
					mu.Lock()
					observed = append(observed, n)
					mu.Unlock()
				}
			}
		}()
	}

	go driver.read()
	select {
	case <-driver.readDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for read() to finish")
	}
	// A short extra window: read() closing readDone only guarantees ITS
	// OWN last critical section has already happened, not that every
	// poller has already recorded its last sample -- give them a moment
	// to settle before stopping.
	time.Sleep(5 * time.Millisecond)
	close(stop)
	wg.Wait()

	if got := driver.Raw(); got != full {
		t.Fatalf("final raw = %d bytes, want %d bytes -- bytes were lost or corrupted, independent of this test's own evidence", len(got), len(full))
	}

	for _, n := range observed {
		if n > 0 && n < len(full) {
			t.Fatalf("observed raw.Len() == %d while read() was still in flight for a %d-byte payload (buffer cap 4096) -- every byte already sitting in the pipe before read() ever ran must be merged into ONE application, not left for a second, separately-signalled Read() call -- exactly the torn-frame mechanism behind themes.feature:96", n, len(full))
		}
	}
}
