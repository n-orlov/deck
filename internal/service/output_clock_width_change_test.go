package service

import (
	"testing"
	"time"
)

// R306/R304: a live width change re-buckets what the clock already holds at
// the new width. A baseline read at 1700000000 under 2.000000001s falls in
// bucket 424999999 at 4.000000007s, and a transcript change at 1700000003
// opens bucket 425000000, so it counts.
func TestOutputClockWideningCountsOutputInTheNextNewWidthBucket(t *testing.T) {
	h := newOutputHarness(t)
	h.clock = NewOutputClock(2*time.Second + 1)
	h.pane = pane("base", "f")
	h.tick(1700000000)
	h.clock.SetBucket(4*time.Second + 7)
	h.pane = pane("base\nmore", "f")
	if !h.tick(1700000003) {
		t.Fatal("a transcript change in a new bucket at the widened width wrote nothing")
	}
	if got := h.lastOutputAt(); got != 1700000003*1000 {
		t.Fatalf("last_output_at = %d, want %d", got, int64(1700000003*1000))
	}
}

// Same rule at whole-second widths the finding did not name: widening from
// 10s to 30s must count a change in the next 30s bucket (1020 is bucket 34,
// the baseline 1005 bucket 33), and a second change in that same 30s bucket
// must not write again.
func TestOutputClockWideningWholeSecondsCountsOncePerNewBucket(t *testing.T) {
	h := newOutputHarness(t)
	h.pane = pane("one", "f")
	h.tick(1005)
	h.clock.SetBucket(30 * time.Second)
	h.pane = pane("two", "f")
	if !h.tick(1020) {
		t.Fatal("widened to 30s: a change in bucket 34 after a baseline in bucket 33 wrote nothing")
	}
	h.pane = pane("three", "f")
	if h.tick(1029) {
		t.Fatal("widened to 30s: a second change inside bucket 34 wrote again")
	}
	h.pane = pane("four", "f")
	if !h.tick(1050) {
		t.Fatal("widened to 30s: a change in bucket 35 wrote nothing")
	}
	if h.writes != 2 {
		t.Fatalf("writes = %d, want 2", h.writes)
	}
}

// Narrowing must not write twice inside one bucket of the new width: a change
// counted at 1001 under 30s lies in the 10s bucket [1000, 1010), so a change
// at 1005 after narrowing to 10s is in the bucket already counted, and 1010
// opens the next one.
func TestOutputClockNarrowingNeverWritesTwiceInOneNewBucket(t *testing.T) {
	h := newOutputHarness(t)
	h.clock = NewOutputClock(30 * time.Second)
	h.pane = pane("one", "f")
	h.tick(985)
	h.pane = pane("two", "f")
	if !h.tick(1001) {
		t.Fatal("30s: a change in a new bucket wrote nothing")
	}
	h.clock.SetBucket(10 * time.Second)
	h.pane = pane("three", "f")
	if h.tick(1005) {
		t.Fatal("narrowed to 10s: a change inside the 10s bucket already counted wrote again")
	}
	h.pane = pane("four", "f")
	if !h.tick(1010) {
		t.Fatal("narrowed to 10s: a change in the next 10s bucket wrote nothing")
	}
	if got := h.lastOutputAt(); got != 1010*1000 {
		t.Fatalf("last_output_at = %d, want 1010000", got)
	}
}

// R316's movement fallback judges "consecutive" and "the bucket after the
// newest counted one" at the exact configured width, fractions included, and
// across a live width change.
func TestOutputClockMovingUsesTheExactConfiguredWidth(t *testing.T) {
	h := newOutputHarness(t)
	now := int64(0)
	h.clock = NewOutputClock(1500 * time.Millisecond)
	h.clock.Now = func() time.Time { return time.UnixMilli(now) }
	h.pane = pane("a", "f")
	h.tick(998) // bucket 665 at 1.5s
	h.pane = pane("b", "f")
	h.tick(1000) // bucket 666
	h.pane = pane("c", "f")
	h.tick(1001) // bucket 667: two consecutive counted buckets
	for _, tc := range []struct {
		nowMs int64
		want  bool
	}{
		{1_001_500, true},  // bucket 667, the newest counted one
		{1_003_000, true},  // bucket 668, the one after it, not yet passed quiet
		{1_003_499, true},  // still bucket 668
		{1_003_500, false}, // bucket 669: one whole quiet bucket
	} {
		now = tc.nowMs
		if got := h.clock.Moving(h.id); got != tc.want {
			t.Fatalf("Moving at %d ms (1.5s buckets) = %v, want %v", tc.nowMs, got, tc.want)
		}
	}
	// Widen to 4s: 1001 is bucket 250, so the session stays moving through
	// bucket 251 ([1004, 1008) s) and stops at 1008 s.
	h.clock.SetBucket(4 * time.Second)
	for _, tc := range []struct {
		nowMs int64
		want  bool
	}{
		{1_007_999, true},
		{1_008_000, false},
	} {
		now = tc.nowMs
		if got := h.clock.Moving(h.id); got != tc.want {
			t.Fatalf("Moving at %d ms after widening to 4s = %v, want %v", tc.nowMs, got, tc.want)
		}
	}
}

// The streak's "consecutive" is judged at the width in force when the next
// change counts: counted at 1001 under 1s, then at 1004 after widening to
// 4s, they are 4s buckets 250 and 251, consecutive, so the session is moving.
func TestOutputClockStreakIsJudgedAtTheCurrentWidth(t *testing.T) {
	h := newOutputHarness(t)
	now := int64(1_004_000)
	h.clock = NewOutputClock(time.Second)
	h.clock.Now = func() time.Time { return time.UnixMilli(now) }
	h.pane = pane("a", "f")
	h.tick(999)
	h.pane = pane("b", "f")
	h.tick(1001) // counted, bucket 1001 at 1s
	h.clock.SetBucket(4 * time.Second)
	h.pane = pane("c", "f")
	if !h.tick(1004) { // bucket 251 at 4s; 1001 is bucket 250
		t.Fatal("a change in the next 4s bucket wrote nothing")
	}
	if !h.clock.Moving(h.id) {
		t.Fatal("two consecutive counted 4s buckets did not make the session moving")
	}
}
