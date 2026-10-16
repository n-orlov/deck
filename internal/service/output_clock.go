package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/n-orlov/deck/internal/agent"
)

// DefaultOutputBucket is the width of one output-clock bucket (R304). The
// clock moves at most once per bucket per session, so a running agent that
// prints constantly costs one UPDATE per bucket, not one per tick.
const DefaultOutputBucket = 10 * time.Second

// MaxDiscountWindow caps the discount window (R305). The window is
// min(Bucket, MaxDiscountWindow): never longer than one output bucket.
const MaxDiscountWindow = 5 * time.Second

// DiscountCause names what deck did to a session's pane that makes the
// output it provokes deck's own doing and not the agent's (R305).
type DiscountCause string

const (
	// DiscountResize is a deck-caused resize: preview fit or interactive
	// entry/exit geometry, each of which makes the pane's program redraw.
	DiscountResize DiscountCause = "resize"
	// DiscountKey is a key deck forwarded into the pane (interactive mode).
	DiscountKey DiscountCause = "key"
)

// OutputStore is the one store write the output clock makes.
type OutputStore interface {
	RecordOutput(ctx context.Context, sessionID string, at int64) error
}

// OutputClock feeds sessions.last_output_at (R304). tmux's window_activity
// says WHEN a window last produced output, but it also moves for footer and
// statusline churn (ctx:%, branch, model), so it only nominates: when a
// session's window_activity moves into a new bucket the clock captures the
// pane, hashes the transcript region (agent.TranscriptRegion, above the last
// two separator lines) and counts the bucket only when that hash differs from
// the one it last saw. The column is written with a plain UPDATE, never an
// event row, and only on a counted bucket.
//
// The state is in memory: the first sight of a session after deck starts sets
// a baseline and counts nothing, so a restart never makes every row look
// freshly active. It is safe for concurrent use.
type OutputClock struct {
	Bucket time.Duration
	// Now reads the wall clock for discount windows; nil means time.Now.
	Now func() time.Time

	mu       sync.Mutex
	state    map[string]outputState
	discount map[string][]discountSpan
}

// discountSpan is one stretch of Unix milliseconds in which output on a
// session is deck-caused. Every span is bounded by one DiscountWindow.
type discountSpan struct{ from, until int64 }

const maxDiscountSpans = 32

type outputState struct {
	bucket   int64 // the window_activity bucket last examined
	activity int64 // the window_activity second last read
	hash     [sha256.Size]byte
	// counted is the last bucket that wrote last_output_at (0 = none) and
	// streak the run of consecutive counted buckets ending there (R316).
	counted int64
	streak  int
}

// NewOutputClock returns a clock with the given bucket width, or
// DefaultOutputBucket when bucket is not positive.
func NewOutputClock(bucket time.Duration) *OutputClock {
	if bucket <= 0 {
		bucket = DefaultOutputBucket
	}
	return &OutputClock{Bucket: bucket, state: map[string]outputState{}, discount: map[string][]discountSpan{}}
}

// SetBucket changes the bucket width while the clock runs (R306: [ui]
// output_bucket applies live). A non-positive width is ignored. The next
// Observe reads the new width; the baselines already held stay as they are.
func (c *OutputClock) SetBucket(bucket time.Duration) {
	if c == nil || bucket <= 0 {
		return
	}
	c.mu.Lock()
	c.Bucket = bucket
	c.mu.Unlock()
}

func (c *OutputClock) bucketWidth() time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Bucket
}

// DiscountWindow is how long output stays deck-caused after a deck-caused
// resize or forwarded key (R305): MaxDiscountWindow, and never more than one
// output bucket.
func (c *OutputClock) DiscountWindow() time.Duration {
	window := MaxDiscountWindow
	if c != nil {
		if bucket := c.bucketWidth(); bucket > 0 && bucket < window {
			window = bucket
		}
	}
	return window
}

func (c *OutputClock) nowMillis() int64 {
	if c.Now != nil {
		return c.Now().UnixMilli()
	}
	return time.Now().UnixMilli()
}

// Discount marks the DiscountWindow after now as deck-caused for a session
// (a deck-caused resize or a forwarded key); any other cause is ignored. Output the pane window_activity
// places inside it does not advance last_output_at. Safe on a nil clock.
func (c *OutputClock) Discount(sessionID string, cause DiscountCause) {
	if c == nil || (cause != DiscountResize && cause != DiscountKey) {
		return
	}
	now := c.nowMillis()
	c.addSpan(sessionID, discountSpan{from: now, until: now + c.DiscountWindow().Milliseconds()})
}

// AttachStarted discounts the DiscountWindow after the hand-off: handing the
// terminal to a tmux client resizes the window and makes the pane redraw. The
// discount is bounded like every other cause (R305): it never lasts longer
// than one window, so an agent's own output later in a long attach counts.
func (c *OutputClock) AttachStarted(sessionID string) {
	if c == nil {
		return
	}
	now := c.nowMillis()
	c.addSpan(sessionID, discountSpan{from: now, until: now + c.DiscountWindow().Milliseconds()})
}

// AttachEnded gives the detach repaint its own DiscountWindow. What was
// printed between the attach and the detach is not discounted.
func (c *OutputClock) AttachEnded(sessionID string) {
	if c == nil {
		return
	}
	now := c.nowMillis()
	c.addSpan(sessionID, discountSpan{from: now, until: now + c.DiscountWindow().Milliseconds()})
}

func (c *OutputClock) addSpan(sessionID string, span discountSpan) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.addSpanLocked(sessionID, span)
}

// addSpanLocked merges the new span into the newest one when they touch, so a
// run of keystrokes is one span, and keeps the list short.
func (c *OutputClock) addSpanLocked(sessionID string, span discountSpan) {
	if c.discount == nil {
		c.discount = map[string][]discountSpan{}
	}
	spans := c.discount[sessionID]
	if n := len(spans); n > 0 && span.from <= spans[n-1].until {
		if span.until > spans[n-1].until {
			spans[n-1].until = span.until
		}
		return
	}
	spans = append(spans, span)
	if len(spans) > maxDiscountSpans {
		spans = spans[len(spans)-maxDiscountSpans:]
	}
	c.discount[sessionID] = spans
}

// discounted reports whether tmux's window_activity second (which may lie
// anywhere in [at, at+999] ms) falls in a deck-caused span, and drops spans
// that ended before it: window_activity only moves forward.
func (c *OutputClock) discounted(sessionID string, windowActivity int64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	spans := c.discount[sessionID]
	if len(spans) == 0 {
		return false
	}
	atMs := windowActivity * 1000
	kept := spans[:0]
	hit := false
	for _, span := range spans {
		if span.until < atMs {
			continue
		}
		kept = append(kept, span)
		if span.from <= atMs+999 {
			hit = true
		}
	}
	if len(kept) == 0 {
		delete(c.discount, sessionID)
	} else {
		c.discount[sessionID] = kept
	}
	return hit
}

// Observe takes one reconcile tick's reading for a session: windowActivity is
// tmux's window_activity in Unix seconds, and capture reads the pane. It
// reports whether it wrote last_output_at. capture is called when the bucket
// advanced, or when window_activity moved inside a discount window (to
// remember the deck-caused text); any other tick captures and writes nothing.
func (c *OutputClock) Observe(ctx context.Context, store OutputStore, sessionID string, windowActivity int64, capture func(context.Context) (string, error)) (bool, error) {
	if c == nil || windowActivity <= 0 {
		return false, nil
	}
	width, err := c.bucketSeconds()
	if err != nil {
		return false, err
	}
	bucket := windowActivity / width

	c.mu.Lock()
	previous, seen := c.state[sessionID]
	c.mu.Unlock()
	if seen && bucket <= previous.bucket {
		return false, c.observeExaminedBucket(ctx, sessionID, windowActivity, previous, capture)
	}
	return c.observeNewBucket(ctx, store, sessionID, windowActivity, bucket, previous, seen, capture)
}

// observeNewBucket reads the pane for a bucket not yet examined: a transcript
// change it shows counts (writes last_output_at) unless it lies inside a
// deck-caused window (R305). The new hash is remembered either way, so the
// same text never counts later. It reports whether it wrote.
func (c *OutputClock) observeNewBucket(ctx context.Context, store OutputStore, sessionID string, windowActivity, bucket int64, previous outputState, seen bool, capture func(context.Context) (string, error)) (bool, error) {
	hash, err := captureTranscriptHash(ctx, sessionID, capture)
	if err != nil {
		return false, err
	}
	discounted := c.discounted(sessionID, windowActivity)
	changed := seen && hash != previous.hash && !discounted
	if changed {
		if err := store.RecordOutput(ctx, sessionID, windowActivity*1000); err != nil {
			return false, fmt.Errorf("record output for session %q: %w", sessionID, err)
		}
	}
	next := outputState{bucket: bucket, activity: windowActivity, hash: hash, counted: previous.counted, streak: previous.streak}
	if changed {
		next.countBucket(previous)
	}
	c.mu.Lock()
	c.state[sessionID] = next
	c.mu.Unlock()
	return changed, nil
}

// bucketSeconds is the bucket width in whole seconds (at least one).
func (c *OutputClock) bucketSeconds() (int64, error) {
	bucketWidth := c.bucketWidth()
	if bucketWidth <= 0 {
		return 0, errors.New("output clock bucket must be positive")
	}
	return max(int64(bucketWidth/time.Second), 1), nil
}

// captureTranscriptHash reads the pane and hashes its transcript region.
func captureTranscriptHash(ctx context.Context, sessionID string, capture func(context.Context) (string, error)) ([sha256.Size]byte, error) {
	pane, err := capture(ctx)
	if err != nil {
		return [sha256.Size]byte{}, fmt.Errorf("capture pane for output clock of session %q: %w", sessionID, err)
	}
	return sha256.Sum256([]byte(agent.TranscriptRegion(pane))), nil
}

// countBucket marks next as the bucket a change was counted in: consecutive
// counted buckets extend the streak, any gap restarts it at one.
func (next *outputState) countBucket(previous outputState) {
	if previous.counted != 0 && previous.counted == next.bucket-1 {
		next.streak++
	} else {
		next.streak = 1
	}
	next.counted = next.bucket
}

// observeExaminedBucket handles a tick whose bucket was already examined
// (R305): output inside a deck-caused window in such a bucket is still hashed,
// so the deck-caused text becomes the baseline and the next bucket's read does
// not count it. Only a window_activity that moved and lies in a window costs a
// capture. It never writes last_output_at.
func (c *OutputClock) observeExaminedBucket(ctx context.Context, sessionID string, windowActivity int64, previous outputState, capture func(context.Context) (string, error)) error {
	if windowActivity <= previous.activity {
		return nil
	}
	if c.discounted(sessionID, windowActivity) {
		hash, err := captureTranscriptHash(ctx, sessionID, capture)
		if err != nil {
			return err
		}
		previous.hash = hash
	}
	previous.activity = windowActivity
	c.mu.Lock()
	c.state[sessionID] = previous
	c.mu.Unlock()
	return nil
}

// Moving reports the movement fallback's reading for a session (R316): its
// transcript region changed in two consecutive buckets (a counted bucket is
// one Observe wrote last_output_at for, so footer-only churn and deck-caused
// output never count) and the bucket after the newest counted one has not yet
// passed quiet. One whole quiet bucket ends it. A session this process has
// not counted output for is not moving.
func (c *OutputClock) Moving(sessionID string) bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	state, ok := c.state[sessionID]
	if !ok || state.streak < 2 {
		return false
	}
	width := int64(c.Bucket / time.Second)
	if width < 1 {
		width = 1
	}
	return c.nowMillis()/1000/width <= state.counted+1
}
