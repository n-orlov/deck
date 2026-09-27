package service

import (
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// TestSessionContextEnvExportsProfileDefaultAndNamed is task 012's (R154)
// dedicated unit evidence that DECK_PROFILE sits in the same unoverridable
// layer as DECK_HOME (session_context.go's sessionContextEnv), called
// directly (no tmux, no adapter) so it cannot pass by construction against
// a production bug that only manifests through the real launch path.
// SPEC §3.4/§6.1: every pane carries DECK_PROFILE, `default` included, so a
// Service whose Profile field was never set (the zero value, matching
// every pre-task-012 test fixture in this package) must still export
// "default" rather than an empty string, and a Service given a named
// profile must export that name verbatim.
func TestSessionContextEnvExportsProfileDefaultAndNamed(t *testing.T) {
	session := store.Session{ID: "s1", Name: "n1", Slug: "n1", CWD: "/work/a", Agent: "shell"}

	defaultSvc := Service{DeckHome: "/home/deck"}
	env := defaultSvc.sessionContextEnv(session, LaunchKindCreate)
	if got, present := env["DECK_PROFILE"]; !present || got != config.DefaultProfile {
		t.Fatalf("DECK_PROFILE for a Service with no Profile set = %q (present %v), want present with %q", got, present, config.DefaultProfile)
	}

	namedSvc := Service{DeckHome: "/home/deck", Profile: "acme"}
	env = namedSvc.sessionContextEnv(session, LaunchKindResume)
	if got, present := env["DECK_PROFILE"]; !present || got != "acme" {
		t.Fatalf("DECK_PROFILE for Service.Profile=%q = %q (present %v), want present with %q", "acme", got, present, "acme")
	}
}
