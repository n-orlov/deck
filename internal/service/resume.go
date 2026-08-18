package service

import (
	"context"
	"errors"
	"fmt"

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

	argv, err := adapter.Resume(agent.ResumeInput{
		CWD: session.CWD, ConversationID: session.ConversationID, Profile: session.PermissionProfile, ExtraArgs: session.LaunchArgs,
	})
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
	session.StatusSource = "tmux"
	return session, ResumeStarted, nil
}
