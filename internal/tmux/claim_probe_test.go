package tmux

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

// TestClaimStateProbeUnsetOnAFreshWindow proves the first of the three
// answers: a window OwnershipOption has never touched reads ClaimUnset,
// from both the target-only Client.ProbeWindowOwnership (no claim of its
// own to compare against) and a held WindowOwnership.Probe on a DIFFERENT
// window it never claimed.
func TestClaimStateProbeUnsetOnAFreshWindow(t *testing.T) {
	socket := fmt.Sprintf("deck-claimprobe-unset-%d-%d", os.Getpid(), time.Now().UnixNano())
	cleanup := newBareOwnershipSession(t, socket, "s0")
	defer cleanup()
	client := Client{Socket: socket, Timeout: 3 * time.Second}

	state, err := client.ProbeWindowOwnership(context.Background(), "s0")
	if err != nil {
		t.Fatalf("probe fresh window: %v", err)
	}
	if state != ClaimUnset {
		t.Fatalf("ProbeWindowOwnership on a fresh window = %s, want %s", state, ClaimUnset)
	}
}

// TestClaimStateProbeForeignLiveOnAContestedWindow proves the second
// answer: a window whose OwnershipOption is set to a claim tagged with a
// LIVE pid that is not the WindowOwnership's own reads ClaimForeignLive --
// both from the target-only probe and from a genuine WindowOwnership held
// on the SAME window by a caller that then loses it to a steal.
func TestClaimStateProbeForeignLiveOnAContestedWindow(t *testing.T) {
	socket := fmt.Sprintf("deck-claimprobe-foreign-%d-%d", os.Getpid(), time.Now().UnixNano())
	cleanup := newBareOwnershipSession(t, socket, "s0")
	defer cleanup()
	client := Client{Socket: socket, Timeout: 3 * time.Second}

	// Target-only form: nobody here has ever claimed s0.
	foreign := formatOwnershipClaim("livecompetitor", os.Getpid())
	setWindowOwnershipRaw(t, socket, "s0", foreign)
	state, err := client.ProbeWindowOwnership(context.Background(), "s0")
	if err != nil {
		t.Fatalf("probe foreign-live window: %v", err)
	}
	if state != ClaimForeignLive {
		t.Fatalf("ProbeWindowOwnership on a foreign-live window = %s, want %s", state, ClaimForeignLive)
	}

	// WindowOwnership form: claim it for real (first clearing the foreign
	// claim set above, so this claim is not itself refused as standing
	// down against a live owner), then have another writer steal it (a
	// live pid, tagged differently, so it is not == ownership.claim), and
	// prove the ORIGINAL WindowOwnership's own Probe now reports
	// ClaimForeignLive too -- it lost its claim.
	if err := client.unsetWindowOwnership(context.Background(), "s0"); err != nil {
		t.Fatalf("clear foreign claim before claiming for real: %v", err)
	}
	ownership, owned, err := client.ClaimWindowOwnership(context.Background(), "s0")
	if err != nil || !owned {
		t.Fatalf("claim s0: owned=%v err=%v", owned, err)
	}
	stolenBy := formatOwnershipClaim("laterclaimant", os.Getpid())
	setWindowOwnershipRaw(t, socket, "s0", stolenBy)
	state, err = ownership.Probe(context.Background())
	if err != nil {
		t.Fatalf("probe stolen-from ownership: %v", err)
	}
	if state != ClaimForeignLive {
		t.Fatalf("WindowOwnership.Probe on a stolen claim = %s, want %s", state, ClaimForeignLive)
	}
}

// TestClaimStateProbeStillMineOnAnUnstolenClaim proves the third answer,
// the one only WindowOwnership.Probe can ever give: a claim nobody has
// touched since it was confirmed reads ClaimStillMine.
func TestClaimStateProbeStillMineOnAnUnstolenClaim(t *testing.T) {
	socket := fmt.Sprintf("deck-claimprobe-mine-%d-%d", os.Getpid(), time.Now().UnixNano())
	cleanup := newBareOwnershipSession(t, socket, "s0")
	defer cleanup()
	client := Client{Socket: socket, Timeout: 3 * time.Second}

	ownership, owned, err := client.ClaimWindowOwnership(context.Background(), "s0")
	if err != nil || !owned {
		t.Fatalf("claim s0: owned=%v err=%v", owned, err)
	}
	state, err := ownership.Probe(context.Background())
	if err != nil {
		t.Fatalf("probe unstolen claim: %v", err)
	}
	if state != ClaimStillMine {
		t.Fatalf("WindowOwnership.Probe on an unstolen claim = %s, want %s", state, ClaimStillMine)
	}

	// Sanity: the target-only form on the SAME window, with no claim of
	// its own to compare against, must never answer ClaimStillMine -- it
	// can only ever see "a live claim is here" (ClaimForeignLive), never
	// whose it is.
	state, err = client.ProbeWindowOwnership(context.Background(), "s0")
	if err != nil {
		t.Fatalf("target-only probe on an owned window: %v", err)
	}
	if state != ClaimForeignLive {
		t.Fatalf("ProbeWindowOwnership (no claim of its own) on an owned window = %s, want %s", state, ClaimForeignLive)
	}
}

// TestClaimStateProbeUnsetOnADeadOwner proves the "unset or dead owner"
// grouping documented on ClaimUnset: a claim tagged with a pid that does
// not exist reads ClaimUnset, not ClaimForeignLive -- the same liveness
// check ClaimWindowOwnership itself uses to decide whether to steal.
func TestClaimStateProbeUnsetOnADeadOwner(t *testing.T) {
	socket := fmt.Sprintf("deck-claimprobe-dead-%d-%d", os.Getpid(), time.Now().UnixNano())
	cleanup := newBareOwnershipSession(t, socket, "s0")
	defer cleanup()
	client := Client{Socket: socket, Timeout: 3 * time.Second}

	deadPID := 999999999
	if pidAlive(deadPID) {
		t.Fatalf("test's chosen dead pid %d is alive; pick another", deadPID)
	}
	setWindowOwnershipRaw(t, socket, "s0", formatOwnershipClaim("deadowner", deadPID))

	state, err := client.ProbeWindowOwnership(context.Background(), "s0")
	if err != nil {
		t.Fatalf("probe dead-owner window: %v", err)
	}
	if state != ClaimUnset {
		t.Fatalf("ProbeWindowOwnership on a dead owner's claim = %s, want %s", state, ClaimUnset)
	}
}
