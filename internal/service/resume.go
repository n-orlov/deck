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

// ResumeOutcome distinguishes a resume that actually launched a pane from
// one that lost the SPEC §9.3 launch-lease race to another deck client.
type ResumeOutcome int

const (
	// ResumeStarted means this call acquired the launch lease, relaunched
	// the pane with the adapter's resume argv, and left the row starting.
	ResumeStarted ResumeOutcome = iota
	// ResumeStartingElsewhere means another (live, in-TTL) owner already
	// holds the launch lease for this session; this call created no tmux
	// session and left the row untouched.
	ResumeStartingElsewhere
)

// Resume relaunches an existing session's conversation (SPEC §8/§9.3): it
// acquires the launch lease, recreates deck_<slug> at the session's cwd,
// runs pre_launch, and launches the adapter's resume argv with the
// session's persisted env and permission profile. It never re-sends a
// prompt or previous message — the resume argv only ever carries the
// conversation id, profile and the session's own launch_args. A caller that
// loses the lease race gets ResumeStartingElsewhere and no tmux session is
// created for it.
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

	lease, err := s.Store.AcquireLaunchLease(ctx, sessionID, store.CurrentLaunchLeaseOwner(), store.DefaultLaunchLeaseTTL)
	if err != nil {
		return session, ResumeStartingElsewhere, fmt.Errorf("acquire launch lease for session %q: %w", session.Name, err)
	}
	if lease.Outcome != store.LaunchLeaseAcquired {
		// Another (live, in-TTL) owner holds the lease, or the row was not
		// leasable at all: no tmux session is created for this loser.
		return session, ResumeStartingElsewhere, nil
	}
	session.Status = "starting"
	if err := s.Audit.Transition(session.ID, "starting"); err != nil {
		return session, ResumeStarted, fmt.Errorf("audit starting resumed session %q: %w", session.Name, err)
	}

	var argv []string
	if freshOnce {
		argv, err = adapter.Launch(agent.LaunchInput{
			CWD: session.CWD, ConversationID: conversationID, Profile: session.PermissionProfile, ExtraArgs: session.LaunchArgs,
		})
	} else {
		argv, err = adapter.Resume(agent.ResumeInput{
			CWD: session.CWD, ConversationID: conversationID, Profile: session.PermissionProfile, ExtraArgs: session.LaunchArgs,
		})
	}
	if err != nil {
		session, failErr := s.launchFailed(ctx, session, fmt.Errorf("build resume argv for session %q: %w", session.Name, err))
		return session, ResumeStarted, failErr
	}
	paneCommand, err := buildPaneCommand(session.PreLaunch, session.LoginShell, argv)
	if err != nil {
		session, failErr := s.launchFailed(ctx, session, fmt.Errorf("build resume pane command for session %q: %w", session.Name, err))
		return session, ResumeStarted, failErr
	}

	// login_shell=1 is mutually exclusive with relying on captured_path, as
	// at create time (SPEC §6.4).
	envCapturedPath := session.CapturedPath
	if session.LoginShell {
		envCapturedPath = ""
	}
	launchEnv := s.resolveLaunchEnv(envCapturedPath, session.Env)
	// A login shell resolves its own PATH via its own profile/rc scripts
	// (that is the point of login_shell=1, SPEC §6.4), so deck cannot judge
	// PATH membership for it and must not fail resume on that basis.
	if !session.LoginShell {
		if lookErr := lookPathIn(argv[0], launchEnv["PATH"]); lookErr != nil {
			session, failErr := s.launchFailed(ctx, session, fmt.Errorf("resume session %q: agent binary %q not found on PATH: %w", session.Name, argv[0], lookErr))
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
		if err := s.Store.SetConversationID(ctx, session.ID, conversationID, "resume-fresh-once"); err != nil {
			session, failErr := s.launchFailed(ctx, session, fmt.Errorf("persist fresh conversation id for session %q: %w", session.Name, err))
			return session, ResumeStarted, failErr
		}
		if err := s.Store.ConsumeFreshOnce(ctx, session.ID, "resume-fresh-once"); err != nil {
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
