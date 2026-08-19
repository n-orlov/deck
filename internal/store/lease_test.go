package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

const leaseTestNow int64 = 1_735_789_245_000

func newLeaseTestSession(t *testing.T, store *Store, id, status string) {
	t.Helper()
	ctx := context.Background()
	if _, err := store.CreateSession(ctx, CreateSessionInput{
		ID: id, Name: "lease-" + id, CWD: "/work/" + id, Agent: "claude", CapturedPath: "/bin",
		Status: status, StatusAt: 1, CreatedAt: 1,
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

func TestAcquireLaunchLeaseRequiresTimestamp(t *testing.T) {
	home := t.TempDir()
	store, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	const id = "00000000-0000-4000-8000-000000000100"
	newLeaseTestSession(t, store, id, "stopped")

	_, err = store.AcquireLaunchLease(context.Background(), id, "12345@boot-a", 30*time.Second, 0)
	if err == nil || err.Error() != "launch lease timestamp is required" {
		t.Fatalf("missing timestamp error = %v; want launch lease timestamp is required", err)
	}
	got, err := store.GetSession(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "stopped" {
		t.Fatalf("status = %q after rejected missing timestamp; want stopped", got.Status)
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
	result, err := store.AcquireLaunchLease(context.Background(), id, owner, time.Second*30, leaseTestNow)
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
	var eventAt, leaseUntil int64
	if err := store.DB().QueryRow(`SELECT COUNT(*), at FROM events WHERE session_id = ? AND kind = 'launch_lease_acquired'`, id).
		Scan(&eventCount, &eventAt); err != nil {
		t.Fatal(err)
	}
	if eventCount != 1 {
		t.Fatalf("launch_lease_acquired events = %d; want 1", eventCount)
	}
	if eventAt != leaseTestNow {
		t.Fatalf("launch_lease_acquired at = %d; want explicit timestamp %d", eventAt, leaseTestNow)
	}
	if err := store.DB().QueryRow(`SELECT launch_lease_until FROM sessions WHERE id = ?`, id).Scan(&leaseUntil); err != nil {
		t.Fatal(err)
	}
	if want := leaseTestNow + (30 * time.Second).Milliseconds(); leaseUntil != want {
		t.Fatalf("launch_lease_until = %d; want %d derived from explicit timestamp", leaseUntil, want)
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
	future := leaseTestNow + time.Minute.Milliseconds()
	setRawLease(t, store, id, liveOwner, future)

	result, err := store.AcquireLaunchLease(context.Background(), id, "99999@boot-other", time.Second*30, leaseTestNow)
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
	past := leaseTestNow - time.Minute.Milliseconds()
	setRawLease(t, store, id, liveOwner, past)

	newOwner := "22222@boot-new"
	result, err := store.AcquireLaunchLease(context.Background(), id, newOwner, time.Second*30, leaseTestNow)
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
	future := leaseTestNow + time.Minute.Milliseconds()
	setRawLease(t, store, id, deadOwner, future)

	newOwner := "33333@boot-new"
	result, err := store.AcquireLaunchLease(context.Background(), id, newOwner, time.Second*30, leaseTestNow)
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
	future := leaseTestNow + time.Minute.Milliseconds()
	setRawLease(t, store, id, liveOwner, future)

	// First attempt is correctly refused (live, in-TTL owner).
	blocked, err := store.AcquireLaunchLease(context.Background(), id, "44444@boot-x", time.Second*30, leaseTestNow)
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
	retry, err := store.AcquireLaunchLease(context.Background(), id, newOwner, time.Second*30, leaseTestNow)
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
