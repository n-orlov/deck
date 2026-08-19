package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/n-orlov/deck/internal/store"
)

// PinResume pins a session's resume behavior to its own current
// conversation id (SPEC §8/§9.3, task 021): future resumes use resume_pin
// rather than whatever the session's own conversation id happens to be at
// resume time, sticky across a deck restart. There is no history of prior
// conversation ids to choose from, so "pin to the session's own current
// conversation id" is the only sensible action here; an agent without an
// assigned conversation id (e.g. shell) cannot be pinned.
func (s Service) PinResume(ctx context.Context, sessionID string) (store.Session, error) {
	if s.Store == nil || s.Clock == nil {
		return store.Session{}, errors.New("pinning a resume conversation requires a store and clock")
	}
	if sessionID == "" {
		return store.Session{}, errors.New("session id is required")
	}
	session, err := s.Store.GetSession(ctx, sessionID)
	if err != nil {
		return store.Session{}, fmt.Errorf("get session %q: %w", sessionID, err)
	}
	if session.ConversationID == "" {
		return store.Session{}, fmt.Errorf("session %q has no conversation id to pin", session.Name)
	}
	if err := s.Store.SetResumePin(ctx, sessionID, session.ConversationID, "user", s.Clock.Now().UnixMilli()); err != nil {
		return store.Session{}, err
	}
	return s.Store.GetSession(ctx, sessionID)
}

// SetResumeAuto returns a session's resume behavior to the default (auto):
// resume the session's own last-known conversation, clearing any pin.
func (s Service) SetResumeAuto(ctx context.Context, sessionID string) (store.Session, error) {
	if s.Store == nil || s.Clock == nil {
		return store.Session{}, errors.New("clearing a resume pin requires a store and clock")
	}
	if sessionID == "" {
		return store.Session{}, errors.New("session id is required")
	}
	if err := s.Store.SetResumeStateAuto(ctx, sessionID, "user", s.Clock.Now().UnixMilli()); err != nil {
		return store.Session{}, err
	}
	return s.Store.GetSession(ctx, sessionID)
}

// ArmFreshOnce arms a one-shot "start fresh" launch (SPEC §8/§9.3, task
// 021): the very next Resume call for this session starts a brand-new
// conversation (a fresh launch argv, never --resume/--continue) instead of
// resuming the pinned or last-known one, and Resume itself reverts
// resume_state back to auto once that fresh launch has actually happened —
// never back to pinned, and never left as fresh-once.
func (s Service) ArmFreshOnce(ctx context.Context, sessionID string) (store.Session, error) {
	if s.Store == nil || s.Clock == nil {
		return store.Session{}, errors.New("arming a fresh-once resume requires a store and clock")
	}
	if sessionID == "" {
		return store.Session{}, errors.New("session id is required")
	}
	if err := s.Store.SetResumeStateFreshOnce(ctx, sessionID, "user", s.Clock.Now().UnixMilli()); err != nil {
		return store.Session{}, err
	}
	return s.Store.GetSession(ctx, sessionID)
}

// ResumeMode dispatches the `p` dialog's chosen resume mode ("pinned",
// "auto", or "fresh-once") to PinResume, SetResumeAuto, or ArmFreshOnce
// respectively, so the TUI can wire one function value for the whole dialog
// the same way it wires SetPermissionProfile for the `P` dialog.
func (s Service) ResumeMode(ctx context.Context, sessionID, mode string) (store.Session, error) {
	switch mode {
	case "pinned":
		return s.PinResume(ctx, sessionID)
	case "auto":
		return s.SetResumeAuto(ctx, sessionID)
	case "fresh-once":
		return s.ArmFreshOnce(ctx, sessionID)
	default:
		return store.Session{}, fmt.Errorf("unknown resume mode %q", mode)
	}
}
