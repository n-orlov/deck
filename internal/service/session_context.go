package service

import "github.com/n-orlov/deck/internal/store"

// LaunchKindCreate and LaunchKindResume are the two SPEC §6.1
// DECK_SESSION_LAUNCH_KIND values: every session's first launch is
// "create" (CreateAgent, CreateShell); every relaunch after that --
// including `r`, `R`, and the `r` that follows a `U` -- is "resume"
// (Resume, and therefore Restart, which routes through it).
const (
	LaunchKindCreate = "create"
	LaunchKindResume = "resume"
)

// sessionContextEnv is SPEC §6.1's single construction function for the
// deck-owned session-context layer: the nine DECK_SESSION_* variables
// (including DECK_SESSION_ID) computed from session's own row facts and
// the given launch kind, plus DECK_HOME (deck-owned per §13.1, from the
// service's own DeckHome, not a row column). Every one of the three launch
// call sites -- CreateAgent, CreateShell, Resume -- builds this same map
// from this one function and merges it into the pane's launch environment
// strictly AFTER applyInstrumentation, so it is the last, unoverridable
// layer (§6.1): a session `env` key or a config `[env]` key of the same
// name can never lie to a hook about which session it is running for.
// Every value is always exported -- empty rather than absent when the row's
// own column is unset (e.g. an unset group or conversation id) -- so a
// hook can branch on a value without first testing for existence, exactly
// as the SPEC table requires. DECK_SESSION_GROUP (R128; this key replaces
// the removed DECK_SESSION_WORKSPACE -- the old name is never aliased
// alongside it) therefore reads store.Session.GroupName verbatim: SPEC
// §6.1's "the manual group's name (§11), empty for the implicit default
// group", which is exactly what GroupName already reads back empty for --
// a nil GroupID, or a GroupID that no longer resolves to a live groups
// row (§11: "renders under default rather than vanishing") -- with no
// cwd-derived fallback of any kind, unlike the removed Workspace label.
func (s Service) sessionContextEnv(session store.Session, launchKind string) map[string]string {
	return map[string]string{
		"DECK_SESSION_ID":              session.ID,
		"DECK_SESSION_NAME":            session.Name,
		"DECK_SESSION_SLUG":            session.Slug,
		"DECK_SESSION_CWD":             session.CWD,
		"DECK_SESSION_AGENT":           session.Agent,
		"DECK_SESSION_GROUP":           session.GroupName,
		"DECK_SESSION_PROFILE":         session.PermissionProfile,
		"DECK_SESSION_CONVERSATION_ID": session.ConversationID,
		"DECK_SESSION_LAUNCH_KIND":     launchKind,
		"DECK_HOME":                    s.DeckHome,
	}
}

// NotificationSession is the session-scoped half of SPEC §10's documented,
// versioned notification payload -- `session: {name, cwd, agent, status,
// reason, permission_profile, group, important}` -- as the Go value a
// body template renders over. It lives here, beside sessionContextEnv,
// because the two are the same projection of one store.Session row onto
// the two documented surfaces a hook ever sees: the launch environment
// and the rendered notification body. Keeping them in one file is what
// makes R128's rename one rename rather than two that can drift: the
// `group` field below and DECK_SESSION_GROUP above read the same column
// under the same rule.
//
// The json tags ARE the payload's field names (§10.1 calls the shape
// documented and versioned, so the names are contract, not incidental
// serialisation detail); dispatch itself is Phase 5's internal/notify and
// is deliberately absent here.
type NotificationSession struct {
	Name              string `json:"name"`
	CWD               string `json:"cwd"`
	Agent             string `json:"agent"`
	Status            string `json:"status"`
	Reason            string `json:"reason"`
	PermissionProfile string `json:"permission_profile"`
	// Group is SPEC §10.1's `group` payload field (R128). It replaces the
	// removed `workspace` field outright -- the old name is never aliased
	// beside it, in this struct or in the rendered body -- and carries
	// store.Session.GroupName verbatim: the manual group's name (§11),
	// empty for the implicit default group and equally empty for a
	// group_id that no longer resolves (§11 renders both under default).
	// No cwd-derived fallback of any kind, exactly as DECK_SESSION_GROUP
	// has none.
	Group string `json:"group"`
	// Important is §10.2's `only = "important"` milestone flag. It is a
	// parameter of NotificationSessionPayload rather than a store.Session
	// field because the sessions.important column (§4) is not surfaced on
	// the row struct yet; the payload names the field SPEC names, and the
	// caller supplies the flag it already holds.
	Important bool `json:"important"`
}

// NotificationSessionPayload projects one store.Session row onto SPEC
// §10.1's session payload fields. `reason` is the row's StatusReason (§7's
// reason for the current status, empty when it has none) and `group` is
// GroupName verbatim per the field comment above; important comes from the
// caller for the reason recorded there.
func NotificationSessionPayload(session store.Session, important bool) NotificationSession {
	return NotificationSession{
		Name:              session.Name,
		CWD:               session.CWD,
		Agent:             session.Agent,
		Status:            session.Status,
		Reason:            session.StatusReason,
		PermissionProfile: session.PermissionProfile,
		Group:             session.GroupName,
		Important:         important,
	}
}
