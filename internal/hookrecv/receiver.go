package hookrecv

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/n-orlov/deck/internal/store"
)

// ErrUnresolved means the payload could not be associated with a live deck
// row. Receive preserves such payloads as orphan events before returning this
// error, so callers may report the stale hook without losing evidence.
var ErrUnresolved = errors.New("hook session could not be resolved")

// Store is the deliberately small durable surface used by the hook receiver.
// UpdateSessionStatus performs the status and event writes atomically.
type Store interface {
	ListSessions(context.Context) ([]store.Session, error)
	UpdateSessionStatus(context.Context, store.StatusUpdateInput) error
	RecordOrphanEvent(context.Context, store.EventInput) error
}

// Mapping combines SPEC §8.1's hook-to-status mapping with §7's exhaustive
// legal source states. ReasonField and MessageField keep payload interpretation
// reviewable as table data rather than scattering event-specific conditionals
// through Receive.
type Mapping struct {
	Status       string
	Kind         string
	AllowedFrom  []string
	ReasonField  string
	MessageField string
}

// Mappings is the single hook mapping and transition-policy table. Its keys are
// Claude's upstream event names, not names invented by deck.
var Mappings = map[string]Mapping{
	"SessionStart":     {Status: "running", Kind: "session_start", AllowedFrom: []string{"starting"}, ReasonField: "source"},
	"UserPromptSubmit": {Status: "running", Kind: "user_prompt_submitted", AllowedFrom: []string{"idle", "error"}},
	"Notification":     {Status: "waiting", Kind: "notification", AllowedFrom: []string{"running"}, ReasonField: "notification_type"},
	"Stop":             {Status: "idle", Kind: "stop", AllowedFrom: []string{"running"}, MessageField: "last_assistant_message"},
	"StopFailure":      {Status: "error", Kind: "stop_failure", AllowedFrom: []string{"running"}, ReasonField: "error_type"},
	"SessionEnd":       {Status: "stopped", Kind: "session_end", AllowedFrom: []string{"starting", "running", "waiting", "idle", "error", "stopped"}, ReasonField: "reason"},
}

// Result describes the durable write attempted by Receive.
type Result struct {
	SessionID string
	Status    string
	Kind      string
	Reason    string
	Orphan    bool
}

// payload contains only fields deck interprets. The original JSON, not a
// re-marshaled form of this struct, is persisted in the matching event.
type payload struct {
	EventName      string `json:"hook_event_name"`
	ConversationID string `json:"session_id"`
	Source         string `json:"source"`
	Notification   string `json:"notification_type"`
	ErrorType      string `json:"error_type"`
	EndReason      string `json:"reason"`
	LastMessage    string `json:"last_assistant_message"`
}

// Receive maps and persists one already-framed JSON hook object. Session
// resolution follows SPEC §8.1 exactly: payload conversation id first, then
// the deck row id injected into the pane environment. Shell rows are resolved
// but rejected because shell instrumentation is forbidden.
func Receive(ctx context.Context, db Store, raw []byte, injectedSessionID string, at int64) (Result, error) {
	if db == nil {
		return Result{}, errors.New("hook store is required")
	}
	if at == 0 {
		return Result{}, errors.New("hook timestamp is required")
	}
	var p payload
	if err := json.Unmarshal(raw, &p); err != nil {
		return Result{}, fmt.Errorf("decode hook payload: %w", err)
	}
	mapping, ok := Mappings[p.EventName]
	if !ok {
		return Result{}, fmt.Errorf("unsupported hook event %q", p.EventName)
	}

	reason := payloadField(p, mapping.ReasonField)
	result := Result{Status: mapping.Status, Kind: mapping.Kind, Reason: reason}
	session, found, err := resolve(ctx, db, p.ConversationID, injectedSessionID)
	if err != nil {
		return result, err
	}
	if !found {
		result.Orphan = true
		if err := db.RecordOrphanEvent(ctx, store.EventInput{
			At: at, Kind: mapping.Kind, Reason: reason, Payload: string(raw),
		}); err != nil {
			return result, fmt.Errorf("preserve unresolved hook: %w", err)
		}
		return result, fmt.Errorf("%w (conversation_id=%q injected_session_id=%q)", ErrUnresolved, p.ConversationID, injectedSessionID)
	}
	if session.Agent == "shell" {
		return result, fmt.Errorf("hook target %q is a shell session", session.ID)
	}

	result.SessionID = session.ID
	if err := db.UpdateSessionStatus(ctx, store.StatusUpdateInput{
		SessionID:              session.ID,
		Status:                 mapping.Status,
		Reason:                 reason,
		Source:                 "hook",
		At:                     at,
		EventKind:              mapping.Kind,
		Payload:                string(raw),
		LastMessage:            payloadField(p, mapping.MessageField),
		AllowedCurrentStatuses: mapping.AllowedFrom,
	}); err != nil {
		return result, fmt.Errorf("apply %s hook: %w", p.EventName, err)
	}
	return result, nil
}

func payloadField(p payload, name string) string {
	switch name {
	case "source":
		return p.Source
	case "notification_type":
		return p.Notification
	case "error_type":
		return p.ErrorType
	case "reason":
		return p.EndReason
	case "last_assistant_message":
		return p.LastMessage
	default:
		return ""
	}
}

func resolve(ctx context.Context, db Store, conversationID, injectedSessionID string) (store.Session, bool, error) {
	sessions, err := db.ListSessions(ctx)
	if err != nil {
		return store.Session{}, false, fmt.Errorf("list sessions for hook resolution: %w", err)
	}
	if conversationID != "" {
		var match store.Session
		matches := 0
		for _, session := range sessions {
			if session.ConversationID == conversationID {
				match = session
				matches++
			}
		}
		if matches == 1 {
			return match, true, nil
		}
		// A duplicate conversation id is not safe to guess. Continue to the
		// injected row identity, which is the specified fallback and can
		// disambiguate an otherwise corrupt/legacy store.
	}
	for _, session := range sessions {
		if injectedSessionID != "" && session.ID == injectedSessionID {
			return session, true, nil
		}
	}
	return store.Session{}, false, nil
}
