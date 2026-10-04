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
// as a first launch. An archived row is refused too, but by Resume's own
// up-front guard rather than a second copy of it here (SPEC.md:718, #8:
// "`R` routes through resume, so one guard covers both").
//
// Restart is also the only path that ever applies a pending launch-input
// edit (task 020's pre_launch/post_destroy/launch_args/login_shell) to a
// freshly relaunched pane, since Resume always re-reads the row from the
// store before building that pane's command line -- so a successful
// restart clears launch_dirty (the `launch↻` badge) right alongside
// env_dirty. InjectEnv (inject.go) never relaunches a pane at all, so it
// clears env_dirty alone and never touches launch_dirty.
func (s Service) Restart(ctx context.Context, sessionID string) (store.Session, ResumeOutcome, error) {
	session, outcome, err := s.loadRestartableSession(ctx, sessionID)
	if err != nil {
		return session, outcome, err
	}
	if err := s.killLivePaneForRestart(ctx, session); err != nil {
		return session, ResumeStartingElsewhere, err
	}
	if err := s.recordRestartStop(ctx, session); err != nil {
		return session, ResumeStartingElsewhere, err
	}

	resumed, outcome, err := s.Resume(ctx, sessionID)
	if err != nil || outcome != ResumeStarted {
		return resumed, outcome, err
	}
	if err := s.clearDirtyAfterRestart(ctx, sessionID, session); err != nil {
		return resumed, outcome, err
	}
	resumed.EnvDirty = false
	resumed.LaunchDirty = false
	return resumed, outcome, nil
}

// loadRestartableSession is Restart's up-front half: the dependency and id
// guards, the row lookup, and the slug / already-stopped refusals.
func (s Service) loadRestartableSession(ctx context.Context, sessionID string) (store.Session, ResumeOutcome, error) {
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
	return session, ResumeStartingElsewhere, nil
}

// killLivePaneForRestart kills the session's tmux pane when one exists.
func (s Service) killLivePaneForRestart(ctx context.Context, session store.Session) error {
	live, err := s.TMux.Exists(ctx, session.Slug)
	if err != nil {
		return fmt.Errorf("check live pane for session %q: %w", session.Name, err)
	}
	if !live {
		return nil
	}
	if err := s.TMux.Kill(ctx, session.Slug); err != nil {
		return fmt.Errorf("kill tmux session %q for restart: %w", session.Name, err)
	}
	return nil
}

// recordRestartStop flips the durable row to stopped exactly as an explicit
// user kill does (killed_by_user=1), so AcquireLaunchLease inside Resume can
// take the launch lease (it requires status=='stopped') and its own CAS
// update clears killed_by_user again -- the same terminal-guard release an
// explicit `r` resume already relies on.
func (s Service) recordRestartStop(ctx context.Context, session store.Session) error {
	if err := s.Store.UpdateSessionStatus(ctx, store.StatusUpdateInput{
		SessionID:    session.ID,
		Status:       "stopped",
		Reason:       "restarted by user",
		Source:       "user",
		At:           s.Clock.Now().UnixMilli(),
		EventKind:    "restart",
		KilledByUser: true,
	}); err != nil {
		return fmt.Errorf("record restart stop for session %q: %w", session.Name, err)
	}
	if err := s.Audit.Transition(session.ID, "restart"); err != nil {
		return fmt.Errorf("audit restart stop for session %q: %w", session.Name, err)
	}
	return nil
}

// clearDirtyAfterRestart clears env_dirty then launch_dirty with one shared
// timestamp read, since the relaunched pane applied both edits.
func (s Service) clearDirtyAfterRestart(ctx context.Context, sessionID string, session store.Session) error {
	now := s.Clock.Now().UnixMilli()
	if err := s.Store.ClearEnvDirty(ctx, sessionID, now); err != nil {
		return fmt.Errorf("clear env_dirty after restarting session %q: %w", session.Name, err)
	}
	if err := s.Store.ClearLaunchDirty(ctx, sessionID, now); err != nil {
		return fmt.Errorf("clear launch_dirty after restarting session %q: %w", session.Name, err)
	}
	return nil
}
