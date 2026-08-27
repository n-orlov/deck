package service

import (
	"context"
	"testing"

	"github.com/n-orlov/deck/internal/hookrecv"
	"github.com/n-orlov/deck/internal/store"
)

// sessionEndFromLaunch pushes one SessionEnd through the REAL hook receiver on
// behalf of a named launch generation: `raw` is the payload Claude's SessionEnd
// hook sends, `generation` is what that pane's DECK_LAUNCH_GENERATION holds,
// and the row identity is the injected DECK_SESSION_ID. Nothing here simulates
// the receiver's decision -- the drop, if any, is made by internal/hookrecv.
func sessionEndFromLaunch(t *testing.T, db *store.Store, sessionID, generation string, at int64) hookrecv.Result {
	t.Helper()
	raw := []byte(`{"hook_event_name":"SessionEnd","reason":"other"}`)
	result, err := hookrecv.Receive(context.Background(), db, raw, sessionID, generation, at)
	if err != nil {
		t.Fatalf("receive SessionEnd for generation %q: %v", generation, err)
	}
	return result
}

func sessionStatus(t *testing.T, db *store.Store, sessionID string) store.Session {
	t.Helper()
	row, err := db.GetSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("read session: %v", err)
	}
	return row
}

// TestKilledLaunchsSessionEndAfterTheLeaseNeverStopsTheRow is R74 leg 2's
// discriminator (issue #11). The field failure: the operator kills a session
// (`x`) and resumes it (`r`); the killed agent's SessionEnd lands a moment
// later, AFTER the resume already took the launch lease and started the new
// pane, and stops the row deck has just brought up. The row then shows
// `stopped` while its pane is alive, and `r` refuses it because it is not
// actually gone.
//
// The ordering is the whole test. A SessionEnd delivered BEFORE
// AcquireLaunchLease cannot reproduce anything: the row is still stopped at
// that point, so the hook's `stopped` write is a no-op (and store.go's "a hook
// cannot resurrect a stopped row" guard drops it anyway), and the resume that
// follows leaves the row running -- that variant passes on unfixed code, which
// is why it is kept below as an explicit non-discriminator rather than as the
// test.
//
// The assertion shape matters as much as the ordering: the row must be
// unstopped IMMEDIATELY after the hook (never-applied, not repaired-later), it
// must still be unstopped after a reconcile pass that sees no further hook, and
// the ordinary `x` then `r` recovery must still work afterwards.
func TestKilledLaunchsSessionEndAfterTheLeaseNeverStopsTheRow(t *testing.T) {
	ctx := context.Background()
	cwd := t.TempDir()
	stubExecutableOnPath(t, "claude")
	service, db, _, socket := newAgentTestService(t, nil, "superseded-hook-test")

	created, err := service.CreateAgent(ctx, AgentCreateInput{
		Name: "Claude: superseded", CWD: cwd, Agent: "claude", PermissionProfile: "safe",
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	// The launch the user is about to kill. It has to be a LEASED launch, not
	// the create-time one, so that both panes in this story carry a token and
	// the test cannot pass merely because tokens were absent.
	killedGeneration := relaunchForGeneration(t, service, db, created.ID, created.Slug, "killed launch")
	assertTMuxEnvironment(t, socket, created.Slug, "DECK_LAUNCH_GENERATION", killedGeneration)

	// `x`: the pane goes away, the row is stopped. Then `r`: the replacement
	// launch takes the lease and mints the generation the row now names.
	currentGeneration := relaunchForGeneration(t, service, db, created.ID, created.Slug, "replacement launch")
	if currentGeneration == killedGeneration {
		t.Fatalf("both launches carry generation %q; the fixture cannot discriminate", currentGeneration)
	}
	assertTMuxEnvironment(t, socket, created.Slug, "DECK_LAUNCH_GENERATION", currentGeneration)
	live := sessionStatus(t, db, created.ID)
	if live.Status == "stopped" {
		t.Fatalf("fixture is not discriminating: row is already stopped before the hook: %#v", live)
	}

	// The killed agent's SessionEnd, arriving late -- after the new launch's
	// AcquireLaunchLease and after its pane exists.
	result := sessionEndFromLaunch(t, db, created.ID, killedGeneration, 1_700_000_000_000)
	if row := sessionStatus(t, db, created.ID); row.Status == "stopped" {
		t.Fatalf("the killed launch's SessionEnd stopped the row deck had just started: status=%q reason=%q source=%q",
			row.Status, row.StatusReason, row.StatusSource)
	}
	if !result.Superseded {
		t.Fatal("receiver did not recognize the killed launch's SessionEnd as superseded")
	}
	// Nothing may repair this after the fact either: a reconcile pass with no
	// further hook must find the same row it left.
	if err := service.Reconcile(ctx); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if row := sessionStatus(t, db, created.ID); row.Status == "stopped" {
		t.Fatalf("row was stopped by reconciliation after the superseded hook: status=%q reason=%q source=%q",
			row.Status, row.StatusReason, row.StatusSource)
	}
	exists, err := service.TMux.Exists(ctx, created.Slug)
	if err != nil {
		t.Fatalf("check pane liveness: %v", err)
	}
	if !exists {
		t.Fatal("the replacement pane is gone; the row's status is not the only thing at stake")
	}

	// And the ordinary recovery still works: `x` then `r`.
	recovered := relaunchForGeneration(t, service, db, created.ID, created.Slug, "recovery")
	if recovered == currentGeneration || recovered == killedGeneration {
		t.Fatalf("recovery launch reused generation %q", recovered)
	}
	if row := sessionStatus(t, db, created.ID); row.Status == "stopped" {
		t.Fatalf("`x` then `r` did not recover the row: %#v", row)
	}
}

// TestKilledLaunchsSessionEndBeforeTheLeaseIsNotTheDiscriminator records, as a
// test rather than as a claim in prose, that the same fixture with the hook
// delivered BEFORE AcquireLaunchLease proves nothing: it is green on unfixed
// code. Keeping it beside the discriminator is what stops a later change from
// quietly replacing the ordering that matters with the ordering that does not.
func TestKilledLaunchsSessionEndBeforeTheLeaseIsNotTheDiscriminator(t *testing.T) {
	ctx := context.Background()
	cwd := t.TempDir()
	stubExecutableOnPath(t, "claude")
	service, db, _, _ := newAgentTestService(t, nil, "superseded-hook-before")

	created, err := service.CreateAgent(ctx, AgentCreateInput{
		Name: "Claude: before lease", CWD: cwd, Agent: "claude", PermissionProfile: "safe",
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	killedGeneration := relaunchForGeneration(t, service, db, created.ID, created.Slug, "killed launch")

	// `x` only: the row is stopped and the killed agent's SessionEnd arrives
	// while it still is, i.e. before any replacement launch.
	if err := service.TMux.Kill(ctx, created.Slug); err != nil {
		t.Fatalf("kill pane: %v", err)
	}
	stopSession(t, db, created.ID)
	sessionEndFromLaunch(t, db, created.ID, killedGeneration, 1_700_000_000_001)

	// `r` afterwards, and the row comes up: no fix is needed for this ordering.
	expireLaunchLease(t, db, created.ID)
	if _, outcome, err := service.Resume(ctx, created.ID); err != nil || outcome != ResumeStarted {
		t.Fatalf("resume after an already-delivered SessionEnd: outcome=%v err=%v", outcome, err)
	}
	if row := sessionStatus(t, db, created.ID); row.Status == "stopped" {
		t.Fatalf("resume left the row stopped: %#v", row)
	}
}

// relaunchForGeneration is `x` then `r`: kill the live pane, record the row as
// stopped the way reconciliation would, and resume it. It returns the
// generation the resulting launch minted, which is the one the row now names.
// The lease is expired first because the test clock never advances, so the
// 30 s TTL of the previous launch would otherwise still be held by this very
// process -- that is R75's subject (task 031), not this test's.
func relaunchForGeneration(t *testing.T, service Service, db *store.Store, sessionID, slug, label string) string {
	t.Helper()
	ctx := context.Background()
	if err := service.TMux.Kill(ctx, slug); err != nil {
		t.Fatalf("%s: kill pane: %v", label, err)
	}
	stopSession(t, db, sessionID)
	expireLaunchLease(t, db, sessionID)
	if _, outcome, err := service.Resume(ctx, sessionID); err != nil || outcome != ResumeStarted {
		t.Fatalf("%s: resume outcome=%v err=%v", label, outcome, err)
	}
	generation := rowLaunchGeneration(t, db, sessionID)
	if generation == "" {
		t.Fatalf("%s: row carries no launch generation", label)
	}
	return generation
}
