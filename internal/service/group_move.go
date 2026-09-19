package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/n-orlov/deck/internal/store"
)

// SetSessionGroup implements the `i` detail dialog's `g` move picker (SPEC
// §11, R130 part 2): it moves ONE session into a different group, or back
// to the structural default group when groupID <= 0. It is deliberately
// the group-side twin of SetLaunchInputs/RenameSession -- a pre-read to
// fail an unknown/tombstoned session before anything is written, then one
// store call that persists the move and records the matching event -- and
// it never touches any column but sessions.group_id.
//
// No existence check is made against groupID itself: store.SetSessionGroup
// writes it verbatim (schemaV7 carries no foreign key on group_id by
// design, store.go:2330), so a groupID naming a group deleted between the
// picker's own fetch (computeAvailableGroups, internal/tui) and this call
// degrades to rendering under default on the next load, exactly like any
// other dangling group_id (SPEC §11).
func (s Service) SetSessionGroup(ctx context.Context, sessionID string, groupID int64) (store.Session, error) {
	if s.Store == nil || s.Clock == nil {
		return store.Session{}, errors.New("moving a session's group requires a store and clock")
	}
	if sessionID == "" {
		return store.Session{}, errors.New("session id is required")
	}
	if _, err := s.Store.GetSession(ctx, sessionID); err != nil {
		return store.Session{}, fmt.Errorf("get session %q: %w", sessionID, err)
	}
	if err := s.Store.SetSessionGroup(ctx, sessionID, groupID, "user", s.Clock.Now().UnixMilli()); err != nil {
		return store.Session{}, err
	}
	return s.Store.GetSession(ctx, sessionID)
}
