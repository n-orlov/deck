package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/n-orlov/deck/internal/agent"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// LaunchLeaseReleaser is the narrow store seam Resume needs to release a
// launch lease once its own attempt concludes (R75, issue #11): only the one
// UPDATE, none of GetSession, AcquireLaunchLease or any of the other store
// operations Resume and the rest of Service depend on. *store.Store already
// satisfies it as it stands, so every existing caller (cmd/deck, the
// features harness, every internal/service test) keeps building a Service
// with a concrete *store.Store Store field and compiles unchanged; a test
// substitutes a different implementation via Service.LeaseReleaser to make
// the release call itself fail, without a test-only branch, an env knob, or
// any other change to product code.
type LaunchLeaseReleaser interface {
	ReleaseLaunchLease(ctx context.Context, sessionID, heldOwner string) (bool, error)
}

// leaseReleaser resolves the seam Resume's deferred release uses: the
// caller-supplied override if one was given, otherwise Store itself, which
// already satisfies LaunchLeaseReleaser.
func (s Service) leaseReleaser() LaunchLeaseReleaser {
	if s.LeaseReleaser != nil {
		return s.LeaseReleaser
	}
	return s.Store
}

// ResumeOutcome distinguishes a resume that launched a pane, a genuine
// launch-lease loser, and a row that became non-leasable before the attempt.
type ResumeOutcome int

const (
	// ResumeStarted means this call acquired the launch lease, relaunched
	// the pane with the adapter's resume argv, and left the row starting.
	ResumeStarted ResumeOutcome = iota
	// ResumeStartingElsewhere means another (live, in-TTL) owner already
	// holds the launch lease for this session; this call created no tmux
	// session and left the row untouched.
	ResumeStartingElsewhere
	// ResumeNotLeasable means the durable row is not startable: it is no
	// longer stopped, or it is archived (SPEC.md:718, issue #8). The
	// returned session contains its current status and reason for display,
	// and for the archived case the accompanying error names `U`.
	ResumeNotLeasable
	// ResumeAlreadyRunning means a tmux session for this row already exists
	// on deck's private server (requirement 46): deck already owns that
	// pane, so this call adopted it as an honest no-op instead of attempting
	// `new-session` over it (which tmux would refuse as "duplicate session:
	// deck_<name>", a deck bug that must never reach the user disguised as
	// an agent failure). No launch lease was taken, no tmux command was run,
	// and the durable row is untouched — the returned session is whatever
	// GetSession found.
	ResumeAlreadyRunning
)

// Resume relaunches an existing session's conversation (SPEC §8/§9.3): it
// checks that no tmux session for this row already exists (requirement 46,
// see ResumeAlreadyRunning), acquires the launch lease, recreates
// deck_<slug> at the session's cwd, runs pre_launch, and launches the
// adapter's resume argv with the session's persisted env and permission
// profile. It never re-sends a prompt or previous message — the resume argv
// only ever carries the conversation id, profile and the session's own
// launch_args. A caller that loses the lease race gets
// ResumeStartingElsewhere and no tmux session is created for it. An
// archived row is refused up front (SPEC.md:718, #8) with a message naming
// `U`, before the lease and before tmux is touched at all. The launch lease is
// held only while this launch is in flight: when the attempt concludes — pane
// up or launch failed — it is released (issue #11, R75), so the row's own last
// launcher is never reported to the next resume as another client.
func (s Service) Resume(ctx context.Context, sessionID string) (store.Session, ResumeOutcome, error) {
	if s.Store == nil || s.Audit == nil || s.Clock == nil || s.Agents == nil {
		return store.Session{}, ResumeStartingElsewhere, errors.New("resume requires store, audit logger, clock, and adapter registry")
	}
	if sessionID == "" {
		return store.Session{}, ResumeStartingElsewhere, errors.New("session id is required")
	}
	session, err := s.Store.GetSession(ctx, sessionID)
	if err != nil {
		return store.Session{}, ResumeStartingElsewhere, fmt.Errorf("get session %q: %w", sessionID, err)
	}

	// SPEC.md:718 (#8): an archived session is not startable, and `R` routes
	// through Resume, so this single guard covers restart too. It is checked
	// FIRST -- before the launch lease (which would flip the row to
	// `starting`), before any tmux command, and before every other rejection
	// -- so an archived row never even briefly reads `starting` and nothing
	// at all is created: no tmux session, no pane, no audit launch record.
	// The row is retained untouched (archived_at included) and the message
	// names `U` as the way forward, because the archived -> live half of
	// §4's invariant is the whole point: without this, `stopped + archived`
	// + `r` yields a live agent hidden behind the archived filter, with its
	// hooks unroutable and its status frozen.
	if session.ArchivedAt != 0 {
		return session, ResumeNotLeasable, fmt.Errorf("resume session %q: it is archived (press U to unarchive it first)", session.Name)
	}

	// Requirement 46: check for an already-running tmux session BEFORE the
	// launch lease is even taken (§9.3's lease is about *concurrent*
	// launchers racing to start a new pane; this is the *already-launched*
	// case, which is not a race at all). Reported as an honest no-op, never
	// as `duplicate session: deck_<name>` reaching the user disguised as an
	// agent failure, and never written to the row as an error status.
	//
	// "Already running" means the session still HAS A LIVE PANE, which is why
	// this consults HasLivePane and not Exists (#6, leg 2). Under deck's
	// server-wide `remain-on-exit failed`, a non-zero exit retains the dead
	// pane and its session, so `has-session` succeeded on a corpse and every
	// `r` adopted a pane with nothing left in it — a silent no-op with no way
	// out of the UI. A corpse is not the pane requirement 46 is about
	// adopting; it is collected below so the launch can proceed.
	if live, liveErr := s.TMux.HasLivePane(ctx, session.Slug); liveErr != nil {
		return session, ResumeStartingElsewhere, fmt.Errorf("check for an already-running tmux session for %q: %w", session.Name, liveErr)
	} else if live {
		return session, ResumeAlreadyRunning, nil
	}

	adapter, ok := s.Agents.Lookup(session.Agent)
	if !ok {
		return session, ResumeStartingElsewhere, fmt.Errorf("unknown agent kind %q", session.Agent)
	}
	caps := adapter.Capabilities()

	// SPEC §8/§9.3 resume_state (task 021): "pinned" resumes a specific
	// conversation id (resume_pin) rather than whatever the session's own
	// conversation id happens to be; "fresh-once" starts a brand-new
	// conversation exactly once and then reverts to "auto" below, once
	// that fresh launch has actually happened.
	resumeState := session.ResumeState
	if resumeState == "" {
		resumeState = "auto"
	}
	freshOnce := resumeState == "fresh-once"
	conversationID := session.ConversationID
	if resumeState == "pinned" && session.ResumePin != "" {
		conversationID = session.ResumePin
	}
	if freshOnce && caps.AssignsConversationID {
		freshID, err := s.IDs.UUID()
		if err != nil {
			session, failErr := s.launchFailed(ctx, session, fmt.Errorf("assign fresh conversation id for session %q: %w", session.Name, err))
			return session, ResumeStarted, failErr
		}
		conversationID = freshID
	}

	// Reject the three SPEC-named resume failure causes before ever
	// touching the launch lease or tmux, so none of them can be mistaken
	// for (or accidentally produce) a fresh-conversation launch: an
	// unknown/rejected conversation id, a missing or non-directory cwd,
	// and (below, once the resume argv and its env are known) the agent
	// binary not being on PATH. A fresh-once launch mints its own
	// conversation id above, so it can never be missing one.
	if !freshOnce && caps.AssignsConversationID && conversationID == "" {
		session, failErr := s.launchFailed(ctx, session, fmt.Errorf("resume session %q: no conversation id is assigned to resume (unknown/rejected conversation id)", session.Name))
		return session, ResumeStarted, failErr
	}
	if info, statErr := os.Stat(session.CWD); statErr != nil || !info.IsDir() {
		reason := fmt.Sprintf("resume session %q: cwd %q is missing or not a directory", session.Name, session.CWD)
		if statErr != nil {
			reason = fmt.Sprintf("resume session %q: cwd %q is missing or not a directory: %v", session.Name, session.CWD, statErr)
		}
		session, failErr := s.launchFailed(ctx, session, errors.New(reason))
		return session, ResumeStarted, failErr
	}

	lease, err := s.Store.AcquireLaunchLease(ctx, sessionID, store.CurrentLaunchLeaseOwner(), store.DefaultLaunchLeaseTTL, s.Clock.Now().UnixMilli())
	if err != nil {
		return session, ResumeStartingElsewhere, fmt.Errorf("acquire launch lease for session %q: %w", session.Name, err)
	}
	if lease.Outcome == store.LaunchLeaseNotLeasable {
		// The list may have shown a stale stopped row. Return the durable row
		// instead of misreporting its real verdict as a launch happening in
		// another client.
		current, getErr := s.Store.GetSession(ctx, sessionID)
		if getErr != nil {
			return session, ResumeNotLeasable, fmt.Errorf("refresh non-leasable session %q: %w", session.Name, getErr)
		}
		return current, ResumeNotLeasable, nil
	}
	if lease.Outcome != store.LaunchLeaseAcquired {
		// Another live, in-TTL owner holds the lease: no tmux session is
		// created for this loser.
		return session, ResumeStartingElsewhere, nil
	}
	// R75 (issue #11): this launch attempt now holds the lease, and it holds it
	// only for as long as it is IN FLIGHT. Every exit path below concludes the
	// attempt -- the pane is up, or the launch failed and the row is `error` --
	// so the hold ends here, by defer, on all of them. Left to expire on the
	// ~30 s TTL instead, the row's own last launcher kept answering *starting
	// elsewhere* to the next resume of a legitimately stopped row, which SPEC
	// §9.3 forbids: that message is a claim that another client is there.
	//
	// The release clears only launch_lease_until; launch_lease_owner (and with
	// it R74's generation) stays on the row, because the row must go on naming
	// which launch is current for as long as that pane can still send hooks.
	defer func() {
		if _, releaseErr := s.leaseReleaser().ReleaseLaunchLease(ctx, session.ID, lease.HeldBy); releaseErr != nil {
			// A lease that cannot be released is not a failed launch and must
			// not change the verdict the user is given for one. The §9.3 TTL is
			// still the backstop, so the row becomes leasable again within the
			// window exactly as it did before R75; the audit log carries the
			// fact that the faster path did not run.
			_ = s.Audit.Transition(session.ID, "launch_lease.release_failed")
		}
	}()
	session.Status = "starting"
	session.KilledByUser = false
	if err := s.Audit.Transition(session.ID, "starting"); err != nil {
		return session, ResumeStarted, fmt.Errorf("audit starting resumed session %q: %w", session.Name, err)
	}

	launchInput := agent.LaunchInput{
		CWD: session.CWD, ConversationID: conversationID, Profile: session.PermissionProfile, ExtraArgs: session.LaunchArgs,
		DeckExecutable: s.DeckExecutable, DeckSessionID: session.ID, DeckHome: s.DeckHome,
		// The lease this call just acquired names this launch (issue #11,
		// R74); the adapter exports it so the pane's hooks carry it.
		LaunchGeneration: lease.LaunchGeneration,
	}
	var argv []string
	if freshOnce {
		argv, err = adapter.Launch(launchInput)
	} else {
		argv, err = adapter.Resume(agent.ResumeInput{
			CWD: session.CWD, ConversationID: conversationID, Profile: session.PermissionProfile, ExtraArgs: session.LaunchArgs,
		})
	}
	if err != nil {
		session, failErr := s.launchFailed(ctx, session, fmt.Errorf("build resume argv for session %q: %w", session.Name, err))
		return session, ResumeStarted, failErr
	}
	// An adapter that declares no executable (`shell`) names no argv[0] of
	// its own, so the launcher supplies the shell it resolves for the pane --
	// the same single resolution CreateShell used when this row was created
	// (paneArgv/resolveUserShell), which is why a resumed shell pane cannot
	// disagree with its own create about which shell it runs.
	argv, err = s.paneArgv(caps, argv)
	if err != nil {
		session, failErr := s.launchFailed(ctx, session, fmt.Errorf("resolve resume argv for session %q: %w", session.Name, err))
		return session, ResumeStarted, failErr
	}

	// login_shell=1 is mutually exclusive with relying on captured_path, as
	// at create time (SPEC §6.4).
	envCapturedPath := session.CapturedPath
	if session.LoginShell {
		envCapturedPath = ""
	}
	launchEnv := s.resolveLaunchEnv(envCapturedPath, session.Env)
	argv, launchEnv, err = applyInstrumentation(adapter, launchInput, argv, launchEnv)
	if err != nil {
		session, failErr := s.launchFailed(ctx, session, fmt.Errorf("instrument resumed session %q: %w", session.Name, err))
		return session, ResumeStarted, failErr
	}
	// SPEC section 6.1 (R104): deck's own session context is merged last,
	// above the instrumentation adapters own; the launch kind is "resume"
	// for every relaunch, including a fresh-once one, and the conversation
	// id exported here is the one this very launch is using -- which may
	// not yet be the session row's own field when a fresh-once launch just
	// minted a new one (persisted only once this launch has actually
	// succeeded, below).
	contextSession := session
	contextSession.ConversationID = conversationID
	for key, value := range s.sessionContextEnv(contextSession, LaunchKindResume) {
		launchEnv[key] = value
	}
	paneCommand, err := buildPaneCommand(s.GlobalPreLaunch, session.PreLaunch, session.LoginShell, argv)
	if err != nil {
		session, failErr := s.launchFailed(ctx, session, fmt.Errorf("build resume pane command for session %q: %w", session.Name, err))
		return session, ResumeStarted, failErr
	}
	// A login shell resolves its own PATH via its own profile/rc scripts
	// (that is the point of login_shell=1, SPEC §6.4), so deck cannot judge
	// PATH membership for it and must not fail resume on that basis.
	if !session.LoginShell {
		if lookErr := lookPathIn(argv[0], launchEnv["PATH"]); lookErr != nil {
			session, failErr := s.launchFailed(ctx, session, fmt.Errorf("resume session %q: agent binary %q not found on PATH: %w", session.Name, argv[0], lookErr))
			return session, ResumeStarted, failErr
		}
	}
	// This call holds the launch lease and the check above proved the session
	// has no live pane, so anything still on the socket under this name is a
	// retained corpse (`remain-on-exit failed`) holding the name against
	// `new-session` — tmux would refuse the launch as `duplicate session:
	// deck_<name>`, the very report requirement 46 exists to prevent. Collect
	// it here rather than waiting for a reconcile pass that resume cannot
	// order: SPEC.md:547 is "a dead pane is collected on sight, never
	// retained", and the crash verdict a pass already stored is untouched by
	// killing what it describes.
	if retained, existsErr := s.TMux.Exists(ctx, session.Slug); existsErr != nil {
		session, failErr := s.launchFailed(ctx, session, fmt.Errorf("check for a retained dead pane for session %q: %w", session.Name, existsErr))
		return session, ResumeStarted, failErr
	} else if retained {
		if killErr := s.TMux.Kill(ctx, session.Slug); killErr != nil {
			session, failErr := s.launchFailed(ctx, session, fmt.Errorf("collect the retained dead pane of session %q before resuming it: %w", session.Name, killErr))
			return session, ResumeStarted, failErr
		}
	}
	if _, err := s.TMux.Create(ctx, tmux.Launch{Slug: session.Slug, CWD: session.CWD, Command: paneCommand, Env: launchEnv}); err != nil {
		session, failErr := s.launchFailed(ctx, session, fmt.Errorf("resume session %q: %w", session.Name, err))
		return session, ResumeStarted, failErr
	}
	if err := s.Audit.Launch(session.ID, paneCommand, launchEnv); err != nil {
		_ = s.TMux.Kill(ctx, session.Slug)
		session, failErr := s.launchFailed(ctx, session, fmt.Errorf("audit resume launch %q: %w", session.Name, err))
		return session, ResumeStarted, failErr
	}
	if err := s.Store.UpdateSessionStatus(ctx, store.StatusUpdateInput{
		SessionID: session.ID, Status: "starting", Reason: "", Source: "tmux",
		At: s.Clock.Now().UnixMilli(), EventKind: "launch.ready",
	}); err != nil {
		session, failErr := s.launchFailed(ctx, session, fmt.Errorf("record ready resumed session %q: %w", session.Name, err))
		return session, ResumeStarted, failErr
	}
	if err := s.Audit.Transition(session.ID, "launch.ready"); err != nil {
		session, failErr := s.launchFailed(ctx, session, fmt.Errorf("audit ready resumed session %q: %w", session.Name, err))
		return session, ResumeStarted, failErr
	}
	// The fresh-once launch above has now actually happened: persist the
	// newly minted conversation id and revert resume_state to auto (never
	// back to pinned, and never left as fresh-once) so a later resume goes
	// back to normal auto behavior.
	if freshOnce {
		if err := s.Store.SetConversationID(ctx, session.ID, conversationID, "resume-fresh-once", s.Clock.Now().UnixMilli()); err != nil {
			session, failErr := s.launchFailed(ctx, session, fmt.Errorf("persist fresh conversation id for session %q: %w", session.Name, err))
			return session, ResumeStarted, failErr
		}
		if err := s.Store.ConsumeFreshOnce(ctx, session.ID, "resume-fresh-once", s.Clock.Now().UnixMilli()); err != nil {
			session, failErr := s.launchFailed(ctx, session, fmt.Errorf("revert fresh-once resume state for session %q: %w", session.Name, err))
			return session, ResumeStarted, failErr
		}
		session.ConversationID = conversationID
		session.ResumeState = "auto"
	}
	session.StatusSource = "tmux"
	return session, ResumeStarted, nil
}

// lookPathIn reports whether file (an adapter launch argv[0], e.g. "claude")
// is executable under pathEnv (a colon-separated PATH value, not the current
// process's own environment), mirroring exec.LookPath's search rules but
// against an arbitrary PATH string rather than os.Getenv("PATH"). A file
// containing a path separator is checked directly instead of searched.
func lookPathIn(file, pathEnv string) error {
	if file == "" {
		return errors.New("empty command")
	}
	if strings.ContainsRune(file, os.PathSeparator) || strings.Contains(file, "/") {
		info, err := os.Stat(file)
		if err != nil {
			return err
		}
		if info.IsDir() {
			return fmt.Errorf("%s is a directory", file)
		}
		return nil
	}
	for _, dir := range filepath.SplitList(pathEnv) {
		if dir == "" {
			dir = "."
		}
		candidate := filepath.Join(dir, file)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return nil
		}
	}
	return fmt.Errorf("%s: executable file not found in $PATH", file)
}
