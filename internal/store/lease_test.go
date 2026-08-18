package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func newLeaseTestSession(t *testing.T, store *Store, id, status string) {
	t.Helper()
	ctx := context.Background()
	if _, err := store.CreateSession(ctx, CreateSessionInput{
		ID: id, Name: "lease-" + id, CWD: "/work/" + id, Agent: "claude", CapturedPath: "/bin",
		Status: status,
	}); err != nil {
		t.Fatalf("create lease test session: %v", err)
	}
}

func setRawLease(t *testing.T, store *Store, id, owner string, until int64) {
	t.Helper()
	if _, err := store.DB().Exec(`UPDATE sessions SET launch_lease_owner = ?, launch_lease_until = ? WHERE id = ?`,
		owner, until, id); err != nil {
		t.Fatalf("set raw lease fixture: %v", err)
	}
}

func TestAcquireLaunchLeaseFreshSucceeds(t *testing.T) {
	home := t.TempDir()
	store, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	const id = "00000000-0000-4000-8000-000000000101"
	newLeaseTestSession(t, store, id, "stopped")

	owner := "12345@boot-a"
	result, err := store.AcquireLaunchLease(context.Background(), id, owner, time.Second*30)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != LaunchLeaseAcquired {
		t.Fatalf("outcome = %v; want acquired", result.Outcome)
	}
	got, err := store.GetSession(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "starting" {
		t.Fatalf("status = %q; want starting", got.Status)
	}
	var eventCount int
	if err := store.DB().QueryRow(`SELECT COUNT(*) FROM events WHERE session_id = ? AND kind = 'launch_lease_acquired'`, id).
		Scan(&eventCount); err != nil {
		t.Fatal(err)
	}
	if eventCount != 1 {
		t.Fatalf("launch_lease_acquired events = %d; want 1", eventCount)
	}
}

func TestAcquireLaunchLeaseLiveOwnerInTTLIsNotBreakable(t *testing.T) {
	home := t.TempDir()
	store, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	const id = "00000000-0000-4000-8000-000000000102"
	newLeaseTestSession(t, store, id, "stopped")

	// This test process itself is guaranteed alive, and its own boot id is
	// whatever CurrentLaunchLeaseOwner reports, so a lease "owned" by this
	// process is a live, in-boot owner by construction.
	liveOwner := CurrentLaunchLeaseOwner()
	future := time.Now().Add(time.Minute).UnixMilli()
	setRawLease(t, store, id, liveOwner, future)

	result, err := store.AcquireLaunchLease(context.Background(), id, "99999@boot-other", time.Second*30)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != LaunchLeaseHeldElsewhere {
		t.Fatalf("outcome = %v; want held elsewhere", result.Outcome)
	}
	if result.HeldBy != liveOwner {
		t.Fatalf("held by = %q; want %q", result.HeldBy, liveOwner)
	}
	got, err := store.GetSession(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "stopped" {
		t.Fatalf("status = %q; want unchanged stopped", got.Status)
	}
}

func TestAcquireLaunchLeaseExpiredTTLIsBreakable(t *testing.T) {
	home := t.TempDir()
	store, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	const id = "00000000-0000-4000-8000-000000000103"
	newLeaseTestSession(t, store, id, "stopped")

	liveOwner := CurrentLaunchLeaseOwner()
	past := time.Now().Add(-time.Minute).UnixMilli()
	setRawLease(t, store, id, liveOwner, past)

	newOwner := "22222@boot-new"
	result, err := store.AcquireLaunchLease(context.Background(), id, newOwner, time.Second*30)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != LaunchLeaseAcquired {
		t.Fatalf("outcome = %v; want acquired despite live owner, because TTL expired", result.Outcome)
	}
	got, err := store.GetSession(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "starting" {
		t.Fatalf("status = %q; want starting", got.Status)
	}
}

func TestAcquireLaunchLeaseDeadPIDIsBreakable(t *testing.T) {
	home := t.TempDir()
	store, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	const id = "00000000-0000-4000-8000-000000000104"
	newLeaseTestSession(t, store, id, "stopped")

	// A pid this large is never a real process; boot id matches the current
	// boot deliberately so only the pid liveness check is exercised.
	deadOwner := "9999999@" + currentBootIDForTest()
	future := time.Now().Add(time.Minute).UnixMilli()
	setRawLease(t, store, id, deadOwner, future)

	newOwner := "33333@boot-new"
	result, err := store.AcquireLaunchLease(context.Background(), id, newOwner, time.Second*30)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != LaunchLeaseAcquired {
		t.Fatalf("outcome = %v; want acquired, dead pid should be breakable", result.Outcome)
	}
}

func TestAcquireLaunchLeaseNeverWedgesTheRow(t *testing.T) {
	home := t.TempDir()
	store, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	const id = "00000000-0000-4000-8000-000000000105"
	newLeaseTestSession(t, store, id, "stopped")

	liveOwner := CurrentLaunchLeaseOwner()
	future := time.Now().Add(time.Minute).UnixMilli()
	setRawLease(t, store, id, liveOwner, future)

	// First attempt is correctly refused (live, in-TTL owner).
	blocked, err := store.AcquireLaunchLease(context.Background(), id, "44444@boot-x", time.Second*30)
	if err != nil {
		t.Fatal(err)
	}
	if blocked.Outcome != LaunchLeaseHeldElsewhere {
		t.Fatalf("outcome = %v; want held elsewhere", blocked.Outcome)
	}

	// Simulate the holder legitimately releasing the lease (a later phase
	// clears it when a launch attempt concludes) — a subsequent legitimate
	// acquire by a different owner must still work; the row must not stay
	// wedged just because it was once refused.
	setRawLease(t, store, id, "", 0)
	newOwner := "55555@boot-y"
	retry, err := store.AcquireLaunchLease(context.Background(), id, newOwner, time.Second*30)
	if err != nil {
		t.Fatal(err)
	}
	if retry.Outcome != LaunchLeaseAcquired {
		t.Fatalf("retry outcome = %v; want acquired, row must not be wedged", retry.Outcome)
	}
	got, err := store.GetSession(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "starting" {
		t.Fatalf("status = %q; want starting", got.Status)
	}
}

func currentBootIDForTest() string {
	return bootID()
}
