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
// own column is unset (e.g. an unset workspace or conversation id) -- so a
// hook can branch on a value without first testing for existence, exactly
// as the SPEC table requires. DECK_SESSION_WORKSPACE therefore reads the
// sessions.workspace column verbatim (store.Session.WorkspaceColumn), not
// store.Session.Workspace's §11 grouping label with its basename-of-cwd
// fallback: §6.1's rule is "empty rather than absent when the column behind
// it is unset", and reading the label instead would make a row whose
// workspace has never been recorded export "" on its create launch (where
// no read path has applied the fallback) and the cwd's basename on every
// resume of that same untouched row -- a launch-kind-dependent value for a
// fact that did not change.
func (s Service) sessionContextEnv(session store.Session, launchKind string) map[string]string {
	return map[string]string{
		"DECK_SESSION_ID":              session.ID,
		"DECK_SESSION_NAME":            session.Name,
		"DECK_SESSION_SLUG":            session.Slug,
		"DECK_SESSION_CWD":             session.CWD,
		"DECK_SESSION_AGENT":           session.Agent,
		"DECK_SESSION_WORKSPACE":       session.WorkspaceColumn,
		"DECK_SESSION_PROFILE":         session.PermissionProfile,
		"DECK_SESSION_CONVERSATION_ID": session.ConversationID,
		"DECK_SESSION_LAUNCH_KIND":     launchKind,
		"DECK_HOME":                    s.DeckHome,
	}
}
