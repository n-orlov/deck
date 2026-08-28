package service

import (
	"context"
	"errors"
	"testing"

	"github.com/n-orlov/deck/internal/config"
)

// failingLeaseReleaser substitutes for the store through Service.LeaseReleaser
// (task 036's seam) and always fails the one call Resume's deferred release
// makes, standing in for a store whose ReleaseLaunchLease UPDATE errors (a
// disk-full write, a lock timeout, ...).
type failingLeaseReleaser struct{}

func (failingLeaseReleaser) ReleaseLaunchLease(ctx context.Context, sessionID, heldOwner string) (bool, error) {
	return false, errors.New("simulated release failure")
}

// TestResumeAuditsAndStaysTTLBoundedWhenReleasingTheLaunchLeaseFails is R75's
// release-failure fallback (SPEC §9.3): when the deferred release itself
// cannot write, Resume must not let that failure change the verdict it just
// gave the user, and must not leave the row wedged past the lease's own TTL.
func TestResumeAuditsAndStaysTTLBoundedWhenReleasingTheLaunchLeaseFails(t *testing.T) {
	cwd := t.TempDir()
	stubExecutableOnPath(t, "claude")
	service, db, logger, _ := newAgentTestService(t, nil, "lease-release-failure-test")
	ctx := context.Background()

	// A stepped clock lets the test jump past the 30s TTL on demand instead
	// of sleeping for it; the base instant matches newAgentTestService's own
	// frozen clock so the acquired lease's until value is easy to reason about.
	clock, err := config.NewClock("2025-01-02T03:04:05Z", "40s")
	if err != nil {
		t.Fatal(err)
	}
	service.Clock = clock

	created, err := service.CreateAgent(ctx, AgentCreateInput{
		Name: "Claude: lease release failure", CWD: cwd, Agent: "claude", PermissionProfile: "safe",
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	// CreateAgent's own launch is still live; stop it first the same way the
	// R75 happy-path test does, so this call's Resume actually goes through
	// AcquireLaunchLease instead of taking the already-running no-op path.
	if err := service.TMux.Kill(ctx, created.Slug); err != nil {
		t.Fatalf("kill pane: %v", err)
	}
	stopSession(t, db, created.ID)

	service.LeaseReleaser = failingLeaseReleaser{}

	_, outcome, err := service.Resume(ctx, created.ID)
	if err != nil {
		t.Fatalf("resume with a failing releaser: %v", err)
	}
	// The user-facing verdict must be exactly what a successful release would
	// have given: the release is a background cleanup, not part of the
	// contract this call reports.
	if outcome != ResumeStarted {
		t.Fatalf("outcome = %v; want ResumeStarted unchanged by the release failure", outcome)
	}

	records := auditRecords(t, logger.Path())
	var releaseFailed map[string]any
	for _, record := range records {
		if record["event"] == "launch_lease.release_failed" && record["session_id"] == created.ID {
			releaseFailed = record
		}
	}
	if releaseFailed == nil {
		t.Fatalf("no launch_lease.release_failed audit record for session %q among %#v", created.ID, records)
	}

	owner, until := rawLeaseColumns(t, db, created.ID)
	if until == 0 {
		t.Fatalf("launch_lease_until = 0 after a release that failed; a failed UPDATE must not have cleared it")
	}
	// R74's discriminator must still be intact: the release failure is not
	// licence to lose which launch is current.
	if !containsGenerationSep(owner) {
		t.Fatalf("launch_lease_owner = %q after the failed release; want the owner (with its generation) kept", owner)
	}

	// The pane this launch started has since ended (simulated the same way
	// the R75 happy-path test does), and the lease's TTL has now elapsed:
	// the §9.3 backstop must make the row leasable again even though its
	// faster release path never ran.
	if err := service.TMux.Kill(ctx, created.Slug); err != nil {
		t.Fatalf("kill pane: %v", err)
	}
	stopSession(t, db, created.ID)
	// One 40s step comfortably clears the 30s TTL the acquisition set.
	service.Clock.Advance()

	_, outcome, err = service.Resume(ctx, created.ID)
	if err != nil {
		t.Fatalf("resume after the lease's TTL elapsed: %v", err)
	}
	if outcome == ResumeStartingElsewhere {
		t.Fatalf("outcome = ResumeStartingElsewhere after the TTL elapsed; the row must not be wedged by the earlier release failure")
	}
	if outcome != ResumeStarted {
		t.Fatalf("outcome = %v; want ResumeStarted once the row is leasable again", outcome)
	}
}

func containsGenerationSep(owner string) bool {
	for i := 0; i < len(owner); i++ {
		if owner[i] == '#' {
			return true
		}
	}
	return false
}
