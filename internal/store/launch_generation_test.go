package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// forceStopped puts a row back into the only status AcquireLaunchLease will
// lease, without clearing the lease columns: exactly the shape reconcile
// leaves behind when a pane goes away while its launch lease is still on the
// row, and the shape a second launch of the same row has to cope with.
func forceStopped(t *testing.T, store *Store, id string) {
	t.Helper()
	if _, err := store.DB().Exec(`UPDATE sessions SET status = 'stopped' WHERE id = ?`, id); err != nil {
		t.Fatalf("force stopped: %v", err)
	}
}

func storedLeaseOwner(t *testing.T, store *Store, id string) string {
	t.Helper()
	var owner string
	if err := store.DB().QueryRow(`SELECT COALESCE(launch_lease_owner, '') FROM sessions WHERE id = ?`, id).Scan(&owner); err != nil {
		t.Fatalf("read stored lease owner: %v", err)
	}
	return owner
}

// TestAcquireLaunchLeaseMintsAFreshGenerationPerLaunch is R74's leg-1 core
// claim (issue #11): every acquisition produces a discriminator that names
// that one launch. Both acquisitions here use the SAME owner string and the
// SAME timestamp, so any generation derived from the launcher identity or
// from the clock would come out identical -- which is precisely the failure
// mode a late hook from the superseded launch exploits.
func TestAcquireLaunchLeaseMintsAFreshGenerationPerLaunch(t *testing.T) {
	home := t.TempDir()
	store, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	const id = "00000000-0000-4000-8000-000000000180"
	newLeaseTestSession(t, store, id, "stopped")

	owner := "12345@boot-a"
	first, err := store.AcquireLaunchLease(context.Background(), id, owner, 30*time.Second, leaseTestNow)
	if err != nil {
		t.Fatal(err)
	}
	if first.Outcome != LaunchLeaseAcquired {
		t.Fatalf("first outcome = %v; want acquired", first.Outcome)
	}
	if first.LaunchGeneration == "" {
		t.Fatal("first acquisition produced no launch generation")
	}
	if strings.Contains(first.LaunchGeneration, "12345") || strings.Contains(first.LaunchGeneration, "boot-a") {
		t.Fatalf("launch generation %q repeats the launcher identity; it must discriminate launches, not launchers", first.LaunchGeneration)
	}
	// A timestamp-derived token would show up here: the acquisition ran with
	// an explicit `at` of leaseTestNow, and its seconds/millis prefix cannot
	// appear in a random token.
	for _, stamp := range []string{"1735789245", "1735789245000", "17357892"} {
		if strings.Contains(first.LaunchGeneration, stamp) {
			t.Fatalf("launch generation %q contains the acquisition timestamp %s; the discriminator must not be a timestamp", first.LaunchGeneration, stamp)
		}
	}
	if got, want := storedLeaseOwner(t, store, id), owner+"#"+first.LaunchGeneration; got != want {
		t.Fatalf("stored launch_lease_owner = %q; want %q", got, want)
	}

	forceStopped(t, store, id)
	second, err := store.AcquireLaunchLease(context.Background(), id, owner, 30*time.Second, leaseTestNow)
	if err != nil {
		t.Fatal(err)
	}
	if second.Outcome != LaunchLeaseAcquired {
		t.Fatalf("second outcome = %v; want acquired", second.Outcome)
	}
	if second.LaunchGeneration == first.LaunchGeneration {
		t.Fatalf("two successive launches of one row share generation %q; a late hook from the first would be indistinguishable from the second", first.LaunchGeneration)
	}
	if got, want := storedLeaseOwner(t, store, id), owner+"#"+second.LaunchGeneration; got != want {
		t.Fatalf("stored launch_lease_owner after second launch = %q; want %q (the row must name the CURRENT launch)", got, want)
	}
	var events int
	if err := store.DB().QueryRow(`SELECT COUNT(*) FROM events WHERE session_id = ? AND kind = 'launch_lease_acquired' AND payload = ?`,
		id, owner+"#"+second.LaunchGeneration).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 1 {
		t.Fatalf("launch_lease_acquired events naming the second generation = %d; want 1", events)
	}
}

// TestLiveOwnerWithGenerationIsStillNotBreakable pins the liveness parse
// against R74's owner format change. A generation suffix left inside the
// "pid@boot_id" identity makes the boot id compare unequal, so a lease held
// by a LIVE launcher would read as one from a previous boot and be broken --
// turning the §9.3 double-launch guard off for every leased row.
func TestLiveOwnerWithGenerationIsStillNotBreakable(t *testing.T) {
	home := t.TempDir()
	store, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	const id = "00000000-0000-4000-8000-000000000181"
	newLeaseTestSession(t, store, id, "stopped")

	// This process really is alive, so this owner is genuinely unbreakable.
	liveOwner := CurrentLaunchLeaseOwner() + "#a1b2c3d4e5f60718"
	setRawLease(t, store, id, liveOwner, leaseTestNow+10_000)

	result, err := store.AcquireLaunchLease(context.Background(), id, "99999@boot-other", 30*time.Second, leaseTestNow)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != LaunchLeaseHeldElsewhere {
		t.Fatalf("outcome = %v; want held elsewhere for a live in-TTL owner carrying a generation", result.Outcome)
	}
	if result.HeldBy != liveOwner {
		t.Fatalf("held by = %q; want %q", result.HeldBy, liveOwner)
	}
	if result.LaunchGeneration != "" {
		t.Fatalf("losing acquisition reported generation %q; only an acquired lease has one", result.LaunchGeneration)
	}
	if got := storedLeaseOwner(t, store, id); got != liveOwner {
		t.Fatalf("stored launch_lease_owner = %q; want the incumbent %q untouched", got, liveOwner)
	}
}

// TestSplitOwnerGenerationHandlesPreR74Owners keeps the pre-R74 owner shape
// (no generation at all) parsing as pure identity, so a lease written by an
// older deck build, or by a fixture, is still recognized rather than treated
// as unparseable.
func TestSplitOwnerGenerationHandlesPreR74Owners(t *testing.T) {
	identity, generation := splitOwnerGeneration("4321@boot-b")
	if identity != "4321@boot-b" || generation != "" {
		t.Fatalf("split(pre-R74) = (%q, %q); want the whole string as identity and no generation", identity, generation)
	}
	identity, generation = splitOwnerGeneration("4321@boot-b#ff00")
	if identity != "4321@boot-b" || generation != "ff00" {
		t.Fatalf("split(R74) = (%q, %q); want identity and generation", identity, generation)
	}
	if got := composeLeaseOwner("4321@boot-b", ""); got != "4321@boot-b" {
		t.Fatalf("compose with no generation = %q; want the bare identity", got)
	}
	pid, boot, ok := parseLeaseOwner("4321@boot-b#ff00")
	if !ok || pid != 4321 || boot != "boot-b" {
		t.Fatalf("parseLeaseOwner(with generation) = (%d, %q, %v); want (4321, boot-b, true)", pid, boot, ok)
	}
}
