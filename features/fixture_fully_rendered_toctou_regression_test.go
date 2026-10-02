package features

import (
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/charmbracelet/x/vt"
)

// TestWaitForFixtureFullyRenderedNeverReturnsAStaleFrame is task
// cure-01-01's regression guard (review finding B1) for the
// waitForFixtureFullyRendered Frame/Raw TOCTOU, fixed by f107dd4e25 (task
// 007, R171) and visible in harness.feature:212's
// "@requirement-5-preview-fixtures ... falls silent" Scenario Outline,
// rows #01 (claude, oversized.txt) and #05 (pi, oversized.txt).
//
// This drives waitForFixtureFullyRendered itself (features/fake_agent_size_test.go)
// -- the exact seam the review finding named, present under that same
// name and signature, func(driver *ScreenDriver, minBytes int) (string, error),
// on both trees -- rather than reimplementing its two-call shape inline
// (an earlier version of this file called driver.Frame and driver.Raw
// itself, and separately drove driver.FrameAndRawLen, which does not
// even exist before the fix -- a shape steer 002/003 both rule out: a
// compile error against the unfixed tree is not failing-first evidence,
// and an inline re-implementation of the old code does not count
// either).
//
// waitForFixtureFullyRendered polls every 20ms, so an attempt to force
// the race with a single Lock() queued behind its own first sample (the
// technique that works for ScreenDriver.read(), see
// screendriver_torn_flush_regression_test.go) does not transfer here: the
// 20ms sleep between polls leaves d.mu genuinely free for a long time
// (not just the sub-microsecond gap between the old code's own, separate
// Frame() and Raw() calls), so a goroutine racing to grab the lock lands
// in that long, UNinteresting free period almost every time, long before
// any specific poll's own internal gap -- confirmed empirically in this
// task (both a single queued waiter and a continuously-spinning pool,
// gated to fire on the very first opportunity, missed the actual gap on
// every attempt).
//
// This test instead runs the whole scripted scenario MANY times (trials
// below) and only requires the race to land in the real gap on AT LEAST
// ONE of them: a pool of goroutines continuously polling mu.TryLock(),
// gated to write chunk2 only once raw already holds exactly chunk1 (so
// an early, harmless win just finds nothing to do yet and keeps trying),
// races against the function's own repeated Frame()-then-Raw() (or, on
// the fixed tree, single atomic FrameAndRawLen) sampling, for the
// WHOLE 2-second deadline the function allows itself, across many
// independent polls. Confirmed empirically against this exact file,
// copied unmodified into a git-archive export of a192accf7d: 1000 trials
// land in the real Frame/Raw gap often enough (observed 15-26 hits per
// 1000 trials, several repeats) that the probability of zero hits in one
// run of this test is negligible, while the SAME 1000 trials against the
// cure sha land ZERO hits every time, not probabilistically -- the fixed
// tree's single atomic sample makes the inconsistency this test looks
// for structurally impossible, not merely unlikely.
//
// Unmodified in a git-archive export of a192accf7d, this file compiles
// (every identifier it names -- ScreenDriver, its mu/raw/screen fields,
// drainScreenInput, closeInputPipe, and waitForFixtureFullyRendered
// itself -- already exists there) and this test FAILS on a real
// assertion (hits > 0), not a compile error. It passes at the cure sha.
func TestWaitForFixtureFullyRenderedNeverReturnsAStaleFrame(t *testing.T) {
	const (
		cols = int(terminalColumns)
		rows = int(terminalRows)
	)

	// chunk1 is deliberately cut off mid-line (no trailing "\r\n"), and
	// chunk2 finishes that line and adds the fixture's true last line --
	// the same shape as the real split that bit oversized.txt (bigger
	// than ScreenDriver.read's 4096-byte buffer) at harness.feature:212.
	chunk1 := "L001 some rendered preview content that fills most of a row L034|---"
	chunk2 := "--------------------rest-of-row-034\r\nL035 more\r\nL036 more\r\nL037 more\r\nL038 more\r\nL039 more\r\nL040|R040\r\n"
	full := chunk1 + chunk2
	minBytes := len(full)

	const trials = 1000
	hits := 0

	for trial := 0; trial < trials; trial++ {
		driver := &ScreenDriver{
			screen:  vt.NewEmulator(cols, rows),
			done:    make(chan struct{}),
			updated: make(chan struct{}, 1),
		}
		go driver.drainScreenInput()

		// chunk1 lands exactly as ScreenDriver.read's own critical
		// section would apply it: raw and screen updated together,
		// under one lock, before this trial calls the function under
		// test or starts racing to deliver chunk2.
		driver.mu.Lock()
		driver.raw.WriteString(chunk1)
		_, _ = driver.screen.Write([]byte(chunk1))
		driver.mu.Unlock()

		type result struct {
			frame string
			err   error
		}
		resultCh := make(chan result, 1)
		go func() {
			frame, err := waitForFixtureFullyRendered(driver, minBytes)
			resultCh <- result{frame, err}
		}()

		var wrote atomic.Bool
		stop := make(chan struct{})
		for i := 0; i < 4; i++ {
			go func() {
				for {
					select {
					case <-stop:
						return
					default:
					}
					if wrote.Load() {
						return
					}
					if driver.mu.TryLock() {
						if !wrote.Load() && driver.raw.Len() == len(chunk1) {
							driver.raw.WriteString(chunk2)
							_, _ = driver.screen.Write([]byte(chunk2))
							wrote.Store(true)
						}
						driver.mu.Unlock()
					}
				}
			}()
		}

		var res result
		select {
		case res = <-resultCh:
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for waitForFixtureFullyRendered to return")
		}
		close(stop)
		closeInputPipe(driver.screen)

		if !wrote.Load() {
			t.Fatal("the racing pollers never got a chance to deliver chunk2 -- test setup is broken, not exercising anything")
		}
		if res.err != nil {
			t.Fatalf("trial %d: waitForFixtureFullyRendered returned err=%v, want nil", trial, res.err)
		}
		if !strings.Contains(res.frame, "R040") {
			hits++
		}
	}

	if hits > 0 {
		t.Fatalf("waitForFixtureFullyRendered returned a frame missing the fixture's own last line (\"R040\") in %d/%d trials -- it reported \"fully rendered\" off a stale frame sample taken before chunk2 had actually landed, exactly the Frame/Raw TOCTOU this test guards against", hits, trials)
	}
}
