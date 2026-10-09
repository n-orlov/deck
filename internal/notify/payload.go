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

// sessionEnvEntry reports whether an inherited entry is one of the session's
// own env keys. `deck _hook` runs inside the agent, whose environment is the
// session env, so those entries carry session env values (of any length) and
// are not passed on to the script (SPEC §6.4, §10.1).
func sessionEnvEntry(entry string, sessionEnv map[string]string) bool {
	name, _, _ := strings.Cut(entry, "=")
	_, ok := sessionEnv[name]
	return ok
}

// buildEnv layers the nine DECK_SESSION_* variables (always exported, empty
// rather than absent, exactly as pre_launch sees them) and the four
// DECK_EVENT_* variables over the inherited environment. Session env values
// are never exported here, and neither is an inherited variable the session
// env defines.
func buildEnv(req Request, message string) []string {
	s := req.Session
	env := make([]string, 0, len(req.BaseEnv)+13)
	for _, entry := range req.BaseEnv {
		if !ownedEnv(entry) && !sessionEnvEntry(entry, req.SessionEnv) {
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
