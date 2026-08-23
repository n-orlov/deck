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

// Restore is task 106's `u` undo for a completed dd: within
// DECK_DELETE_GRACE_MS it clears the tombstone (store.RestoreSession),
// returning the row to the default ListSessions view. It never resumes or
// relaunches a live pane -- Delete's own kill step is not undone by this;
// restoring only ever un-hides the row, exactly as the confirm dialog's
// own "kills the live pane (if any)" wording implies that kill is final.
func (s Service) Restore(ctx context.Context, sessionID string) (store.Session, error) {
	if s.Store == nil || s.Audit == nil || s.Clock == nil {
		return store.Session{}, errors.New("session restore requires store, audit logger, and clock")
	}
	if sessionID == "" {
		return store.Session{}, errors.New("session restore requires a durable session id")
	}
	at := s.Clock.Now().UnixMilli()
	if err := s.Store.RestoreSession(ctx, sessionID, at); err != nil {
		return store.Session{}, fmt.Errorf("restore session %q: %w", sessionID, err)
	}
	if err := s.Audit.Transition(sessionID, "restored"); err != nil {
		return store.Session{}, fmt.Errorf("audit restored session %q: %w", sessionID, err)
	}
	session, err := s.Store.GetSession(ctx, sessionID)
	if err != nil {
		return store.Session{}, fmt.Errorf("read restored session %q: %w", sessionID, err)
	}
	return session, nil
}

// Reap is task 106's DECK_DELETE_GRACE_MS expiry: once the grace window
// has elapsed since Delete tombstoned a row and it was never restored,
// the row is permanently removed (store.ReapSession). Requirement 24's
// deeper cascade -- deck's own per-session files, e.g. the captures
// directory -- is task 107's; this is only the store half.
func (s Service) Reap(ctx context.Context, sessionID string) error {
	if s.Store == nil || s.Clock == nil {
		return errors.New("session reap requires store and clock")
	}
	if sessionID == "" {
		return errors.New("session reap requires a durable session id")
	}
	at := s.Clock.Now().UnixMilli()
	if err := s.Store.ReapSession(ctx, sessionID, at); err != nil {
		return fmt.Errorf("reap session %q: %w", sessionID, err)
	}
	return nil
}
