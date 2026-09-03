package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

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
	// PreLaunch, when set, runs in the pane before the agent argv (SPEC
	// §6.4); a failing pre_launch prevents the agent from ever starting.
	// LoginShell runs the whole pane command via `$SHELL -lc` instead of
	// execing the adapter argv directly, and is mutually exclusive with
	// relying on captured_path for PATH resolution.
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
	profile, _, degradationReason := caps.ResolveProfile(adapter.Kind(), input.PermissionProfile)

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
	// SPEC §9.2 (R77), as in CreateShell: note which tombstoned rows hold
	// this name before the create reaps them in its own transaction, and
	// clean up their files only once that transaction has committed.
	reapedHolders, err := s.tombstonedNameHolders(ctx, input.Name)
	if err != nil {
		return store.Session{}, err
	}
	session, err := s.Store.CreateSession(ctx, store.CreateSessionInput{
		ID: id, Name: input.Name, CWD: input.CWD, Agent: adapter.Kind(), CapturedPath: capturedPath,
		Status: "starting", StatusSource: "user", StatusAt: now, CreatedAt: now,
		LaunchArgs: input.LaunchArgs, Env: input.Env, PreLaunch: input.PreLaunch, LoginShell: input.LoginShell,
		PermissionProfile: profile, PermissionProfileReason: degradationReason, ConversationID: conversationID,
	})
	if err != nil {
		return store.Session{}, fmt.Errorf("create durable agent session %q: %w", input.Name, err)
	}
	if err := s.reapedHolderFiles(ctx, reapedHolders); err != nil {
		return session, fmt.Errorf("clean up the session reaped by reusing name %q: %w", input.Name, err)
	}
	s.promoteRecentCwd(ctx, session.CWD)
	if err := s.Audit.Transition(session.ID, "starting"); err != nil {
		return session, fmt.Errorf("audit starting agent session %q: %w", session.Name, err)
	}

	// No LaunchGeneration here: a brand-new row's first launch takes no launch
	// lease (the row is created directly as `starting`), so there is no earlier
	// launch of it that a hook could be confused with (issue #11, R74). The
	// row's generation is first written by the AcquireLaunchLease of its first
	// resume/restart.
	launchInput := agent.LaunchInput{
		CWD: session.CWD, ConversationID: conversationID, Profile: profile, ExtraArgs: input.LaunchArgs,
		DeckExecutable: s.DeckExecutable, DeckSessionID: session.ID, DeckHome: s.DeckHome,
	}
	argv, err := adapter.Launch(launchInput)
	if err != nil {
		return s.launchFailed(ctx, session, fmt.Errorf("build launch argv for agent session %q: %w", session.Name, err))
	}

	// login_shell=1 is mutually exclusive with relying on captured_path: the
	// login shell resolves its own PATH via its own profile/rc scripts, so
	// deck must not also inject a PATH override here.
	envCapturedPath := capturedPath
	if input.LoginShell {
		envCapturedPath = ""
	}
	launchEnv := s.resolveLaunchEnv(envCapturedPath, input.Env)
	argv, launchEnv, err = applyInstrumentation(adapter, launchInput, argv, launchEnv)
	if err != nil {
		return s.launchFailed(ctx, session, fmt.Errorf("instrument agent session %q: %w", session.Name, err))
	}
	// SPEC §6.1 (R104): deck's own session context is merged last, above the
	// instrumentation adapters own, so a session `env` or config `[env]` key
	// of the same name can never lie to a hook about which session it is.
	for key, value := range s.sessionContextEnv(session, LaunchKindCreate) {
		launchEnv[key] = value
	}
	paneCommand, err := buildPaneCommand(s.GlobalPreLaunch, input.PreLaunch, input.LoginShell, argv)
	if err != nil {
		return s.launchFailed(ctx, session, fmt.Errorf("build pane command for agent session %q: %w", session.Name, err))
	}
	if _, err := s.TMux.Create(ctx, tmux.Launch{Slug: session.Slug, CWD: session.CWD, Command: paneCommand, Env: launchEnv}); err != nil {
		return s.launchFailed(ctx, session, fmt.Errorf("launch agent session %q: %w", session.Name, err))
	}
	if err := s.Audit.Launch(session.ID, paneCommand, launchEnv); err != nil {
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
	session.PermissionProfileReason = degradationReason
	return session, nil
}

// buildPaneCommand wraps the adapter's launch/resume argv in a shell so
// globalPreLaunch (config-wide) and preLaunch (per-session, SPEC §6.4) run
// first in the same pane, and so login_shell=1 runs the whole pane command
// via `$SHELL -lc` rather than execing the adapter argv directly. When
// neither hook is set and login_shell is not requested, the adapter argv
// passes through unchanged, matching CreateAgent's pre-task-010 behavior
// exactly.
//
// globalPreLaunch, preLaunch and the adapter argv are joined with shell
// `&&`, global first, so a failing hook short-circuits: neither the later
// hook nor the agent is ever exec'd, the pane's shell exits with the
// failing hook's own non-zero status, and (because deck's tmux server runs
// with `remain-on-exit failed`) the pane is retained with the hook's own
// output visible rather than silently starting the agent. The launch audit
// record still captures the full wrapped command, so the failure is
// recorded even though CreateAgent does not itself wait for either hook to
// finish.
//
// SPEC §6.4's guarantee: "a hook's exports reach that session's agent and
// nothing else." This is exactly what the `export K=V && ... && exec "$@"`
// shape above delivers, and delivers by construction, not by extra plumbing:
// export mutates the one shell process's own environment, and exec replaces
// that process's image in place (same pid) with the adapter argv, which
// therefore inherits the mutated environment automatically. Three boundaries
// follow from that same mechanism:
//   - the session's agent process: YES. It is the `exec`'d process, in the
//     same shell, so whatever either hook exported is simply that process's
//     own environment -- inheritance is automatic, not a deck feature to keep
//     working.
//   - the tmux *session* environment table (the one §3.2's `-e` mirror
//     populates for future panes, read back with `tmux show-environment`):
//     NO. Nothing here ever calls tmux's `set-environment`; a hook's export
//     is invisible to that table and to any later pane tmux creates from it.
//   - `state.db`: NO. Neither hook's output nor its exported values are
//     written to any column; `pre_launch`/`GlobalPreLaunch` are stored only
//     as the shell line itself, never as whatever it produces at runtime,
//     which is the whole reason a hook is the preferred home for a secret.
func buildPaneCommand(globalPreLaunch, preLaunch string, loginShell bool, argv []string) ([]string, error) {
	if len(argv) == 0 || argv[0] == "" {
		return nil, errors.New("agent launch argv is empty")
	}
	if globalPreLaunch == "" && preLaunch == "" && !loginShell {
		return argv, nil
	}
	shell := "/bin/sh"
	flag := "-c"
	if loginShell {
		if s := os.Getenv("SHELL"); s != "" {
			shell = s
		}
		flag = "-lc"
	}
	script := "exec \"$@\""
	if preLaunch != "" {
		script = preLaunch + " && " + script
	}
	if globalPreLaunch != "" {
		script = globalPreLaunch + " && " + script
	}
	// `$0` after the script is a dummy positional so `"$@"` inside the
	// script starts at the real argv, not at the script text itself.
	command := append([]string{shell, flag, script, "deck-agent"}, argv...)
	return command, nil
}

// applyInstrumentation appends adapter-owned argv and merges its environment
// last, so deck's hook routing facts cannot be replaced by user configuration.
func applyInstrumentation(adapter agent.Adapter, input agent.LaunchInput, argv []string, launchEnv map[string]string) ([]string, map[string]string, error) {
	instrumentArgv, instrumentEnv := adapter.Instrument(input)
	if len(instrumentArgv) == 0 && len(instrumentEnv) == 0 {
		return argv, launchEnv, nil
	}
	if !filepath.IsAbs(input.DeckExecutable) {
		return nil, nil, errors.New("deck executable for instrumentation must be absolute")
	}
	if input.DeckHome == "" {
		return nil, nil, errors.New("deck home for instrumentation is required")
	}
	argv = append(argv, instrumentArgv...)
	// Instrumentation is deck-owned and wins over config/session keys with
	// the same names, without mutating either persisted input map.
	for key, value := range instrumentEnv {
		launchEnv[key] = value
	}
	return argv, launchEnv, nil
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
