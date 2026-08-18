package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/n-orlov/deck/internal/store"
)

// Reconcile compares durable rows with deck's private tmux server. A missing
// tmux session is a stopped (and therefore resumable) session; reconciliation
// never launches a replacement. Each observed disappearance is recorded both
// in the store's event log and in the JSONL audit log.
func (s Service) Reconcile(ctx context.Context) error {
	if s.Store == nil || s.Audit == nil || s.Clock == nil {
		return errors.New("reconciliation requires store, audit logger, and clock")
	}
	live, err := s.TMux.List(ctx)
	if err != nil {
		return fmt.Errorf("list tmux sessions for reconciliation: %w", err)
	}
	liveByName := make(map[string]struct{}, len(live))
	for _, session := range live {
		liveByName[session.Name] = struct{}{}
	}
	rows, err := s.Store.ListSessions(ctx)
	if err != nil {
		return fmt.Errorf("list durable sessions for reconciliation: %w", err)
	}
	for _, session := range rows {
		if session.Status == "stopped" || (session.Status == "starting" && session.StatusSource == "user") {
			// A user-sourced row is between the durable create and tmux launch.
			// Once CreateShell has observed tmux it changes the source to tmux,
			// making this row eligible for liveness reconciliation.
			continue
		}
		if _, ok := liveByName["deck_"+session.Slug]; ok {
			continue
		}
		if err := s.Store.UpdateSessionStatus(ctx, store.StatusUpdateInput{
			SessionID: session.ID,
			Status:    "stopped",
			Reason:    "tmux session disappeared",
			Source:    "tmux",
			At:        s.Clock.Now().UnixMilli(),
			EventKind: "tmux.session_gone",
		}); err != nil {
			return fmt.Errorf("mark session %q stopped: %w", session.ID, err)
		}
		if err := s.Audit.Transition(session.ID, "tmux.session_gone"); err != nil {
			return fmt.Errorf("audit disappeared tmux session %q: %w", session.ID, err)
		}
	}
	return nil
}

// RunReconciler performs an immediate reconciliation and repeats it at the
// configured interval until ctx is cancelled. It does not create or bootstrap
// a tmux server, so a killed server is observed as an empty server rather than
// being relaunched.
func (s Service) RunReconciler(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		return errors.New("reconciliation interval must be positive")
	}
	if err := s.reconcileWithin(ctx, interval); err != nil {
		return err
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := s.reconcileWithin(ctx, interval); err != nil {
				return err
			}
		}
	}
}

// reconcileWithin gives every liveness pass the same bounded budget as the
// configured cadence. In particular, a stalled tmux command cannot consume
// many reconcile intervals and make an otherwise healthy client report stale
// liveness indefinitely.
func (s Service) reconcileWithin(ctx context.Context, interval time.Duration) error {
	passCtx, cancel := context.WithTimeout(ctx, interval)
	defer cancel()
	return s.Reconcile(passCtx)
}
