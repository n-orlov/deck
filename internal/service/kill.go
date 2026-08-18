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
		return errors.New("session is already stopped")
	}
	if err := s.TMux.Kill(ctx, session.Slug); err != nil {
		return fmt.Errorf("kill tmux session %q: %w", session.Name, err)
	}
	if err := s.Store.UpdateSessionStatus(ctx, store.StatusUpdateInput{
		SessionID: session.ID,
		Status:    "stopped",
		Reason:    "killed by user",
		Source:    "user",
		At:        s.Clock.Now().UnixMilli(),
		EventKind: "killed",
	}); err != nil {
		return fmt.Errorf("record killed session %q: %w", session.Name, err)
	}
	if err := s.Audit.Transition(session.ID, "killed"); err != nil {
		return fmt.Errorf("audit killed session %q: %w", session.Name, err)
	}
	return nil
}
