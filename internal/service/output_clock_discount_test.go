package service

import (
	"testing"
	"time"
)

// discountHarness is the R304 harness with a scripted wall clock, a baseline
// read already taken at tmux window_activity 995 (bucket 99, 10 s buckets),
// and the discount window anchored at wall time 1001 s. The window is
// min(bucket, MaxDiscountWindow) = 5 s, so it covers window_activity 1001
// through 1006.
func discountHarness(t *testing.T) *outputHarness {
	t.Helper()
	h := newOutputHarness(t)
	now := time.Unix(1001, 0)
	h.clock.Now = func() time.Time { return now }
	h.pane = pane("hello", "~/proj  main  ctx:12%")
	if h.tick(995) {
		t.Fatal("the baseline read must count nothing")
	}
	return h
}

// provoked is the transcript change the deck-caused action makes the pane
// print (a redraw after a resize, an echo of a key, a client's repaint).
func (h *outputHarness) provoke() { h.pane = pane("hello\nredraw", "~/proj  main  ctx:12%") }

func TestOutputAfterADeckCausedResizeDoesNotAdvanceLastOutputAt(t *testing.T) {
	h := discountHarness(t)
	h.clock.Discount(h.id, DiscountResize)
	h.provoke()
	if h.tick(1003) {
		t.Fatalf("output %d s after a deck-caused resize advanced last_output_at to %d", 2, h.lastOutputAt())
	}
	if got := h.lastOutputAt(); got != 0 {
		t.Fatalf("last_output_at = %d, want 0", got)
	}
}

func TestOutputAfterAnAttachDoesNotAdvanceLastOutputAt(t *testing.T) {
	h := discountHarness(t)
	h.clock.AttachStarted(h.id)
	// The user types in the attached client for a long while; deck is
	// suspended and sees none of it. Detach lands 60 s later.
	later := time.Unix(1061, 0)
	h.clock.Now = func() time.Time { return later }
	h.clock.AttachEnded(h.id)
	h.provoke()
	if h.tick(1050) {
		t.Fatalf("output during an attach advanced last_output_at to %d", h.lastOutputAt())
	}
	if got := h.lastOutputAt(); got != 0 {
		t.Fatalf("last_output_at = %d, want 0", got)
	}
}

func TestOutputAfterAForwardedKeyDoesNotAdvanceLastOutputAt(t *testing.T) {
	h := discountHarness(t)
	h.clock.Discount(h.id, DiscountKey)
	h.provoke()
	if h.tick(1002) {
		t.Fatalf("the echo of a forwarded key advanced last_output_at to %d", h.lastOutputAt())
	}
	if got := h.lastOutputAt(); got != 0 {
		t.Fatalf("last_output_at = %d, want 0", got)
	}
}

func TestOutputAfterTheDiscountWindowEndsCounts(t *testing.T) {
	for _, cause := range []DiscountCause{DiscountResize, DiscountKey} {
		t.Run(string(cause), func(t *testing.T) {
			h := discountHarness(t)
			h.clock.Discount(h.id, cause)
			if win := h.clock.DiscountWindow(); win > h.clock.Bucket {
				t.Fatalf("window %v is longer than one bucket (%v)", win, h.clock.Bucket)
			}
			// 1007 is past the window's end at 1006, in the next bucket.
			h.provoke()
			if !h.tick(1007) {
				t.Fatal("output after the window ended must advance last_output_at")
			}
			if got := h.lastOutputAt(); got != 1007_000 {
				t.Fatalf("last_output_at = %d, want %d", got, 1007_000)
			}
		})
	}
}

// A discounted change is remembered: the same text is not counted by a
// later tick once the window is over.
func TestDiscountedOutputIsNotCountedLater(t *testing.T) {
	h := discountHarness(t)
	h.clock.Discount(h.id, DiscountResize)
	h.provoke()
	h.tick(1003)
	if h.tick(1012) {
		t.Fatalf("unchanged pane after the window advanced last_output_at to %d", h.lastOutputAt())
	}
}

// The window never exceeds one bucket, even for the smallest bucket.
func TestDiscountWindowIsAtMostOneBucket(t *testing.T) {
	for _, bucket := range []time.Duration{time.Second, 3 * time.Second, 10 * time.Second, time.Minute} {
		if win := NewOutputClock(bucket).DiscountWindow(); win > bucket || win <= 0 {
			t.Fatalf("bucket %v: window %v", bucket, win)
		}
	}
}

// sameBucketHarness takes its baseline read at window_activity 1000, the same
// 10 s bucket (100) as the discounted redraw at 1003, with the discount window
// anchored at wall time 1001 s.
func sameBucketHarness(t *testing.T) *outputHarness {
	t.Helper()
	h := newOutputHarness(t)
	now := time.Unix(1001, 0)
	h.clock.Now = func() time.Time { return now }
	h.pane = pane("hello", "~/proj  main  ctx:12%")
	if h.tick(1000) {
		t.Fatal("the baseline read must count nothing")
	}
	return h
}

// A discounted change in the bucket already examined is remembered too: the
// next bucket's read of the unchanged transcript (footer-only activity at
// 1012) does not count the deck-caused redraw.
func TestDiscountedSameBucketOutputIsNotCountedLater(t *testing.T) {
	causes := map[string]func(h *outputHarness){
		"resize": func(h *outputHarness) { h.clock.Discount(h.id, DiscountResize) },
		"key":    func(h *outputHarness) { h.clock.Discount(h.id, DiscountKey) },
		"attach": func(h *outputHarness) { h.clock.AttachStarted(h.id); h.clock.AttachEnded(h.id) },
	}
	for name, start := range causes {
		t.Run(name, func(t *testing.T) {
			h := sameBucketHarness(t)
			start(h)
			h.provoke()
			if h.tick(1003) {
				t.Fatalf("deck-caused redraw advanced last_output_at to %d", h.lastOutputAt())
			}
			h.pane = pane("hello\nredraw", "~/proj  main  ctx:13%")
			if h.tick(1012) {
				t.Fatalf("footer-only activity after a same-bucket discounted redraw advanced last_output_at to %d", h.lastOutputAt())
			}
			if got := h.lastOutputAt(); got != 0 {
				t.Fatalf("last_output_at = %d, want 0", got)
			}
		})
	}
}

// Real output after a same-bucket discounted redraw still counts, and a
// same-bucket tick outside any window still captures nothing.
func TestSameBucketDiscountKeepsLaterOutputCounting(t *testing.T) {
	h := sameBucketHarness(t)
	h.clock.Discount(h.id, DiscountResize)
	h.provoke()
	h.tick(1003)
	reads := h.reads
	if h.tick(1008) {
		t.Fatal("a same-bucket tick outside the window must write nothing")
	}
	if h.reads != reads {
		t.Fatalf("a same-bucket tick outside the window captured the pane (%d -> %d reads)", reads, h.reads)
	}
	h.pane = pane("hello\nredraw\nagent output", "~/proj  main  ctx:12%")
	if !h.tick(1012) {
		t.Fatal("real output after the window must advance last_output_at")
	}
	if got := h.lastOutputAt(); got != 1012_000 {
		t.Fatalf("last_output_at = %d, want %d", got, 1012_000)
	}
}
