package notify

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/n-orlov/deck/internal/config"
)

// Policy is the global half of the dispatch decision (SPEC §6.5): the
// configured event_hook argv, event_hook_default and event_hook_events.
type Policy struct {
	// Command is event_hook; empty means the whole feature is inert.
	Command []string
	// Default is event_hook_default: the enabled flag of a session whose own
	// flag is "inherit".
	Default bool
	// Events is event_hook_events, the list a session without its own uses.
	Events []string
}

// Fired is one (kind, reason) pair already spawned in the current
// notify_epoch (SPEC §10.3, the hook_fired column of §4).
type Fired struct {
	Kind   string `json:"kind"`
	Reason string `json:"reason"`
}

// SessionSource is the per-session half of the decision. Dispatch reads it
// only after the script and the offered-kind checks have passed, in the
// order the SPEC §10 states, so an inert feature never touches a row field.
type SessionSource interface {
	// EventHookEnabled is the session's tri-state flag: nil inherits
	// event_hook_default, otherwise on (true) or off (false).
	EventHookEnabled() *bool
	// EventHookEvents is the session's own kind list; nil inherits the
	// global list, while a non-nil empty list offers the hook nothing.
	EventHookEvents() []string
	// HookFired is the pairs fired in the current notify_epoch. The store
	// clears it whenever the epoch advances, so a new epoch reads empty.
	HookFired() []Fired
}

// Skip names why Dispatch declined to spawn; SkipNone means it spawns.
type Skip string

// The reasons Dispatch declines, in the order they are checked.
const (
	SkipNone      Skip = ""
	SkipInert     Skip = "no event_hook configured"
	SkipNotOffer  Skip = "kind is not in the offered set"
	SkipDisabled  Skip = "event hook disabled for the session"
	SkipNotListed Skip = "kind not in the effective list"
	SkipDeduped   Skip = "already fired in this epoch"
)

// Decision is Dispatch's verdict. When Spawn is true, Fired is the complete
// hook_fired set the caller must persist (the previous set plus this pair)
// before it spawns, so a second process reading the row sees the pair.
type Decision struct {
	Spawn bool
	Skip  Skip
	Fired []Fired
}

// Dispatch is the one function that decides whether an event spawns the
// hook, in SPEC §10.1-§10.3's order: no event_hook is inert (the session is
// never read); a kind outside the offered set never spawns; the effective
// enabled flag is the session's, else Policy.Default; the effective list is
// the session's, replacing (never merging with) Policy.Events; finally a
// (kind, reason) pair already in HookFired spawns nothing.
func Dispatch(policy Policy, session SessionSource, kind, reason string) Decision {
	if len(policy.Command) == 0 || policy.Command[0] == "" {
		return Decision{Skip: SkipInert}
	}
	if !slices.Contains(config.EventHookKinds, kind) {
		return Decision{Skip: SkipNotOffer}
	}
	if !enabled(policy, session) {
		return Decision{Skip: SkipDisabled}
	}
	if !slices.Contains(effectiveList(policy, session), kind) {
		return Decision{Skip: SkipNotListed}
	}
	pair := Fired{Kind: kind, Reason: reason}
	fired := session.HookFired()
	if slices.Contains(fired, pair) {
		return Decision{Skip: SkipDeduped}
	}
	return Decision{Spawn: true, Fired: append(slices.Clone(fired), pair)}
}

// enabled is the effective enabled flag: the session's own, else the default.
func enabled(policy Policy, session SessionSource) bool {
	if own := session.EventHookEnabled(); own != nil {
		return *own
	}
	return policy.Default
}

// effectiveList is the session's own list when it has one, else the global
// one: a replacement, never a merge.
func effectiveList(policy Policy, session SessionSource) []string {
	if own := session.EventHookEvents(); own != nil {
		return own
	}
	return policy.Events
}

// EncodeFired renders a hook_fired set as the column's JSON text; an empty
// set encodes as "" so the column can stay NULL.
func EncodeFired(fired []Fired) string {
	if len(fired) == 0 {
		return ""
	}
	raw, err := json.Marshal(fired)
	if err != nil {
		return ""
	}
	return string(raw)
}

// DecodeFired is EncodeFired's inverse. Empty or unreadable text decodes to
// the empty set: a damaged column must at worst re-fire, never block.
func DecodeFired(text string) []Fired {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	var fired []Fired
	if err := json.Unmarshal([]byte(text), &fired); err != nil {
		return nil
	}
	return fired
}

// OfferedKind maps a recorded status change to the SPEC §10.1 offered kind
// its event offers the hook. storedKind is the events.kind the writer
// stored (not the §4 vocabulary: session_start, stop, session_end,
// probe.<status>, tmux.pane_dead, killed, ...) and reason its status
// reason. ok is false for a change that offers nothing (a prompt, a launch
// bookkeeping row, a superseded or identity-mismatched hook, ...), so a
// stored kind this table does not name never spawns.
func OfferedKind(storedKind, reason string) (kind string, ok bool) {
	if offered, found := storedKindOffers[storedKind]; found {
		return offered, true
	}
	if status, found := strings.CutPrefix(storedKind, "probe."); found {
		offered, found := probeOffers[status]
		return offered, found
	}
	if storedKind == "session_start" {
		return sessionStartKind(reason)
	}
	return "", false
}

// storedKindOffers is the fixed part of the mapping from stored events.kind
// to an offered kind (SPEC §10.4's table).
var storedKindOffers = map[string]string{
	"notification":       "waiting",
	"permission_request": "waiting",
	"stop":               "idle",
	"stop_failure":       "error",
	"session_end":        "ended",
	"tmux.pane_dead":     "error",
	"killed":             "killed",
}

// probeOffers maps the status a probe classified to an offered kind.
var probeOffers = map[string]string{
	"waiting": "waiting",
	"idle":    "idle",
	"error":   "error",
}

// sessionStartKind separates the three session-start sources: a resume is
// resumed, a compaction is not a start at all, and everything else
// (startup, clear, a missing source) is started.
func sessionStartKind(source string) (string, bool) {
	switch source {
	case "resume":
		return "resumed", true
	case "compact":
		return "", false
	default:
		return "started", true
	}
}
