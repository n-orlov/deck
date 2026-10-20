package service

import (
	"testing"
	"time"
)

// R305: an attach discount is bounded, but a bound only helps if the clock is
// read while the client is attached. The TUI's reconcile ticks do not run for
// the length of an attach, so the attach watcher reads the clock itself
// (Service.ObserveAttachedOutput); these tests drive the clock the way that
// watcher and the detach tick do.

// Output the agent prints mid-attach, read by the watcher after the attach
// window, counts and survives the detach repaint that follows.
func TestAutonomousOutputReadMidAttachCountsAndSurvivesTheDetachRepaint(t *testing.T) {
	for _, bucket := range []time.Duration{time.Second, 3 * time.Second, 10 * time.Second} {
		t.Run(bucket.String(), func(t *testing.T) {
			h := discountHarness(t)
			h.clock.SetBucket(bucket)
			h.clock.AttachStarted(h.id, h.clock.at())
			// The agent prints well after the hand-off window.
			h.pane = pane("hello\nagent output", "~/proj  main  ctx:12%")
			if !h.tick(1030) {
				t.Fatal("output read mid-attach, past the attach window, did not count")
			}
			if got := h.lastOutputAt(); got != 1030_000 {
				t.Fatalf("last_output_at = %d, want 1030000", got)
			}
			// Detach: the repaint is discounted and must not move it.
			now := time.Unix(1060, 0)
			h.clock.Now = func() time.Time { return now }
			h.clock.AttachEnded(h.id, h.clock.at())
			h.pane = pane("hello\nagent output\nrepaint", "~/proj  main  ctx:12%")
			if h.tick(1061) {
				t.Fatalf("the detach repaint advanced last_output_at to %d", h.lastOutputAt())
			}
			if got := h.lastOutputAt(); got != 1030_000 {
				t.Fatalf("last_output_at = %d after the detach repaint, want 1030000", got)
			}
		})
	}
}

// Several autonomous buckets in one attach each count (and feed the streak),
// while a resize the user causes mid-attach stays discounted.
func TestMidAttachOutputCountsPerBucketButAResizeInsideItStaysDiscounted(t *testing.T) {
	h := discountHarness(t)
	h.clock.AttachStarted(h.id, h.clock.at())
	h.pane = pane("hello\none", "~/proj  main  ctx:12%")
	if !h.tick(1020) {
		t.Fatal("first autonomous bucket did not count")
	}
	h.pane = pane("hello\none\ntwo", "~/proj  main  ctx:12%")
	if !h.tick(1030) {
		t.Fatal("second autonomous bucket did not count")
	}
	if h.clock.state[h.id].streak != 2 {
		t.Fatalf("streak = %d, want 2", h.clock.state[h.id].streak)
	}
	// A terminal resize mid-attach redraws the pane: discounted.
	now := time.Unix(1040, 0)
	h.clock.Now = func() time.Time { return now }
	h.clock.Discount(h.id, DiscountResize, h.clock.at())
	h.pane = pane("hello\none\ntwo\nredraw", "~/proj  main  ctx:12%")
	if h.tick(1042) {
		t.Fatalf("resize-caused redraw advanced last_output_at to %d", h.lastOutputAt())
	}
	// ...and the next autonomous output after its window counts again.
	h.pane = pane("hello\none\ntwo\nredraw\nthree", "~/proj  main  ctx:12%")
	if !h.tick(1050) {
		t.Fatal("output after the resize window did not count")
	}
}
