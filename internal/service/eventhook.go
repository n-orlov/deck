package service

import (
	"context"
	"time"

	"github.com/n-orlov/deck/internal/notify"
	"github.com/n-orlov/deck/internal/store"
)

// claimAttempts bounds how often a dispatch re-reads hook_fired after losing
// the claim to another deck process. A lost claim whose re-read shows the
// pair is a dedupe skip; the bound only caps a pathological writer storm.
const claimAttempts = 3

// EventHookDispatcher is the one place a deck process turns a recorded
// status change into at most one spawn of the configured event hook (SPEC
// §10.4). Whichever process wrote the event calls it, after the event row is
// committed, so a failing or slow script can never lose the event.
type EventHookDispatcher struct {
	Store *store.Store
	// Policy mirrors event_hook, event_hook_default and event_hook_events.
	Policy notify.Policy
	// Timeout is event_hook_timeout, the bound of an attached spawn.
	Timeout time.Duration
	// Deck identifies this process in the payload.
	Deck notify.Deck
	// BaseEnv is the environment the script inherits (os.Environ()).
	BaseEnv []string
}

// HookEvent is one recorded change to offer the hook.
type HookEvent struct {
	SessionID string
	// StoredKind is the events.kind the writer stored (session_start, stop,
	// session_end, probe.<status>, ...); notify.OfferedKind maps it.
	StoredKind string
	// Reason is the event's status reason.
	Reason string
	// Message is the last assistant message or crash summary; the spawner
	// redacts and caps it.
	Message string
	At      time.Time
	// AppliedStatus, when set, is the status the change set. An event whose
	// status write lost to a higher-precedence source (a killed or stopped
	// row) leaves the row on another status and offers nothing.
	AppliedStatus string
}

// HookOutcome says what Dispatch did. Skip is notify.SkipNone exactly when a
// spawn was attempted; Result is set for an attached spawn that started and
// Err for one that could not (a missing or non-executable script, or a store
// failure), which the caller may record but must never turn into a failure of
// the agent-facing operation.
type HookOutcome struct {
	Skip     notify.Skip
	Spawned  bool
	Detached bool
	Result   notify.Result
	Err      error
}

// Dispatch offers ev to the event hook. detached starts the script and
// returns without waiting, with no timeout and no recorded result: the
// session-end path never holds the agent's exit (SPEC §10.3). Otherwise it
// waits up to the dispatcher's Timeout and returns the recordable Result.
func (d EventHookDispatcher) Dispatch(ctx context.Context, ev HookEvent, detached bool) HookOutcome {
	kind, skip := d.offered(ev)
	if skip != notify.SkipNone {
		return HookOutcome{Skip: skip}
	}
	session, err := d.Store.GetSession(ctx, ev.SessionID)
	if err != nil {
		return HookOutcome{Err: err}
	}
	if ev.AppliedStatus != "" && session.Status != ev.AppliedStatus {
		return HookOutcome{Skip: notify.SkipNotApplied}
	}
	decision, state, err := d.claim(ctx, ev.SessionID, kind, ev.Reason)
	if err != nil || !decision.Spawn {
		return HookOutcome{Skip: decision.Skip, Err: err}
	}
	req := d.request(session, state, ev, kind)
	if detached {
		err := notify.Start(req)
		return HookOutcome{Spawned: err == nil, Detached: true, Err: err}
	}
	res, err := notify.Spawn(ctx, req)
	return HookOutcome{Spawned: err == nil, Result: res, Err: err}
}

// offered maps the stored kind to the offered one before any store read:
// a change that offers nothing, or a feature with no script, costs nothing.
func (d EventHookDispatcher) offered(ev HookEvent) (string, notify.Skip) {
	kind, ok := notify.OfferedKind(ev.StoredKind, ev.Reason)
	switch {
	case !ok:
		return "", notify.SkipNotOffer
	case len(d.Policy.Command) == 0 || d.Policy.Command[0] == "":
		return "", notify.SkipInert
	}
	return kind, notify.SkipNone
}

// claim decides whether ev spawns and, when it does, persists the new
// hook_fired set before returning, so a second process reading the row sees
// the pair (SPEC §10.4). Losing the compare-and-set to another process
// re-reads and decides again, so exactly one process spawns.
func (d EventHookDispatcher) claim(ctx context.Context, sessionID, kind, reason string) (notify.Decision, store.EventHookState, error) {
	var decision notify.Decision
	var state store.EventHookState
	for range claimAttempts {
		var err error
		state, err = d.Store.EventHookState(ctx, sessionID)
		if err != nil {
			return decision, state, err
		}
		decision = notify.Dispatch(d.Policy, hookSession{state}, kind, reason)
		if !decision.Spawn {
			return decision, state, nil
		}
		won, err := d.Store.ClaimHookFired(ctx, sessionID, state.Fired, notify.EncodeFired(decision.Fired))
		if err != nil {
			return decision, state, err
		}
		if won {
			return decision, state, nil
		}
	}
	return notify.Decision{Skip: notify.SkipDeduped}, state, nil
}

// request builds the spawner's input from the committed row.
func (d EventHookDispatcher) request(session store.Session, state store.EventHookState, ev HookEvent, kind string) notify.Request {
	return notify.Request{
		Command: d.Policy.Command,
		Session: notify.Session{
			ID: session.ID, Name: session.Name, Slug: session.Slug, CWD: session.CWD,
			Agent: session.Agent, Group: session.GroupName, PermissionProfile: session.PermissionProfile,
			ConversationID: session.ConversationID, LaunchKind: hookLaunchKind(session),
			Status: session.Status, Reason: session.StatusReason,
			Important: state.Important, Sensitive: state.Sensitive,
		},
		Event:      notify.Event{Kind: kind, Reason: ev.Reason, Message: ev.Message, At: ev.At},
		Deck:       d.Deck,
		BaseEnv:    d.BaseEnv,
		SessionEnv: session.Env,
		Timeout:    d.Timeout,
	}
}

// hookLaunchKind is DECK_SESSION_LAUNCH_KIND for an event: a row whose current
// launch took a lease came from a relaunch (Resume takes one, CreateAgent and
// CreateShell never do), so it is "resume"; anything else is the first launch.
func hookLaunchKind(session store.Session) string {
	if session.LaunchGeneration != "" {
		return LaunchKindResume
	}
	return LaunchKindCreate
}

// hookSession adapts the store's read to notify.SessionSource.
type hookSession struct{ state store.EventHookState }

func (h hookSession) EventHookEnabled() *bool   { return h.state.Enabled }
func (h hookSession) EventHookEvents() []string { return h.state.Events }
func (h hookSession) HookFired() []notify.Fired { return notify.DecodeFired(h.state.Fired) }
