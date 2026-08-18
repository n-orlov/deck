package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/n-orlov/deck/internal/store"
)

// SetPermissionProfile changes an existing session's persisted permission
// profile (SPEC §5/§8, task 020). It never touches a live pane: the caller
// must state, separately, that the new profile only applies on the
// session's next launch or restart, and this function itself never
// re-issues launch/resume argv to any already-running pane. Requesting a
// profile the session's adapter does not declare support for is refused
// with a specific error rather than silently degraded, so the TUI can show
// the user why nothing changed.
func (s Service) SetPermissionProfile(ctx context.Context, sessionID, profile string) (store.Session, error) {
	if s.Store == nil || s.Agents == nil {
		return store.Session{}, errors.New("changing the permission profile requires a store and an adapter registry")
	}
	if sessionID == "" || profile == "" {
		return store.Session{}, errors.New("session id and permission profile are required")
	}
	session, err := s.Store.GetSession(ctx, sessionID)
	if err != nil {
		return store.Session{}, fmt.Errorf("get session %q: %w", sessionID, err)
	}
	adapter, ok := s.Agents.Lookup(session.Agent)
	if !ok {
		return store.Session{}, fmt.Errorf("agent %q is not registered", session.Agent)
	}
	caps := adapter.Capabilities()
	if len(caps.Profiles) == 0 {
		return store.Session{}, fmt.Errorf("agent %q has no permission profiles", session.Agent)
	}
	if !caps.SupportsProfile(profile) {
		return store.Session{}, fmt.Errorf("agent %q does not support permission profile %q", session.Agent, profile)
	}
	if err := s.Store.SetPermissionProfile(ctx, sessionID, profile, "user"); err != nil {
		return store.Session{}, err
	}
	return s.Store.GetSession(ctx, sessionID)
}
