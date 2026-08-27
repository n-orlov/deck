package service

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/store"
)

// rawLeaseColumns reads the row's lease columns as stored, so a test can tell
// "released" (owner kept, until zeroed) apart from "cleared" (owner blanked,
// which would destroy R74's generation discriminator).
func rawLeaseColumns(t *testing.T, db *store.Store, sessionID string) (owner string, until int64) {
	t.Helper()
	if err := db.DB().QueryRow(
		`SELECT COALESCE(launch_lease_owner, ''), launch_lease_until FROM sessions WHERE id = ?`, sessionID).
		Scan(&owner, &until); err != nil {
		t.Fatalf("read lease columns: %v", err)
	}
	return owner, until
}

// setRawLaunchLease installs a lease directly, standing in for a launch started
// by a different deck client. Nothing else about the row is touched.
func setRawLaunchLease(t *testing.T, db *store.Store, sessionID, owner string, until int64) {
	t.Helper()
	if _, err := db.DB().Exec(`UPDATE sessions SET launch_lease_owner = ?, launch_lease_until = ? WHERE id = ?`,
		owner, until, sessionID); err != nil {
		t.Fatalf("set raw launch lease: %v", err)
	}
}

// liveForeignLeaseOwner returns an owner string naming a process that is
// genuinely alive right now and is NOT this one, plus a func that kills and
// reaps it. Only the child this test itself spawned is ever signalled, and it
// is reaped with Wait so the pid stops answering the store's signal-0 probe (a
// zombie still would).
func liveForeignLeaseOwner(t *testing.T, generation string) (owner string, kill func()) {
	t.Helper()
	child := exec.Command("sleep", "300")
	if err := child.Start(); err != nil {
		t.Fatalf("start a live foreign lease holder: %v", err)
	}
	killed := false
	kill = func() {
		if killed {
			return
		}
		killed = true
		_ = child.Process.Kill()
		_ = child.Wait()
	}
	t.Cleanup(kill)
	// Same boot id as this process, so only the pid's liveness is under test;
	// the pid differs, so the holder is a different live process.
	_, boot, _ := strings.Cut(store.CurrentLaunchLeaseOwner(), "@")
	return fmt.Sprintf("%d@%s#%s", child.Process.Pid, boot, generation), kill
}

// TestResumeReleasesItsLaunchLeaseWhenTheLaunchCompletes is R75 (issue #11)
// end to end through the real service and real tmux: once a launch has
// concluded it stops holding the lease, so the SAME owner resuming the same
// row again -- with no wall-clock time passed at all, the test clock is frozen
// -- is not told the row is starting somewhere else. Before R75 this second
// resume returned ResumeStartingElsewhere, naming a "other client" that was
// this very process's finished launch.
func TestResumeReleasesItsLaunchLeaseWhenTheLaunchCompletes(t *testing.T) {
	cwd := t.TempDir()
	stubExecutableOnPath(t, "claude")
	service, db, _, _ := newAgentTestService(t, nil, "lease-release-test")
	ctx := context.Background()

	created, err := service.CreateAgent(ctx, AgentCreateInput{
		Name: "Claude: lease release", CWD: cwd, Agent: "claude", PermissionProfile: "safe",
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	resumeAgain := func(label string) string {
		t.Helper()
		if err := service.TMux.Kill(ctx, created.Slug); err != nil {
			t.Fatalf("%s: kill pane: %v", label, err)
		}
		stopSession(t, db, created.ID)
		// Nothing ages the lease out of its TTL here, and nothing may: the
		// product itself has to have stopped holding it. The test fixture
		// that used to do this (expireLaunchLease) is gone from the package
		// for the same reason.
		_, outcome, err := service.Resume(ctx, created.ID)
		if err != nil {
			t.Fatalf("%s: resume: %v", label, err)
		}
		if outcome == ResumeStartingElsewhere {
			t.Fatalf("%s: outcome = ResumeStartingElsewhere; the only launcher in this test is this process's own concluded launch", label)
		}
		if outcome != ResumeStarted {
			t.Fatalf("%s: outcome = %v; want ResumeStarted", label, outcome)
		}
		// Reported non-fatally on purpose: the column state and the OUTCOME of
		// the next resume are two separate claims, and a run against unfixed
		// code should show both, not stop at the first.
		owner, until := rawLeaseColumns(t, db, created.ID)
		if until != 0 {
			t.Errorf("%s: launch_lease_until = %d after the launch completed; want 0 (the launch is no longer in flight)", label, until)
		}
		// R74's discriminator must outlive the lease: the row goes on naming
		// which launch is current, or every late hook from a replaced pane
		// becomes "nothing to discriminate, therefore apply" again.
		if !strings.Contains(owner, "#") {
			t.Errorf("%s: launch_lease_owner = %q after the release; want the owner (with its generation) kept", label, owner)
		}
		generation := rowLaunchGeneration(t, db, created.ID)
		if live, err := service.TMux.HasLivePane(ctx, created.Slug); err != nil {
			t.Fatalf("%s: check live pane: %v", label, err)
		} else if !live {
			t.Fatalf("%s: the resumed session has no live pane", label)
		}
		return generation
	}

	first := resumeAgain("first resume")
	second := resumeAgain("second resume inside the lease window")
	if first == second {
		t.Fatalf("both resumes carry generation %q; releasing the lease must not stop each launch minting its own", first)
	}
}

// TestResumeStillRefusesALeaseHeldByADifferentLiveOwner is the other half of
// R75: the §9.3 guard is narrowed to launches that are actually in flight, not
// deleted. A lease held by a different, genuinely live process inside its TTL
// still yields ResumeStartingElsewhere and still creates no pane -- and the row
// is not wedged, because once that holder dies the very same resume works.
func TestResumeStillRefusesALeaseHeldByADifferentLiveOwner(t *testing.T) {
	cwd := t.TempDir()
	stubExecutableOnPath(t, "claude")
	service, db, _, _ := newAgentTestService(t, nil, "lease-foreign-owner-test")
	ctx := context.Background()

	created, err := service.CreateAgent(ctx, AgentCreateInput{
		Name: "Claude: foreign lease", CWD: cwd, Agent: "claude", PermissionProfile: "safe",
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := service.TMux.Kill(ctx, created.Slug); err != nil {
		t.Fatalf("kill pane: %v", err)
	}
	stopSession(t, db, created.ID)

	foreignOwner, killHolder := liveForeignLeaseOwner(t, "foreigngeneration")
	until := service.Clock.Now().UnixMilli() + (30 * time.Second).Milliseconds()
	setRawLaunchLease(t, db, created.ID, foreignOwner, until)

	_, outcome, err := service.Resume(ctx, created.ID)
	if err != nil {
		t.Fatalf("resume against a foreign lease: %v", err)
	}
	if outcome != ResumeStartingElsewhere {
		t.Fatalf("outcome = %v while another live process holds the lease in its TTL; want ResumeStartingElsewhere", outcome)
	}
	if live, err := service.TMux.HasLivePane(ctx, created.Slug); err != nil {
		t.Fatalf("check live pane: %v", err)
	} else if live {
		t.Fatal("a resume that lost the lease created a pane anyway")
	}
	if gotOwner, gotUntil := rawLeaseColumns(t, db, created.ID); gotOwner != foreignOwner || gotUntil != until {
		t.Fatalf("lease columns = (%q, %d) after the refused resume; want the holder's (%q, %d) untouched", gotOwner, gotUntil, foreignOwner, until)
	}
	if session, err := db.GetSession(ctx, created.ID); err != nil {
		t.Fatal(err)
	} else if session.Status != "stopped" {
		t.Fatalf("status = %q after the refused resume; want unchanged stopped", session.Status)
	}

	// The holder dies (crashed client). Nothing else changes -- same frozen
	// clock, same lease row -- and the row must be launchable again.
	killHolder()
	_, outcome, err = service.Resume(ctx, created.ID)
	if err != nil {
		t.Fatalf("resume after the holder died: %v", err)
	}
	if outcome != ResumeStarted {
		t.Fatalf("outcome = %v after the lease holder died; want ResumeStarted, the row must not be wedged", outcome)
	}
	if _, gotUntil := rawLeaseColumns(t, db, created.ID); gotUntil != 0 {
		t.Fatalf("launch_lease_until = %d after that launch completed; want 0", gotUntil)
	}
}
