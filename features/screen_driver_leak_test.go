package features

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"
)

// countScreenDriverDrainGoroutines reports how many goroutines are
// currently running ScreenDriver.drainScreenInput or drainBudgetInput, by
// grepping a full runtime goroutine-stack dump for their fully-qualified
// names. Neither goroutine registers itself anywhere else a test could
// poll, so this is the only way to observe them directly rather than
// inferring their presence from a package-wide hang.
func countScreenDriverDrainGoroutines() int {
	buf := make([]byte, 1<<16)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			dump := string(buf[:n])
			return strings.Count(dump, ").drainScreenInput(") + strings.Count(dump, ").drainBudgetInput(")
		}
		buf = make([]byte, 2*len(buf))
	}
}

// TestScreenDriverStopReleasesDrainGoroutines is task 032's probe. It
// starts and stops several ScreenDriver clients in turn and asserts that,
// soon after every one of them has had Stop return, none of their
// drainScreenInput/drainBudgetInput goroutines are still running.
//
// Before the fix, ScreenDriver.Stop closes only the PTY (d.terminal) --
// never the screen/budget vt.Emulators drainScreenInput/drainBudgetInput
// read from. Each of those two goroutines blocks in vt.(*Emulator).Read,
// which itself blocks in an io.PipeReader.Read whose writer side
// (e.pw, closed only by Emulator.Close, which nothing ever calls) is never
// closed -- so both goroutines leak for the rest of the test binary's
// life, one pair per ScreenDriver ever started. features/TestFeatures
// starts one ScreenDriver per client per scenario, hundreds of times in a
// single `go test` process; CI run 36205511917 accumulated enough of these
// leaked goroutines across ~200+ scenarios' worth of clients that the
// whole features/TestFeatures package eventually panicked with
// `test timed out after 10m0s` (the goroutine dump in that panic shows
// dozens of them, parked in exactly drainScreenInput/drainBudgetInput --
// see artifacts/032/race-run1.log), first in
// panel_background_themes.feature:170 and, in a second execution of the
// same package that same run, in themes.feature:96.
//
// This test reproduces the underlying leak itself, deterministically and
// in well under a second, rather than waiting out a 10-minute package
// timeout to observe its eventual consequence: it is the exact same
// leaked goroutines the panic's own dump named, just counted directly.
func TestScreenDriverStopReleasesDrainGoroutines(t *testing.T) {
	binary := buildDeckBinary(t)
	const clients = 5

	for i := 0; i < clients; i++ {
		func() {
			home := t.TempDir()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			driver, err := StartScreenDriver(ctx, binary, []string{
				"DECK_HOME=" + home,
				"DECK_TMUX_SOCKET=" + fmt.Sprintf("deck_leak_test_%d_%d", i, time.Now().UnixNano()),
				"DECK_ASCII=1", "DECK_ANIM=0", "NO_COLOR=1",
			})
			if err != nil {
				t.Fatalf("client %d: start driver: %v", i, err)
			}
			if err := driver.WaitForFrame(ctx, false, "No sessions"); err != nil {
				t.Fatalf("client %d: %v", i, err)
			}
			if err := driver.Send("q"); err != nil {
				t.Fatalf("client %d: send q: %v", i, err)
			}
			if err := driver.Stop(3 * time.Second); err != nil {
				t.Fatalf("client %d: deck did not quit cleanly: %v", i, err)
			}
		}()
	}

	const wait = 5 * time.Second
	deadline := time.Now().Add(wait)
	var leaked int
	for {
		leaked = countScreenDriverDrainGoroutines()
		if leaked == 0 {
			return
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("%d ScreenDriver drainScreenInput/drainBudgetInput goroutine(s) still running %s after every one of %d clients' Stop returned -- ScreenDriver.Stop never closes the screen/budget emulators, so their Read() blocks forever on an io.Pipe nothing ever closes (task 032)", leaked, wait, clients)
}
