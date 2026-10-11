package store

import (
	"context"
	"errors"
)

// SetHarnessNameAdopt stores a session's own adoption override, the
// sessions.harness_name_adopt tri-state (SPEC §11.4): nil inherits the global
// adopt_harness_names, otherwise the session's own on or off. Like
// SetEventHook it sets no dirty flag, because adoption and push read the
// column when they run. One event row records the change.
func (s *Store) SetHarnessNameAdopt(ctx context.Context, sessionID string, adopt *bool, source string, at int64) error {
	if sessionID == "" {
		return errors.New("session id is required")
	}
	if source == "" {
		source = "user"
	}
	return s.mutateSessionWithEvent(ctx, sessionID, "harness name adoption", "set_harness_name_adopt", source, "", at,
		`UPDATE sessions SET harness_name_adopt = ? WHERE id = ?`, eventHookEnabledArg(adopt))
}
