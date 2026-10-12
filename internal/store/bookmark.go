package store

import (
	"context"
	"errors"
	"fmt"
	"sort"
)

// BookmarkSlots is the number of bookmark slots, 0 through 9 (R277).
const BookmarkSlots = 10

func validBookmarkSlot(slot int) error {
	if slot < 0 || slot >= BookmarkSlots {
		return fmt.Errorf("bookmark slot %d is outside 0..%d", slot, BookmarkSlots-1)
	}
	return nil
}

// AssignBookmark points slot at sessionID. A taken slot is moved to the new
// session in the same write, never duplicated; a session may hold several
// slots. A session that does not exist, or is tombstoned, is refused.
func (s *Store) AssignBookmark(ctx context.Context, slot int, sessionID string) error {
	if err := validBookmarkSlot(slot); err != nil {
		return err
	}
	if sessionID == "" {
		return errors.New("session id is required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin assign bookmark: %w", err)
	}
	defer rollbackTx(tx)
	var deletedAt int64
	if err := tx.QueryRowContext(ctx, `SELECT deleted_at FROM sessions WHERE id = ?`, sessionID).Scan(&deletedAt); err != nil {
		return fmt.Errorf("bookmark session %q: %w", sessionID, err)
	}
	if deletedAt != 0 {
		return fmt.Errorf("session %q is deleted, refusing to bookmark it", sessionID)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO bookmarks (slot, session_id) VALUES (?, ?)
		ON CONFLICT(slot) DO UPDATE SET session_id = excluded.session_id`, slot, sessionID); err != nil {
		return fmt.Errorf("assign bookmark %d: %w", slot, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit assign bookmark %d: %w", slot, err)
	}
	return nil
}

// ClearBookmark empties slot. Clearing an empty slot is not an error.
func (s *Store) ClearBookmark(ctx context.Context, slot int) error {
	if err := validBookmarkSlot(slot); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM bookmarks WHERE slot = ?`, slot); err != nil {
		return fmt.Errorf("clear bookmark %d: %w", slot, err)
	}
	return nil
}

// Bookmarks returns every assigned slot as slot -> session id.
func (s *Store) Bookmarks(ctx context.Context) (map[int]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT slot, session_id FROM bookmarks`)
	if err != nil {
		return nil, fmt.Errorf("list bookmarks: %w", err)
	}
	defer closeRows(rows)
	out := map[int]string{}
	for rows.Next() {
		var slot int
		var id string
		if err := rows.Scan(&slot, &id); err != nil {
			return nil, fmt.Errorf("scan bookmark: %w", err)
		}
		out[slot] = id
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list bookmarks: %w", err)
	}
	return out, nil
}

// BookmarkSlotsOf returns the slots sessionID holds, ascending.
func (s *Store) BookmarkSlotsOf(ctx context.Context, sessionID string) ([]int, error) {
	all, err := s.Bookmarks(ctx)
	if err != nil {
		return nil, err
	}
	var slots []int
	for slot, id := range all {
		if id == sessionID {
			slots = append(slots, slot)
		}
	}
	sort.Ints(slots)
	return slots, nil
}
