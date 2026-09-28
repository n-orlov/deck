package features

import (
	"sync/atomic"
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

// TestReviewCounterStillMovingAtDeadlineWithSlowInitialReadFails pins
// R151's own probe: a counter that changes on EVERY read, whose very
// first read is itself slower than the configured settle window (but
// still well below the deadline), must never be reported settled after
// that one slow read. Before the fix, waitForSettledCount started its
// stability clock at the moment the wait began (before the slow initial
// read even returned) rather than at the moment that read actually
// completed -- so the slow read's own wall-clock cost alone satisfied
// the quiet window on the very first loop iteration, with no second
// observation ever confirming the counter had actually stopped moving.
func TestReviewCounterStillMovingAtDeadlineWithSlowInitialReadFails(t *testing.T) {
	var n int32
	first := true
	read := func() (int, error) {
		if first {
			first = false
			// The initial read alone outlasts the settle window
			// (but stays well under the deadline): this is the
			// exact shape the review probe reported false
			// quiescence for.
			time.Sleep(60 * time.Millisecond)
		}
		v := atomic.AddInt32(&n, 1)
		return int(v), nil
	}
	got, err := waitForSettledCount(300*time.Millisecond, 40*time.Millisecond, 5*time.Millisecond, read)
	if err == nil {
		t.Fatalf("counter still moving on every read, slow initial read: want an error, got settled value %d", got)
	}
}

// TestReviewSlowInitialReadBelowDeadlineMustStillSettle pins the second
// half of R151's probe: a counter still moving on every read, whose slow
// initial read exceeds the deadline itself (not just the settle window),
// must fail -- and must fail with the deadline-exceeded contract, never
// be waved through as a settled success because a stale quiet-window
// clock happened to already look satisfied.
func TestReviewSlowInitialReadBelowDeadlineMustStillSettle(t *testing.T) {
	var n int32
	first := true
	read := func() (int, error) {
		if first {
			first = false
			// The initial read alone outlasts the deadline.
			time.Sleep(50 * time.Millisecond)
		}
		v := atomic.AddInt32(&n, 1)
		return int(v), nil
	}
	got, err := waitForSettledCount(40*time.Millisecond, 20*time.Millisecond, 5*time.Millisecond, read)
	if err == nil {
		t.Fatalf("initial read alone exceeded the deadline: want an error, got settled value %d", got)
	}
}
