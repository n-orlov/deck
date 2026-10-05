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
	if err := s.checkResumeInputs(sessionID); err != nil {
		return store.Session{}, ResumeStartingElsewhere, err
	}
	session, adapter, early := s.resumeEligibility(ctx, sessionID)
	if early != nil {
		return early.unpack()
	}
	caps := adapter.Capabilities()

	conversationID, freshOnce, err := s.resumeConversationID(session, caps)
	if err != nil {
		return s.resumeFailed(ctx, session, err)
	}
	if err := checkResumeCWD(session); err != nil {
		return s.resumeFailed(ctx, session, err)
	}

	lease, early := s.acquireResumeLease(ctx, sessionID, session)
	if early != nil {
		return early.unpack()
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
	defer s.releaseResumeLease(ctx, session.ID, lease.HeldBy)
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
	paneCommand, launchEnv, err := s.buildResumeLaunch(session, adapter, caps, launchInput, freshOnce)
	if err != nil {
		return s.resumeFailed(ctx, session, err)
	}
	if err := s.startResumePane(ctx, session, paneCommand, launchEnv); err != nil {
		return s.resumeFailed(ctx, session, err)
	}
	// The fresh-once launch above has now actually happened: persist the
	// newly minted conversation id and revert resume_state to auto (never
	// back to pinned, and never left as fresh-once) so a later resume goes
	// back to normal auto behavior.
	if freshOnce {
		if err := s.consumeFreshOnce(ctx, session, conversationID); err != nil {
			return s.resumeFailed(ctx, session, err)
		}
		session.ConversationID = conversationID
		session.ResumeState = "auto"
	}
	session.StatusSource = "tmux"
	return session, ResumeStarted, nil
}

// resumeVerdict is an early, final answer of one of Resume's phases: exactly
// the (session, outcome, error) triple Resume returns.
type resumeVerdict struct {
	session store.Session
	outcome ResumeOutcome
	err     error
}

func (v *resumeVerdict) unpack() (store.Session, ResumeOutcome, error) {
	return v.session, v.outcome, v.err
}

// resumeFailed concludes a launch attempt that failed after the row was
// resolved: the row is written as a launch failure and the caller gets
// ResumeStarted with that failure.
func (s Service) resumeFailed(ctx context.Context, session store.Session, cause error) (store.Session, ResumeOutcome, error) {
	session, failErr := s.launchFailed(ctx, session, cause)
	return session, ResumeStarted, failErr
}

func (s Service) checkResumeInputs(sessionID string) error {
	if s.Store == nil || s.Audit == nil || s.Clock == nil || s.Agents == nil {
		return errors.New("resume requires store, audit logger, clock, and adapter registry")
	}
	if sessionID == "" {
		return errors.New("session id is required")
	}
	return nil
}

// resumeEligibility loads the row and answers the questions that need neither
// the launch lease nor any change to tmux: archived, already running, and
// which adapter the row's agent names. A non-nil verdict is Resume's answer.
func (s Service) resumeEligibility(ctx context.Context, sessionID string) (store.Session, agent.Adapter, *resumeVerdict) {
	session, err := s.Store.GetSession(ctx, sessionID)
	if err != nil {
		return store.Session{}, nil, &resumeVerdict{store.Session{}, ResumeStartingElsewhere, fmt.Errorf("get session %q: %w", sessionID, err)}
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
		return session, nil, &resumeVerdict{session, ResumeNotLeasable, fmt.Errorf("resume session %q: it is archived (press U to unarchive it first)", session.Name)}
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
	// adopting; it is collected in startResumePane so the launch can proceed.
	live, liveErr := s.TMux.HasLivePane(ctx, session.Slug)
	if liveErr != nil {
		return session, nil, &resumeVerdict{session, ResumeStartingElsewhere, fmt.Errorf("check for an already-running tmux session for %q: %w", session.Name, liveErr)}
	}
	if live {
		return session, nil, &resumeVerdict{session, ResumeAlreadyRunning, nil}
	}

	adapter, ok := s.Agents.Lookup(session.Agent)
	if !ok {
		return session, nil, &resumeVerdict{session, ResumeStartingElsewhere, fmt.Errorf("unknown agent kind %q", session.Agent)}
	}
	return session, adapter, nil
}

// resumeConversationID picks the conversation id this launch uses, and
// whether it is a fresh-once launch.
//
// SPEC §8/§9.3 resume_state (task 021): "pinned" resumes a specific
// conversation id (resume_pin) rather than whatever the session's own
// conversation id happens to be; "fresh-once" starts a brand-new
// conversation exactly once and then reverts to "auto" once that fresh launch
// has actually happened. A fresh-once launch mints its own conversation id,
// so it can never be missing one; any other launch of an adapter that assigns
// ids with none on the row is the SPEC-named unknown/rejected conversation id
// failure, rejected before ever touching the launch lease or tmux.
func (s Service) resumeConversationID(session store.Session, caps agent.Caps) (conversationID string, freshOnce bool, err error) {
	resumeState := session.ResumeState
	if resumeState == "" {
		resumeState = "auto"
	}
	freshOnce = resumeState == "fresh-once"
	conversationID = session.ConversationID
	if resumeState == "pinned" && session.ResumePin != "" {
		conversationID = session.ResumePin
	}
	if freshOnce && caps.AssignsConversationID {
		freshID, idErr := s.IDs.UUID()
		if idErr != nil {
			return "", freshOnce, fmt.Errorf("assign fresh conversation id for session %q: %w", session.Name, idErr)
		}
		conversationID = freshID
	}
	if !freshOnce && caps.AssignsConversationID && conversationID == "" {
		return "", freshOnce, fmt.Errorf("resume session %q: no conversation id is assigned to resume (unknown/rejected conversation id)", session.Name)
	}
	return conversationID, freshOnce, nil
}

// checkResumeCWD rejects a missing or non-directory cwd before the launch
// lease or tmux is touched.
//
// The returned error is a plain message: the stat failure is rendered into
// its text (%v) and deliberately not wrapped, so errors.Is/As on Resume's
// error never sees the underlying *fs.PathError.
func checkResumeCWD(session store.Session) error {
	info, statErr := os.Stat(session.CWD)
	if statErr == nil && info.IsDir() {
		return nil
	}
	reason := fmt.Sprintf("resume session %q: cwd %q is missing or not a directory", session.Name, session.CWD)
	if statErr != nil {
		reason = fmt.Sprintf("resume session %q: cwd %q is missing or not a directory: %v", session.Name, session.CWD, statErr)
	}
	return errors.New(reason)
}

// acquireResumeLease takes the launch lease. A non-nil verdict is Resume's
// answer: a lost race, a non-leasable row, or a store failure.
func (s Service) acquireResumeLease(ctx context.Context, sessionID string, session store.Session) (store.LaunchLeaseResult, *resumeVerdict) {
	lease, err := s.Store.AcquireLaunchLease(ctx, sessionID, store.CurrentLaunchLeaseOwner(), store.DefaultLaunchLeaseTTL, s.Clock.Now().UnixMilli())
	if err != nil {
		return lease, &resumeVerdict{session, ResumeStartingElsewhere, fmt.Errorf("acquire launch lease for session %q: %w", session.Name, err)}
	}
	switch lease.Outcome {
	case store.LaunchLeaseAcquired:
		return lease, nil
	case store.LaunchLeaseNotLeasable:
		// The list may have shown a stale stopped row. Return the durable row
		// instead of misreporting its real verdict as a launch happening in
		// another client.
		current, getErr := s.Store.GetSession(ctx, sessionID)
		if getErr != nil {
			return lease, &resumeVerdict{session, ResumeNotLeasable, fmt.Errorf("refresh non-leasable session %q: %w", session.Name, getErr)}
		}
		return lease, &resumeVerdict{current, ResumeNotLeasable, nil}
	default:
		// Another live, in-TTL owner holds the lease: no tmux session is
		// created for this loser.
		return lease, &resumeVerdict{session, ResumeStartingElsewhere, nil}
	}
}

// releaseResumeLease ends the hold taken by acquireResumeLease.
func (s Service) releaseResumeLease(ctx context.Context, sessionID, heldBy string) {
	if _, releaseErr := s.leaseReleaser().ReleaseLaunchLease(ctx, sessionID, heldBy); releaseErr != nil {
		// A lease that cannot be released is not a failed launch and must
		// not change the verdict the user is given for one. The §9.3 TTL is
		// still the backstop, so the row becomes leasable again within the
		// window exactly as it did before R75; the audit log carries the
		// fact that the faster path did not run.
		_ = s.Audit.Transition(sessionID, "launch_lease.release_failed")
	}
}

// resumeArgv builds the argv of the resumed launch: the adapter's launch
// (fresh-once) or resume command, resolved through paneArgv.
func (s Service) resumeArgv(session store.Session, adapter agent.Adapter, caps agent.Caps, launchInput agent.LaunchInput, freshOnce bool) ([]string, error) {
	var argv []string
	var err error
	if freshOnce {
		argv, err = adapter.Launch(launchInput)
	} else {
		argv, err = adapter.Resume(agent.ResumeInput{
			CWD: session.CWD, ConversationID: launchInput.ConversationID, Profile: session.PermissionProfile, ExtraArgs: session.LaunchArgs,
		})
	}
	if err != nil {
		return nil, fmt.Errorf("build resume argv for session %q: %w", session.Name, err)
	}
	// An adapter that declares no executable (`shell`) names no argv[0] of
	// its own, so the launcher supplies the shell it resolves for the pane --
	// the same single resolution CreateShell used when this row was created
	// (paneArgv/resolveUserShell), which is why a resumed shell pane cannot
	// disagree with its own create about which shell it runs.
	argv, err = s.paneArgv(caps, argv)
	if err != nil {
		return nil, fmt.Errorf("resolve resume argv for session %q: %w", session.Name, err)
	}
	return argv, nil
}

// buildResumeLaunch resolves the pane command and environment of the resumed
// launch; every error is the launch-failure cause Resume records.
func (s Service) buildResumeLaunch(session store.Session, adapter agent.Adapter, caps agent.Caps, launchInput agent.LaunchInput, freshOnce bool) ([]string, map[string]string, error) {
	argv, err := s.resumeArgv(session, adapter, caps, launchInput, freshOnce)
	if err != nil {
		return nil, nil, err
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
		return nil, nil, fmt.Errorf("instrument resumed session %q: %w", session.Name, err)
	}
	// SPEC section 6.1 (R104): deck's own session context is merged last,
	// above the instrumentation adapters own; the launch kind is "resume"
	// for every relaunch, including a fresh-once one, and the conversation
	// id exported here is the one this very launch is using -- which may
	// not yet be the session row's own field when a fresh-once launch just
	// minted a new one (persisted only once this launch has actually
	// succeeded).
	contextSession := session
	contextSession.ConversationID = launchInput.ConversationID
	for key, value := range s.sessionContextEnv(contextSession, LaunchKindResume) {
		launchEnv[key] = value
	}
	paneCommand, err := buildPaneCommand(s.GlobalPreLaunch, session.PreLaunch, session.LoginShell, argv)
	if err != nil {
		return nil, nil, fmt.Errorf("build resume pane command for session %q: %w", session.Name, err)
	}
	// A login shell resolves its own PATH via its own profile/rc scripts
	// (that is the point of login_shell=1, SPEC §6.4), so deck cannot judge
	// PATH membership for it and must not fail resume on that basis.
	if err = checkResumeBinary(session, argv, launchEnv); err != nil {
		return nil, nil, err
	}
	return paneCommand, launchEnv, nil
}

// checkResumeBinary fails a resume whose agent binary is not on the launch
// PATH; a login shell is exempt, as it resolves its own PATH.
func checkResumeBinary(session store.Session, argv []string, launchEnv map[string]string) error {
	if session.LoginShell {
		return nil
	}
	if lookErr := lookPathIn(argv[0], launchEnv["PATH"]); lookErr != nil {
		return fmt.Errorf("resume session %q: agent binary %q not found on PATH: %w", session.Name, argv[0], lookErr)
	}
	return nil
}

// startResumePane creates the pane and records the launch as ready; every
// error is the launch-failure cause Resume records.
func (s Service) startResumePane(ctx context.Context, session store.Session, paneCommand []string, launchEnv map[string]string) error {
	if err := s.collectRetainedPane(ctx, session); err != nil {
		return err
	}
	if _, err := s.TMux.Create(ctx, tmux.Launch{Slug: session.Slug, CWD: session.CWD, Command: paneCommand, Env: launchEnv}); err != nil {
		return fmt.Errorf("resume session %q: %w", session.Name, err)
	}
	if err := s.Audit.Launch(session.ID, paneCommand, launchEnv); err != nil {
		_ = s.TMux.Kill(ctx, session.Slug)
		return fmt.Errorf("audit resume launch %q: %w", session.Name, err)
	}
	return s.recordResumeReady(ctx, session)
}

// collectRetainedPane removes a retained dead pane still holding the
// session's name before the resume launches.
func (s Service) collectRetainedPane(ctx context.Context, session store.Session) error {
	// This call holds the launch lease and the eligibility check proved the
	// session has no live pane, so anything still on the socket under this
	// name is a retained corpse (`remain-on-exit failed`) holding the name
	// against `new-session` — tmux would refuse the launch as `duplicate
	// session: deck_<name>`, the very report requirement 46 exists to
	// prevent. Collect it here rather than waiting for a reconcile pass that
	// resume cannot order: SPEC.md:547 is "a dead pane is collected on sight,
	// never retained", and the crash verdict a pass already stored is
	// untouched by killing what it describes.
	if retained, existsErr := s.TMux.Exists(ctx, session.Slug); existsErr != nil {
		return fmt.Errorf("check for a retained dead pane for session %q: %w", session.Name, existsErr)
	} else if retained {
		if killErr := s.TMux.Kill(ctx, session.Slug); killErr != nil {
			return fmt.Errorf("collect the retained dead pane of session %q before resuming it: %w", session.Name, killErr)
		}
	}
	return nil
}

// recordResumeReady records the resumed launch as starting and audits it.
func (s Service) recordResumeReady(ctx context.Context, session store.Session) error {
	if err := s.Store.UpdateSessionStatus(ctx, store.StatusUpdateInput{
		SessionID: session.ID, Status: "starting", Reason: "", Source: "tmux",
		At: s.Clock.Now().UnixMilli(), EventKind: "launch.ready",
	}); err != nil {
		return fmt.Errorf("record ready resumed session %q: %w", session.Name, err)
	}
	if err := s.Audit.Transition(session.ID, "launch.ready"); err != nil {
		return fmt.Errorf("audit ready resumed session %q: %w", session.Name, err)
	}
	return nil
}

// consumeFreshOnce persists the conversation id a fresh-once launch minted and
// reverts the row's resume_state to auto.
func (s Service) consumeFreshOnce(ctx context.Context, session store.Session, conversationID string) error {
	if err := s.Store.SetConversationID(ctx, session.ID, conversationID, "resume-fresh-once", s.Clock.Now().UnixMilli()); err != nil {
		return fmt.Errorf("persist fresh conversation id for session %q: %w", session.Name, err)
	}
	if err := s.Store.ConsumeFreshOnce(ctx, session.ID, "resume-fresh-once", s.Clock.Now().UnixMilli()); err != nil {
		return fmt.Errorf("revert fresh-once resume state for session %q: %w", session.Name, err)
	}
	return nil
}
