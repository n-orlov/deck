package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

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
	// PostDestroy, when set, is this session's own teardown hook (SPEC
	// §9.2, R107): run as its own deck subprocess (never a pane) after
	// this row's Archive or Delete has durably succeeded, before the
	// global post_destroy hook. Stored verbatim on the row; task 013 is
	// what actually runs it.
	PostDestroy string
	// GroupID is SPEC §11's manual group this session is created into
	// (R130), verbatim onto store.CreateSessionInput.GroupID -- nil is the
	// structural default group, exactly as it is there. The create modal's
	// Group field (internal/tui) is the only populated caller today.
	GroupID *int64
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
	plan, err := s.planAgentCreate(input)
	if err != nil {
		return store.Session{}, err
	}
	session, err := s.insertAgentRow(ctx, input, plan)
	if err != nil {
		return session, err
	}
	return s.launchAgent(ctx, input, plan, session)
}

// agentCreatePlan is everything CreateAgent resolves before any row exists:
// the adapter, its resolved permission profile, the two generated ids and the
// captured PATH.
type agentCreatePlan struct {
	adapter           agent.Adapter
	caps              agent.Caps
	profile           string
	degradationReason string
	id                string
	conversationID    string
	capturedPath      string
}

// planAgentCreate validates the input and resolves the adapter, profile, ids
// and captured PATH, then probes the adapter's executable (R111), all before
// any row or pane exists.
func (s Service) planAgentCreate(input AgentCreateInput) (agentCreatePlan, error) {
	if err := s.checkAgentCreate(input); err != nil {
		return agentCreatePlan{}, err
	}
	adapter, err := s.Agents.Require(input.Agent)
	if err != nil {
		return agentCreatePlan{}, err
	}
	caps := adapter.Capabilities()
	profile, _, degradationReason := caps.ResolveProfile(adapter.Kind(), input.PermissionProfile)
	plan := agentCreatePlan{adapter: adapter, caps: caps, profile: profile, degradationReason: degradationReason}

	if err := s.assignAgentIDs(&plan); err != nil {
		return agentCreatePlan{}, err
	}
	plan.capturedPath = os.Getenv("PATH")
	if plan.capturedPath == "" {
		return agentCreatePlan{}, errors.New("PATH is required to create an agent session")
	}
	if err := s.probeAgentExecutable(input, plan); err != nil {
		return agentCreatePlan{}, err
	}
	return plan, nil
}

// checkAgentCreate rejects a create whose collaborators or required input
// fields are missing, before the adapter is even looked up.
func (s Service) checkAgentCreate(input AgentCreateInput) error {
	if s.Store == nil || s.Audit == nil || s.Clock == nil || s.IDs == nil || s.Agents == nil {
		return errors.New("agent creation requires store, audit logger, clock, id generator, and adapter registry")
	}
	if input.Name == "" || input.CWD == "" {
		return errors.New("agent session name and working directory are required")
	}
	return nil
}

// assignAgentIDs draws the session id and, for an adapter that assigns its
// own conversation id up front, the conversation id.
func (s Service) assignAgentIDs(plan *agentCreatePlan) error {
	id, err := s.IDs.UUID()
	if err != nil {
		return fmt.Errorf("generate agent session id: %w", err)
	}
	plan.id = id
	if plan.caps.AssignsConversationID {
		plan.conversationID, err = s.IDs.UUID()
		if err != nil {
			return fmt.Errorf("assign conversation id: %w", err)
		}
	}
	return nil
}

// probeAgentExecutable is R111: probe the adapter's declared executable
// against the PATH this session's own pane would launch under, before any row
// or pane exists, so a doomed create never gets as far as a durable "starting"
// row (task 010). This is the same lookPathIn (availability.go) that
// resume's own preflight (resume.go) and AvailableKinds use, against
// the same resolveLaunchEnv PATH resume's preflight checks against, so
// create and resume can never disagree about whether a binary is on
// PATH. An adapter with no declared executable (shell) has nothing to
// probe, same as kindAvailable.
//
// A login shell resolves its own PATH via its own profile/rc scripts
// (SPEC §6.4), so deck cannot judge PATH membership for it and must
// not fail create on that basis -- the same exemption resume.go's own
// preflight makes for a resumed login shell.
func (s Service) probeAgentExecutable(input AgentCreateInput, plan agentCreatePlan) error {
	if input.LoginShell || plan.caps.Executable == "" {
		return nil
	}
	launchPath := s.resolveLaunchEnv(plan.capturedPath, input.Env)["PATH"]
	if lookErr := lookPathIn(plan.caps.Executable, launchPath); lookErr != nil {
		return fmt.Errorf("create agent session %q: agent binary %q not found on PATH: %w", input.Name, plan.caps.Executable, lookErr)
	}
	return nil
}

// insertAgentRow creates the durable "starting" row and its follow-ups. On a
// failure after the row exists it returns the row alongside the error, as
// CreateAgent always has.
func (s Service) insertAgentRow(ctx context.Context, input AgentCreateInput, plan agentCreatePlan) (store.Session, error) {
	now := s.Clock.Now().UnixMilli()
	// SPEC §9.2 (R77), as in CreateShell: note which tombstoned rows hold
	// this name before the create reaps them in its own transaction, and
	// clean up their files only once that transaction has committed.
	reapedHolders, err := s.tombstonedNameHolders(ctx, input.Name)
	if err != nil {
		return store.Session{}, err
	}
	session, err := s.Store.CreateSession(ctx, store.CreateSessionInput{
		ID: plan.id, Name: input.Name, CWD: input.CWD, Agent: plan.adapter.Kind(), CapturedPath: plan.capturedPath,
		Status: "starting", StatusSource: "user", StatusAt: now, CreatedAt: now,
		LaunchArgs: input.LaunchArgs, Env: input.Env, PreLaunch: input.PreLaunch, LoginShell: input.LoginShell,
		PostDestroy:       input.PostDestroy,
		PermissionProfile: plan.profile, PermissionProfileReason: plan.degradationReason, ConversationID: plan.conversationID,
		GroupID: input.GroupID,
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
	return session, nil
}

// launchAgent builds the pane command and environment for an existing
// "starting" row, launches it in tmux and records the ready transition. Every
// failure leaves a durable error row through launchFailed.
func (s Service) launchAgent(ctx context.Context, input AgentCreateInput, plan agentCreatePlan, session store.Session) (store.Session, error) {
	paneCommand, launchEnv, err := s.buildAgentLaunch(ctx, input, plan, session)
	if err != nil {
		return s.launchFailed(ctx, session, err)
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
	if err := s.recordHookExecutable(ctx, &session, plan.adapter, s.hookProbeInput(session)); err != nil {
		return s.launchFailed(ctx, session, err)
	}
	if err := s.recordAgentReady(ctx, session); err != nil {
		return s.launchFailed(ctx, session, err)
	}
	session.StatusSource = "tmux"
	session.ConversationID = plan.conversationID
	session.PermissionProfile = plan.profile
	session.PermissionProfileReason = plan.degradationReason
	return session, nil
}

// recordAgentReady stores and audits the launch.ready transition.
func (s Service) recordAgentReady(ctx context.Context, session store.Session) error {
	if err := s.Store.UpdateSessionStatus(ctx, store.StatusUpdateInput{
		SessionID: session.ID, Status: "starting", Reason: "", Source: "tmux",
		At: s.Clock.Now().UnixMilli(), EventKind: "launch.ready",
	}); err != nil {
		return fmt.Errorf("record ready agent session %q: %w", session.Name, err)
	}
	if err := s.Audit.Transition(session.ID, "launch.ready"); err != nil {
		return fmt.Errorf("audit ready agent session %q: %w", session.Name, err)
	}
	return nil
}

// buildAgentLaunch resolves the pane command and environment a created row
// launches with. Its errors are already the final, wrapped launch-failure
// messages.
func (s Service) buildAgentLaunch(ctx context.Context, input AgentCreateInput, plan agentCreatePlan, session store.Session) ([]string, map[string]string, error) {
	// No LaunchGeneration here: a brand-new row's first launch takes no launch
	// lease (the row is created directly as `starting`), so there is no earlier
	// launch of it that a hook could be confused with (issue #11, R74). The
	// row's generation is first written by the AcquireLaunchLease of its first
	// resume/restart.
	launchInput := agent.LaunchInput{
		CWD: session.CWD, ConversationID: plan.conversationID, Profile: plan.profile, ExtraArgs: input.LaunchArgs,
		DeckExecutable: s.DeckExecutable, DeckSessionID: session.ID, DeckHome: s.DeckHome,
	}
	argv, err := plan.adapter.Launch(launchInput)
	if err != nil {
		return nil, nil, fmt.Errorf("build launch argv for agent session %q: %w", session.Name, err)
	}
	// An adapter that declares no executable (`shell`) names no argv[0] of its
	// own, so the launcher supplies the shell it resolves for the pane -- the
	// same single resolution CreateShell uses (paneArgv/resolveUserShell).
	argv, err = s.paneArgv(plan.caps, argv)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve launch argv for agent session %q: %w", session.Name, err)
	}

	// login_shell=1 is mutually exclusive with relying on captured_path: the
	// login shell resolves its own PATH via its own profile/rc scripts, so
	// deck must not also inject a PATH override here.
	envCapturedPath := plan.capturedPath
	if input.LoginShell {
		envCapturedPath = ""
	}
	launchEnv := s.resolveLaunchEnv(envCapturedPath, input.Env)
	argv, launchEnv, note, err := applyInstrumentationNoted(plan.adapter, launchInput, argv, launchEnv)
	if err != nil {
		return nil, nil, fmt.Errorf("instrument agent session %q: %w", session.Name, err)
	}
	s.recordInstrumentNote(ctx, session.ID, note)
	// SPEC §6.1 (R104): deck's own session context is merged last, above the
	// instrumentation adapters own, so a session `env` or config `[env]` key
	// of the same name can never lie to a hook about which session it is.
	for key, value := range s.sessionContextEnv(session, LaunchKindCreate) {
		launchEnv[key] = value
	}
	paneCommand, err := buildPaneCommand(s.GlobalPreLaunch, input.PreLaunch, input.LoginShell, argv)
	if err != nil {
		return nil, nil, fmt.Errorf("build pane command for agent session %q: %w", session.Name, err)
	}
	return paneCommand, launchEnv, nil
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
	shell, flag := paneShell(loginShell)
	script := preLaunchScript(globalPreLaunch, preLaunch)
	// `$0` after the script is a dummy positional so `"$@"` inside the
	// script starts at the real argv, not at the script text itself.
	command := append([]string{shell, flag, script, "deck-agent"}, argv...)
	return command, nil
}

// paneShell is the shell and flag that run a wrapped pane command: /bin/sh -c,
// or, for a login shell, $SHELL -lc (still /bin/sh when SHELL is unset).
func paneShell(loginShell bool) (shell, flag string) {
	if !loginShell {
		return "/bin/sh", "-c"
	}
	if s := os.Getenv("SHELL"); s != "" {
		return s, "-lc"
	}
	return "/bin/sh", "-lc"
}

// preLaunchScript joins the global and per-session pre-launch hooks, global
// first, in front of the `exec "$@"` that replaces the shell with the agent.
func preLaunchScript(globalPreLaunch, preLaunch string) string {
	script := "exec \"$@\""
	if preLaunch != "" {
		script = preLaunch + " && " + script
	}
	if globalPreLaunch != "" {
		script = globalPreLaunch + " && " + script
	}
	return script
}

// applyInstrumentation appends adapter-owned argv and merges its environment
// last, so deck's hook routing facts cannot be replaced by user configuration.
func applyInstrumentation(adapter agent.Adapter, input agent.LaunchInput, argv []string, launchEnv map[string]string) ([]string, map[string]string, error) {
	argv, launchEnv, _, err := applyInstrumentationNoted(adapter, input, argv, launchEnv)
	return argv, launchEnv, err
}

// applyInstrumentationNoted is applyInstrumentation that also reports the
// non-fatal note of an optional instrumentation file it could not install
// (empty when everything installed): the launch goes ahead without that
// file's argv, and the caller records the note against the session.
func applyInstrumentationNoted(adapter agent.Adapter, input agent.LaunchInput, argv []string, launchEnv map[string]string) ([]string, map[string]string, string, error) {
	instrumentArgv, instrumentEnv := adapter.Instrument(input)
	if len(instrumentArgv) == 0 && len(instrumentEnv) == 0 {
		return argv, launchEnv, "", nil
	}
	if !filepath.IsAbs(input.DeckExecutable) {
		return nil, nil, "", errors.New("deck executable for instrumentation must be absolute")
	}
	if input.DeckHome == "" {
		return nil, nil, "", errors.New("deck home for instrumentation is required")
	}
	instrumentArgv, note, err := installInstrumentFiles(adapter, input, instrumentArgv)
	if err != nil {
		return nil, nil, "", err
	}
	argv = append(argv, instrumentArgv...)
	// Instrumentation is deck-owned and wins over config/session keys with
	// the same names, without mutating either persisted input map.
	for key, value := range instrumentEnv {
		launchEnv[key] = value
	}
	return argv, launchEnv, note, nil
}

// recordInstrumentNote stores a degraded-instrumentation note on the session,
// best effort: a note that cannot be recorded never fails the launch.
func (s Service) recordInstrumentNote(ctx context.Context, sessionID, note string) {
	if note == "" || s.Store == nil || s.Clock == nil {
		return
	}
	_ = s.Store.RecordSessionNote(ctx, sessionID, note, s.Clock.Now().UnixMilli())
}

// installInstrumentFiles puts the deck-owned files an adapter's instrumentation
// names on disk before the agent starts (Pi's extension, Copilot's plugin).
// A file is only rewritten when its bytes differ, through a temporary file
// and a rename, so a concurrent launch or a running agent never reads a
// half-written file. A failure on a required file fails the launch; a failure
// on an Optional one removes that file's DropArgv from the returned argv and
// is reported as a note instead.
func installInstrumentFiles(adapter agent.Adapter, input agent.LaunchInput, instrumentArgv []string) ([]string, string, error) {
	provider, ok := adapter.(agent.FileInstrumenter)
	if !ok {
		return instrumentArgv, "", nil
	}
	var note string
	for _, file := range provider.InstrumentFiles(input) {
		err := writeFileAtomic(file.Path, file.Content, file.DirMode)
		if err == nil {
			continue
		}
		if !file.Optional {
			return nil, "", fmt.Errorf("install %s instrumentation: %w", adapter.Kind(), err)
		}
		instrumentArgv = dropArgvRun(instrumentArgv, file.DropArgv)
		note = fmt.Sprintf("%s hook instrumentation unavailable, launched without it: %v", adapter.Kind(), err)
	}
	return instrumentArgv, note, nil
}

// writeInstrumentFiles is installInstrumentFiles for a caller that only needs
// to know whether a required file could not be installed.
func writeInstrumentFiles(adapter agent.Adapter, input agent.LaunchInput) error {
	_, _, err := installInstrumentFiles(adapter, input, nil)
	return err
}

// dropArgvRun returns argv without the first contiguous run equal to drop.
func dropArgvRun(argv, drop []string) []string {
	for i := 0; i+len(drop) <= len(argv) && len(drop) > 0; i++ {
		if slices.Equal(argv[i:i+len(drop)], drop) {
			return append(slices.Clone(argv[:i]), argv[i+len(drop):]...)
		}
	}
	return argv
}

// writeFileAtomic writes content to path through a sibling temporary file and
// a rename, creating the directory (mode dirMode, 0750 when zero) when needed.
// A file that already holds exactly content is left untouched, and a created
// or existing directory is held to dirMode when one is requested.
func writeFileAtomic(path string, content []byte, dirMode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := ensureInstrumentDir(dir, dirMode); err != nil {
		return err
	}
	existing, readErr := os.ReadFile(path) //nolint:gosec // G304: path is a deck-owned instrumentation file under the data root, never user input
	if readErr == nil && bytes.Equal(existing, content) {
		return nil
	}
	return replaceFile(dir, path, content)
}

// ensureInstrumentDir creates dir (mode dirMode, 0750 when zero) and, when a
// mode was requested, holds an already-existing directory to exactly it.
func ensureInstrumentDir(dir string, dirMode os.FileMode) error {
	mode := dirMode
	if mode == 0 {
		mode = 0o750
	}
	if err := os.MkdirAll(dir, mode); err != nil {
		return err
	}
	if dirMode == 0 {
		return nil
	}
	info, err := os.Stat(dir)
	if err != nil {
		return err
	}
	if info.Mode().Perm() == dirMode {
		return nil
	}
	return os.Chmod(dir, dirMode)
}

// replaceFile writes content to a temporary file in dir and renames it over
// path, removing the temporary file when either step fails.
func replaceFile(dir, path string, content []byte) error {
	tmp, err := os.CreateTemp(dir, ".instrument-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	_, writeErr := tmp.Write(content)
	closeErr := tmp.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(name) // best-effort cleanup of the scratch file
		return errors.Join(writeErr, closeErr)
	}
	if err := os.Rename(name, path); err != nil {
		_ = os.Remove(name) // best-effort cleanup of the scratch file
		return err
	}
	return nil
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

// recordHookExecutable persists the deck binary this launch's hook command is
// bound to (R204c): the session's agent keeps that path for as long as it
// runs, and the TUI compares it with its own to hint at a stale binding. It
// runs once the pane is up, so a launch that failed never overwrites the
// binding of the agent still running, and only for an adapter that
// instruments hooks at all (a shell or Pi launch binds nothing, so it has
// nothing to go stale). The in-memory row is updated too, so the caller
// returns the row the store now holds.
func (s Service) recordHookExecutable(ctx context.Context, session *store.Session, adapter agent.Adapter, launch agent.LaunchInput) error {
	if instrumentArgv, instrumentEnv := adapter.Instrument(launch); len(instrumentArgv) == 0 && len(instrumentEnv) == 0 {
		return nil
	}
	if err := s.Store.SetHookExecutable(ctx, session.ID, s.DeckExecutable); err != nil {
		return err
	}
	session.HookExecutable = s.DeckExecutable
	return nil
}

// hookProbeInput is the launch input launchAgent hands recordHookExecutable:
// the deck-owned facts an adapter's Instrument reads, which are the same for
// every launch of the row (a created row has no lease generation yet).
func (s Service) hookProbeInput(session store.Session) agent.LaunchInput {
	return agent.LaunchInput{CWD: session.CWD, DeckExecutable: s.DeckExecutable, DeckSessionID: session.ID, DeckHome: s.DeckHome}
}
