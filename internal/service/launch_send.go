package service

import (
	"context"

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
// is recorded once, sent or failed, and never repeated.
func (s Service) applyLaunchName(ctx context.Context, session store.Session) {
	adapter, name, ok := s.launchSendTarget(session)
	if !ok {
		return
	}
	if sent, found, err := s.Store.LastSessionEventReason(ctx, session.ID, EventHarnessNameSent); err != nil || (found && sent == name) {
		return
	}
	if s.pushRefusal(ctx, adapter, session) != "" {
		return
	}
	pushed, ok := agent.SanitisePushName(session.Agent, name)
	if !ok {
		return
	}
	s.sendRename(ctx, adapter, session, pushed)
	_ = s.Store.RecordSessionEvent(ctx, session.ID, EventHarnessNameSent, name, s.Clock.Now().UnixMilli())
}

// launchSendTarget is the adapter and name a launch-time keystroke rename
// applies to, or ok=false for any row it does not.
func (s Service) launchSendTarget(session store.Session) (agent.Adapter, string, bool) {
	if s.Store == nil || s.Clock == nil || s.Agents == nil || session.Agent == "shell" || session.Status != "starting" || rowIsTerminal(session) {
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
