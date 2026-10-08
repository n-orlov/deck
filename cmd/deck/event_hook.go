package main

import (
	"context"
	"os"

	"github.com/n-orlov/deck/internal/audit"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/hookrecv"
	"github.com/n-orlov/deck/internal/notify"
	"github.com/n-orlov/deck/internal/service"
	"github.com/n-orlov/deck/internal/store"
)

// dispatchHookEvent offers the event a `deck _hook` run just committed to the
// configured event hook (SPEC §10.4). It runs strictly after the status update
// and its event row are durable, so a missing, failing or slow script can
// never lose the event, and it never fails the hook: whatever the dispatch
// does, `deck _hook`'s exit code and agent-facing output are what they were
// without an event hook. A session-end payload is dispatched detached (SPEC
// §10.3): the script is started and left running, with no timeout and no
// recorded result, so the agent's exit is never held.
func dispatchHookEvent(ctx context.Context, db *store.Store, logger *audit.Logger, settings config.Settings, result hookrecv.Result) {
	if !hookResultOffersEvent(result) {
		return
	}
	dispatcher := hookDispatcher(settings)
	if len(dispatcher.Policy.Command) == 0 {
		return
	}
	dispatcher.Store, dispatcher.Audit = db, logger
	_ = dispatcher.Dispatch(ctx, service.HookEvent{
		SessionID:     result.SessionID,
		StoredKind:    result.Kind,
		Reason:        result.Reason,
		Message:       result.Message,
		At:            settings.Clock.Now(),
		AppliedStatus: result.Status,
		EventSeq:      result.EventSeq,
	}, result.Kind == "session_end")
}

// hookResultOffersEvent is false for a hook that recorded no change of the
// session: an orphan, a launch the row has moved past, an identity mismatch
// or a non-status Copilot event, and a SessionEnd that only ended a
// conversation inside a pane that lives on.
func hookResultOffersEvent(result hookrecv.Result) bool {
	return !result.Orphan && !result.Superseded && !result.InSession &&
		result.Status != "" && result.SessionID != ""
}

// hookDispatcher is the event-hook dispatcher of one `deck _hook` run: the
// settings it started with, this host and build, and its own environment. Its
// Store is set by the caller (the Service sets its own).
func hookDispatcher(settings config.Settings) service.EventHookDispatcher {
	host, _ := os.Hostname()
	return service.EventHookDispatcher{
		Policy:  notify.PolicyFromSettings(settings),
		Timeout: settings.EventHookTimeout,
		Deck:    notify.Deck{Host: host, Version: buildVersion()},
		BaseEnv: os.Environ(),
	}
}

// hookEventHookSource is the post-hook liveness pass's Service.EventHook: a
// process death that pass records (SPEC §7, "the next `_hook` invocation")
// is offered to the event hook like the TUI's reconcile offers it. With no
// script configured it is nil, so the pass stays inert and reads nothing.
func hookEventHookSource(settings config.Settings) func() service.EventHookDispatcher {
	dispatcher := hookDispatcher(settings)
	if len(dispatcher.Policy.Command) == 0 {
		return nil
	}
	return func() service.EventHookDispatcher { return dispatcher }
}
