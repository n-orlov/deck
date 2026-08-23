package service

import (
	"context"
	"errors"
	"strings"

	"github.com/n-orlov/deck/internal/store"
)

// Rename implements the `i` detail dialog's rename action (SPEC §11.4, PRD
// requirement 31, I-8): it changes a session's display name only. The tmux
// session name (`deck_<slug>`) is never touched — store.RenameSession never
// writes the `slug` column — so a rename can never move, recreate, or
// otherwise disturb a live pane's identity. newName is trimmed the same
// way the create modal's own name field is (SPEC.md:194: rename treats the
// name like any other one, no special blank-default handling here); a
// blank result after trimming is refused rather than silently keeping the
// old name.
func (s Service) Rename(ctx context.Context, sessionID, newName string) (store.Session, error) {
	if s.Store == nil || s.Clock == nil {
		return store.Session{}, errors.New("renaming a session requires a store and clock")
	}
	if sessionID == "" {
		return store.Session{}, errors.New("session id is required")
	}
	trimmed := strings.TrimSpace(newName)
	if trimmed == "" {
		return store.Session{}, errors.New("new name is required")
	}
	if err := s.Store.RenameSession(ctx, sessionID, trimmed, "user", s.Clock.Now().UnixMilli()); err != nil {
		return store.Session{}, err
	}
	return s.Store.GetSession(ctx, sessionID)
}
