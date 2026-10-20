package service

import (
	"testing"
	"time"
)

// TestOutputClockDiscountCoversTheSecondItWasCausedIn is R410's finite cases.
// A discount span starts at the instant the caller took before it issued the
// action, and a window_activity second is covered iff the span overlaps
// [second*1000, second*1000+999] ms. The baseline is read at window_activity
// 995 (bucket 99, 10 s buckets); the window is 5 s.
func TestOutputClockDiscountCoversTheSecondItWasCausedIn(t *testing.T) {
	kinds := []string{"resize", "key", "attach-start", "attach-end"}
	ms := func(sec, milli int64) time.Time { return time.UnixMilli(sec*1000 + milli) }

	for _, kind := range kinds {
		// Case 1: the redraw is stamped in the second before the call
		// returned (1000), which is a new bucket.
		t.Run("case1/"+kind+"/redraw stamped in the second before the call returned, new bucket", func(t *testing.T) {
			h := discountHarness(t)
			h.clock.Now = func() time.Time { return ms(1001, 100) } // the call returned
			issueDiscount(h.clock, h.id, kind, ms(1000, 900))       // taken before the call
			h.provoke()
			if h.tick(1000) {
				t.Fatalf("a redraw stamped in second 1000, before the call returned, advanced last_output_at to %d", h.lastOutputAt())
			}
			if got := h.lastOutputAt(); got != 0 {
				t.Fatalf("last_output_at = %d, want 0", got)
			}
		})

		// Case 2: the same, in a bucket already examined (99); the next bucket
		// shows only footer churn on top of the redraw's text.
		t.Run("case2/"+kind+"/examined bucket, then footer churn in the next bucket", func(t *testing.T) {
			h := discountHarness(t)
			h.clock.Now = func() time.Time { return ms(997, 100) }
			issueDiscount(h.clock, h.id, kind, ms(996, 900))
			h.provoke()
			if h.tick(996) {
				t.Fatalf("a redraw in an examined bucket wrote last_output_at %d", h.lastOutputAt())
			}
			h.pane = pane("hello\nredraw", "~/proj  main  ctx:13%")
			if h.tick(1003) {
				t.Fatalf("footer churn after the redraw advanced last_output_at to %d", h.lastOutputAt())
			}
			if got := h.lastOutputAt(); got != 0 {
				t.Fatalf("last_output_at = %d, want 0", got)
			}
		})
	}

	// Case 3: the controls keep their outcomes. now is the span start and the
	// clock's now both.
	controls := []struct {
		name     string
		start    time.Time
		activity int64
		counts   bool
	}{
		{"same second as the span start", ms(1001, 0), 1001, false},
		{"second whose tail overlaps the span start", ms(1001, 999), 1001, false},
		{"inside the window", ms(1001, 0), 1004, false},
		{"last second the window overlaps", ms(1001, 0), 1006, false},
		{"past the window", ms(1001, 0), 1007, true},
		{"second that ended before the span", ms(1001, 0), 1000, true},
	}
	for _, c := range controls {
		t.Run("case3/"+c.name, func(t *testing.T) {
			h := discountHarness(t)
			h.clock.Now = func() time.Time { return c.start }
			issueDiscount(h.clock, h.id, "resize", c.start)
			h.provoke()
			if got := h.tick(c.activity); got != c.counts {
				t.Fatalf("tick(%d) wrote=%v, want %v", c.activity, got, c.counts)
			}
			want := int64(0)
			if c.counts {
				want = c.activity * 1000
			}
			if got := h.lastOutputAt(); got != want {
				t.Fatalf("last_output_at = %d, want %d", got, want)
			}
		})
	}
}
