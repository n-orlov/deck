package tui

import (
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/service"
	"github.com/n-orlov/deck/internal/store"
)

// R306: the activity order is last_output_at, bucketed by [ui] output_bucket,
// newest bucket first, the session id breaking a tie inside a bucket.
func TestActivitySortOrdersByOutputBucketDescendingThenID(t *testing.T) {
	sessions := []store.Session{
		{ID: "a", LastOutputAt: 100_000},
		{ID: "b", LastOutputAt: 119_999}, // bucket 11
		{ID: "c", LastOutputAt: 109_999}, // bucket 10, tied with "a": id breaks it
		{ID: "d", LastOutputAt: 0},       // never printed: last
		{ID: "e", LastOutputAt: 200_000},
	}
	got := sortSessionsByOrder(nil, sessions, SortOrderActivity, 10*time.Second)
	assertIDOrder(t, got, []string{"e", "b", "a", "c", "d"})
}

// A narrower bucket splits what a wider one ties: the same rows, bucket 1 s,
// order by exact second, newest first.
func TestActivitySortFollowsTheBucketWidth(t *testing.T) {
	sessions := []store.Session{
		{ID: "a", LastOutputAt: 100_000},
		{ID: "b", LastOutputAt: 101_000},
		{ID: "c", LastOutputAt: 109_000},
	}
	assertIDOrder(t, sortSessionsByOrder(nil, sessions, SortOrderActivity, 10*time.Second), []string{"a", "b", "c"})
	assertIDOrder(t, sortSessionsByOrder(nil, sessions, SortOrderActivity, time.Second), []string{"c", "b", "a"})
	// A zero width (a Settings built without config.Load) is the 10 s default.
	assertIDOrder(t, sortSessionsByOrder(nil, sessions, SortOrderActivity, 0), []string{"a", "b", "c"})
}

// A status change is not output: StatusAt plays no part in the activity order.
func TestActivitySortIgnoresStatusChanges(t *testing.T) {
	sessions := []store.Session{
		{ID: "quiet", LastOutputAt: 50_000, StatusAt: 9_999_999},
		{ID: "chatty", LastOutputAt: 500_000, StatusAt: 1},
	}
	assertIDOrder(t, sortSessionsByOrder(nil, sessions, SortOrderActivity, 10*time.Second), []string{"chatty", "quiet"})
}

// The tie-break inside a bucket is the id, so output landing in the same
// bucket on later ticks never reshuffles the list, whatever frame came before.
func TestActivitySortTieBreakIsTheIDAndStableAcrossTicks(t *testing.T) {
	start := []store.Session{
		{ID: "zeta", LastOutputAt: 100_000},
		{ID: "alpha", LastOutputAt: 101_000},
		{ID: "mid", LastOutputAt: 102_000},
	}
	frame := sortSessionsByOrder(nil, start, SortOrderActivity, 10*time.Second)
	assertIDOrder(t, frame, []string{"alpha", "mid", "zeta"})
	for tick := 1; tick <= 4; tick++ {
		next := make([]store.Session, len(frame))
		copy(next, frame)
		// Each tick the last row prints again, still inside the same bucket.
		next[len(next)-1].LastOutputAt = 100_000 + int64(tick)*2_000
		next[0], next[2] = next[2], next[0] // and the load returns rows in another order
		frame = sortSessionsByOrder(frame, next, SortOrderActivity, 10*time.Second)
		assertIDOrder(t, frame, []string{"alpha", "mid", "zeta"})
	}
	// Moving into the next bucket does reorder, once.
	next := make([]store.Session, len(frame))
	copy(next, frame)
	next[2].LastOutputAt = 110_000
	assertIDOrder(t, sortSessionsByOrder(frame, next, SortOrderActivity, 10*time.Second), []string{"zeta", "alpha", "mid"})
}

type bucketClock struct{ bucket time.Duration }

func (c *bucketClock) Discount(string, service.DiscountCause) {}
func (c *bucketClock) AttachStarted(string)                   {}
func (c *bucketClock) AttachEnded(string)                     {}
func (c *bucketClock) SetBucket(b time.Duration)              { c.bucket = b }

// R306 live apply: a reload of [ui] output_bucket changes the width the
// running activity sort uses, re-sorts the rows already on screen and hands
// the width to the output clock, with no restart.
func TestReloadAppliesUiOutputBucketLive(t *testing.T) {
	h := newReloadHarness(t, "[ui]\nsort_order = \"activity\"\noutput_bucket = 10\n")
	clock := &bucketClock{}
	h.m = h.m.WithOutputDiscounter(clock)
	if h.m.settings.OutputBucket != 10*time.Second {
		t.Fatalf("setup: OutputBucket = %v, want 10s", h.m.settings.OutputBucket)
	}
	sessions := []store.Session{
		{ID: "a", Name: "a", Agent: "shell", Status: "idle", LastOutputAt: 100_000},
		{ID: "b", Name: "b", Agent: "shell", Status: "idle", LastOutputAt: 105_000},
		{ID: "c", Name: "c", Agent: "shell", Status: "idle", LastOutputAt: 109_000},
	}
	next, _ := h.m.Update(sessionsLoaded{sessions: sessions})
	h.m = next.(Model)
	if got := idsOf(h.m.sessions); !equalStrings(got, []string{"a", "b", "c"}) {
		t.Fatalf("10 s buckets: order = %v, want the id order [a b c]", got)
	}
	h.writeConfig("[ui]\nsort_order = \"activity\"\noutput_bucket = 1\n")
	h.reload()
	if h.m.settings.OutputBucket != time.Second {
		t.Fatalf("OutputBucket = %v after one reload, want 1s", h.m.settings.OutputBucket)
	}
	if got := idsOf(h.m.sessions); !equalStrings(got, []string{"c", "b", "a"}) {
		t.Fatalf("1 s buckets after the reload: order = %v, want [c b a]", got)
	}
	if clock.bucket != time.Second {
		t.Fatalf("output clock bucket = %v, want 1s handed to the clock", clock.bucket)
	}
}

// R306: the activity sort buckets by the exact configured width, fractional
// seconds included, on the same boundaries the output clock writes at.
func TestActivitySortHonoursAFractionalBucketWidth(t *testing.T) {
	sessions := []store.Session{
		{ID: "a", LastOutputAt: 3_000}, // bucket 2 at 1.5s
		{ID: "b", LastOutputAt: 4_000}, // bucket 2, tied with "a"
		{ID: "c", LastOutputAt: 5_000}, // bucket 3
		{ID: "d", LastOutputAt: 1_000}, // bucket 0
	}
	assertIDOrder(t, sortSessionsByOrder(nil, sessions, SortOrderActivity, 1500*time.Millisecond), []string{"c", "a", "b", "d"})
	// At 1 s the same rows no longer tie: a fraction is not truncated away
	// into a different width.
	assertIDOrder(t, sortSessionsByOrder(nil, sessions, SortOrderActivity, time.Second), []string{"c", "b", "a", "d"})
	// 2.5 s: 3000 and 4000 share bucket 1, 5000 opens bucket 2.
	assertIDOrder(t, sortSessionsByOrder(nil, sessions, SortOrderActivity, 2500*time.Millisecond), []string{"c", "a", "b", "d"})
}

// A reload of a quoted fractional duration reaches the settings, the output
// clock and the on-screen order exactly, not truncated to 1 s.
func TestReloadAppliesAFractionalOutputBucketLive(t *testing.T) {
	h := newReloadHarness(t, "[ui]\nsort_order = \"activity\"\noutput_bucket = 10\n")
	clock := &bucketClock{}
	h.m = h.m.WithOutputDiscounter(clock)
	sessions := []store.Session{
		{ID: "a", Name: "a", Agent: "shell", Status: "idle", LastOutputAt: 3_000},
		{ID: "b", Name: "b", Agent: "shell", Status: "idle", LastOutputAt: 4_000},
		{ID: "c", Name: "c", Agent: "shell", Status: "idle", LastOutputAt: 5_000},
	}
	next, _ := h.m.Update(sessionsLoaded{sessions: sessions})
	h.m = next.(Model)
	h.writeConfig("[ui]\nsort_order = \"activity\"\noutput_bucket = \"1500ms\"\n")
	h.reload()
	want := 1500 * time.Millisecond
	if h.m.settings.OutputBucket != want {
		t.Fatalf("OutputBucket = %v after reload, want exactly %v", h.m.settings.OutputBucket, want)
	}
	if clock.bucket != want {
		t.Fatalf("output clock bucket = %v, want %v", clock.bucket, want)
	}
	if got := idsOf(h.m.sessions); !equalStrings(got, []string{"c", "a", "b"}) {
		t.Fatalf("order at 1.5s = %v, want [c a b]", got)
	}
}
