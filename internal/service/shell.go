// Package service coordinates durable deck state with tmux operations.
package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/n-orlov/deck/internal/agent"
	"github.com/n-orlov/deck/internal/audit"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// ShellCreateInput contains the user supplied fields for a plain shell session.
type ShellCreateInput struct {
	Name string
	CWD  string
	Env  map[string]string
}

// Service performs operations which must keep the SQLite store and private
// tmux server coherent. All dependencies are explicit so callers can give each
// client its own store connection and audit writer.
type Service struct {
	Store *store.Store
	TMux  tmux.Client
	Audit *audit.Logger
	Clock *config.Clock
	IDs   *config.IDGenerator

	// Agents looks up an adapter by its declared kind (e.g. "claude", "pi")
	// for CreateAgent and, later, Resume. Only CreateShell tolerates it
	// being nil; agent creation requires it.
	Agents *agent.Registry
	// ConfigEnv mirrors config.toml's [env] table (SPEC §6.3): the layer
	// between captured_path and the session's own env in PATH resolution
	// order. A nil map is the common, valid case of no configured overrides.
	ConfigEnv map[string]string
	// GlobalPreLaunch mirrors config.toml's top-level pre_launch key (task
	// 004, phase3j): the global launch hook, a shell command run in every
	// session's pane before that session's own launch argv, in addition to
	// (never instead of) a session's own PreLaunch (store.Session/
	// CreateAgentInput's own field of that name). Empty is the common, valid
	// case of nothing configured. Composing this with a session's own
	// pre_launch in buildPaneCommand is a later task's own scope.
	GlobalPreLaunch string
	// DeckExecutable and DeckHome are deck-owned launch facts supplied to
	// adapters for hook instrumentation. They are never persisted as user
	// launch arguments or session environment.
	DeckExecutable string
	DeckHome       string

	// Shell overrides the user's $SHELL. It is primarily useful to embedded
	// callers; an empty value selects $SHELL, falling back to /bin/sh.
	Shell string

	// RecentCwdLimit is SPEC §6.5's [ui] recent_cwd_limit (default 5): how
	// many entries survive in the §11.7 directory history after a session's
	// cwd is promoted. It is caller-supplied, never assumed, so a zero value
	// means exactly what store.PromoteRecentCwd documents: keep nothing.
	RecentCwdLimit int

	// LeaseReleaser overrides the narrow store seam Resume uses to release a
	// launch lease once its attempt concludes (R75, issue #11); see
	// LaunchLeaseReleaser. Left nil -- true for every production caller and
	// every existing test, none of which set it -- Resume falls back to
	// Store itself, which already satisfies the interface. A test substitutes
	// a different implementation here to exercise the release-failure
	// fallback without touching any other store operation.
	LeaseReleaser LaunchLeaseReleaser
}

// promoteRecentCwd moves cwd to the front of the §11.7 directory history on
// session creation, resolving it to an absolute path first (store.
// PromoteRecentCwd requires one). Recent-directory history is explicitly
// not load-bearing (SPEC §4): a failure here must never turn a created
// session into a failed one, so it is swallowed rather than propagated.
func (s Service) promoteRecentCwd(ctx context.Context, cwd string) {
	if s.Store == nil || cwd == "" {
		return
	}
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return
	}
	_ = s.Store.PromoteRecentCwd(ctx, abs, s.RecentCwdLimit)
}

// CreateShell creates the durable row before starting its one-pane private
// tmux session. A failed tmux launch is represented as an error row plus a
// transition event, rather than leaving a misleading "starting" row behind.
func (s Service) CreateShell(ctx context.Context, input ShellCreateInput) (store.Session, error) {
	if s.Store == nil || s.Audit == nil || s.Clock == nil || s.IDs == nil {
		return store.Session{}, errors.New("shell creation requires store, audit logger, clock, and id generator")
	}
	if input.Name == "" || input.CWD == "" {
		return store.Session{}, errors.New("shell session name and working directory are required")
	}
	id, err := s.IDs.UUID()
	if err != nil {
		return store.Session{}, fmt.Errorf("generate shell session id: %w", err)
	}
	shell := s.Shell
	if shell == "" {
		shell = os.Getenv("SHELL")
	}
	if shell == "" {
		shell = "/bin/sh"
	}
	if !filepath.IsAbs(shell) {
		return store.Session{}, fmt.Errorf("user shell %q must be an absolute path", shell)
	}
	capturedPath := os.Getenv("PATH")
	if capturedPath == "" {
		return store.Session{}, errors.New("PATH is required to create a shell session")
	}
	now := s.Clock.Now().UnixMilli()
	// SPEC §9.2 (R77): if this name (or its slug) is held only by a
	// tombstoned row, CreateSession reaps that row inside its own
	// transaction. Note the holders now, remove their files only after that
	// commit -- a create that is refused must leave them untouched.
	reapedHolders, err := s.tombstonedNameHolders(ctx, input.Name)
	if err != nil {
		return store.Session{}, err
	}
	session, err := s.Store.CreateSession(ctx, store.CreateSessionInput{
		ID: id, Name: input.Name, CWD: input.CWD, Agent: "shell", CapturedPath: capturedPath,
		Status: "starting", StatusSource: "user", StatusAt: now, CreatedAt: now,
	})
	if err != nil {
		return store.Session{}, fmt.Errorf("create durable shell session: %w", err)
	}
	if err := s.reapedHolderFiles(ctx, reapedHolders); err != nil {
		return session, fmt.Errorf("clean up the session reaped by reusing name %q: %w", input.Name, err)
	}
	s.promoteRecentCwd(ctx, session.CWD)
	if err := s.Audit.Transition(session.ID, "starting"); err != nil {
		return session, fmt.Errorf("audit starting shell session %q: %w", session.Name, err)
	}
	// Route the shell adapter through the same applyInstrumentation call
	// every other launch path uses (CreateAgent, Resume), even though
	// Shell.Instrument always returns nil (SPEC §8.1: shell has no agent
	// hook source) -- this keeps CreateShell on the identical
	// applyInstrumentation-then-session-context sequence rather than a
	// second, drifting copy of it. s.Agents is nil-tolerant (only agent
	// creation requires it), so this falls back to constructing the
	// adapter directly when no registry was supplied.
	shellAdapter, ok := agent.Adapter(nil), false
	if s.Agents != nil {
		shellAdapter, ok = s.Agents.Lookup("shell")
	}
	if !ok {
		shellAdapter = agent.NewShell()
	}
	launchInput := agent.LaunchInput{
		CWD: session.CWD, DeckExecutable: s.DeckExecutable, DeckSessionID: session.ID, DeckHome: s.DeckHome,
	}
	launchEnv := make(map[string]string, len(input.Env)+10)
	for key, value := range input.Env {
		launchEnv[key] = value
	}
	argv := []string{shell}
	argv, launchEnv, err = applyInstrumentation(shellAdapter, launchInput, argv, launchEnv)
	if err != nil {
		return s.launchFailed(ctx, session, fmt.Errorf("instrument shell session %q: %w", session.Name, err))
	}
	// SPEC §6.1 (R104): deck's own session context is merged last, above
	// the instrumentation adapter's own -- last here too, even though
	// Shell's own instrumentation never adds anything, so a shell pane
	// carries the same nine variables a claude pane does.
	for key, value := range s.sessionContextEnv(session, LaunchKindCreate) {
		launchEnv[key] = value
	}
	if _, err := s.TMux.Create(ctx, tmux.Launch{Slug: session.Slug, CWD: session.CWD, Command: argv, Env: launchEnv}); err != nil {
		return s.launchFailed(ctx, session, fmt.Errorf("launch shell session %q: %w", session.Name, err))
	}
	if err := s.Audit.Launch(session.ID, argv, launchEnv); err != nil {
		// The pane is not a successful deck launch if its required audit record
		// cannot be written, so remove it and leave an observable durable error.
		_ = s.TMux.Kill(ctx, session.Slug)
		return s.launchFailed(ctx, session, fmt.Errorf("audit shell launch %q: %w", session.Name, err))
	}
	// A user-sourced starting row is still being launched and must not be
	// mistaken for a disappeared pane by another live deck client. Once tmux
	// and its launch audit are both complete, mark that same visible state as
	// tmux-observed so reconciliation may safely own its liveness.
	if err := s.Store.UpdateSessionStatus(ctx, store.StatusUpdateInput{
		SessionID: session.ID, Status: "starting", Reason: "", Source: "tmux",
		At: s.Clock.Now().UnixMilli(), EventKind: "launch.ready",
	}); err != nil {
		return s.launchFailed(ctx, session, fmt.Errorf("record ready shell session %q: %w", session.Name, err))
	}
	if err := s.Audit.Transition(session.ID, "launch.ready"); err != nil {
		return s.launchFailed(ctx, session, fmt.Errorf("audit ready shell session %q: %w", session.Name, err))
	}
	session.StatusSource = "tmux"
	return session, nil
}

func (s Service) launchFailed(ctx context.Context, session store.Session, cause error) (store.Session, error) {
	if s.Clock == nil {
		return session, fmt.Errorf("%w (cannot record launch failure without clock)", cause)
	}
	if err := s.Store.UpdateSessionStatus(ctx, store.StatusUpdateInput{
		SessionID: session.ID, Status: "error", Reason: cause.Error(), Source: "tmux", At: s.Clock.Now().UnixMilli(), EventKind: "launch.failed",
	}); err != nil {
		return session, fmt.Errorf("%w (also record launch failure: %v)", cause, err)
	}
	if err := s.Audit.Transition(session.ID, "launch.failed"); err != nil {
		return session, fmt.Errorf("%w (also audit launch failure: %v)", cause, err)
	}
	return session, cause
}
