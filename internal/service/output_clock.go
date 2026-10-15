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

	mu    sync.Mutex
	state map[string]outputState
}

type outputState struct {
	bucket int64 // the window_activity bucket last examined
	hash   [sha256.Size]byte
}

// NewOutputClock returns a clock with the given bucket width, or
// DefaultOutputBucket when bucket is not positive.
func NewOutputClock(bucket time.Duration) *OutputClock {
	if bucket <= 0 {
		bucket = DefaultOutputBucket
	}
	return &OutputClock{Bucket: bucket, state: map[string]outputState{}}
}

// Observe takes one reconcile tick's reading for a session: windowActivity is
// tmux's window_activity in Unix seconds, and capture reads the pane. It
// reports whether it wrote last_output_at. capture is called only when the
// bucket advanced; a tick whose bucket did not change reads and writes nothing.
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
		return false, nil
	}
	pane, err := capture(ctx)
	if err != nil {
		return false, fmt.Errorf("capture pane for output clock of session %q: %w", sessionID, err)
	}
	hash := sha256.Sum256([]byte(agent.TranscriptRegion(pane)))
	changed := seen && hash != previous.hash
	if changed {
		if err := store.RecordOutput(ctx, sessionID, windowActivity*1000); err != nil {
			return false, fmt.Errorf("record output for session %q: %w", sessionID, err)
		}
	}
	c.mu.Lock()
	c.state[sessionID] = outputState{bucket: bucket, hash: hash}
	c.mu.Unlock()
	return changed, nil
}
