package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// SetPendingHarnessName records the name deck just pushed into a running
// harness (SPEC §4 pending_harness_name) and the millisecond it was sent, so
// the observe path can tell deck's own echo from a rename made inside the
// harness. name_source and name are not touched.
func (s *Store) SetPendingHarnessName(ctx context.Context, sessionID, name string, at int64) error {
	if sessionID == "" || name == "" {
		return errors.New("session id and pending harness name are required")
	}
	return s.writePendingHarnessName(ctx, sessionID, name, at)
}

// ClearPendingHarnessName empties pending_harness_name and its timestamp. It
// is idempotent: clearing an already-clear row is not an error.
func (s *Store) ClearPendingHarnessName(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return errors.New("session id is required")
	}
	return s.writePendingHarnessName(ctx, sessionID, "", 0)
}

func (s *Store) writePendingHarnessName(ctx context.Context, sessionID, name string, at int64) error {
	var nameArg, atArg any
	if name != "" {
		nameArg, atArg = name, at
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE sessions SET pending_harness_name = ?, pending_harness_name_at = ? WHERE id = ?`,
		nameArg, atArg, sessionID); err != nil {
		return fmt.Errorf("write pending harness name: %w", err)
	}
	return nil
}

// RecordSessionEvent appends one event of an arbitrary kind to a live row's
// history, with a human reason and no payload. It never changes the row.
func (s *Store) RecordSessionEvent(ctx context.Context, sessionID, kind, reason string, at int64) error {
	if sessionID == "" || kind == "" {
		return errors.New("session id and event kind are required")
	}
	if at == 0 {
		return errors.New("event timestamp is required")
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO events (session_id, at, kind, reason, payload)
		VALUES (?, ?, ?, ?, '')`, sessionID, at, kind, reason); err != nil {
		return fmt.Errorf("record session event %q: %w", kind, err)
	}
	return nil
}

// LastSessionEventReason returns the reason of a row's most recent event of
// kind, and whether there is one.
func (s *Store) LastSessionEventReason(ctx context.Context, sessionID, kind string) (string, bool, error) {
	var reason string
	err := s.db.QueryRowContext(ctx, `SELECT reason FROM events WHERE session_id = ? AND kind = ?
		ORDER BY seq DESC LIMIT 1`, sessionID, kind).Scan(&reason)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read last %q event: %w", kind, err)
	}
	return reason, true, nil
}
