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
// Resolution reads ListSessionsIncludingArchived, NOT ListSessions: the
// latter's `archived_at = 0` term is a display rule and resolving through it
// orphaned every hook an archived row emitted (issue #8).
type Store interface {
	ListSessionsIncludingArchived(context.Context) ([]store.Session, error)
	UpdateSessionStatus(context.Context, store.StatusUpdateInput) error
	RecordOrphanEvent(context.Context, store.EventInput) error
	// SetConversationID follows requirement 44: a SessionStart whose payload
	// names a different conversation than the row currently holds moves the
	// row's stored identity, recording the change as its own durable event.
	SetConversationID(ctx context.Context, sessionID, conversationID, source string, at int64) error
}

// Mapping combines SPEC §8.1's hook-to-status mapping with the event's own
// payload fields. It deliberately carries no AllowedFrom list: §7's
// `user-terminal > hook > probe > tmux` precedence is what decides whether a
// hook's write lands, not an enumeration of the current status VALUE. A
// per-event predecessor-status allow-list conflated "this is the only legal
// prior FSM state" with "this is the only prior state I trust" -- and the
// second question is answered by precedence (source), not by status value.
// Enforcing it therefore defeated precedence exactly where it mattered: a
// hook (higher precedence) arriving after a stale tmux- or probe-sourced
// verdict (lower precedence) that happened to land the row on a status
// outside the list -- notably a tmux-sourced launch failure at `error` --
// was rejected instead of applied. What actually needs to hold is enforced
// once, generically, in internal/store.Store.UpdateSessionStatus: a
// killed_by_user row is not resurrected (user-terminal outranks hook), a
// `stopped` row is not resurrected (no return edge from `stopped` except the
// explicit `r` resume, which does not go through this table), and a hook
// cannot revive a process-crash verdict into `running`. See the before/after
// table in docs/reports/phase2b2-findings.md.
type Mapping struct {
	Status       string
	Kind         string
	ReasonField  string
	MessageField string
}

// Mappings is the single hook mapping table. Its keys are the upstream hook
// event names deck receives -- most are Claude's, and codex reuses the same
// names for the four events its own hook table shares with Claude
// (SessionStart, UserPromptSubmit, Stop, SessionEnd; see internal/agent's
// codexHookEvents). PermissionRequest is codex's own fifth event -- SPEC
// §8.2's "deck must branch on tool_name" instruction lives here as this one
// mapping's ReasonField, not as a second receiver, a per-agent table or a
// codex-only payload struct: the receiver has no notion of which agent kind
// sent a hook, only of which event name arrived.
var Mappings = map[string]Mapping{
	"SessionStart":      {Status: "running", Kind: "session_start", ReasonField: "source"},
	"UserPromptSubmit":  {Status: "running", Kind: "user_prompt_submitted"},
	"Notification":      {Status: "waiting", Kind: "notification", ReasonField: "notification_type"},
	"PermissionRequest": {Status: "waiting", Kind: "permission_request", ReasonField: "tool_name"},
	"Stop":              {Status: "idle", Kind: "stop", MessageField: "last_assistant_message"},
	"StopFailure":       {Status: "error", Kind: "stop_failure", ReasonField: "error_type"},
	"SessionEnd":        {Status: "stopped", Kind: "session_end", ReasonField: "reason"},
}

// sessionEndInSessionReasons are the SessionEnd `reason` values that are
// documented, in requirement 43's evidence, as an in-session restart rather
// than the process going away: Claude's own `/resume` and `/clear` end one
// conversation and start another in the SAME pane, so the tmux session, the
// pane and the process all survive. Every other reason (including any this
// deck build has not seen yet) keeps the pre-existing terminal behaviour --
// see the reason taxonomy recorded in docs/reports/phase2b2-findings.md for
// the rationale and the residual risk of an unenumerated non-terminal reason.
var sessionEndInSessionReasons = map[string]bool{
	"resume": true,
	"clear":  true,
}

// noCurrentStatusMatches is a deliberately unreachable AllowedFrom sentinel:
// no session's status column ever holds this value, so a StatusUpdateInput
// carrying it always fails its precondition and never mutates the row --
// while store.UpdateSessionStatus still records the event unconditionally.
// This is how an in-session SessionEnd is "recorded but not applied"
// without adding a second, event-only write path to the store.
var noCurrentStatusMatches = []string{"__no_current_status_matches__"}

// Result describes the durable write attempted by Receive.
type Result struct {
	SessionID string
	Status    string
	Kind      string
	Reason    string
	Orphan    bool
	// Superseded means the hook named a launch generation that is not the
	// one the row currently holds, so its status write was recorded as an
	// event but deliberately never applied (issue #11, R74). The stored
	// event's own kind and reason columns say so too: supersededEventKind
	// and supersededReason are what Receive writes for it, distinct from
	// the plain mapping.Kind/payload-reason an applied hook of the same
	// event name would get.
	Superseded bool
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
	// ToolName is PermissionRequest's own reason field (codex's own event;
	// see Mappings). Not a codex-only payload struct: this is the same
	// generic payload every hook decodes into, one more field on it.
	ToolName string `json:"tool_name"`
}

// supersededLaunch decides, for the whole hook class at once, whether a hook
// write belongs to a launch deck has already replaced (issue #11, R74).
//
// The row's launch_lease_owner names the generation of the launch whose pane is
// the current one; every instrumented launch that took a lease exports that
// token into its pane (DECK_LAUNCH_GENERATION), so a hook hands back the token
// of the launch it actually came from. A token that is not the row's current
// one therefore came from a pane deck has already killed and replaced, and its
// verdict describes a process that is gone -- SessionEnd->stopped is the one
// that visibly wrecked a live row (it stopped a row whose new pane was
// running), but Stop->idle and Notification->waiting are the same lie about the
// same dead pane, so the rule is applied to the whole class rather than to one
// event name.
//
// The two token-absent cases are decided deliberately, not by accident:
//
//   - row token empty: nothing to discriminate. No launch lease has ever been
//     taken on this row (its only launch came from CreateAgent, which takes
//     none), so deck has no opinion about which launch is current and every
//     hook applies exactly as it did before R74. This is what keeps pre-R74
//     rows, and any uninstrumented path, working unchanged.
//   - hook token empty while the row holds one: superseded. The row says a
//     leased launch is current, and such a launch always exports its token, so
//     a hook with no token cannot have come from it -- it comes from the row's
//     pre-lease (create-time) pane, which is exactly the older launch this rule
//     exists to discount. Treating it as "unknown, therefore allow" would leave
//     the first-launch hooks of every resumed row still able to stop it.
func supersededLaunch(rowGeneration, hookGeneration string) bool {
	if rowGeneration == "" {
		return false
	}
	return hookGeneration != rowGeneration
}

// supersededEventKind names the stored event for a hook supersededLaunch has
// declined, distinct from the plain mapping.Kind an applied hook of the same
// event name gets. ".superseded" keeps the base kind visible (so a reader
// scanning for e.g. every "stop" event still finds it with a LIKE/prefix
// query) while making it unambiguous, from the kind column alone, that this
// row's write never reached the session.
func supersededEventKind(baseKind string) string {
	return baseKind + ".superseded"
}

// supersededReason explains, in the stored event's own reason column, why a
// superseded hook's status write was declined: the launch generation it
// named is not the row's current one, so it came from a pane deck has
// already replaced (see supersededLaunch for the token-absent cases this
// covers). The original payload -- including whatever reason field the hook
// itself carried -- is untouched in the event's payload column, so nothing
// is lost by overwriting the reason column with this explanation instead.
func supersededReason(rowGeneration, hookGeneration string) string {
	if hookGeneration == "" {
		return fmt.Sprintf("declined: hook carries no launch generation, row is on %q", rowGeneration)
	}
	return fmt.Sprintf("declined: hook launch generation %q does not match row launch generation %q", hookGeneration, rowGeneration)
}

// Receive maps and persists one already-framed JSON hook object. Session
// resolution follows SPEC §8.1 exactly: payload conversation id first, then
// the deck row id injected into the pane environment. Shell rows are resolved
// but rejected because shell instrumentation is forbidden.
//
// injectedLaunchGeneration is the DECK_LAUNCH_GENERATION value the hook's pane
// carries (empty when it carries none); see supersededLaunch for what a
// mismatch means and how the token-absent cases are decided.
func Receive(ctx context.Context, db Store, raw []byte, injectedSessionID, injectedLaunchGeneration string, at int64) (Result, error) {
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
	// eventKind is what actually gets persisted as the event's kind column; it
	// stays mapping.Kind unless supersededLaunch below declines the write, in
	// which case it is overridden to supersededEventKind's distinct name.
	eventKind := mapping.Kind

	reason := payloadField(p, mapping.ReasonField)
	var allowedFrom []string
	if p.EventName == "SessionEnd" && sessionEndInSessionReasons[reason] {
		// Requirement 43: this is not the process going away. Keep the event
		// (kind session_end, this reason) but never let it stop the row.
		allowedFrom = noCurrentStatusMatches
	}
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
	if supersededLaunch(session.LaunchGeneration, injectedLaunchGeneration) {
		// Recorded, never applied -- the same mechanism requirement 43 uses
		// for an in-session SessionEnd. The event keeps the evidence that a
		// superseded pane spoke, while the unsatisfiable AllowedFrom makes the
		// status write a no-op inside the store's own transaction, so the row
		// is never wrong even momentarily and nothing has to repair it after.
		result.Superseded = true
		allowedFrom = noCurrentStatusMatches
		eventKind = supersededEventKind(mapping.Kind)
		reason = supersededReason(session.LaunchGeneration, injectedLaunchGeneration)
	}
	if !result.Superseded && p.EventName == "SessionStart" && p.ConversationID != "" && p.ConversationID != session.ConversationID {
		// Requirement 44: the row deck already owns follows the live
		// conversation, independent of whether the status transition below
		// is itself allowed from the row's current state. A superseded launch
		// is the exception: its conversation belongs to the replaced pane, so
		// moving the row's identity onto it would hand the row the id of a
		// conversation that is already over.
		if err := db.SetConversationID(ctx, session.ID, p.ConversationID, "hook", at); err != nil {
			return result, fmt.Errorf("update conversation id: %w", err)
		}
	}
	if err := db.UpdateSessionStatus(ctx, store.StatusUpdateInput{
		SessionID:              session.ID,
		Status:                 mapping.Status,
		Reason:                 reason,
		Source:                 "hook",
		At:                     at,
		EventKind:              eventKind,
		Payload:                string(raw),
		LastMessage:            payloadField(p, mapping.MessageField),
		AllowedCurrentStatuses: allowedFrom,
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
	case "tool_name":
		return p.ToolName
	default:
		return ""
	}
}

func resolve(ctx context.Context, db Store, conversationID, injectedSessionID string) (store.Session, bool, error) {
	// Every row deck still retains, archived ones included: an archived row
	// is hidden from the sidebar, not disowned, and SPEC §8.1's two keys both
	// name a row that exists. Resolving against the sidebar's own query
	// (ListSessions, which also requires archived_at = 0) meant a live agent
	// behind the `/` filter had its every hook recorded as an orphan with
	// session_id NULL while its status column froze -- issue #8, whose
	// captured banner named a correct conversation id AND a correct injected
	// row id. Tombstoned rows stay unresolvable (deleted_at = 0 is still in
	// the accessor's WHERE clause): a row awaiting the reaper is deliberately
	// no longer a hook target.
	sessions, err := db.ListSessionsIncludingArchived(ctx)
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
