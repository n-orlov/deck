package features

import (
	"testing"
	"time"
)

// These three cases are R151's own required coverage for the settle
// helper: a counter still moving at the deadline must fail loudly, one
// that settles at the expected value must pass, and one that settles one
// off from the expected value must fail -- proving the comparison stays
// exact (never `>=`, never a range) once the value is known to be settled.
//
// All three drive the helper with a call-counted fake rather than a real
// wall-clock sleep sequence, so they stay fast and deterministic: the fake
// changes its answer for the first few calls and then flatlines, and the
// helper's own poll/settle timings (a few tens of milliseconds) are what
// actually paces the wait.

const (
	settleCounterTestDeadline = 200 * time.Millisecond
	settleCounterTestWindow   = 40 * time.Millisecond
	settleCounterTestPoll     = 5 * time.Millisecond
)

// TestWaitForSettledCountStillMovingAtDeadlineFails: a counter that never
// stops changing (every read differs from the one before it) must never be
// reported settled -- it must fail loudly once the deadline elapses.
func TestWaitForSettledCountStillMovingAtDeadlineFails(t *testing.T) {
	n := 0
	read := func() (int, error) {
		n++
		return n, nil
	}
	got, err := waitForSettledCount(settleCounterTestDeadline, settleCounterTestWindow, settleCounterTestPoll, read)
	if err == nil {
		t.Fatalf("counter still moving at the deadline: want an error, got settled value %d", got)
	}
}

// TestAssertSettledCountEqualsExpectedPasses: a counter that stops changing
// at exactly the expected value settles and compares equal.
func TestAssertSettledCountEqualsExpectedPasses(t *testing.T) {
	n := 0
	const final = 3
	read := func() (int, error) {
		n++
		if n < final {
			return n, nil
		}
		return final, nil
	}
	got, err := assertSettledCountEquals(2*time.Second, settleCounterTestWindow, settleCounterTestPoll, read, final)
	if err != nil {
		t.Fatalf("settles at the expected value: want no error, got %v", err)
	}
	if got != final {
		t.Fatalf("settled value = %d, want %d", got, final)
	}
}

// TestAssertSettledCountOffByOneFails: a counter that settles cleanly, but
// one off from what the caller wanted, must fail the exact comparison --
// never be waved through by a `>=` or range check.
func TestAssertSettledCountOffByOneFails(t *testing.T) {
	n := 0
	const final = 3
	read := func() (int, error) {
		n++
		if n < final {
			return n, nil
		}
		return final, nil
	}
	got, err := assertSettledCountEquals(2*time.Second, settleCounterTestWindow, settleCounterTestPoll, read, final+1)
	if err == nil {
		t.Fatalf("settled one off from what was wanted: want an error, got settled value %d treated as a pass", got)
	}
}
