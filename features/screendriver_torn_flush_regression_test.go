package features

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/vt"
	"github.com/creack/pty"
)

// TestDrainPTYChunkFoldsEveryReadyByteIntoOneChunk is task 017's own
// regression guard for the "hint chrome" flake
// (/run/ralphd/artifacts/stability/analysis/hint_chrome-flake.md):
// features/themes.feature:96's "a built-in theme colours
// border_focus/border, selection/selection_idle, title, group and key/hint
// chrome, read per cell from a real client" scenario failing its
// `text "attach" has foreground token "hint"` step (themes.feature:111)
// with a captured frame showing a correct, shorter session-list TOP over a
// stale, taller create-dialog's own field copy still visible at the
// BOTTOM -- a frame bubbletea's own renderer never asked the terminal to
// show (standardRenderer.flush always writes one CursorHomePosition, the
// new content, and -- whenever the new frame is shorter than the last one
// -- an ansi.EraseScreenBelow erasing exactly that leftover tail, all in
// ONE buffer, in ONE call to the underlying io.Writer).
//
// ScreenDriver.read is what turns that one underlying Write into what a
// waiter (Frame, WaitForFrame, WaitForFrameFunc -- including the
// predicate waitForSettledSessionRow polls) actually observes. Before
// this fix it called exactly one os.File.Read(buf) per loop iteration,
// applied whatever that one syscall returned to d.raw/d.screen/d.budget,
// and signalled d.updated immediately -- with no regard for whether more
// bytes belonging to the SAME logical write were already sitting in the
// kernel's pty buffer, just not yet fetched because buf (4096 bytes) was
// smaller than the write. A full alt-screen repaint of a themed 100x30
// frame carrying both a session list and (moments earlier) a multi-field
// create dialog is comfortably larger than 4096 bytes once ANSI SGR
// sequences are counted, so this is not hypothetical: shrinking this
// file's own read buffer to 32 bytes (never committed, kept only in this
// investigation's own notes) reproduced themes.feature:96's exact failure
// solo and deterministically -- DECK_GODOG_PATHS=themes.feature:96
// ci/run.sh go test -count=20 ./features/ -run '^TestFeatures$' failed
// within the 20 runs, no stability-sweep load needed at all -- proving
// the mechanism, not just correlating with it.
//
// This test proves the fix at the unit level instead of trying to force
// that real race through timing: a 5000-byte single pty Write, read via
// the real 4096-byte buffer, was tried first and found NOT to
// discriminate reliably -- the real producer goroutine's two Read() calls
// land close enough together that an external observer's channel receive
// almost always finds BOTH already merged by the time it gets scheduled,
// on fixed and unfixed code alike (the real bug needs either a much
// smaller buffer across many more reads, or genuine CPU contention, i.e.
// ci/stability.sh's own parallel load, to land a scheduler preemption
// inside that gap -- not safely reproducible by plain timing in a fast,
// deterministic unit test). drainPTYChunk (features/pty_driver_test.go)
// is read()'s own merge step pulled out as a pure function for exactly
// this reason: scripting its ready()/read() callbacks lets this test pin
// the exact interleaving read() must handle correctly, with no
// dependency on the scheduler at all.
//
// Copied unmodified into an export of a192accf7d (this bug predates this
// run entirely -- ScreenDriver.read has not been touched by any task
// 002-016 commit; see git log --oneline -- features/pty_driver_test.go),
// this test FAILS TO COMPILE there (undefined: drainPTYChunk) -- the same
// shape task 007's TestWaitForFixtureFullyRenderedNeverTrustsAStaleFrame
// doc comment describes for driver.FrameAndRawLen, which also did not
// exist pre-fix. It compiles and passes at HEAD.
func TestDrainPTYChunkFoldsEveryReadyByteIntoOneChunk(t *testing.T) {
	// Scripts the exact shape of ScreenDriver.read's own call: a first
	// Read() that already returned n>0 (firstChunk), then readiness
	// toggling true exactly once (simulating "more of this same flush is
	// already queued"), one more Read() returning the rest, then
	// readiness going false (simulating "nothing more queued right now").
	firstChunk := []byte("the session-list TOP of a torn frame, ")
	restChunk := []byte("plus the create-dialog TAIL that should never have survived the erase")
	full := string(firstChunk) + string(restChunk)

	buf := make([]byte, 4096)
	copy(buf, firstChunk)

	readyCalls := 0
	readCalls := 0
	ready := func() bool {
		readyCalls++
		// True on the first check (there IS more queued -- restChunk),
		// false on every check after (nothing left).
		return readyCalls == 1
	}
	read := func(p []byte) (int, error) {
		readCalls++
		if readCalls != 1 {
			t.Fatalf("read() called %d times, want exactly 1 (ready() should have gone false after the first)", readCalls)
		}
		n := copy(p, restChunk)
		return n, nil
	}

	chunk, err := drainPTYChunk(buf, len(firstChunk), nil, ready, read)
	if err != nil {
		t.Fatalf("drainPTYChunk returned err=%v, want nil", err)
	}
	if string(chunk) != full {
		t.Fatalf("drainPTYChunk returned %q, want the first chunk folded together with everything ready() reported as already queued (%q) -- a merge that drops or truncates the ready byte run is exactly the mechanism that let themes.feature:96 observe a torn frame", string(chunk), full)
	}
	if readyCalls < 2 {
		t.Fatalf("ready() was called %d time(s), want >= 2 (once to find more, once to find none) -- the drain loop must keep asking, not stop after one fold", readyCalls)
	}

	t.Run("stops at the first error without losing bytes already read", func(t *testing.T) {
		buf := make([]byte, 4096)
		copy(buf, firstChunk)
		boom := errors.New("boom")
		calls := 0
		ready := func() bool { return true } // always claims more is queued
		read := func(p []byte) (int, error) {
			calls++
			n := copy(p, restChunk)
			return n, boom // the PTY itself errored (e.g. EIO) on this read
		}
		chunk, err := drainPTYChunk(buf, len(firstChunk), nil, ready, read)
		if !errors.Is(err, boom) {
			t.Fatalf("err = %v, want boom", err)
		}
		if string(chunk) != full {
			t.Fatalf("chunk = %q, want %q -- the bytes a failing read still returned must not be discarded", string(chunk), full)
		}
		if calls != 1 {
			t.Fatalf("read() called %d times, want exactly 1 -- the loop must stop the instant it sees a non-nil error, not call ready() again", calls)
		}
	})

	t.Run("a single ready byte run of many pieces folds into one chunk", func(t *testing.T) {
		// Models the real shape more closely: a flush big enough to need
		// several small Read() calls (as a shrunk buffer forced in the
		// manual investigation), not just two.
		pieces := []string{"AAAA", "BBBB", "CCCC", "DDDD", "EEEE"}
		want := strings.Join(pieces, "")
		buf := make([]byte, 4)
		copy(buf, pieces[0])
		idx := 1 // pieces[0] was already "read" as firstN below
		ready := func() bool { return idx < len(pieces) }
		read := func(p []byte) (int, error) {
			n := copy(p, pieces[idx])
			idx++
			return n, nil
		}
		chunk, err := drainPTYChunk(buf, len(pieces[0]), nil, ready, read)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
		if string(chunk) != want {
			t.Fatalf("chunk = %q, want %q -- every piece of a multi-read burst must land in the SAME chunk, in order", string(chunk), want)
		}
	})
}

// TestScreenDriverReadMergesAnOversizedWriteBeforeSignalling is a
// secondary, best-effort smoke check against a REAL pty: it is not relied
// on as this task's failing-first evidence (see the deterministic test
// above for that -- a real pty's two Read() calls for one write land too
// close together for an external observer to reliably catch the
// unmerged intermediate state; this was verified empirically against
// a192accf7d's own unfixed read(), which still passed this exact check
// most of the time), but it does exercise the real ScreenDriver.read
// goroutine end to end and asserts the invariant it must uphold whenever
// it DOES get the chance to observe a first signal: that signal's byte
// count is never short of what was actually written.
func TestScreenDriverReadMergesAnOversizedWriteBeforeSignalling(t *testing.T) {
	master, slave, err := pty.Open()
	if err != nil {
		t.Fatalf("open pty: %v", err)
	}
	t.Cleanup(func() { _ = master.Close() })
	t.Cleanup(func() { _ = slave.Close() })

	const cols, rows = int(terminalColumns), int(terminalRows)
	driver := &ScreenDriver{
		terminal: master,
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
	go driver.read()

	const payloadLen = 4096 + 904
	payload := strings.Repeat("Q", payloadLen)
	if _, err := slave.Write([]byte(payload)); err != nil {
		t.Fatalf("write payload to pty slave: %v", err)
	}

	select {
	case <-driver.updated:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the first d.updated signal")
	}

	rawLen := len(driver.Raw())
	if rawLen != 0 && rawLen != payloadLen {
		t.Fatalf("first d.updated signal fired with %d of %d payload bytes applied -- never a partial count other than 0 or the full payload", rawLen, payloadLen)
	}
}
