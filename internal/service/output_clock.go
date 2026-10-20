package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/n-orlov/deck/internal/agent"
	"github.com/n-orlov/deck/internal/config"
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
// pane, hashes the transcript region (agent.TranscriptRegion, above the
// composer frame or codex's composer) and counts the bucket only when that hash
// differs from the one it last saw (for a framed draft that mimics the frame:
// when no possible composer top leaves it, pickReading). The column is written with a plain UPDATE, never an
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

// It holds instants, never bucket indices: an index is only meaningful at the
// width it was computed with, and [ui] output_bucket changes live (R306). Every
// comparison re-buckets these instants at the clock's current width, so the
// boundaries the clock compares are always the configured ones.
type outputState struct {
	// activity is the window_activity second last read. The bucket it falls
	// in at the current width is the last bucket examined: under a fixed
	// width it only moves inside the bucket the last capture examined.
	activity int64
	// hash is the baseline transcript region's hash, composer the hash of the
	// composer frame read with it (pickReading).
	hash     [sha256.Size]byte
	composer [sha256.Size]byte
	// size is the pane size (pane_width x pane_height) the reading was taken
	// at (R409); the zero PaneSize means the caller did not know it.
	size PaneSize
	// countedAt is the window_activity second of the last counted change
	// (one that wrote last_output_at; 0 = none) and streak the run of
	// consecutive counted buckets ending at its bucket (R316).
	countedAt int64
	streak    int
}

// PaneSize is a pane's width x height in cells (tmux pane_width, pane_height).
// The zero value means the size is unknown, and an unknown size never keys a
// baseline.
type PaneSize struct{ Width, Height int }

func (p PaneSize) known() bool { return p.Width > 0 && p.Height > 0 }

// PaneCapture reads the pane. It returns the text and the pane size read just
// after the capture (the zero PaneSize when it did not re-read the size).
type PaneCapture func(context.Context) (string, PaneSize, error)

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
// Observe reads the new width; the baselines already held stay as they are,
// and are re-bucketed at the new width, so the bucket a held reading falls in
// is always judged by the boundaries now configured.
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

// startMillis is the span start for the instant the caller took before it
// issued the action; the zero time means the caller did not take one, and the
// clock's own now stands in.
func (c *OutputClock) startMillis(at time.Time) int64 {
	if at.IsZero() {
		return c.nowMillis()
	}
	return at.UnixMilli()
}

// Discount marks the DiscountWindow after at as deck-caused for a session (a
// deck-caused resize or a forwarded key); any other cause is ignored. at is the
// instant the caller took BEFORE it issued the tmux call or write (R410): the
// span starts there, so the redraw the action causes can never be stamped
// before the span opens. Output the pane window_activity places inside it does
// not advance last_output_at. Safe on a nil clock.
func (c *OutputClock) Discount(sessionID string, cause DiscountCause, at time.Time) {
	if c == nil || (cause != DiscountResize && cause != DiscountKey) {
		return
	}
	start := c.startMillis(at)
	c.addSpan(sessionID, discountSpan{from: start, until: start + c.DiscountWindow().Milliseconds()})
}

// AttachStarted discounts the DiscountWindow after at, the instant taken before
// the hand-off: handing the terminal to a tmux client resizes the window and
// makes the pane redraw. The discount is bounded like every other cause
// (R305): it never lasts longer than one window, so an agent's own output later
// in a long attach counts.
func (c *OutputClock) AttachStarted(sessionID string, at time.Time) {
	if c == nil {
		return
	}
	start := c.startMillis(at)
	c.addSpan(sessionID, discountSpan{from: start, until: start + c.DiscountWindow().Milliseconds()})
}

// AttachEnded gives the detach repaint its own DiscountWindow, starting at the
// instant the client was seen to end. What was printed between the attach and
// the detach is not discounted.
func (c *OutputClock) AttachEnded(sessionID string, at time.Time) {
	if c == nil {
		return
	}
	start := c.startMillis(at)
	c.addSpan(sessionID, discountSpan{from: start, until: start + c.DiscountWindow().Milliseconds()})
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

// discounted reports whether tmux's window_activity second covers a
// deck-caused span, and drops spans that ended before it: window_activity only
// moves forward. The second names the whole of [second*1000, second*1000+999]
// ms, and it is covered iff a span overlaps that interval in any way (R410).
func (c *OutputClock) discounted(sessionID string, windowActivity int64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	spans := c.discount[sessionID]
	if len(spans) == 0 {
		return false
	}
	secondStart := windowActivity * 1000
	secondEnd := secondStart + 999
	kept := spans[:0]
	hit := false
	for _, span := range spans {
		if span.until < secondStart {
			continue
		}
		kept = append(kept, span)
		if span.from <= secondEnd {
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

// Observe is ObserveSized for a caller that does not know the pane size.
func (c *OutputClock) Observe(ctx context.Context, store OutputStore, sessionID string, windowActivity int64, capture func(context.Context) (string, error)) (bool, error) {
	return c.ObserveSized(ctx, store, sessionID, windowActivity, PaneSize{}, func(ctx context.Context) (string, PaneSize, error) {
		text, err := capture(ctx)
		return text, PaneSize{}, err
	})
}

// ObserveSized takes one reconcile tick's reading for a session: windowActivity
// is tmux's window_activity in Unix seconds, size the pane size the same pane
// facts report, and capture reads the pane. It reports whether it wrote
// last_output_at.
//
// A stored reading carries the size it was taken at (R409). A pane whose size
// differs from its stored reading's is re-captured on that tick whether or not
// its bucket advanced, and the new reading becomes the baseline: a resize from
// any source reflows the transcript, which is never output, so nothing is
// counted. A size that differs between the read before the capture and the one
// capture returns after it makes the reading a baseline too. At an unchanged
// size, capture is called when the bucket advanced, or when window_activity
// moved inside a discount window (to remember the deck-caused text); any other
// tick captures and writes nothing.
func (c *OutputClock) ObserveSized(ctx context.Context, store OutputStore, sessionID string, windowActivity int64, size PaneSize, capture PaneCapture) (bool, error) {
	if c == nil || windowActivity <= 0 {
		return false, nil
	}
	c.mu.Lock()
	width := c.Bucket
	previous, seen := c.state[sessionID]
	c.mu.Unlock()
	if width <= 0 {
		return false, errors.New("output clock bucket must be positive")
	}
	if seen && size.known() && previous.size != size {
		return false, c.rebaseline(ctx, sessionID, windowActivity, size, previous, capture)
	}
	bucket := secondBucket(windowActivity, width)
	if seen && bucket <= secondBucket(previous.activity, width) {
		return false, c.observeExaminedBucket(ctx, sessionID, windowActivity, size, previous, capture)
	}
	return c.observeNewBucket(ctx, store, sessionID, windowActivity, width, size, previous, seen, capture)
}

// rebaseline is the tick of a pane whose size differs from its stored
// reading's: the pane is captured, the capture becomes the baseline, and
// nothing is counted. window_activity only advances, so a bucket that moved
// with the resize is examined here and never counts later. The size kept is
// the one read before the capture; when the size read after it differs, the
// capture may be a mix of two layouts, so the stored size is made unequal to
// the real one and the next tick re-captures at the settled size.
func (c *OutputClock) rebaseline(ctx context.Context, sessionID string, windowActivity int64, size PaneSize, previous outputState, capture PaneCapture) error {
	readings, after, err := captureTranscriptReadings(ctx, sessionID, capture)
	if err != nil {
		return err
	}
	next := previous
	next.hash, next.composer, next.size = readings[0].region, readings[0].composer, size
	if windowActivity > next.activity {
		next.activity = windowActivity
	}
	if after.known() && after != size {
		next.size = PaneSize{}
	}
	c.mu.Lock()
	c.state[sessionID] = next
	c.mu.Unlock()
	return nil
}

// secondBucket is the bucket a Unix second falls in at width, by the one rule
// the activity sort uses too (config.OutputBucketIndex).
func secondBucket(unixSeconds int64, width time.Duration) int64 {
	return config.OutputBucketIndex(unixSeconds*1000, width)
}

// observeNewBucket reads the pane for a bucket not yet examined: a transcript
// change it shows counts (writes last_output_at) unless it lies inside a
// deck-caused window (R305). The new hash is remembered either way, so the
// same text never counts later. It reports whether it wrote.
func (c *OutputClock) observeNewBucket(ctx context.Context, store OutputStore, sessionID string, windowActivity int64, width time.Duration, size PaneSize, previous outputState, seen bool, capture PaneCapture) (bool, error) {
	readings, after, err := captureTranscriptReadings(ctx, sessionID, capture)
	if err != nil {
		return false, err
	}
	resized := sizeFlipped(size, after)
	reading, same := pickReading(readings, previous)
	discounted := c.discounted(sessionID, windowActivity)
	changed := seen && !same && !discounted && !resized
	if changed {
		if err := store.RecordOutput(ctx, sessionID, windowActivity*1000); err != nil {
			return false, fmt.Errorf("record output for session %q: %w", sessionID, err)
		}
	}
	next := newBucketState(previous, reading, windowActivity, width, size, changed, resized)
	c.mu.Lock()
	c.state[sessionID] = next
	c.mu.Unlock()
	return changed, nil
}

// sizeFlipped reports whether the size read after a capture differs from the
// one read before it, both known: the capture may mix two layouts.
func sizeFlipped(before, after PaneSize) bool {
	return after.known() && before.known() && after != before
}

// newBucketState is the reading observeNewBucket keeps: the new hash at the
// new activity, a counted change marked (countBucket), and a size that
// flipped mid-capture stored as unknown so the next tick re-captures.
func newBucketState(previous outputState, reading transcriptReading, windowActivity int64, width time.Duration, size PaneSize, changed, resized bool) outputState {
	next := outputState{activity: windowActivity, hash: reading.region, composer: reading.composer, size: size, countedAt: previous.countedAt, streak: previous.streak}
	if changed {
		next.countBucket(previous, width)
	}
	if resized {
		next.size = PaneSize{}
	}
	return next
}

// bucketOf is the bucket window_activity (Unix seconds) falls in at the
// clock's exact width, fractional seconds included, by the same rule the
// activity sort uses (config.OutputBucketIndex).
func (c *OutputClock) bucketOf(windowActivity int64) (int64, error) {
	bucketWidth := c.bucketWidth()
	if bucketWidth <= 0 {
		return 0, errors.New("output clock bucket must be positive")
	}
	return secondBucket(windowActivity, bucketWidth), nil
}

// transcriptReading is one agent.TranscriptCut, hashed.
type transcriptReading struct {
	region, composer [sha256.Size]byte
	framed           bool
}

// captureTranscriptReadings reads the pane and hashes its readings
// (agent.TranscriptCuts), the chosen boundary first. It also returns the pane
// size the capture reported after reading.
func captureTranscriptReadings(ctx context.Context, sessionID string, capture PaneCapture) ([]transcriptReading, PaneSize, error) {
	pane, after, err := capture(ctx)
	if err != nil {
		return nil, PaneSize{}, fmt.Errorf("capture pane for output clock of session %q: %w", sessionID, err)
	}
	cuts := agent.TranscriptCuts(pane)
	readings := make([]transcriptReading, len(cuts))
	for i, cut := range cuts {
		readings[i] = transcriptReading{
			region:   sha256.Sum256([]byte(cut.Region)),
			composer: sha256.Sum256([]byte(cut.Composer)),
			framed:   cut.Composer != "",
		}
	}
	return readings, after, nil
}

// pickReading chooses the reading that becomes the new baseline and reports
// whether the transcript is unchanged against previous. A pane whose framed
// draft holds a line drawn exactly like the frame has more than one possible
// composer top (agent.TranscriptCuts); the transcript is unchanged when ANY of
// them leaves exactly the baseline region above it, so a draft-only edit never
// counts once the clock has read the frame, however the draft mimics it. When
// none does, the transcript changed whichever cut is right, and the baseline
// follows the cut whose composer is the one read last (output arrived above an
// unchanged draft), else the chosen cut. A pane with one reading is compared
// exactly as before.
func pickReading(readings []transcriptReading, previous outputState) (transcriptReading, bool) {
	for _, r := range readings {
		if r.region == previous.hash {
			return r, true
		}
	}
	for _, r := range readings {
		if r.framed && r.composer == previous.composer {
			return r, false
		}
	}
	return readings[0], false
}

// countBucket marks next's reading as a counted change: consecutive counted
// buckets at the current width extend the streak, any gap restarts it at one.
func (next *outputState) countBucket(previous outputState, width time.Duration) {
	if previous.countedAt != 0 && secondBucket(previous.countedAt, width) == secondBucket(next.activity, width)-1 {
		next.streak++
	} else {
		next.streak = 1
	}
	next.countedAt = next.activity
}

// observeExaminedBucket handles a tick whose bucket was already examined
// (R305): output inside a deck-caused window in such a bucket is still hashed,
// so the deck-caused text becomes the baseline and the next bucket's read does
// not count it. Only a window_activity that moved and lies in a window costs a
// capture. It never writes last_output_at.
func (c *OutputClock) observeExaminedBucket(ctx context.Context, sessionID string, windowActivity int64, size PaneSize, previous outputState, capture PaneCapture) error {
	if windowActivity <= previous.activity {
		return nil
	}
	if c.discounted(sessionID, windowActivity) {
		readings, after, err := captureTranscriptReadings(ctx, sessionID, capture)
		if err != nil {
			return err
		}
		reading, _ := pickReading(readings, previous)
		previous.hash, previous.composer = reading.region, reading.composer
		if after.known() && size.known() && after != size {
			previous.size = PaneSize{}
		}
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
	if c.Bucket <= 0 {
		return false
	}
	return config.OutputBucketIndex(c.nowMillis(), c.Bucket) <= secondBucket(state.countedAt, c.Bucket)+1
}
