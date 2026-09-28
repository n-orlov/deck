package features

import (
	"fmt"
	"time"
)

// waitForSettledCount polls read (a query against some exact counter -- a
// fixture's own dedicated count file, a store's row count, anything that
// returns an int) until its value has stayed unchanged for at least
// settleWindow, timed on the monotonic clock: time.Now/time.Since carry a
// monotonic reading precisely so elapsed-time comparisons like this one are
// immune to a wall-clock adjustment landing mid-wait (see the time package's
// own "Monotonic Clocks" doc). It returns that settled value.
//
// If deadline elapses while the counter is STILL changing (its last read is
// younger than settleWindow), it fails loudly with the last value and how
// long ago it last changed, rather than silently returning an unsettled
// read -- a caller that went on to compare that value against "exactly N"
// could not otherwise tell a genuine mismatch from a read that just
// happened to land mid-change.
//
// This deliberately only characterises STABILITY, never the wanted value:
// it is not a "poll until it reads want" loop. A poll-until-want loop would
// turn "exactly N" into "at least N was reached" (returning the instant want
// is seen, even if the counter later overshoots past it) and would also
// hang forever settling a counter that legitimately wants 0 and never
// changes at all unless bounded by settleWindow the way this is. The
// caller (assertSettledCountEquals below) does the exact comparison
// separately, once, with plain equality.
func waitForSettledCount(deadline, settleWindow, pollInterval time.Duration, read func() (int, error)) (int, error) {
	start := time.Now()
	last, err := read()
	if err != nil {
		return 0, err
	}
	// settledSince marks the moment the LAST completed observation was
	// taken, never the moment the wait began: the initial read above can
	// itself take arbitrarily long (a slow fixture, a cold cache), and
	// starting the stability clock from `start` would count that read's
	// own wall-clock cost as if it were quiet time with no observation
	// backing it -- reporting settled success after a single read even
	// though the counter was never actually observed to be unchanged
	// across settleWindow. Anchoring on the read's completion time means
	// the quiet window can only ever be satisfied by two or more actual
	// observations agreeing.
	settledSince := time.Now()
	for {
		now := time.Now()
		// The deadline check comes first: an expired deadline must
		// never be shadowed by a quiet-window check that could
		// otherwise still report a stale "settled" success on the
		// same iteration it expires.
		if now.Sub(start) >= deadline {
			return last, fmt.Errorf("counter still moving after %v (last value %d, changed %v ago, want %v of no further change): never settled", deadline, last, now.Sub(settledSince), settleWindow)
		}
		if now.Sub(settledSince) >= settleWindow {
			return last, nil
		}
		time.Sleep(pollInterval)
		cur, err := read()
		if err != nil {
			return 0, err
		}
		if cur != last {
			last = cur
			settledSince = time.Now()
		}
	}
}

// assertSettledCountEquals settles read via waitForSettledCount and then
// compares the settled value against want with plain equality -- never
// `>=`, never a range -- so an off-by-one is caught exactly as reliably as
// it would be from an instantaneous read, once the value is known to have
// actually stopped moving first.
func assertSettledCountEquals(deadline, settleWindow, pollInterval time.Duration, read func() (int, error), want int) (int, error) {
	got, err := waitForSettledCount(deadline, settleWindow, pollInterval, read)
	if err != nil {
		return got, err
	}
	if got != want {
		return got, fmt.Errorf("settled count = %d, want exactly %d", got, want)
	}
	return got, nil
}
