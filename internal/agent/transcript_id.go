package agent

import "strings"

// safeConversationID reports whether id may be used as one path component of
// a transcript path. A conversation id arrives in a hook payload, so it is
// untrusted: an empty id, "." or "..", or an id holding a path separator
// ("/" or "\") could otherwise point the lookup outside the agent's
// transcript directory. Every adapter that builds a transcript path from an
// id calls it first and declines (ok=false) on false.
func safeConversationID(id string) bool {
	if id == "" || id == "." || id == ".." {
		return false
	}
	return !strings.ContainsAny(id, `/\`)
}
