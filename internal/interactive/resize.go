package interactive

import (
	"context"
	"fmt"
)

// Resize reseeds the session for a new pane size rather than calling
// Resize on the existing grid in place (PRD II-21/II-22). Two separate
// measurements ruled out both of the tempting alternatives: calling the
// existing emulator's own Resize() alone as "the correction" left ~1000
// wrong cells for 3.4s against an append-only pane (resizing the canvas
// changes nothing about content that has already drifted from the
// pane's real current state), and doing nothing at all left a stable
// divergence for 31.6s that never healed on its own. A full reseed --
// a fresh capture written into a fresh grid -- corrected the divergence
// in the very sample that detected it.
//
// "Fresh grid" is not merely "fresh content": it is also a fresh
// escape-sequence parser (PRD II-22). The old grid may have a live
// stream's control sequence parsed up through its introducer but not
// yet its final byte -- ordinary output can stop there at any byte
// boundary a resize happens to land on. Writing the new seed into that
// SAME instance would hand the parser's unfinished state the new seed's
// leading bytes as if they were that sequence's own continuation,
// consuming and thereby losing however many of them complete it (see
// TestReseedIntoSameGridCorruptsAfterTruncatedControlSequence for the
// demonstrated red control). A brand-new *Grid has no parser history to
// misinterpret anything as a continuation of.
//
// seed is invoked (exactly once, at the NEW width/height -- callers are
// expected to have already resized the underlying tmux pane/window
// before calling Resize, the same as Start expects the pipe to already
// be armed) to capture that fresh state; whatever it returns is written
// into the fresh grid before that grid replaces the session's current
// one under the write lock, so a concurrent Grid() call or the drain
// goroutine's next iteration (see currentGrid) either sees the OLD grid
// in full or the NEW one in full, never a grid mid-swap.
func (s *Session) Resize(ctx context.Context, width, height int, seed func(ctx context.Context) ([]byte, error)) error {
	if seed == nil {
		return fmt.Errorf("resize: seed is required (every resize is a full reseed, never a bare resize)")
	}
	data, err := seed(ctx)
	if err != nil {
		return fmt.Errorf("reseed capture for resize to %dx%d: %w", width, height, err)
	}

	fresh := newGrid(width, height)
	if _, err := fresh.Write(data); err != nil {
		return fmt.Errorf("write reseed capture into fresh grid for resize to %dx%d: %w", width, height, err)
	}

	s.mu.Lock()
	s.grid = fresh
	s.mu.Unlock()
	// A resize replaces grid content wholesale (see this function's own
	// doc); a consumer selecting on Renders() must be told a repaint is
	// due for the same reason drain/fallbackLoop/writeNotice all do.
	s.renders.MarkDirty()
	return nil
}
