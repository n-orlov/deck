package interactive

import (
	"sync"
	"time"
)

// renderCoalesceInterval is the grid's render-coalescing interval (PRD
// II-27, SPEC §11.9/§13.1's DECK_INTERACTIVE_MS), mirroring the default
// internal/config/schema.go's "interactive_ms" key declares (60ms) --
// chosen there, per that key's own comment, because it is the exact
// interval the Part II spike measured render cost against. Like
// paneDeadPollInterval/pipeDisplacedFallbackInterval above, this is a
// package var (not a Start parameter) so a test can lower it; wiring
// settings.InteractiveMS into it at runtime is the job of whichever later
// task plumbs interactive mode into internal/tui -- Session itself takes
// no config dependency today, the same honest-pending-consumer shape
// config/schema.go's own comment on interactive_ms already documents.
var renderCoalesceInterval = 60 * time.Millisecond

// RenderCoalescer batches many independent "content changed" signals into
// render notifications spaced at least `interval` apart. PRD II-27:
// "Coalesce renders at DECK_INTERACTIVE_MS. Render frequency dominates
// the transport's cost, not parsing." Session's drain/fallback/notice
// paths feed every write that changes grid content -- one per pipe read,
// which without coalescing would mean one Render() call per read, exactly
// the cost II-27 measures against -- through MarkDirty instead of
// rendering directly; a consumer selects on Renders() to learn when a
// repaint is actually due, at most once per interval, however many
// MarkDirty calls happened in between.
//
// The design is a plain trailing-edge debounce on a fixed tick, not a
// leaky bucket or a per-call timer: a ticker fires every interval, and
// each tick renders (produces exactly one notification) if and only if at
// least one MarkDirty call happened since the previous tick. This is
// deliberately simple -- coalescing needs to guarantee an upper bound on
// render frequency (at most 1/interval), not a lower bound on latency,
// and the PRD's own wording ("coalesced AT DECK_INTERACTIVE_MS") names
// the interval as the thing being pinned.
type RenderCoalescer struct {
	interval time.Duration

	mu    sync.Mutex
	dirty bool

	renderCh chan struct{}
	stopCh   chan struct{}
	doneCh   chan struct{}

	// closeOnce makes Close idempotent (mirrors internal/tmux.PanePipe's
	// own Close contract, which Session's tests already rely on being
	// safe to call twice -- e.g. a deferred Close after an explicit one).
	closeOnce sync.Once
}

// NewRenderCoalescer starts the coalescing goroutine immediately; Close
// must be called to stop it and release it.
func NewRenderCoalescer(interval time.Duration) *RenderCoalescer {
	c := &RenderCoalescer{
		interval: interval,
		renderCh: make(chan struct{}, 1),
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
	}
	go c.run()
	return c
}

func (c *RenderCoalescer) run() {
	defer close(c.doneCh)
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.mu.Lock()
			wasDirty := c.dirty
			c.dirty = false
			c.mu.Unlock()
			if !wasDirty {
				continue
			}
			select {
			case c.renderCh <- struct{}{}:
			default:
				// A render notification is already pending and has not
				// yet been consumed. Dropping this one IS the
				// coalescing this type exists to do: the pending
				// notification already tells the consumer a repaint is
				// due, and whenever it gets to it, it renders the
				// CURRENT grid content (whatever has changed since),
				// never a stale snapshot from whichever MarkDirty call
				// first set the flag.
			}
		}
	}
}

// MarkDirty records that the underlying grid's content changed. It never
// blocks and never itself renders; only the next tick decides that.
func (c *RenderCoalescer) MarkDirty() {
	c.mu.Lock()
	c.dirty = true
	c.mu.Unlock()
}

// Renders delivers one value per coalesced render, at most once per
// interval. The channel is never closed during normal operation (Close
// stops the goroutine but does not close this channel -- see Close's own
// doc), so a caller selects on it alongside its own shutdown signal
// instead of ranging over it directly, the same shape Session's other
// channels (e.g. Dead()) already use.
func (c *RenderCoalescer) Renders() <-chan struct{} { return c.renderCh }

// Close stops the coalescing goroutine and waits for it to have actually
// returned, so a caller never observes MarkDirty/the ticker still running
// after Close returns. It deliberately does not close the Renders()
// channel: closing a channel a consumer might still be selecting on
// alongside other cases would deliver a spurious zero-value "render due"
// notification at shutdown, which is not what happened.
//
// Close is idempotent: calling it twice must not panic on a double
// close(c.stopCh) -- Session.Close() (grid.go) is itself idempotent, and
// at least one existing Session test calls Close both explicitly and via
// a deferred call, relying on that.
func (c *RenderCoalescer) Close() {
	c.closeOnce.Do(func() {
		close(c.stopCh)
		<-c.doneCh
	})
}
