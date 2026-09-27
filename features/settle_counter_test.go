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
	settledSince := start
	for {
		now := time.Now()
		if now.Sub(settledSince) >= settleWindow {
			return last, nil
		}
		if now.Sub(start) >= deadline {
			return last, fmt.Errorf("counter still moving after %v (last value %d, changed %v ago, want %v of no further change): never settled", deadline, last, now.Sub(settledSince), settleWindow)
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
