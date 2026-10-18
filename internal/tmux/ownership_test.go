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
func newBareOwnershipSession(t *testing.T, socket string) (cleanup func()) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if output, err := exec.CommandContext(ctx, "tmux", "-L", socket, "new-session", "-d", "-s", "s0", "-x", "80", "-y", "24").CombinedOutput(); err != nil {
		t.Fatalf("start bare tmux session: %v: %s", err, output)
	}
	return func() {
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer closeCancel()
		_ = exec.CommandContext(closeCtx, "tmux", "-L", socket, "kill-server").Run()
	}
}

func setWindowOwnershipRaw(t *testing.T, socket, value string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if output, err := exec.CommandContext(ctx, "tmux", "-L", socket, "set-option", "-w", "-t", "s0", OwnershipOption, value).CombinedOutput(); err != nil {
		t.Fatalf("set-option -w -t %s %s %s: %v: %s", "s0", OwnershipOption, value, err, output)
	}
}

// TestClaimWindowOwnershipAcquiresOnAnUnclaimedWindow proves the base case:
// an unclaimed window's confirm-read matches what was just written, and the
// value on the wire is the documented `<tag>:<pid>` form carrying this
// process's own pid.
func TestClaimWindowOwnershipAcquiresOnAnUnclaimedWindow(t *testing.T) {
	socket := fmt.Sprintf("priv-ownership-fresh-%d-%d", os.Getpid(), time.Now().UnixNano())
	cleanup := newBareOwnershipSession(t, socket)
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
	got, err := readTmuxOptionForOwnershipTest(t, socket)
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
	socket := fmt.Sprintf("priv-ownership-live-%d-%d", os.Getpid(), time.Now().UnixNano())
	cleanup := newBareOwnershipSession(t, socket)
	defer cleanup()
	client := Client{Socket: socket, Timeout: 3 * time.Second}

	competing := formatOwnershipClaim("livecompetitor", os.Getpid())
	setWindowOwnershipRaw(t, socket, competing)

	ownership, owned, err := client.ClaimWindowOwnership(context.Background(), "s0")
	if err != nil {
		t.Fatalf("claim against a live competing owner: %v", err)
	}
	if owned || ownership != nil {
		t.Fatalf("owned = %v, ownership = %v; want stand-down against a live competing owner", owned, ownership)
	}
	got, err := readTmuxOptionForOwnershipTest(t, socket)
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
	socket := fmt.Sprintf("priv-ownership-dead-%d-%d", os.Getpid(), time.Now().UnixNano())
	cleanup := newBareOwnershipSession(t, socket)
	defer cleanup()
	client := Client{Socket: socket, Timeout: 3 * time.Second}

	deadPID := 999999999
	if pidAlive(deadPID) {
		t.Fatalf("test's chosen dead pid %d is alive; pick another", deadPID)
	}
	setWindowOwnershipRaw(t, socket, formatOwnershipClaim("deadowner", deadPID))

	ownership, owned, err := client.ClaimWindowOwnership(context.Background(), "s0")
	if err != nil {
		t.Fatalf("claim against a dead competing owner: %v", err)
	}
	if !owned || ownership == nil {
		t.Fatalf("owned = %v, ownership = %v; want a dead owner's claim to be stolen", owned, ownership)
	}
	got, err := readTmuxOptionForOwnershipTest(t, socket)
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
// mandatory "a competing writer wins and the loser stands down" case with a
// REAL concurrent race, not a scripted sequence: two goroutines call
// ClaimWindowOwnership against the very same unclaimed window at the same
// time. Both may read the option as unset before either writes. A
// read-then-write-then-confirm-read protocol does NOT resolve that: if one
// claimant writes and confirm-reads before the other writes, the other
// overwrites it and confirm-reads its own value too, and both win (nightly
// CI caught exactly that, wins=2). What makes exactly one goroutine win is
// that the write is a single tmux compare-and-set against the value each
// claimant read: the tmux server applies only the first; the loser's
// comparison fails, its confirm-read shows the winner's claim, and its next
// read finds that claim live (both share this test's own pid) and stands
// down rather than looping or both believing they won.
func TestClaimWindowOwnershipConfirmReadLosesToACompetingWriter(t *testing.T) {
	socket := fmt.Sprintf("priv-ownership-race-%d-%d", os.Getpid(), time.Now().UnixNano())
	cleanup := newBareOwnershipSession(t, socket)
	defer cleanup()
	client := Client{Socket: socket, Timeout: 3 * time.Second}
	raceClaimantsOnce(t, client, socket, 2)
}

// TestClaimWindowOwnershipIsExclusiveUnderStress repeats the concurrent race
// many times with more claimants, releasing between rounds, so a protocol
// that only usually resolves the race (as the old read/write/confirm-read
// one did) fails here reliably instead of once in a while on nightly CI.
func TestClaimWindowOwnershipIsExclusiveUnderStress(t *testing.T) {
	socket := fmt.Sprintf("priv-ownership-stress-%d-%d", os.Getpid(), time.Now().UnixNano())
	cleanup := newBareOwnershipSession(t, socket)
	defer cleanup()
	client := Client{Socket: socket, Timeout: 3 * time.Second}
	const rounds, claimants = 50, 4
	for round := 0; round < rounds; round++ {
		winner := raceClaimantsOnce(t, client, socket, claimants)
		if err := winner.Release(context.Background()); err != nil {
			t.Fatalf("round %d: release winner: %v", round, err)
		}
		if got, err := client.readWindowOwnership(context.Background(), "s0"); err != nil || got.Set {
			t.Fatalf("round %d: option after release = %+v, err=%v; want unset", round, got, err)
		}
	}
}

// raceClaimantsOnce starts n goroutines that all call ClaimWindowOwnership on
// s0 at once and asserts exactly one acquired, every other one stood down
// without error, and the option holds the sole winner's claim. It returns
// the winner.
func raceClaimantsOnce(t *testing.T, client Client, socket string, n int) *WindowOwnership {
	t.Helper()
	type result struct {
		ownership *WindowOwnership
		owned     bool
		err       error
	}
	results := make(chan result, n)
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		go func() {
			<-start
			ownership, owned, err := client.ClaimWindowOwnership(context.Background(), "s0")
			results <- result{ownership, owned, err}
		}()
	}
	close(start)

	var wins, standDowns int
	var winner *WindowOwnership
	for i := 0; i < n; i++ {
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
	if wins != 1 || standDowns != n-1 {
		t.Fatalf("wins=%d standDowns=%d; want exactly one winner and %d stand-downs out of %d concurrent claimants racing the same window", wins, standDowns, n-1, n)
	}
	got, err := readTmuxOptionForOwnershipTest(t, socket)
	if err != nil {
		t.Fatalf("read after concurrent race: %v", err)
	}
	if got != winner.claim {
		t.Fatalf("option = %q after the race, want the sole winner's claim %q -- a value belonging to neither would mean the race corrupted the option instead of resolving it", got, winner.claim)
	}
	return winner
}

// TestClaimWindowOwnershipStealsAnUnparseableValue proves the
// compare-and-set's format escaping: a hand-edited value full of characters
// that mean something to tmux's format and command parsers (`#`, `,`, `}`,
// `{`, quotes, `;`) is still matched exactly and stolen, exactly as the old
// unconditional write stole it.
func TestClaimWindowOwnershipStealsAnUnparseableValue(t *testing.T) {
	socket := fmt.Sprintf("priv-ownership-garbage-%d-%d", os.Getpid(), time.Now().UnixNano())
	cleanup := newBareOwnershipSession(t, socket)
	defer cleanup()
	client := Client{Socket: socket, Timeout: 3 * time.Second}

	setWindowOwnershipRaw(t, socket, `x,y}z#{w"q'r;s #,`)
	ownership, owned, err := client.ClaimWindowOwnership(context.Background(), "s0")
	if err != nil || !owned {
		t.Fatalf("claim over an unparseable value: owned=%v err=%v", owned, err)
	}
	got, err := readTmuxOptionForOwnershipTest(t, socket)
	if err != nil {
		t.Fatalf("read after steal: %v", err)
	}
	if got != ownership.claim {
		t.Fatalf("option = %q after steal, want %q", got, ownership.claim)
	}
}

// TestReleaseUnsetsOnlyAnOwnedClaim proves Release only unsets
// OwnershipOption when it still reads exactly the claim this call made,
// leaving a later claimant's value (one that already stole from this one)
// untouched.
func TestReleaseUnsetsOnlyAnOwnedClaim(t *testing.T) {
	socket := fmt.Sprintf("priv-ownership-release-%d-%d", os.Getpid(), time.Now().UnixNano())
	cleanup := newBareOwnershipSession(t, socket)
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
	setWindowOwnershipRaw(t, socket, stolenBy)
	if err := ownership2.Release(context.Background()); err != nil {
		t.Fatalf("release superseded claim: %v", err)
	}
	after, err := readTmuxOptionForOwnershipTest(t, socket)
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
func readTmuxOptionForOwnershipTest(t *testing.T, socket string) (string, error) {
	t.Helper()
	state, err := readTmuxOptionInScopeForOwnershipTest(t, socket, "s0")
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
