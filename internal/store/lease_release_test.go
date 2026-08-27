package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

// setRawStatus puts a row back to a status directly, standing in for whatever
// really ended the pane (a kill, a crash, a reconcile pass). The lease columns
// are deliberately NOT touched, because the state under test is exactly "the
// row is stopped again while the previous launch's lease window is still open".
func setRawStatus(t *testing.T, store *Store, id, status string) {
	t.Helper()
	if _, err := store.DB().Exec(`UPDATE sessions SET status = ? WHERE id = ?`, status, id); err != nil {
		t.Fatalf("set raw status fixture: %v", err)
	}
}

func rawLease(t *testing.T, store *Store, id string) (owner string, until int64) {
	t.Helper()
	if err := store.DB().QueryRow(
		`SELECT COALESCE(launch_lease_owner, ''), launch_lease_until FROM sessions WHERE id = ?`, id).
		Scan(&owner, &until); err != nil {
		t.Fatalf("read raw lease columns: %v", err)
	}
	return owner, until
}

// TestReleaseLaunchLeaseLetsTheSameOwnerLaunchAgainInsideTheTTL is R75's
// forward direction (issue #11): a launch that has concluded stops holding the
// lease, so the next resume of a legitimately stopped row is not answered
// "held elsewhere" by the row's own last launcher. Everything happens at ONE
// timestamp, well inside the 30 s TTL, so nothing here can pass because time
// passed.
func TestReleaseLaunchLeaseLetsTheSameOwnerLaunchAgainInsideTheTTL(t *testing.T) {
	home := t.TempDir()
	store, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	const id = "00000000-0000-4000-8000-000000000110"
	newLeaseTestSession(t, store, id, "stopped")

	owner := CurrentLaunchLeaseOwner()
	first, err := store.AcquireLaunchLease(ctx, id, owner, 30*time.Second, leaseTestNow)
	if err != nil {
		t.Fatal(err)
	}
	if first.Outcome != LaunchLeaseAcquired {
		t.Fatalf("first outcome = %v; want acquired", first.Outcome)
	}
	if _, until := rawLease(t, store, id); until != leaseTestNow+(30*time.Second).Milliseconds() {
		t.Fatalf("launch_lease_until = %d before the release; the fixture must start from a genuinely held lease", until)
	}

	released, err := store.ReleaseLaunchLease(ctx, id, first.HeldBy)
	if err != nil {
		t.Fatal(err)
	}
	if !released {
		t.Fatal("ReleaseLaunchLease reported nothing released; want the lease this owner holds")
	}

	// R74's discriminator must survive the release: the row still names which
	// launch is current, so a late hook from a superseded pane is still
	// recognizable. Only the hold is gone.
	gotOwner, gotUntil := rawLease(t, store, id)
	if gotOwner != first.HeldBy {
		t.Fatalf("launch_lease_owner = %q after the release; want it untouched at %q (R74's generation lives here)", gotOwner, first.HeldBy)
	}
	if gotUntil != 0 {
		t.Fatalf("launch_lease_until = %d after the release; want 0", gotUntil)
	}
	session, err := store.GetSession(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if session.LaunchGeneration != first.LaunchGeneration || session.LaunchGeneration == "" {
		t.Fatalf("row generation = %q after the release; want the launch's own %q still readable", session.LaunchGeneration, first.LaunchGeneration)
	}

	// Releasing again is a no-op rather than an error: the lease is already
	// not held.
	again, err := store.ReleaseLaunchLease(ctx, id, first.HeldBy)
	if err != nil {
		t.Fatal(err)
	}
	if again {
		t.Fatal("second ReleaseLaunchLease reported a release; want a no-op")
	}

	// The pane ends and the row is stopped again -- still at the same instant,
	// i.e. inside the window the first launch's TTL used to cover.
	setRawStatus(t, store, id, "stopped")
	second, err := store.AcquireLaunchLease(ctx, id, owner, 30*time.Second, leaseTestNow)
	if err != nil {
		t.Fatal(err)
	}
	if second.Outcome != LaunchLeaseAcquired {
		t.Fatalf("second outcome = %v at the same timestamp; want acquired, because the first launch no longer holds the lease", second.Outcome)
	}
	if second.LaunchGeneration == first.LaunchGeneration {
		t.Fatalf("both launches minted generation %q; the discriminator must still change per launch", second.LaunchGeneration)
	}
	if gotOwner, _ := rawLease(t, store, id); gotOwner != second.HeldBy {
		t.Fatalf("launch_lease_owner = %q after the second acquire; want the CURRENT launch %q", gotOwner, second.HeldBy)
	}
}

// TestReleaseLaunchLeaseDoesNotEndADifferentLiveOwnersLease is R75's other
// direction: the guard is narrowed, not deleted. A release CASes on the exact
// owner string the releasing launch acquired, so it cannot cut short a lease
// that belongs to somebody else -- and that somebody else still blocks a
// competing acquire, which is the whole of SPEC §9.3.
func TestReleaseLaunchLeaseDoesNotEndADifferentLiveOwnersLease(t *testing.T) {
	home := t.TempDir()
	store, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	const id = "00000000-0000-4000-8000-000000000111"
	newLeaseTestSession(t, store, id, "stopped")

	// This test process is alive by construction and its boot id is the
	// current one, so this is a genuinely live, in-TTL incumbent.
	incumbent := CurrentLaunchLeaseOwner() + "#incumbentgeneration"
	future := leaseTestNow + time.Minute.Milliseconds()
	setRawLease(t, store, id, incumbent, future)

	// A late release from a different launch of the same row: same identity
	// shape, different generation. It must not touch the incumbent's hold.
	stale := CurrentLaunchLeaseOwner() + "#stalegeneration"
	released, err := store.ReleaseLaunchLease(ctx, id, stale)
	if err != nil {
		t.Fatal(err)
	}
	if released {
		t.Fatal("a release naming a different launch reported a release; a launch must only ever release its own lease")
	}
	if gotOwner, gotUntil := rawLease(t, store, id); gotOwner != incumbent || gotUntil != future {
		t.Fatalf("lease columns = (%q, %d) after a foreign release; want the incumbent (%q, %d) untouched", gotOwner, gotUntil, incumbent, future)
	}

	// And the guard R75 must not delete: a competing acquire is still refused
	// while that live owner holds the lease in its TTL.
	blocked, err := store.AcquireLaunchLease(ctx, id, "99999@boot-other", 30*time.Second, leaseTestNow)
	if err != nil {
		t.Fatal(err)
	}
	if blocked.Outcome != LaunchLeaseHeldElsewhere {
		t.Fatalf("outcome = %v while a live in-TTL owner holds the lease; want held elsewhere", blocked.Outcome)
	}
	if blocked.HeldBy != incumbent {
		t.Fatalf("held by = %q; want %q", blocked.HeldBy, incumbent)
	}
	if got, err := store.GetSession(ctx, id); err != nil {
		t.Fatal(err)
	} else if got.Status != "stopped" {
		t.Fatalf("status = %q; want unchanged stopped", got.Status)
	}
}

func TestReleaseLaunchLeaseRejectsMissingArguments(t *testing.T) {
	home := t.TempDir()
	store, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	const id = "00000000-0000-4000-8000-000000000112"
	newLeaseTestSession(t, store, id, "stopped")

	if _, err := store.ReleaseLaunchLease(ctx, "", "12345@boot-a#gen"); err == nil || err.Error() != "session id is required" {
		t.Fatalf("missing session id error = %v; want session id is required", err)
	}
	// An empty owner would otherwise match a row whose owner column is empty
	// and release a lease nobody named.
	if _, err := store.ReleaseLaunchLease(ctx, id, ""); err == nil || err.Error() != "lease owner is required" {
		t.Fatalf("missing owner error = %v; want lease owner is required", err)
	}
}
