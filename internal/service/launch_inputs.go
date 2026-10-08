package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// SetLaunchInputs implements the `i` detail dialog's launch-inputs editor
// (SPEC §6.2/§11.4, PRD R108): it persists all four editable launch inputs
// -- pre_launch, post_destroy, launch_args and login_shell -- in one write
// and marks the row launch_dirty, exactly as SetSessionEnv persists an env
// edit and marks env_dirty. It is deliberately the launch-side twin of
// SetSessionEnv with ONE difference: it never touches tmux at all, not even
// to mirror anything into tmux's environment table. Every one of these four
// inputs is consumed at launch (§6.2) -- buildPaneCommand composes
// pre_launch and login_shell into the pane command, launch_args are
// appended to the adapter's argv, post_destroy is read at teardown -- so
// there is nothing an already-running pane could be told about them. Only
// `R` (Restart) relaunches the pane and clears launch_dirty (task 021), and
// that is exactly what the dialog's own "restart-to-apply" labels promise.
//
// The four inputs are written wholesale rather than one at a time so the
// dialog's Enter can never leave a row half-edited: store.SetLaunchInputs
// writes all four columns plus launch_dirty in a single UPDATE. Nothing
// here can touch agent, cwd, slug or captured_path -- no store mutator for
// those exists (task 022 pins that with a source-scanning guard).
func (s Service) SetLaunchInputs(ctx context.Context, sessionID, preLaunch, postDestroy string, launchArgs []string, loginShell bool) (store.Session, error) {
	if s.Store == nil || s.Clock == nil {
		return store.Session{}, errors.New("editing launch inputs requires a store and clock")
	}
	if sessionID == "" {
		return store.Session{}, errors.New("session id is required")
	}
	// Read the row first so an unknown/tombstoned id fails with the store's
	// own not-found error before anything is written, mirroring
	// SetSessionEnv's own pre-read.
	if _, err := s.Store.GetSession(ctx, sessionID); err != nil {
		return store.Session{}, fmt.Errorf("get session %q: %w", sessionID, err)
	}
	if err := s.Store.SetLaunchInputs(ctx, sessionID, preLaunch, postDestroy, launchArgs, loginShell, "user", s.Clock.Now().UnixMilli()); err != nil {
		return store.Session{}, err
	}
	return s.Store.GetSession(ctx, sessionID)
}

// SetEventHook stores a session's own event-hook controls, event_hook_enabled
// (nil inherits event_hook_default) and event_hook_events (nil inherits the
// global list), SPEC §10.2. Unlike SetLaunchInputs it touches neither
// env_dirty nor launch_dirty and never tmux: the dispatcher reads both columns
// when an event fires, so the change applies immediately and a restart has
// nothing to carry. Kinds outside the offered set are refused.
func (s Service) SetEventHook(ctx context.Context, sessionID string, enabled *bool, events []string) (store.Session, error) {
	if s.Store == nil || s.Clock == nil {
		return store.Session{}, errors.New("editing event hook settings requires a store and clock")
	}
	if sessionID == "" {
		return store.Session{}, errors.New("session id is required")
	}
	if err := config.CheckEventKinds(events); err != nil {
		return store.Session{}, err
	}
	if _, err := s.Store.GetSession(ctx, sessionID); err != nil {
		return store.Session{}, fmt.Errorf("get session %q: %w", sessionID, err)
	}
	if err := s.Store.SetEventHook(ctx, sessionID, enabled, events, "user", s.Clock.Now().UnixMilli()); err != nil {
		return store.Session{}, err
	}
	return s.Store.GetSession(ctx, sessionID)
}
