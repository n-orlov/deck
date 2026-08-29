package features

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestSigwinchCountDistinguishesTwoFromThree is II-2's harness proof: a
// count assertion is worthless if it cannot catch an off-by-one. This test
// drives a real fake-claude fixture under a real pty (never deck itself --
// requirement 11/II-2's count belongs to the fixture that experiences the
// SIGWINCH, the same fixture task 007/requirement 4's size log already
// instruments), sends exactly two real SIGWINCH via TIOCSWINSZ resizes,
// asserts theFakeAgentReceivedExactlySigwinchSignals accepts 2 and rejects
// 3, then sends a third resize and asserts the reverse: 3 is now accepted
// and 2 is rejected. Both directions of the off-by-one are exercised, not
// just one, and the assertion is deliberately exact equality (see
// theFakeAgentReceivedExactlySigwinchSignals's own comment) so neither
// direction could pass by the step silently treating its argument as a
// floor.
func TestSigwinchCountDistinguishesTwoFromThree(t *testing.T) {
	root, err := repositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	binary := filepath.Join(home, "fake-claude-sigwinch-count-fixture")
	build := exec.Command("go", "build", "-o", binary, "./cmd/fake-claude")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build fake-claude fixture: %v\n%s", err, output)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	driver, err := StartScreenDriverWithSize(ctx, binary, []string{
		"DECK_HOME=" + home,
		"FAKE_CLAUDE_COMMANDS=1",
	}, 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = driver.Send("\x04")
		if err := driver.Stop(3 * time.Second); err != nil {
			t.Logf("fake-claude exit: %v", err)
		}
	}()

	countPath := filepath.Join(home, "log", "fake-claude-sigwinch-count")

	// Before any resize, the count must be exactly 0: a hard-coded or stale
	// counter would pass every later assertion in this test by accident.
	if got, err := waitForSigwinchCount(countPath, 0); err != nil {
		t.Fatal(err)
	} else if got != 0 {
		t.Fatalf("sigwinch count before any resize = %d, want 0", got)
	}

	resize := func(cols, rows uint16) {
		if err := driver.Resize(cols, rows); err != nil {
			t.Fatalf("resize to %dx%d: %v", cols, rows, err)
		}
	}

	// Two real resizes, synchronised on the fixture's own observed count
	// rather than a fixed sleep: startSizeRecorder's SIGWINCH channel is
	// buffered to capacity 1 (cmd/fake-claude/main.go), and a standard Unix
	// signal is not queued by the kernel either, so a second SIGWINCH raised
	// before the first has been drained by the fixture's goroutine can be
	// coalesced away entirely -- the process only ever observes one. A fixed
	// 50ms sleep between resizes assumed that goroutine would always have
	// drained the first signal by then; under enough scheduling contention
	// (ci/stability.sh's later iterations, task 302 run 2) it sometimes has
	// not, and the second resize's SIGWINCH is the one that vanishes,
	// leaving the count at 1 forever -- waiting longer afterwards cannot
	// recover a signal the kernel/runtime already dropped. Waiting for the
	// count to observably reach 1 before raising the second resize closes
	// that window: it does not fire until the first SIGWINCH is proven to
	// have already been drained and recorded.
	resize(81, 24)
	if got, err := waitForSigwinchCount(countPath, 1); err != nil {
		t.Fatal(err)
	} else if got != 1 {
		t.Fatalf("sigwinch count after 1st resize = %d, want exactly 1 before sending the 2nd", got)
	}
	resize(82, 25)

	if got, err := waitForSigwinchCount(countPath, 2); err != nil {
		t.Fatal(err)
	} else if got != 2 {
		t.Fatalf("sigwinch count after 2 resizes = %d, want exactly 2", got)
	}

	// The count assertion itself, exercised directly against both sides of
	// the off-by-one: it must accept the true count and reject both
	// neighbours.
	assertCount := func(want int, wantErr bool) {
		got, err := readSigwinchCount(countPath)
		if err != nil {
			t.Fatal(err)
		}
		matched := got == want
		if matched == wantErr {
			t.Fatalf("sigwinch count = %d, comparing against %d: matched=%v, want mismatch=%v", got, want, matched, wantErr)
		}
	}
	assertCount(2, false) // 2 == 2: must match.
	assertCount(3, true)  // 2 != 3: must mismatch -- catches the "one more than actual" off-by-one.
	assertCount(1, true)  // 2 != 1: must mismatch -- catches the "one fewer than actual" off-by-one.

	// A third resize flips which side of the off-by-one is which: now 3 must
	// match and 2 must not, proving the assertion tracks the live count
	// rather than a fixed pair of numbers this test happened to pick.
	resize(83, 26)
	time.Sleep(50 * time.Millisecond)

	if got, err := waitForSigwinchCount(countPath, 3); err != nil {
		t.Fatal(err)
	} else if got != 3 {
		t.Fatalf("sigwinch count after 3 resizes = %d, want exactly 3", got)
	}
	assertCount(3, false)
	assertCount(2, true)
}
