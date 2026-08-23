package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/n-orlov/deck/internal/store"
)

// Archive is task 111's `A` (SPEC requirement 27, SPEC.md:284-287):
// archived_at is a flag, never a status, so a stopped row's Status is left
// completely alone -- only store.ArchiveSession's own timestamp column
// changes. "Archiving requires stopped" is enforced here, exactly the way
// Delete already enforces "kill first" for dd: a non-stopped session is
// killed (the same kill-then-continue steps Delete/Kill already record)
// before being archived, so the UI never has to refuse the keypress and
// can instead offer "kill and archive" as one action (§4's invariant: a
// live agent is never hidden by an archived row). It never touches the
// session's cwd or its conversation id/transcript.
func (s Service) Archive(ctx context.Context, session store.Session) error {
	if s.Store == nil || s.Audit == nil || s.Clock == nil {
		return errors.New("session archive requires store, audit logger, and clock")
	}
	if session.ID == "" {
		return errors.New("session archive requires a durable session id")
	}
	if session.Status != "stopped" {
		if session.Slug == "" {
			return errors.New("session archive requires a durable slug to kill a live pane")
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
	if err := s.Store.ArchiveSession(ctx, session.ID, at); err != nil {
		return fmt.Errorf("archive session %q: %w", session.Name, err)
	}
	if err := s.Audit.Transition(session.ID, "archived"); err != nil {
		return fmt.Errorf("audit archived session %q: %w", session.Name, err)
	}
	return nil
}
