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
// session is deck-caused. until is openEnded while an attach holds it.
type discountSpan struct{ from, until int64 }

const (
	openEnded        = int64(1<<63 - 1)
	maxDiscountSpans = 32
)

type outputState struct {
	bucket   int64 // the window_activity bucket last examined
	activity int64 // the window_activity second last read
	hash     [sha256.Size]byte
}

// NewOutputClock returns a clock with the given bucket width, or
// DefaultOutputBucket when bucket is not positive.
func NewOutputClock(bucket time.Duration) *OutputClock {
	if bucket <= 0 {
		bucket = DefaultOutputBucket
	}
	return &OutputClock{Bucket: bucket, state: map[string]outputState{}, discount: map[string][]discountSpan{}}
}

// DiscountWindow is how long output stays deck-caused after a deck-caused
// resize or forwarded key (R305): MaxDiscountWindow, and never more than one
// output bucket.
func (c *OutputClock) DiscountWindow() time.Duration {
	window := MaxDiscountWindow
	if c != nil && c.Bucket > 0 && c.Bucket < window {
		window = c.Bucket
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

// AttachStarted opens a discount that lasts until AttachEnded: while a real
// tmux client is attached every byte of output is typed or redrawn through
// deck's hand-off, and deck cannot see which.
func (c *OutputClock) AttachStarted(sessionID string) {
	if c == nil {
		return
	}
	c.addSpan(sessionID, discountSpan{from: c.nowMillis(), until: openEnded})
}

// AttachEnded closes the attach discount and gives the detach redraw its own
// DiscountWindow.
func (c *OutputClock) AttachEnded(sessionID string) {
	if c == nil {
		return
	}
	now := c.nowMillis()
	end := now + c.DiscountWindow().Milliseconds()
	c.mu.Lock()
	defer c.mu.Unlock()
	spans := c.discount[sessionID]
	if n := len(spans); n > 0 && spans[n-1].until == openEnded {
		spans[n-1].until = end
		return
	}
	c.addSpanLocked(sessionID, discountSpan{from: now, until: end})
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
	if c.Bucket <= 0 {
		return false, errors.New("output clock bucket must be positive")
	}
	width := int64(c.Bucket / time.Second)
	if width < 1 {
		width = 1
	}
	bucket := windowActivity / width

	c.mu.Lock()
	previous, seen := c.state[sessionID]
	c.mu.Unlock()
	if seen && bucket <= previous.bucket {
		// R305: output inside a deck-caused window in a bucket already
		// examined is still hashed, so the deck-caused text becomes the
		// baseline and the next bucket's read does not count it. Only a
		// window_activity that moved and lies in a window costs a capture.
		if windowActivity <= previous.activity {
			return false, nil
		}
		if !c.discounted(sessionID, windowActivity) {
			previous.activity = windowActivity
			c.mu.Lock()
			c.state[sessionID] = previous
			c.mu.Unlock()
			return false, nil
		}
		pane, err := capture(ctx)
		if err != nil {
			return false, fmt.Errorf("capture pane for output clock of session %q: %w", sessionID, err)
		}
		c.mu.Lock()
		c.state[sessionID] = outputState{bucket: previous.bucket, activity: windowActivity, hash: sha256.Sum256([]byte(agent.TranscriptRegion(pane)))}
		c.mu.Unlock()
		return false, nil
	}
	pane, err := capture(ctx)
	if err != nil {
		return false, fmt.Errorf("capture pane for output clock of session %q: %w", sessionID, err)
	}
	hash := sha256.Sum256([]byte(agent.TranscriptRegion(pane)))
	changed := seen && hash != previous.hash
	// R305: a transcript change the pane shows inside a deck-caused window
	// is deck's own doing. The new hash is still remembered below, so the
	// same text never counts later.
	if c.discounted(sessionID, windowActivity) {
		changed = false
	}
	if changed {
		if err := store.RecordOutput(ctx, sessionID, windowActivity*1000); err != nil {
			return false, fmt.Errorf("record output for session %q: %w", sessionID, err)
		}
	}
	c.mu.Lock()
	c.state[sessionID] = outputState{bucket: bucket, activity: windowActivity, hash: hash}
	c.mu.Unlock()
	return changed, nil
}
