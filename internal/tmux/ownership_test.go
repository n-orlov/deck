package tmux

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"
)

// newBareOwnershipSession starts a single-window tmux session directly on a
// throwaway socket, without Client.Bootstrap, so ownership reads/writes are
// asserted against tmux's own unmodified defaults for OwnershipOption
// (unset).
func newBareOwnershipSession(t *testing.T, socket, session string) (cleanup func()) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if output, err := exec.CommandContext(ctx, "tmux", "-L", socket, "new-session", "-d", "-s", session, "-x", "80", "-y", "24").CombinedOutput(); err != nil {
		t.Fatalf("start bare tmux session: %v: %s", err, output)
	}
	return func() {
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer closeCancel()
		_ = exec.CommandContext(closeCtx, "tmux", "-L", socket, "kill-server").Run()
	}
}

func setWindowOwnershipRaw(t *testing.T, socket, target, value string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if output, err := exec.CommandContext(ctx, "tmux", "-L", socket, "set-option", "-w", "-t", target, OwnershipOption, value).CombinedOutput(); err != nil {
		t.Fatalf("set-option -w -t %s %s %s: %v: %s", target, OwnershipOption, value, err, output)
	}
}

// TestClaimWindowOwnershipAcquiresOnAnUnclaimedWindow proves the base case:
// an unclaimed window's confirm-read matches what was just written, and the
// value on the wire is the documented `<tag>:<pid>` form carrying this
// process's own pid.
func TestClaimWindowOwnershipAcquiresOnAnUnclaimedWindow(t *testing.T) {
	socket := fmt.Sprintf("deck-ownership-fresh-%d-%d", os.Getpid(), time.Now().UnixNano())
	cleanup := newBareOwnershipSession(t, socket, "s0")
	defer cleanup()
	client := Client{Socket: socket, Timeout: 3 * time.Second}

	ownership, owned, err := client.ClaimWindowOwnership(context.Background(), "s0")
	if err != nil {
		t.Fatalf("claim ownership: %v", err)
	}
	if !owned || ownership == nil {
		t.Fatalf("owned = %v, ownership = %v; want acquired on an unclaimed window", owned, ownership)
	}
	_, pid, ok := parseOwnershipClaim(ownership.claim)
	if !ok || pid != os.Getpid() {
		t.Fatalf("claim %q does not carry this process's own pid %d", ownership.claim, os.Getpid())
	}
	got, err := readTmuxOptionForOwnershipTest(t, socket, "s0")
	if err != nil {
		t.Fatalf("read back claim: %v", err)
	}
	if got != ownership.claim {
		t.Fatalf("tmux reads %q, want the exact claim %q", got, ownership.claim)
	}
}

// TestClaimWindowOwnershipRespectsALiveCompetingOwner proves the mandatory
// "live owner is respected" case: a competing claim tagged with a pid that
// answers a signal-0 probe (this test process's own pid, guaranteed alive
// for the duration of the test) causes ClaimWindowOwnership to stand down
// without touching the option again -- the competing value is still there
// afterwards, untouched.
func TestClaimWindowOwnershipRespectsALiveCompetingOwner(t *testing.T) {
	socket := fmt.Sprintf("deck-ownership-live-%d-%d", os.Getpid(), time.Now().UnixNano())
	cleanup := newBareOwnershipSession(t, socket, "s0")
	defer cleanup()
	client := Client{Socket: socket, Timeout: 3 * time.Second}

	competing := formatOwnershipClaim("livecompetitor", os.Getpid())
	setWindowOwnershipRaw(t, socket, "s0", competing)

	ownership, owned, err := client.ClaimWindowOwnership(context.Background(), "s0")
	if err != nil {
		t.Fatalf("claim against a live competing owner: %v", err)
	}
	if owned || ownership != nil {
		t.Fatalf("owned = %v, ownership = %v; want stand-down against a live competing owner", owned, ownership)
	}
	got, err := readTmuxOptionForOwnershipTest(t, socket, "s0")
	if err != nil {
		t.Fatalf("read option after stand-down: %v", err)
	}
	if got != competing {
		t.Fatalf("option = %q after standing down; want the live competitor's claim %q left untouched", got, competing)
	}
}

// TestClaimWindowOwnershipStealsFromADeadOwner proves the mandatory "a dead
// owner is stolen" case: a competing claim tagged with a pid that does not
// exist (a very large, essentially-guaranteed-unused pid) is validated with
// a kill(pid, 0) probe, found dead, and overwritten -- the caller acquires
// ownership and the option now carries its own claim, not the dead one's.
func TestClaimWindowOwnershipStealsFromADeadOwner(t *testing.T) {
	socket := fmt.Sprintf("deck-ownership-dead-%d-%d", os.Getpid(), time.Now().UnixNano())
	cleanup := newBareOwnershipSession(t, socket, "s0")
	defer cleanup()
	client := Client{Socket: socket, Timeout: 3 * time.Second}

	deadPID := 999999999
	if pidAlive(deadPID) {
		t.Fatalf("test's chosen dead pid %d is alive; pick another", deadPID)
	}
	setWindowOwnershipRaw(t, socket, "s0", formatOwnershipClaim("deadowner", deadPID))

	ownership, owned, err := client.ClaimWindowOwnership(context.Background(), "s0")
	if err != nil {
		t.Fatalf("claim against a dead competing owner: %v", err)
	}
	if !owned || ownership == nil {
		t.Fatalf("owned = %v, ownership = %v; want a dead owner's claim to be stolen", owned, ownership)
	}
	got, err := readTmuxOptionForOwnershipTest(t, socket, "s0")
	if err != nil {
		t.Fatalf("read option after steal: %v", err)
	}
	if got != ownership.claim {
		t.Fatalf("option = %q after steal, want the stolen claim %q", got, ownership.claim)
	}
	_, pid, ok := parseOwnershipClaim(got)
	if !ok || pid == deadPID {
		t.Fatalf("option still names the dead pid: %q", got)
	}
}

// TestClaimWindowOwnershipConfirmReadLosesToACompetingWriter proves the
// mandatory "confirm-read loses to a competing writer and stands down"
// case with a REAL concurrent race, not a scripted sequence: two goroutines
// call ClaimWindowOwnership against the very same unclaimed window at the
// same time. Because the read-then-write is not one atomic tmux operation,
// either goroutine's write can land in between the other's read and its own
// write; the confirm-read is what a naive "write then trust it" attempt
// lacks, and it is what makes exactly one goroutine win here, deterministically,
// with no thrash: the loser's confirm-read observes the winner's claim
// (which is live, since both share this test's own pid) and stands down
// rather than looping forever or both believing they won.
func TestClaimWindowOwnershipConfirmReadLosesToACompetingWriter(t *testing.T) {
	socket := fmt.Sprintf("deck-ownership-race-%d-%d", os.Getpid(), time.Now().UnixNano())
	cleanup := newBareOwnershipSession(t, socket, "s0")
	defer cleanup()
	client := Client{Socket: socket, Timeout: 3 * time.Second}

	type result struct {
		ownership *WindowOwnership
		owned     bool
		err       error
	}
	results := make(chan result, 2)
	start := make(chan struct{})
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			ownership, owned, err := client.ClaimWindowOwnership(context.Background(), "s0")
			results <- result{ownership, owned, err}
		}()
	}
	close(start)

	var wins, standDowns int
	var winner *WindowOwnership
	for i := 0; i < 2; i++ {
		r := <-results
		if r.err != nil {
			t.Fatalf("concurrent claim attempt %d: %v", i, r.err)
		}
		if r.owned {
			wins++
			winner = r.ownership
			if r.ownership == nil {
				t.Fatalf("attempt %d: owned=true but ownership=nil", i)
			}
		} else {
			standDowns++
			if r.ownership != nil {
				t.Fatalf("attempt %d: owned=false but ownership=%v", i, r.ownership)
			}
		}
	}
	if wins != 1 || standDowns != 1 {
		t.Fatalf("wins=%d standDowns=%d; want exactly one winner and one stand-down out of two concurrent claimants racing the same window", wins, standDowns)
	}
	got, err := readTmuxOptionForOwnershipTest(t, socket, "s0")
	if err != nil {
		t.Fatalf("read after concurrent race: %v", err)
	}
	if got != winner.claim {
		t.Fatalf("option = %q after the race, want the sole winner's claim %q -- a value belonging to neither would mean the race corrupted the option instead of resolving it", got, winner.claim)
	}
}

// TestReleaseUnsetsOnlyAnOwnedClaim proves Release only unsets
// OwnershipOption when it still reads exactly the claim this call made,
// leaving a later claimant's value (one that already stole from this one)
// untouched.
func TestReleaseUnsetsOnlyAnOwnedClaim(t *testing.T) {
	socket := fmt.Sprintf("deck-ownership-release-%d-%d", os.Getpid(), time.Now().UnixNano())
	cleanup := newBareOwnershipSession(t, socket, "s0")
	defer cleanup()
	client := Client{Socket: socket, Timeout: 3 * time.Second}

	ownership, owned, err := client.ClaimWindowOwnership(context.Background(), "s0")
	if err != nil || !owned {
		t.Fatalf("claim ownership: owned=%v err=%v", owned, err)
	}
	if err := ownership.Release(context.Background()); err != nil {
		t.Fatalf("release owned claim: %v", err)
	}
	got, err := readTmuxOptionInScopeForOwnershipTest(t, socket, "s0")
	if err != nil {
		t.Fatalf("read after release: %v", err)
	}
	if got.Set {
		t.Fatalf("option still set to %q after releasing an owned claim; want unset", got.Value)
	}

	// Claim again, then simulate a steal by a later claimant, then prove
	// this (now-superseded) ownership's Release is a no-op that leaves the
	// later claimant's value alone.
	ownership2, owned2, err := client.ClaimWindowOwnership(context.Background(), "s0")
	if err != nil || !owned2 {
		t.Fatalf("re-claim ownership: owned=%v err=%v", owned2, err)
	}
	stolenBy := formatOwnershipClaim("laterclaimant", os.Getpid())
	setWindowOwnershipRaw(t, socket, "s0", stolenBy)
	if err := ownership2.Release(context.Background()); err != nil {
		t.Fatalf("release superseded claim: %v", err)
	}
	after, err := readTmuxOptionForOwnershipTest(t, socket, "s0")
	if err != nil {
		t.Fatalf("read after no-op release: %v", err)
	}
	if after != stolenBy {
		t.Fatalf("option = %q after a superseded Release; want the later claimant's value %q left untouched", after, stolenBy)
	}
}

// TestPidAliveDistinguishesThisProcessFromAnUnusedPID exercises the
// kill(pid, 0) probe directly: this test process's own pid must read alive,
// and a very large, essentially-guaranteed-unused pid must read dead.
func TestPidAliveDistinguishesThisProcessFromAnUnusedPID(t *testing.T) {
	if !pidAlive(os.Getpid()) {
		t.Fatalf("pidAlive(%d) = false for this test's own live pid", os.Getpid())
	}
	if pidAlive(999999999) {
		t.Fatalf("pidAlive(999999999) = true; want an unused pid to read as dead")
	}
}

// readTmuxOptionForOwnershipTest is a thin helper returning just the value
// half of readTmuxOptionInScopeForOwnershipTest, for tests that only care
// about the value.
func readTmuxOptionForOwnershipTest(t *testing.T, socket, target string) (string, error) {
	t.Helper()
	state, err := readTmuxOptionInScopeForOwnershipTest(t, socket, target)
	if err != nil {
		return "", err
	}
	return state.Value, nil
}

func readTmuxOptionInScopeForOwnershipTest(t *testing.T, socket, target string) (windowOwnershipState, error) {
	t.Helper()
	client := Client{Socket: socket, Timeout: 3 * time.Second}
	return client.readWindowOwnership(context.Background(), target)
}
