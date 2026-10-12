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
//
// A name that came from the harness is data only (R253): a title adopted as
// explicit is frozen as name_source=user but keeps NameAdopted, so it never
// reaches a shell command or a launch/resume argv. Only a name entered in deck
// does.
func launchName(session store.Session) string {
	if session.NameAdopted {
		return ""
	}
	if session.NameSource != "" && session.NameSource != "user" {
		return ""
	}
	return strings.TrimSpace(session.Name)
}
