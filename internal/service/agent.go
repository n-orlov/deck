package service

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/n-orlov/deck/internal/agent"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// AgentCreateInput contains the user-supplied fields for a real coding-agent
// session (SPEC §5/§8), as opposed to a plain shell (see ShellCreateInput).
type AgentCreateInput struct {
	Name string
	CWD  string
	// Agent is the adapter kind to launch, e.g. "claude" or "pi".
	Agent string
	// PermissionProfile is the requested SPEC §5 profile name. If the
	// adapter does not support it, Caps.ResolveProfile degrades it to
	// "safe" and the resolved value (not the request) is what gets
	// persisted and launched.
	PermissionProfile string
	// LaunchArgs are appended verbatim after the adapter's own argv.
	LaunchArgs []string
	// Env is the session-layer environment, the highest-priority layer in
	// the SPEC §6.3 PATH resolution order.
	Env map[string]string
	// PreLaunch and LoginShell are persisted for a later launch step
	// (task 010); CreateAgent stores them but does not run them.
	PreLaunch  string
	LoginShell bool
}

// CreateAgent creates the durable row for a real coding-agent session,
// assigns its conversation id (when the adapter declares
// Caps.AssignsConversationID) before launch and persists it on the row,
// resolves PATH in the SPEC §6.3 order (server env -> captured_path ->
// config [env] -> session env), records captured_path at create time, and
// launches the adapter's launch argv in one tmux pane. As with CreateShell,
// a launch failure after the row exists is represented as a durable error
// row plus a transition event, never a misleading "starting" row.
func (s Service) CreateAgent(ctx context.Context, input AgentCreateInput) (store.Session, error) {
	if s.Store == nil || s.Audit == nil || s.Clock == nil || s.IDs == nil || s.Agents == nil {
		return store.Session{}, errors.New("agent creation requires store, audit logger, clock, id generator, and adapter registry")
	}
	if input.Name == "" || input.CWD == "" {
		return store.Session{}, errors.New("agent session name and working directory are required")
	}
	adapter, ok := s.Agents.Lookup(input.Agent)
	if !ok {
		return store.Session{}, fmt.Errorf("unknown agent kind %q", input.Agent)
	}
	caps := adapter.Capabilities()
	profile, _, _ := caps.ResolveProfile(adapter.Kind(), input.PermissionProfile)

	id, err := s.IDs.UUID()
	if err != nil {
		return store.Session{}, fmt.Errorf("generate agent session id: %w", err)
	}
	var conversationID string
	if caps.AssignsConversationID {
		conversationID, err = s.IDs.UUID()
		if err != nil {
			return store.Session{}, fmt.Errorf("assign conversation id: %w", err)
		}
	}

	capturedPath := os.Getenv("PATH")
	if capturedPath == "" {
		return store.Session{}, errors.New("PATH is required to create an agent session")
	}

	now := s.Clock.Now().UnixMilli()
	session, err := s.Store.CreateSession(ctx, store.CreateSessionInput{
		ID: id, Name: input.Name, CWD: input.CWD, Agent: adapter.Kind(), CapturedPath: capturedPath,
		Status: "starting", StatusSource: "user", StatusAt: now, CreatedAt: now,
		LaunchArgs: input.LaunchArgs, Env: input.Env, PreLaunch: input.PreLaunch, LoginShell: input.LoginShell,
		PermissionProfile: profile, ConversationID: conversationID,
	})
	if err != nil {
		return store.Session{}, fmt.Errorf("create durable agent session %q: %w", input.Name, err)
	}
	if err := s.Audit.Transition(session.ID, "starting"); err != nil {
		return session, fmt.Errorf("audit starting agent session %q: %w", session.Name, err)
	}

	argv, err := adapter.Launch(agent.LaunchInput{
		CWD: session.CWD, ConversationID: conversationID, Profile: profile, ExtraArgs: input.LaunchArgs,
	})
	if err != nil {
		return s.launchFailed(ctx, session, fmt.Errorf("build launch argv for agent session %q: %w", session.Name, err))
	}

	launchEnv := s.resolveLaunchEnv(capturedPath, input.Env)
	if _, err := s.TMux.Create(ctx, tmux.Launch{Slug: session.Slug, CWD: session.CWD, Command: argv, Env: launchEnv}); err != nil {
		return s.launchFailed(ctx, session, fmt.Errorf("launch agent session %q: %w", session.Name, err))
	}
	if err := s.Audit.Launch(session.ID, argv, launchEnv); err != nil {
		// As with CreateShell, an unaudited launch is not a successful deck
		// launch: tear the pane back down and leave an observable error row.
		_ = s.TMux.Kill(ctx, session.Slug)
		return s.launchFailed(ctx, session, fmt.Errorf("audit agent launch %q: %w", session.Name, err))
	}
	if err := s.Store.UpdateSessionStatus(ctx, store.StatusUpdateInput{
		SessionID: session.ID, Status: "starting", Reason: "", Source: "tmux",
		At: s.Clock.Now().UnixMilli(), EventKind: "launch.ready",
	}); err != nil {
		return s.launchFailed(ctx, session, fmt.Errorf("record ready agent session %q: %w", session.Name, err))
	}
	if err := s.Audit.Transition(session.ID, "launch.ready"); err != nil {
		return s.launchFailed(ctx, session, fmt.Errorf("audit ready agent session %q: %w", session.Name, err))
	}
	session.StatusSource = "tmux"
	session.ConversationID = conversationID
	session.PermissionProfile = profile
	return session, nil
}

// resolveLaunchEnv merges the SPEC §6.3 PATH-resolution layers that sit
// above the tmux server's own inherited environment: captured_path, then
// config [env], then the session's own env (highest priority). tmux's `env`
// launch wrapper (internal/tmux) only overrides the keys it is given, so
// every other server-inherited variable passes through unchanged; only PATH
// and any keys explicitly set by config or the session need to appear here.
func (s Service) resolveLaunchEnv(capturedPath string, sessionEnv map[string]string) map[string]string {
	merged := make(map[string]string, len(s.ConfigEnv)+len(sessionEnv)+1)
	if capturedPath != "" {
		merged["PATH"] = capturedPath
	}
	for key, value := range s.ConfigEnv {
		merged[key] = value
	}
	for key, value := range sessionEnv {
		merged[key] = value
	}
	return merged
}
