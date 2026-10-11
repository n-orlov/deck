package service

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/n-orlov/deck/internal/agent"
	"github.com/n-orlov/deck/internal/store"
)

// harnessPlaceholders are the titles a harness shows before it has a real one.
// Adopting one would rename a row to the harness's own product name.
var harnessPlaceholders = map[string]bool{"Claude Code": true, "GitHub Copilot": true, "π": true}

// adoptionEligible reports whether a row may take a name from its harness:
// only a 'derived' or 'harness' row, never a 'user' one (which is frozen), on
// a live agent conversation deck has bound.
func adoptionEligible(session store.Session) bool {
	if session.Agent == "shell" || session.ConversationID == "" {
		return false
	}
	return session.NameSource == "derived" || session.NameSource == "harness"
}

// AdoptName reads the harness's current name for one session and adopts it
// (SPEC §8 "Adopting a harness name"). It is the entry the hook paths call
// (Stop, UserPromptSubmit, agentStop, session_info_changed); the reconcile
// tick reaches the same code through adoptOnTick. The returned bool reports
// whether the row was renamed. A row that is not eligible, an adapter whose
// caps do not claim ReadName and a name that is empty, equal to the current
// one or a harness placeholder all leave the row untouched.
func (s Service) AdoptName(ctx context.Context, sessionID string) (bool, error) {
	if s.Store == nil || s.Clock == nil || s.Agents == nil {
		return false, nil
	}
	session, err := s.Store.GetSession(ctx, sessionID)
	if err != nil {
		return false, err
	}
	return s.adoptRow(ctx, session)
}

// adoptOnTick is adoption on a reconcile pass: a row that is gone, terminal or
// not eligible is skipped, and a failure is never allowed to fail the pass.
func (s Service) adoptOnTick(ctx context.Context, session store.Session) {
	if s.Agents == nil || rowIsTerminal(session) || !adoptionEligible(session) {
		return
	}
	_, _ = s.adoptRow(ctx, session)
}

func (s Service) adoptRow(ctx context.Context, session store.Session) (bool, error) {
	if !adoptionEligible(session) {
		return false, nil
	}
	adapter, ok := s.Agents.Lookup(session.Agent)
	if !ok {
		return false, nil
	}
	name, explicit, ok := agent.ReadName(adapter, s.nameProbe(ctx, adapter, session))
	name = strings.TrimSpace(name)
	if !ok || name == "" || name == session.Name || harnessPlaceholders[name] {
		return false, nil
	}
	return s.applyAdoption(ctx, session, name, explicit)
}

// nameProbe fills the probe input for session: the user's home, the working
// directory, the bound conversation and the env keys the adapter names.
func (s Service) nameProbe(ctx context.Context, adapter agent.Adapter, session store.Session) agent.NameProbeInput {
	home, _ := os.UserHomeDir()
	return agent.NameProbeInput{
		Home: home, CWD: session.CWD, ConversationID: session.ConversationID,
		Env: s.probeEnv(ctx, adapter.Capabilities().TranscriptEnvKeys, session),
	}
}

// probeEnv resolves keys the way the `e` editor does: the session's own env,
// then config [env], then the session's own tmux server's global environment.
func (s Service) probeEnv(ctx context.Context, keys []string, session store.Session) map[string]string {
	if len(keys) == 0 {
		return nil
	}
	env := make(map[string]string, len(keys))
	for _, key := range keys {
		env[key] = s.resolveProbeKey(ctx, key)
		if v, ok := session.Env[key]; ok {
			env[key] = v
		}
	}
	return env
}

func (s Service) resolveProbeKey(ctx context.Context, key string) string {
	if v, ok := s.ConfigEnv[key]; ok {
		return v
	}
	if s.TMux.Socket == "" {
		return ""
	}
	v, _, _ := s.TMux.ServerEnvironment(ctx, key)
	return v
}

// applyAdoption renames session to the harness's name. An explicit name (a
// human chose it inside the harness) is recorded as name_source=user, which
// freezes the row; any other is name_source=harness. The event source is
// harness:<kind>. The tmux slug is never written.
func (s Service) applyAdoption(ctx context.Context, session store.Session, name string, explicit bool) (bool, error) {
	nameSource := "harness"
	if explicit {
		nameSource = "user"
	}
	unique, err := s.uniqueAdoptedName(ctx, session.ID, name)
	if err != nil || unique == session.Name {
		return false, err
	}
	holders, err := s.tombstonedNameHolders(ctx, unique)
	if err != nil {
		return false, err
	}
	adopted, err := s.Store.AdoptSessionName(ctx, session.ID, unique, "harness:"+session.Agent, nameSource, s.Clock.Now().UnixMilli())
	if err != nil || !adopted {
		return false, err
	}
	if err := s.reapedHolderFiles(ctx, holders); err != nil {
		return true, fmt.Errorf("clean up the session reaped by adopting name %q: %w", unique, err)
	}
	return true, nil
}

// uniqueAdoptedName returns name, or name-2, name-3, ... for the first one no
// other row holds (by name or by slug). Names are not unique across harnesses,
// so a clash is resolved here and never refused.
func (s Service) uniqueAdoptedName(ctx context.Context, sessionID, name string) (string, error) {
	rows, err := s.Store.ListSessionsIncludingArchived(ctx)
	if err != nil {
		return "", fmt.Errorf("list sessions to place an adopted name: %w", err)
	}
	for n := 1; ; n++ {
		candidate := name
		if n > 1 {
			candidate = fmt.Sprintf("%s-%d", name, n)
		}
		if !nameTaken(rows, sessionID, candidate) {
			return candidate, nil
		}
	}
}

func nameTaken(rows []store.Session, sessionID, candidate string) bool {
	slug := store.Slug(candidate)
	for _, row := range rows {
		if row.ID != sessionID && (row.Name == candidate || row.Slug == slug) {
			return true
		}
	}
	return false
}
