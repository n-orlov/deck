package service

import (
	"strings"

	"github.com/n-orlov/deck/internal/store"
)

// launchName is the name an adapter may hand to its agent for session: the
// row's name only when its provenance is 'user' (typed at create or set by a
// rename), and never a blank one. A derived or harness row yields "", so the
// agent keeps its own auto-title (R250). An empty NameSource is the in-memory
// spelling of 'user' that store.CreateSession normalises the same way.
func launchName(session store.Session) string {
	if session.NameSource != "" && session.NameSource != "user" {
		return ""
	}
	return strings.TrimSpace(session.Name)
}
