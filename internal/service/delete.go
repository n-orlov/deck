package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/n-orlov/deck/internal/store"
)

// Delete is task 105's `dd` submit: it kills the live pane if one exists
// (a stopped session is left as-is, exactly like a plain Kill would refuse
// to double-kill) and then tombstones the row via store.SoftDeleteSession,
// so it disappears from the default ListSessions view immediately while
// remaining restorable within the grace window task 106 implements. It
// never touches the session's cwd or its conversation id/transcript —
// those are exactly what the confirm dialog states survives.
func (s Service) Delete(ctx context.Context, session store.Session) error {
	if s.Store == nil || s.Audit == nil || s.Clock == nil {
		return errors.New("session delete requires store, audit logger, and clock")
	}
	if session.ID == "" {
		return errors.New("session delete requires a durable session id")
	}
	if session.Status != "stopped" {
		if session.Slug == "" {
			return errors.New("session delete requires a durable slug to kill a live pane")
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
	}
	at := s.Clock.Now().UnixMilli()
	if err := s.Store.SoftDeleteSession(ctx, session.ID, at); err != nil {
		return fmt.Errorf("tombstone session %q: %w", session.Name, err)
	}
	if err := s.Audit.Transition(session.ID, "deleted"); err != nil {
		return fmt.Errorf("audit deleted session %q: %w", session.Name, err)
	}
	return nil
}
