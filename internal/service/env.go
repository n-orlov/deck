package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/n-orlov/deck/internal/store"
)

// SetSessionEnv edits one key in an existing session's own env map (SPEC
// §6.1/§6.3's highest-priority layer, task 021). It always persists the
// edit and marks the row env_dirty (store.SetSessionEnvValue), then, only
// when the session's tmux pane actually exists right now, mirrors the same
// key into tmux's own environment table via `set-environment -t` so any
// FUTURE pane inherits it. It deliberately never restarts the pane and
// never sends anything to the pane's already-running process: an edit is
// visible in tmux's environment table and in the `env↻` badge immediately,
// but the live pane keeps running with whatever it started with until an
// explicit restart (task 022's `R`) relaunches it and clears env_dirty.
func (s Service) SetSessionEnv(ctx context.Context, sessionID, key, value string) (store.Session, error) {
	if s.Store == nil || s.Clock == nil {
		return store.Session{}, errors.New("editing session environment requires a store and clock")
	}
	if sessionID == "" || key == "" {
		return store.Session{}, errors.New("session id and environment key are required")
	}
	session, err := s.Store.GetSession(ctx, sessionID)
	if err != nil {
		return store.Session{}, fmt.Errorf("get session %q: %w", sessionID, err)
	}
	if err := s.Store.SetSessionEnvValue(ctx, sessionID, key, value, "user", s.Clock.Now().UnixMilli()); err != nil {
		return store.Session{}, err
	}
	if session.Slug != "" {
		live, err := s.TMux.Exists(ctx, session.Slug)
		if err != nil {
			return store.Session{}, fmt.Errorf("check live session %q: %w", session.Name, err)
		}
		if live {
			if err := s.TMux.SetEnvironment(ctx, session.Slug, key, value); err != nil {
				return store.Session{}, fmt.Errorf("mirror environment for session %q: %w", session.Name, err)
			}
		}
	}
	return s.Store.GetSession(ctx, sessionID)
}
