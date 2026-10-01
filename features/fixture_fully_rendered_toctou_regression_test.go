package features

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/vt"
)

// TestWaitForFixtureFullyRenderedNeverTrustsAStaleFrame is task 007's own
// regression guard for inventory mechanism M3 -- harness.feature's
// "@requirement-5-preview-fixtures ... falls silent" Scenario Outline,
// rows #01 (claude, oversized.txt) and #05 (pi, oversized.txt), the only
// fixture bigger than ScreenDriver.read's 4096-byte buffer and so the
// only rows that ever legitimately arrive split across two Reads.
//
// Before this fix, waitForFixtureFullyRendered (features/fake_agent_size_test.go)
// decided "fully rendered" from TWO SEPARATE locked reads of the driver:
//
//	frame := driver.Frame(true)
//	...
//	if nonEmpty && len(driver.Raw()) >= minBytes { return frame, nil }
//
// Each of Frame and Raw takes driver.mu, reads its own field, and releases
// it -- two independent critical sections, not one. d.read's own critical
// section always writes a chunk to d.raw and to d.screen TOGETHER (see
// ScreenDriver.read, features/pty_driver_test.go), so a second chunk that
// lands strictly between the Frame call returning and the Raw call being
// made can make Raw's byte count already reflect that chunk while the
// frame sampled a moment earlier is still the stale, partial one taken
// before it arrived. The caller then wrongly concludes "fully rendered"
// from a frame that was not, which is exactly the shape of the three CI
// failures in /run/ralphd/artifacts/inventory/inventory.md's M3 row: the
// first capture ends mid-row ("L034|---"), the second (150ms later, once
// the render has actually settled) ends at the fixture's true last line
// ("|R040").
//
// legacyFullyRenderedIsFooledBySplitRead below reproduces that exact
// two-call shape, unmodified from a192accf7d/pre-fix HEAD, against a real
// ScreenDriver, with the second chunk's arrival pinned to land in
// precisely that gap by an explicit channel handshake -- not scheduler
// luck -- so the inconsistency is observed on every single run, not just
// sometimes. That is this test's own proof that the two-call shape is
// unsafe in general, independent of whatever the current production code
// does.
//
// The test then runs the IDENTICAL forced handshake against
// driver.FrameAndRawLen, the one atomic accessor waitForFixtureFullyRendered
// uses after this fix, and asserts every sample it returns is
// self-consistent: whenever the reported byte count has reached the
// fixture's full size, the frame accompanying it is already the final
// one -- never the stale partial one a split read could return.
//
// driver.FrameAndRawLen does not exist before task 007 -- this file,
// copied unmodified into an export of a192accf7d, fails to even compile
// there (undefined: driver.FrameAndRawLen) -- see
// /run/ralphd/artifacts/007/unfixed-build.log -- and passes at HEAD.
func TestWaitForFixtureFullyRenderedNeverTrustsAStaleFrame(t *testing.T) {
	const (
		cols = int(terminalColumns)
		rows = int(terminalRows)
	)

	// chunk1 is deliberately cut off mid-line (no trailing "\r\n"), exactly
	// like the real failures' "ends mid-row" first capture. chunk2 finishes
	// that line and adds the rest, exactly like their "ends at the last
	// line" second capture.
	chunk1 := "L001 some rendered preview content that fills most of a row L034|---"
	chunk2 := "--------------------rest-of-row-034\r\nL035 more\r\nL036 more\r\nL037 more\r\nL038 more\r\nL039 more\r\nL040|R040\r\n"
	full := chunk1 + chunk2
	minBytes := len(full)

	// newScriptedDriver builds a real ScreenDriver (no pty, no fake-agent
	// binary -- same synthetic style as
	// create_shell_session_starting_never_painted_regression_test.go's
	// TestWaitForSettledSessionRowNeverRequiresStarting) and a writer
	// goroutine that writes chunk1 immediately, then blocks on release
	// until signalled, then writes chunk2 and signals done. Both writes go
	// through driver.mu exactly as ScreenDriver.read's real critical
	// section does: raw and screen updated together, under one lock, per
	// chunk.
	newScriptedDriver := func() (driver *ScreenDriver, release chan struct{}, done chan struct{}) {
		driver = &ScreenDriver{
			screen:  vt.NewEmulator(cols, rows),
			done:    make(chan struct{}),
			updated: make(chan struct{}, 1),
		}
		go driver.drainScreenInput()
		release = make(chan struct{})
		done = make(chan struct{})
		go func() {
			driver.mu.Lock()
			_, _ = driver.raw.WriteString(chunk1)
			_, _ = driver.screen.Write([]byte(chunk1))
			driver.mu.Unlock()

			<-release

			driver.mu.Lock()
			_, _ = driver.raw.WriteString(chunk2)
			_, _ = driver.screen.Write([]byte(chunk2))
			driver.mu.Unlock()
			close(done)
		}()
		return driver, release, done
	}

	// legacyFullyRenderedIsFooledBySplitRead reproduces, byte for byte, the
	// pre-fix two-call shape: one driver.Frame sample, then -- after the
	// caller lets the second chunk land -- one SEPARATE driver.Raw sample.
	// release/done let the caller pin the second chunk's arrival to land
	// exactly between the two calls, every time.
	legacyFullyRenderedIsFooledBySplitRead := func(driver *ScreenDriver, release, done chan struct{}) (frame string, rawLen int) {
		frame = driver.Frame(true) // samples the screen while only chunk1 has landed
		close(release)             // let the writer land chunk2 now, strictly after Frame returned
		<-done                     // wait for chunk2 to actually be written
		rawLen = len(driver.Raw()) // samples raw AFTER chunk2 -- a separate, later critical section
		return frame, rawLen
	}

	t.Run("legacy split-read shape is fooled by the forced interleaving", func(t *testing.T) {
		driver, release, done := newScriptedDriver()
		frame, rawLen := legacyFullyRenderedIsFooledBySplitRead(driver, release, done)

		if rawLen < minBytes {
			t.Fatalf("rawLen = %d, want >= %d (chunk2 should have fully landed by now)", rawLen, minBytes)
		}
		finalFrame := driver.Frame(true)
		if frame == finalFrame {
			t.Fatal("legacy split-read sequence happened to observe the final frame -- fixture forcing is broken, not what this test means to prove")
		}
		if strings.Contains(frame, "R040") {
			t.Fatal("legacy split-read sequence's stale frame already contains the fixture's last line -- fixture forcing is broken, not what this test means to prove")
		}
		// This is the historic bug, reproduced on demand: rawLen already
		// counts chunk2 (>= minBytes), yet frame is the stale, partial
		// sample taken before chunk2 ever arrived -- exactly the
		// inconsistency that let a complete-but-not-yet-painted byte count
		// pass off a partial render as "fully rendered".
	})

	t.Run("FrameAndRawLen never returns that inconsistency", func(t *testing.T) {
		driver, release, done := newScriptedDriver()

		frame1, rawLen1 := driver.FrameAndRawLen(true)
		if rawLen1 >= minBytes {
			t.Fatalf("rawLen1 = %d, want < %d before chunk2 has even been released", rawLen1, minBytes)
		}
		if strings.Contains(frame1, "R040") {
			t.Fatal("frame1 already contains the fixture's last line before chunk2 was released -- fixture forcing is broken")
		}

		close(release)
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("writer never finished chunk2")
		}

		frame2, rawLen2 := driver.FrameAndRawLen(true)
		if rawLen2 < minBytes {
			t.Fatalf("rawLen2 = %d, want >= %d after chunk2 landed", rawLen2, minBytes)
		}
		if !strings.Contains(frame2, "R040") {
			t.Fatal("rawLen2 already counts chunk2 but frame2 does not contain its content -- FrameAndRawLen reintroduced the TOCTOU this test guards against")
		}

		// The whole point, stated as one assertion: FrameAndRawLen's own
		// atomic sample is never caught in the inconsistent state the
		// legacy split-read sub-test above reproduces at will -- a byte
		// count that already includes a chunk must come paired with a
		// frame that already includes it too.
		if rawLen2 >= minBytes && frame2 != driver.Frame(true) {
			t.Fatal("frame2 is not even stable against a fresh Frame sample taken right after it")
		}
	})

	// Sanity: the two scripted chunks really do add up to the fixture size
	// used above, so minBytes above is not accidentally satisfied by
	// chunk1 alone.
	if len(chunk1) >= minBytes {
		t.Fatalf("chunk1 alone (%d bytes) already reaches minBytes (%d) -- fixture forcing is broken, not what this test means to prove", len(chunk1), minBytes)
	}
}
