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

// EventHookResult is what the event hook did for one event (SPEC §10.3): the
// offered kind it ran for, the exit status and a capped tail of its output, or
// the error that kept it from starting. It lives on the event's own row.
type EventHookResult struct {
	// Kind is the offered kind the script received as argv[1].
	Kind string
	// ExitCode is the script's exit status; -1 when it timed out, died on a
	// signal or never started.
	ExitCode int
	// TimedOut reports that event_hook_timeout expired and the script's
	// process group was killed.
	TimedOut bool
	// Output is the capped tail of the script's stdout and stderr.
	Output string
	// Error is why the script could not be started ("" when it ran).
	Error string
}

// RecordEventHookResult stores the result of the hook spawned for the event
// with the given seq against that event row. The event is written before the
// spawn, so a missing row is an error, not an insert: the hook can never
// create an event of its own.
func (s *Store) RecordEventHookResult(ctx context.Context, eventSeq int64, result EventHookResult) error {
	res, err := s.db.ExecContext(ctx, `UPDATE events SET hook_kind = ?, hook_exit = ?, hook_timed_out = ?,
		hook_output = ?, hook_error = ? WHERE seq = ?`,
		result.Kind, result.ExitCode, boolInt(result.TimedOut), result.Output, result.Error, eventSeq)
	if err != nil {
		return fmt.Errorf("record event hook result: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil || n != 1 {
		return fmt.Errorf("record event hook result: event %d not found", eventSeq)
	}
	return nil
}

// EventHookResultOf reads the hook result stored against an event. ok is
// false for an event nothing was spawned for.
func (s *Store) EventHookResultOf(ctx context.Context, eventSeq int64) (result EventHookResult, ok bool, err error) {
	var (
		kind, output, failure sql.NullString
		exit, timedOut        sql.NullInt64
	)
	err = s.db.QueryRowContext(ctx, `SELECT hook_kind, hook_exit, hook_timed_out, hook_output, hook_error
		FROM events WHERE seq = ?`, eventSeq).Scan(&kind, &exit, &timedOut, &output, &failure)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !kind.Valid) {
		return EventHookResult{}, false, nil
	}
	if err != nil {
		return EventHookResult{}, false, fmt.Errorf("read event hook result: %w", err)
	}
	return EventHookResult{Kind: kind.String, ExitCode: int(exit.Int64), TimedOut: timedOut.Int64 != 0,
		Output: output.String, Error: failure.String}, true, nil
}
