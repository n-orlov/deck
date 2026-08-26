package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/n-orlov/deck/internal/store"
)

// Kill stops the live tmux session while retaining its durable identity and
// conversation metadata. The cwd is deliberately never touched: it belongs to
// the user, not to a deck session. A successful kill is recorded as both the
// durable "killed" event and an audit transition, leaving the row stopped and
// resumable for a later explicit resume.
func (s Service) Kill(ctx context.Context, session store.Session) error {
	if s.Store == nil || s.Audit == nil || s.Clock == nil {
		return errors.New("session kill requires store, audit logger, and clock")
	}
	if session.ID == "" || session.Slug == "" {
		return errors.New("session kill requires a durable session id and slug")
	}
	if session.Status == "stopped" {
		// "Already stopped" must mean stopped AND gone: under deck's
		// server-wide `remain-on-exit failed` a non-zero exit RETAINS the
		// dead pane and its session, and a SessionEnd hook can write
		// `stopped` in the same millisecond, so a status-only guard refused
		// `x` on precisely the row that still had a tmux session to remove
		// (#6, leg 3) — leaving no in-app way to free the session name.
		// Leg 1 makes reconcile collect such a corpse on sight, so this
		// branch should now be unreachable in practice; it is deliberate
		// belt-and-braces, kept because the cost of being wrong is a
		// permanently unrecoverable session and the cost of the extra
		// `has-session` is one tmux round trip on a refusal path.
		retained, err := s.TMux.Exists(ctx, session.Slug)
		if err != nil {
			return fmt.Errorf("check for a retained tmux session for %q: %w", session.Name, err)
		}
		if !retained {
			return errors.New("session is already stopped")
		}
	}
	if err := s.TMux.Kill(ctx, session.Slug); err != nil {
		return fmt.Errorf("kill tmux session %q: %w", session.Name, err)
	}
	if err := s.Store.UpdateSessionStatus(ctx, store.StatusUpdateInput{
		SessionID:    session.ID,
		Status:       "stopped",
		Reason:       "killed by user",
		Source:       "user",
		At:           s.Clock.Now().UnixMilli(),
		EventKind:    "killed",
		KilledByUser: true,
	}); err != nil {
		return fmt.Errorf("record killed session %q: %w", session.Name, err)
	}
	if err := s.Audit.Transition(session.ID, "killed"); err != nil {
		return fmt.Errorf("audit killed session %q: %w", session.Name, err)
	}
	return nil
}
