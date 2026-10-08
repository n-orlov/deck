package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// EventHookState is the slice of a sessions row the event-hook dispatcher
// reads (SPEC §10.2, §10.3): the tri-state flag, the optional own kind list,
// the (kind, reason) pairs already fired in the current notify_epoch, and the
// two facts the payload needs that store.Session does not carry.
type EventHookState struct {
	// Enabled is event_hook_enabled: nil inherits event_hook_default.
	Enabled *bool
	// Events is event_hook_events: nil inherits the global list, a non-nil
	// empty list offers the hook nothing.
	Events []string
	// Fired is the hook_fired column verbatim ("" while NULL).
	Fired string
	// Important is the sessions.important flag exported to the hook.
	Important bool
	// Sensitive withholds the event message from the hook (SPEC §8, §10.1).
	Sensitive bool
}

// EventHookState reads the event-hook facts of one session row.
func (s *Store) EventHookState(ctx context.Context, sessionID string) (EventHookState, error) {
	var (
		enabled              sql.NullInt64
		events, fired        sql.NullString
		important, sensitive int
	)
	err := s.db.QueryRowContext(ctx, `SELECT event_hook_enabled, event_hook_events, hook_fired, important, sensitive
		FROM sessions WHERE id = ?`, sessionID).Scan(&enabled, &events, &fired, &important, &sensitive)
	if errors.Is(err, sql.ErrNoRows) {
		return EventHookState{}, fmt.Errorf("session %q not found", sessionID)
	}
	if err != nil {
		return EventHookState{}, fmt.Errorf("read event hook state: %w", err)
	}
	state := EventHookState{Fired: fired.String, Important: important != 0, Sensitive: sensitive != 0}
	if enabled.Valid {
		on := enabled.Int64 != 0
		state.Enabled = &on
	}
	if events.Valid {
		list := []string{}
		// A damaged list reads as "inherit": the hook then follows the global
		// list instead of going silent for a column deck itself wrote.
		if err := json.Unmarshal([]byte(events.String), &list); err == nil {
			state.Events = list
		}
	}
	return state, nil
}

// ClaimHookFired replaces a session's hook_fired set with next, but only if
// it still reads previous. It reports whether this caller won: a false
// result means another deck process recorded a pair first and now owns the
// spawn, so the loser must not spawn (SPEC §10.4, "Dispatch never happens
// twice for one event"). The write is one statement, so the compare and the
// set are atomic across clients.
func (s *Store) ClaimHookFired(ctx context.Context, sessionID, previous, next string) (bool, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE sessions SET hook_fired = ?
		WHERE id = ? AND COALESCE(hook_fired, '') = ?`, next, sessionID, previous)
	if err != nil {
		return false, fmt.Errorf("claim hook_fired: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("claim hook_fired: %w", err)
	}
	return n == 1, nil
}
