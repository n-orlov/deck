package service

import (
	"context"
	"fmt"
	"os"

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

// adoptionOn reports whether name adoption and rename push are enabled for
// session (SPEC §11.4): the session's own override when set, else the global
// adopt_harness_names, which is on when Service.AdoptNames is nil.
func (s Service) adoptionOn(session store.Session) bool {
	if session.HarnessNameAdopt != nil {
		return *session.HarnessNameAdopt
	}
	return s.AdoptNames == nil || s.AdoptNames()
}

// AdoptName reads the harness's current name for one session and adopts it
// (SPEC §8 "Adopting a harness name"). It is the entry the hook paths call
// (Stop, UserPromptSubmit, agentStop, session_info_changed); the reconcile
// tick reaches the same code through adoptOnTick. The returned bool reports
// whether the row was renamed. A row that is not eligible, an adapter whose
// caps do not claim ReadName and a name that is empty, equal to the current
// one or a harness placeholder all leave the row untouched.
func (s Service) AdoptName(ctx context.Context, sessionID string) (bool, error) {
	return s.AdoptNameWith(ctx, sessionID, NameHint{})
}

// NameHint is what a trigger hook's payload adds to an adoption: the transcript
// file the agent named and the title it reports a human chose. Both are
// optional and untrusted; the zero value adds nothing.
type NameHint struct {
	TranscriptPath string
	SessionTitle   string
}

// AdoptNameWith is AdoptName for a hook that carried a hint.
func (s Service) AdoptNameWith(ctx context.Context, sessionID string, hint NameHint) (bool, error) {
	if s.Store == nil || s.Clock == nil || s.Agents == nil {
		return false, nil
	}
	session, err := s.Store.GetSession(ctx, sessionID)
	if err != nil {
		return false, err
	}
	return s.adoptRow(ctx, session, hint)
}

// adoptOnTick is adoption on a reconcile pass: a row that is gone, terminal or
// not eligible is skipped, and a failure is never allowed to fail the pass.
func (s Service) adoptOnTick(ctx context.Context, session store.Session) {
	if s.Agents == nil {
		return
	}
	s.applyLaunchName(ctx, session)
	if session.PendingHarnessName != "" {
		s.settlePush(ctx, session)
		return
	}
	if rowIsTerminal(session) || !adoptionEligible(session) || !s.adoptionOn(session) {
		return
	}
	_, _ = s.adoptRow(ctx, session, NameHint{})
}

func (s Service) adoptRow(ctx context.Context, session store.Session, hint NameHint) (bool, error) {
	if session.PendingHarnessName != "" {
		// A pushed name is awaiting its echo: settle that first, adopt nothing.
		s.settlePushWith(ctx, session, hint)
		return false, nil
	}
	if !adoptionEligible(session) || !s.adoptionOn(session) {
		return false, nil
	}
	if title := usableHarnessName(hint.SessionTitle, session); title != "" {
		// A title the harness reports in its own hook is a name a human chose.
		return s.applyAdoption(ctx, session, title, true)
	}
	adapter, ok := s.Agents.Lookup(session.Agent)
	if !ok {
		return false, nil
	}
	name, explicit, ok := agent.ReadName(adapter, s.nameProbe(ctx, adapter, session, hint))
	if !ok {
		return false, nil
	}
	name = usableHarnessName(name, session)
	if name == "" {
		return false, nil
	}
	return s.applyAdoption(ctx, session, name, explicit)
}

// usableHarnessName sanitises a harness-reported title and returns it, or ""
// when it is empty, equal to the row's current name or a placeholder.
func usableHarnessName(raw string, session store.Session) string {
	name := agent.SanitiseHarnessName(raw)
	if name == session.Name || harnessPlaceholders[name] {
		return ""
	}
	return name
}

// nameProbe fills the probe input for session: the user's home, the working
// directory, the bound conversation and the env keys the adapter names.
func (s Service) nameProbe(ctx context.Context, adapter agent.Adapter, session store.Session, hint NameHint) agent.NameProbeInput {
	home, _ := os.UserHomeDir()
	return agent.NameProbeInput{
		Home: home, CWD: session.CWD, ConversationID: session.ConversationID,
		AdoptedName:    adoptedName(session),
		Env:            s.probeEnv(ctx, adapter.Capabilities().TranscriptEnvKeys, session),
		TranscriptPath: hint.TranscriptPath,
		PaneTitle:      s.paneTitleReader(ctx, session),
	}
}

// paneTitler is the optional pane access that reads a pane's title.
type paneTitler interface {
	PaneTitle(ctx context.Context, slug string) (string, error)
}

// paneTitleReader returns the read of session's pane title, taken per pane and
// only when called. A pane that cannot be read has no title.
func (s Service) paneTitleReader(ctx context.Context, session store.Session) func() string {
	return func() string {
		var reader paneTitler = s.TMux
		if titler, ok := s.NamePane.(paneTitler); ok {
			reader = titler
		}
		title, err := reader.PaneTitle(ctx, session.Slug)
		if err != nil {
			return ""
		}
		return title
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
// harness:<kind>. The tmux slug is never written. An explicit name that
// differs from the stored one freezes the row even when suffix resolution
// leaves the displayed name unchanged (provenance only: no rename, no event).
func (s Service) applyAdoption(ctx context.Context, session store.Session, name string, explicit bool) (bool, error) {
	unique, err := s.uniqueAdoptedName(ctx, session.ID, name)
	if err != nil {
		return false, err
	}
	if unique == session.Name {
		return false, s.freezeUnchangedName(ctx, session.ID, explicit)
	}
	holders, err := s.tombstonedNameHolders(ctx, unique)
	if err != nil {
		return false, err
	}
	adopted, err := s.Store.AdoptSessionName(ctx, session.ID, unique, "harness:"+session.Agent, adoptedNameSource(explicit), s.Clock.Now().UnixMilli())
	if err != nil || !adopted {
		return false, err
	}
	s.offerRenamed(ctx, session.ID, "harness:"+session.Agent, unique)
	if err := s.reapedHolderFiles(ctx, holders); err != nil {
		return true, fmt.Errorf("clean up the session reaped by adopting name %q: %w", unique, err)
	}
	return true, nil
}

// adoptedNameSource is the name_source an adopted name is recorded with.
func adoptedNameSource(explicit bool) string {
	if explicit {
		return "user"
	}
	return "harness"
}

// freezeUnchangedName handles a collision suffix that resolved back to the
// stored name: nothing to rename, but a name a human chose in the harness
// still freezes the row.
func (s Service) freezeUnchangedName(ctx context.Context, sessionID string, explicit bool) error {
	if !explicit {
		return nil
	}
	_, err := s.Store.FreezeSessionName(ctx, sessionID)
	return err
}

// uniqueAdoptedName returns name, or name-2, name-3, ... for the first one no
// other row holds (by name, by slug, or by sanitised name form). Names are not unique across harnesses,
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
	form := agent.NameComparisonForm(candidate)
	for _, row := range rows {
		if row.ID != sessionID && (row.Name == candidate || row.Slug == slug || agent.NameComparisonForm(row.Name) == form) {
			return true
		}
	}
	return false
}

// offerRenamed offers a committed rename to the event hook (SPEC §10.4). The
// reason is the rename's source: "user" for a rename typed in deck and
// "harness:<kind>" for a name adopted from a harness.
func (s Service) offerRenamed(ctx context.Context, sessionID, source, name string) {
	s.offerHook(ctx, HookEvent{
		SessionID: sessionID, StoredKind: "renamed", Reason: source, Message: name, At: s.Clock.Now(),
	})
}

// adoptedName is the name deck took from the harness for session, or "" when
// the row has not adopted one (a derived or user row).
func adoptedName(session store.Session) string {
	if session.NameSource == "harness" {
		return session.Name
	}
	return ""
}
