package service

import (
	"context"
	"errors"
	"fmt"
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
	// SPEC §9.2 (R77): if this name (or its slug) is held only by a
	// tombstoned row, RenameSession reaps that row inside its own
	// transaction, exactly like CreateSession does for CreateShell/
	// CreateAgent. Note the holders now, remove their files only after
	// that commit -- a rename that is refused (a live or archived holder)
	// must leave a still-restorable session's scrollback untouched.
	reapedHolders, err := s.tombstonedNameHolders(ctx, trimmed)
	if err != nil {
		return store.Session{}, err
	}
	if err := s.Store.RenameSession(ctx, sessionID, trimmed, "user", s.Clock.Now().UnixMilli()); err != nil {
		return store.Session{}, err
	}
	if err := s.reapedHolderFiles(ctx, reapedHolders); err != nil {
		return store.Session{}, fmt.Errorf("clean up the session reaped by reusing name %q: %w", trimmed, err)
	}
	return s.Store.GetSession(ctx, sessionID)
}
