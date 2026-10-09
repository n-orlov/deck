package notify

import (
	"encoding/json"
	"strings"
	"time"
)

// PayloadVersion is the version field of the stdin payload (SPEC §10.1).
const PayloadVersion = 1

// DefaultMessageCap bounds DECK_EVENT_MESSAGE and the payload's message
// (SPEC §10.1: "Bodies are size-capped").
const DefaultMessageCap = 1024

// Session is the session half of an event, projected by the caller from its
// row. Sensitive withholds the event message (SPEC §8, §10.1).
type Session struct {
	ID, Name, Slug, CWD, Agent, Group string
	PermissionProfile                 string
	ConversationID, LaunchKind        string
	Status, Reason                    string
	Important, Sensitive              bool
}

// Event is the event being offered. Kind is argv[1].
type Event struct {
	Kind    string
	Reason  string
	Message string
	At      time.Time
}

// Deck identifies the sending deck process in the payload.
type Deck struct {
	Host    string `json:"host"`
	Version string `json:"version"`
}

type payloadSession struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	CWD               string `json:"cwd"`
	Agent             string `json:"agent"`
	Status            string `json:"status"`
	Reason            string `json:"reason"`
	PermissionProfile string `json:"permission_profile"`
	Group             string `json:"group"`
	Important         bool   `json:"important"`
}

type payloadEvent struct {
	Kind    string `json:"kind"`
	At      string `json:"at"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

type payload struct {
	Version int            `json:"version"`
	Session payloadSession `json:"session"`
	Event   payloadEvent   `json:"event"`
	Deck    Deck           `json:"deck"`
}

// safeMessage is the event message as the script may see it: withheld for a
// sensitive session, otherwise redacted per §6.4 and then truncated.
func safeMessage(req Request) string {
	if req.Session.Sensitive {
		return ""
	}
	limit := req.MessageCap
	if limit <= 0 {
		limit = DefaultMessageCap
	}
	return truncate(Redact(req.Event.Message, req.SessionEnv), limit)
}

func eventAt(req Request) string {
	return req.Event.At.UTC().Format(time.RFC3339)
}

// buildPayload renders the SPEC §10.1 stdin JSON.
func buildPayload(req Request, message string) ([]byte, error) {
	s := req.Session
	return json.Marshal(payload{
		Version: PayloadVersion,
		Session: payloadSession{
			ID: s.ID, Name: s.Name, CWD: s.CWD, Agent: s.Agent, Status: s.Status,
			Reason: s.Reason, PermissionProfile: s.PermissionProfile, Group: s.Group,
			Important: s.Important,
		},
		Event: payloadEvent{Kind: req.Event.Kind, At: eventAt(req), Reason: req.Event.Reason, Message: message},
		Deck:  req.Deck,
	})
}

// ownedEnv are the variables Spawn sets itself; a same-named entry in the
// inherited environment is dropped so the script never sees two values.
func ownedEnv(entry string) bool {
	name, _, _ := strings.Cut(entry, "=")
	return strings.HasPrefix(name, "DECK_SESSION_") || strings.HasPrefix(name, "DECK_EVENT_")
}

// carriesSessionEnv reports whether an inherited entry must not be passed on
// to the script: its name is one of the session's env keys, or the entry
// (name or value) contains any non-empty session env value, of any length. `deck _hook` runs
// inside the agent, whose environment carries the session env, so a value can
// reach the script under its own key or under any other name that copied it
// (SPEC §6.4, §10.1: env values never appear in the script's environment).
func carriesSessionEnv(entry string, sessionEnv map[string]string, values []string) bool {
	name, _, _ := strings.Cut(entry, "=")
	if _, ok := sessionEnv[name]; ok {
		return true
	}
	for _, secret := range values {
		if strings.Contains(entry, secret) {
			return true
		}
	}
	return false
}

// buildEnv layers the nine DECK_SESSION_* variables (always exported, empty
// rather than absent, exactly as pre_launch sees them) and the four
// DECK_EVENT_* variables over the inherited environment. Session env values
// are never exported here: an inherited variable that is named like a session
// env key or that contains a session env value is dropped whole.
func buildEnv(req Request, message string) []string {
	s := req.Session
	values := scrubValues(req.SessionEnv)
	env := make([]string, 0, len(req.BaseEnv)+13)
	for _, entry := range req.BaseEnv {
		if !ownedEnv(entry) && !carriesSessionEnv(entry, req.SessionEnv, values) {
			env = append(env, entry)
		}
	}
	for _, kv := range [][2]string{
		{"DECK_SESSION_ID", s.ID},
		{"DECK_SESSION_NAME", s.Name},
		{"DECK_SESSION_SLUG", s.Slug},
		{"DECK_SESSION_CWD", s.CWD},
		{"DECK_SESSION_AGENT", s.Agent},
		{"DECK_SESSION_GROUP", s.Group},
		{"DECK_SESSION_PROFILE", s.PermissionProfile},
		{"DECK_SESSION_CONVERSATION_ID", s.ConversationID},
		{"DECK_SESSION_LAUNCH_KIND", s.LaunchKind},
		{"DECK_EVENT_KIND", req.Event.Kind},
		{"DECK_EVENT_REASON", req.Event.Reason},
		{"DECK_EVENT_MESSAGE", message},
		{"DECK_EVENT_AT", eventAt(req)},
	} {
		env = append(env, kv[0]+"="+kv[1])
	}
	return env
}
