package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/n-orlov/deck/internal/store"
)

// Restart implements SPEC §6.2/§6.3's `R`: kill the selected session's live
// tmux pane if one exists, then relaunch it exactly through the same
// Resume path a stopped session's own `r` uses -- the SAME conversation id
// (never a fresh one, unless resume_state is itself "fresh-once"/"pinned",
// which apply identically to a plain resume), the session's current
// persisted permission profile and launch_args, and, since Resume always
// re-reads the row from the store, whatever environment is currently
// persisted, including an edit made through the `e` editor while the old
// pane was still running. It is the only path that ever applies a pending
// env edit to an already-running process, and the only path that clears
// env_dirty (the `env↻` badge) back to false; a plain `r` resume of an
// already-stopped row never clears it, since that value was never applied
// to a live process in the first place.
//
// A row already "stopped" has no live pane to kill and is refused here
// (the caller should use Resume/`r` instead) so Restart never masquerades
// as a first launch.
func (s Service) Restart(ctx context.Context, sessionID string) (store.Session, ResumeOutcome, error) {
	if s.Store == nil || s.Audit == nil || s.Clock == nil || s.Agents == nil {
		return store.Session{}, ResumeStartingElsewhere, errors.New("restart requires store, audit logger, clock, and adapter registry")
	}
	if sessionID == "" {
		return store.Session{}, ResumeStartingElsewhere, errors.New("session id is required")
	}
	session, err := s.Store.GetSession(ctx, sessionID)
	if err != nil {
		return store.Session{}, ResumeStartingElsewhere, fmt.Errorf("get session %q: %w", sessionID, err)
	}
	if session.Slug == "" {
		return session, ResumeStartingElsewhere, errors.New("restart requires a durable session slug")
	}
	if session.Status == "stopped" {
		return session, ResumeNotLeasable, fmt.Errorf("cannot restart session %q: it is already stopped (resume it instead)", session.Name)
	}

	if live, err := s.TMux.Exists(ctx, session.Slug); err != nil {
		return session, ResumeStartingElsewhere, fmt.Errorf("check live pane for session %q: %w", session.Name, err)
	} else if live {
		if err := s.TMux.Kill(ctx, session.Slug); err != nil {
			return session, ResumeStartingElsewhere, fmt.Errorf("kill tmux session %q for restart: %w", session.Name, err)
		}
	}

	// Flip the durable row to stopped exactly as an explicit user kill
	// does (killed_by_user=1), so AcquireLaunchLease inside Resume below
	// can take the launch lease (it requires status=='stopped') and its
	// own CAS update clears killed_by_user again -- the same terminal-
	// guard release an explicit `r` resume already relies on.
	if err := s.Store.UpdateSessionStatus(ctx, store.StatusUpdateInput{
		SessionID:    session.ID,
		Status:       "stopped",
		Reason:       "restarted by user",
		Source:       "user",
		At:           s.Clock.Now().UnixMilli(),
		EventKind:    "restart",
		KilledByUser: true,
	}); err != nil {
		return session, ResumeStartingElsewhere, fmt.Errorf("record restart stop for session %q: %w", session.Name, err)
	}
	if err := s.Audit.Transition(session.ID, "restart"); err != nil {
		return session, ResumeStartingElsewhere, fmt.Errorf("audit restart stop for session %q: %w", session.Name, err)
	}

	resumed, outcome, err := s.Resume(ctx, sessionID)
	if err != nil || outcome != ResumeStarted {
		return resumed, outcome, err
	}

	if err := s.Store.ClearEnvDirty(ctx, sessionID, s.Clock.Now().UnixMilli()); err != nil {
		return resumed, outcome, fmt.Errorf("clear env_dirty after restarting session %q: %w", session.Name, err)
	}
	resumed.EnvDirty = false
	return resumed, outcome, nil
}
