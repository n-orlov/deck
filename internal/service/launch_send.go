package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/n-orlov/deck/internal/agent"
	"github.com/n-orlov/deck/internal/store"
)

// EventHarnessNameSent records that deck typed a user row's name into a
// harness that takes it only through the keyboard (LaunchName=
// sendkeys_after_ready). Its reason is the name, so a launch whose name changed
// while the row was stopped sends the new one and a relaunch with the same
// name does not type it again.
const EventHarnessNameSent = "harness_name_sent"

// applyLaunchName types a user row's name into a fresh harness once its prompt
// is ready (SPEC §8.2). It runs on the reconcile tick, so readiness is the
// probe's reading of the screen on this tick and never a startup delay: a
// screen that is not ready (a modal, a banner, a working turn) just waits for
// the next tick. Only a starting row is considered, so a harness that is
// already past its first prompt is never typed into by this path. The attempt
// is recorded once, before it is made, and never repeated.
func (s Service) applyLaunchName(ctx context.Context, session store.Session) {
	_ = s.tryLaunchName(ctx, session)
}

// launchAttempt is one try at typing a launch name: done means nothing is left
// to try for this row (sent, already sent, not applicable or failed), and note
// says why a try was refused or failed.
type launchAttempt struct {
	done bool
	sent bool
	note string
}

// tryLaunchName is applyLaunchName reporting what happened. The marker is
// claimed in one statement, so a TUI tick and a detached applier racing for
// the same row type the name once between them.
func (s Service) tryLaunchName(ctx context.Context, session store.Session) launchAttempt {
	adapter, name, ok := s.launchSendTarget(session)
	if !ok {
		return launchAttempt{done: true}
	}
	if sent, found, err := s.Store.LastSessionEventReason(ctx, session.ID, EventHarnessNameSent); err != nil || (found && sent == name) {
		return launchAttempt{done: true}
	}
	if refusal := s.pushRefusal(ctx, adapter, session, true); refusal != "" {
		return launchAttempt{note: refusal}
	}
	pushed, ok := agent.SanitisePushName(session.Agent, name)
	if !ok {
		return launchAttempt{done: true}
	}
	// The attempt is recorded before the keystrokes go out: sendRename holds
	// the adapter's gap between the text and its Enter, and a tick that
	// fires inside that gap must find the marker or it types the name again
	// into the same input line.
	claimed, err := s.Store.ClaimSessionEvent(ctx, session.ID, EventHarnessNameSent, name, s.Clock.Now().UnixMilli())
	if err != nil || !claimed {
		return launchAttempt{done: err == nil}
	}
	result := s.sendRename(ctx, adapter, session, pushed)
	return launchAttempt{done: true, sent: result.Sent, note: result.Note}
}

// ApplyLaunchNameWhenReady is the startup naming of a session no running deck
// is ticking (a detached `deck new`): it makes the same readiness-gated
// attempt applyLaunchName makes on a tick, every poll, until the name is typed,
// the row is past starting or gone, or wait runs out. A name that could not be
// typed leaves deck's name and records harness_rename_failed once; the error
// returned says why.
func (s Service) ApplyLaunchNameWhenReady(ctx context.Context, sessionID string, wait, poll time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()
	note := "the prompt never became ready"
	for {
		session, err := s.Store.GetSession(ctx, sessionID)
		if err != nil {
			return err
		}
		attempt := s.tryLaunchName(ctx, session)
		if attempt.done {
			return launchAttemptError(attempt)
		}
		if attempt.note != "" {
			note = attempt.note
		}
		if s.pause(ctx, poll) != nil {
			break
		}
	}
	_ = s.Store.RecordSessionEvent(context.WithoutCancel(ctx), sessionID, EventHarnessRenameFailed, "launch name not sent: "+note, s.Clock.Now().UnixMilli())
	return fmt.Errorf("launch name not sent: %s", note)
}

func launchAttemptError(a launchAttempt) error {
	if a.sent || a.note == "" {
		return nil
	}
	return errors.New(a.note)
}

// NeedsLaunchNameApply reports whether a freshly created row's name has to be
// typed into its harness once ready, so a caller with no tick can start the
// detached applier.
func (s Service) NeedsLaunchNameApply(session store.Session) bool {
	_, _, ok := s.launchSendTarget(session)
	return ok
}

// launchSendTarget is the adapter and name a launch-time keystroke rename
// applies to, or ok=false for any row it does not.
func (s Service) launchSendTarget(session store.Session) (agent.Adapter, string, bool) {
	if !s.launchSendEligible(session) {
		return nil, "", false
	}
	name := launchName(session)
	adapter, ok := s.Agents.Lookup(session.Agent)
	if name == "" || !ok {
		return nil, "", false
	}
	caps := adapter.Capabilities().Name
	if caps.LaunchName != agent.NameLaunchSendKeysAfterReady || !caps.ClaimsPush() {
		return nil, "", false
	}
	return adapter, name, true
}

// launchSendEligible is the row-level part: an agent row still starting, with
// the service wired to look at it.
func (s Service) launchSendEligible(session store.Session) bool {
	if s.Store == nil || s.Clock == nil || s.Agents == nil {
		return false
	}
	return session.Agent != "shell" && session.Status == "starting" && !rowIsTerminal(session)
}
