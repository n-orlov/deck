package store

import (
	"context"
	"fmt"
)

// SetHookExecutable records path as the deck binary the session's agent was
// just launched with (schemaV9, R204c). It is a plain column write, not a
// lifecycle transition: it records no event and never touches Status, so
// recording a binding can never move a row into any state. An id absent from
// sessions updates zero rows and is not an error, like SetSessionsPinned.
func (s *Store) SetHookExecutable(ctx context.Context, sessionID, path string) error {
	if _, err := s.db.ExecContext(ctx, `UPDATE sessions SET hook_executable = ? WHERE id = ?`, path, sessionID); err != nil {
		return fmt.Errorf("record hook executable for session %q: %w", sessionID, err)
	}
	return nil
}
