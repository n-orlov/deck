// force_ownership_test.go proves task 102's four contested-window
// properties of Client.ForceClaimWindowOwnership (task 101, SPEC.md
// §11.9's force override) against a REAL tmux server on a private socket,
// the same shape ownership_test.go already uses for ClaimWindowOwnership:
//
//  1. a force claim over a LIVE owner acquires anyway -- ForceClaim never
//     reads the option first and never consults pidAlive, unlike the
//     ordinary claim it overrides.
//  2. a force claim whose confirm-read is overwritten by a genuinely
//     concurrent second writer returns acquired=false and err==nil -- a
//     lost single-shot race is not a transport failure.
//  3. the loser's Release (a force claim later superseded by a second one)
//     leaves the winner's option value intact -- Release only ever unsets
//     what it confirms is still its own claim.
//  4. after two force claims the window's OwnershipOption holds EXACTLY
//     one value -- the second claim's, parseable, carrying this process's
//     own pid, with no residue of the first.
package tmux

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

// TestForceClaimWindowOwnershipAcquiresOverALiveOwner proves property 1:
// a competing claim tagged with a pid that answers a signal-0 probe (this
// test process's own pid, guaranteed alive) is exactly the case ordinary
// ClaimWindowOwnership stands down for -- ForceClaimWindowOwnership must
// overwrite it anyway, unconditionally, and acquire.
func TestForceClaimWindowOwnershipAcquiresOverALiveOwner(t *testing.T) {
	socket := fmt.Sprintf("deck-force-live-%d-%d", os.Getpid(), time.Now().UnixNano())
	cleanup := newBareOwnershipSession(t, socket, "s0")
	defer cleanup()
	client := Client{Socket: socket, Timeout: 3 * time.Second}

	competing := formatOwnershipClaim("liveforceowner", os.Getpid())
	setWindowOwnershipRaw(t, socket, "s0", competing)
	if !pidAlive(os.Getpid()) {
		t.Fatalf("test's own pid %d reads as dead; the fixture is broken", os.Getpid())
	}

	ownership, owned, err := client.ForceClaimWindowOwnership(context.Background(), "s0")
	if err != nil {
		t.Fatalf("force claim over a live owner: %v", err)
	}
	if !owned || ownership == nil {
		t.Fatalf("owned = %v, ownership = %v; want a force claim to acquire over a LIVE owner", owned, ownership)
	}
	got, err := readTmuxOptionForOwnershipTest(t, socket, "s0")
	if err != nil {
		t.Fatalf("read after force claim: %v", err)
	}
	if got != ownership.claim {
		t.Fatalf("option = %q after force claim, want the new claim %q", got, ownership.claim)
	}
	if got == competing {
		t.Fatalf("option still reads the live owner's claim %q; force claim did not overwrite it", competing)
	}
}

// TestForceClaimWindowOwnershipConfirmReadLosesToACompetingWriter proves
// property 2 with a REAL concurrent race, not a scripted sequence: two
// goroutines call ForceClaimWindowOwnership against the very same window at
// the same time. ForceClaimWindowOwnership never retries and never checks
// liveness, so the outcome of any one round genuinely depends on how the
// two goroutines' write/confirm-read pairs interleave on the wire --
// running fully sequentially (no interleave) lets BOTH acquire, since each
// one's own confirm-read then only ever sees its own just-written value.
// Only a round where the two calls' operations truly interleave (the
// eventual loser's own confirm-read lands strictly after the eventual
// winner's write) exercises the property this test proves, so this test
// repeats fresh rounds against a real server until it observes one, and
// fails only if no round out of a generous budget ever does.
func TestForceClaimWindowOwnershipConfirmReadLosesToACompetingWriter(t *testing.T) {
	socket := fmt.Sprintf("deck-force-race-%d-%d", os.Getpid(), time.Now().UnixNano())
	cleanup := newBareOwnershipSession(t, socket, "s0")
	defer cleanup()
	client := Client{Socket: socket, Timeout: 3 * time.Second}

	type result struct {
		ownership *WindowOwnership
		owned     bool
		err       error
	}

	const maxRounds = 80
	for round := 0; round < maxRounds; round++ {
		results := make(chan result, 2)
		start := make(chan struct{})
		for i := 0; i < 2; i++ {
			go func() {
				<-start
				ownership, owned, err := client.ForceClaimWindowOwnership(context.Background(), "s0")
				results <- result{ownership, owned, err}
			}()
		}
		close(start)

		r1 := <-results
		r2 := <-results
		if r1.err != nil {
			t.Fatalf("round %d: force claim attempt: %v", round, r1.err)
		}
		if r2.err != nil {
			t.Fatalf("round %d: force claim attempt: %v", round, r2.err)
		}

		var winner, loser result
		switch {
		case r1.owned && !r2.owned:
			winner, loser = r1, r2
		case r2.owned && !r1.owned:
			winner, loser = r2, r1
		default:
			// Either both acquired (a fully sequential round, no
			// interleave) or -- never expected by the algorithm's own
			// design, since the chronologically last writer's own
			// confirm-read always sees its own value -- both lost.
			// Neither exercises the interleaved-loss property; try again.
			continue
		}
		if loser.ownership != nil {
			t.Fatalf("round %d: losing force claim returned owned=false but ownership=%v, want nil", round, loser.ownership)
		}
		if winner.ownership == nil {
			t.Fatalf("round %d: winning force claim returned owned=true but ownership=nil", round)
		}
		got, err := readTmuxOptionForOwnershipTest(t, socket, "s0")
		if err != nil {
			t.Fatalf("round %d: read after the race: %v", round, err)
		}
		if got != winner.ownership.claim {
			t.Fatalf("round %d: option = %q after the race, want the sole winner's claim %q", round, got, winner.ownership.claim)
		}
		return
	}
	t.Fatalf("no round out of %d produced a genuine interleaved loss (one confirm-read overwritten by the other writer); widen maxRounds", maxRounds)
}

// TestForceClaimWindowOwnershipLoserReleaseLeavesWinnerIntact proves
// property 3: a force claim that won its own round (its confirm-read
// matched its own write) but was later superseded by a second, later force
// claim is a "loser" with respect to the window's CURRENT state. Calling
// Release on that superseded claim must be a no-op -- Release only ever
// unsets the option when it still reads exactly the claim it made -- so
// the later, winning claim's value is left untouched on the wire.
func TestForceClaimWindowOwnershipLoserReleaseLeavesWinnerIntact(t *testing.T) {
	socket := fmt.Sprintf("deck-force-release-%d-%d", os.Getpid(), time.Now().UnixNano())
	cleanup := newBareOwnershipSession(t, socket, "s0")
	defer cleanup()
	client := Client{Socket: socket, Timeout: 3 * time.Second}

	loserClaim, loserOwned, err := client.ForceClaimWindowOwnership(context.Background(), "s0")
	if err != nil || !loserOwned || loserClaim == nil {
		t.Fatalf("first force claim: owned=%v ownership=%v err=%v", loserOwned, loserClaim, err)
	}

	winnerClaim, winnerOwned, err := client.ForceClaimWindowOwnership(context.Background(), "s0")
	if err != nil || !winnerOwned || winnerClaim == nil {
		t.Fatalf("second (superseding) force claim: owned=%v ownership=%v err=%v", winnerOwned, winnerClaim, err)
	}
	if winnerClaim.claim == loserClaim.claim {
		t.Fatalf("second force claim's value %q equals the first's; the two claims must differ (fresh tag per call)", winnerClaim.claim)
	}

	if err := loserClaim.Release(context.Background()); err != nil {
		t.Fatalf("release the superseded (loser) claim: %v", err)
	}

	got, err := readTmuxOptionForOwnershipTest(t, socket, "s0")
	if err != nil {
		t.Fatalf("read after the superseded release: %v", err)
	}
	if got != winnerClaim.claim {
		t.Fatalf("option = %q after releasing the superseded claim; want the winner's claim %q left untouched", got, winnerClaim.claim)
	}
}

// TestForceClaimWindowOwnershipTwoClaimsHoldExactlyOneValue proves
// property 4: two successive force claims against the same window leave
// OwnershipOption holding exactly one value -- the second claim's, in the
// documented `<tag>:<pid>` shape, carrying this process's own pid -- never
// a concatenation, a residue of the first claim, or anything else stale.
func TestForceClaimWindowOwnershipTwoClaimsHoldExactlyOneValue(t *testing.T) {
	socket := fmt.Sprintf("deck-force-single-value-%d-%d", os.Getpid(), time.Now().UnixNano())
	cleanup := newBareOwnershipSession(t, socket, "s0")
	defer cleanup()
	client := Client{Socket: socket, Timeout: 3 * time.Second}

	first, firstOwned, err := client.ForceClaimWindowOwnership(context.Background(), "s0")
	if err != nil || !firstOwned || first == nil {
		t.Fatalf("first force claim: owned=%v ownership=%v err=%v", firstOwned, first, err)
	}
	second, secondOwned, err := client.ForceClaimWindowOwnership(context.Background(), "s0")
	if err != nil || !secondOwned || second == nil {
		t.Fatalf("second force claim: owned=%v ownership=%v err=%v", secondOwned, second, err)
	}

	state, err := readTmuxOptionInScopeForOwnershipTest(t, socket, "s0")
	if err != nil {
		t.Fatalf("read after two force claims: %v", err)
	}
	if !state.Set {
		t.Fatalf("option is unset after two force claims; want exactly the second claim's value set")
	}
	if state.Value != second.claim {
		t.Fatalf("option = %q after two force claims, want exactly the second claim's value %q", state.Value, second.claim)
	}
	if state.Value == first.claim {
		t.Fatalf("option = %q still equals the first claim's value; the second force claim did not overwrite it", state.Value)
	}
	tag, pid, ok := parseOwnershipClaim(state.Value)
	if !ok {
		t.Fatalf("option value %q does not parse as a single <tag>:<pid> claim", state.Value)
	}
	if pid != os.Getpid() {
		t.Fatalf("parsed pid = %d, want this process's own pid %d", pid, os.Getpid())
	}
	if tag == "" {
		t.Fatalf("parsed tag is empty in %q", state.Value)
	}
}
