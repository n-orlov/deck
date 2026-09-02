package tmux

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// OwnershipOption is the window-scoped user option interactive mode claims
// before it acts on a tmux window (PRD phase3b II-14, SPEC.md §11.9). It is
// a WINDOW option, never a global or session one: two different deck
// sessions selecting two different windows must be able to hold ownership
// simultaneously, and scoping to the window is what makes that possible.
const OwnershipOption = "@deck_isize_owner"

// maxOwnershipClaimAttempts bounds the read/write/confirm-read loop in
// ClaimWindowOwnership. It is not a retry-until-it-works loop for a live
// contended option -- a live owner is respected on its very first read,
// before this call ever writes anything -- it only bounds how many times a
// write can be raced away by a concurrent writer between this call's own
// write and its confirm-read (each loss loops back to a fresh read, which
// validates the winning writer's liveness in turn). A small fixed bound,
// not a timer, is enough to make that converge; there is deliberately no
// heartbeat and no TTL anywhere in this file (every writer of a tmux socket
// runs on the socket's own host, so liveness is a kill(pid, 0) syscall
// rather than a lease -- PRD II-14).
const maxOwnershipClaimAttempts = 3

// WindowOwnership is a confirmed claim to OwnershipOption on one tmux
// window, held by this process. It is returned only once
// ClaimWindowOwnership's confirm-read has proven the claim survived any
// concurrent writer; Release is the only way to give it up early (it is
// also implicitly given up, from tmux's point of view, whenever a later
// claimant's confirm-read observes this process's pid as dead).
type WindowOwnership struct {
	client Client
	target string
	claim  string
}

// Target is the tmux target this ownership was claimed against.
func (o *WindowOwnership) Target() string { return o.target }

// ownershipClaimTag returns a fresh, cryptographically random 16 hex
// character tag for one ClaimWindowOwnership call. The pid alone identifies
// which OS process holds a claim, but not which specific call inside that
// process made it; a fresh tag per attempt lets the confirm-read tell "the
// exact value I just wrote is still there" apart from "some other write
// landed here that happens to carry my own pid" (e.g. two goroutines in the
// same deck process racing to enter interactive mode on the same window).
func ownershipClaimTag() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate ownership claim tag: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// formatOwnershipClaim renders the PRD II-14 `<tag>:<pid>` form.
func formatOwnershipClaim(tag string, pid int) string {
	return fmt.Sprintf("%s:%d", tag, pid)
}

// parseOwnershipClaim splits a `<tag>:<pid>` value on its LAST colon (the
// tag is hex and never contains one, but splitting on the last occurrence
// keeps this robust the same way internal/store/lease.go's parseLeaseOwner
// does for its own not-dissimilar pid-tagged format).
func parseOwnershipClaim(value string) (tag string, pid int, ok bool) {
	at := strings.LastIndex(value, ":")
	if at < 0 {
		return "", 0, false
	}
	tagPart, pidPart := value[:at], value[at+1:]
	parsed, err := strconv.Atoi(pidPart)
	if err != nil || parsed <= 0 || tagPart == "" {
		return "", 0, false
	}
	return tagPart, parsed, true
}

// pidAlive answers tmux's II-14 "kill(pid, 0)" liveness question: does a
// process with this pid exist and belong to us. Signal(syscall.Signal(0))
// asks the kernel whether the process exists without delivering a signal --
// the same probe internal/store/lease.go's leaseOwnerAlive uses for the
// launch lease, minus that lease's extra boot-id check (there is nothing
// here as long-lived as a lease's TTL window for a pid to be reused across,
// and II-14 asks for exactly the syscall, not a lease).
func pidAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	if errors.Is(err, os.ErrProcessDone) {
		return false
	}
	// EPERM means the pid exists but is owned by someone else; deck's own
	// processes always run as the current user, so a pid we cannot signal
	// was never one of ours and its claim is stale.
	return false
}

// windowOwnershipState is a read of OwnershipOption in the window scope.
// Both of tmux's "unset" shapes for a "@"-prefixed user option collapse to
// Set == false here: `show-options -wv` on a window where it was never set
// exits non-zero with "invalid option: <name>" (unlike an unset BUILTIN
// window option, which prints an empty line and exits 0 -- task 028's
// distinction; OwnershipOption is a user option, so only the error shape
// ever applies to it, but this function does not assume that and treats an
// empty successful read as unset too).
type windowOwnershipState struct {
	Value string
	Set   bool
}

func (c Client) readWindowOwnership(ctx context.Context, target string) (windowOwnershipState, error) {
	commandCtx, cancel := context.WithTimeout(ctx, c.timeout())
	defer cancel()
	output, err := c.command(commandCtx, "show-options", "-wv", "-t", target, OwnershipOption).CombinedOutput()
	trimmed := strings.TrimRight(string(output), "\n")
	if err != nil {
		if strings.Contains(trimmed, "invalid option") {
			return windowOwnershipState{}, nil
		}
		return windowOwnershipState{}, fmt.Errorf("tmux -L %s show-options -wv -t %s %s: %w: %s", c.Socket, target, OwnershipOption, err, trimmed)
	}
	if trimmed == "" {
		return windowOwnershipState{}, nil
	}
	return windowOwnershipState{Value: trimmed, Set: true}, nil
}

func (c Client) writeWindowOwnership(ctx context.Context, target, value string) error {
	if _, err := c.run(ctx, "set-option", "-w", "-t", target, OwnershipOption, value); err != nil {
		return fmt.Errorf("claim %s on %q: %w", OwnershipOption, target, err)
	}
	return nil
}

func (c Client) unsetWindowOwnership(ctx context.Context, target string) error {
	if _, err := c.run(ctx, "set-option", "-w", "-u", "-t", target, OwnershipOption); err != nil {
		return fmt.Errorf("release %s on %q: %w", OwnershipOption, target, err)
	}
	return nil
}

// ClaimWindowOwnership implements PRD phase3b II-14 / SPEC.md §11.9's
// interactive-mode ownership protocol for one tmux window:
//
//  1. Read the option first. If it names a LIVE owner (kill(pid, 0)
//     succeeds), stand down without writing anything -- a live owner is
//     respected untouched, never raced against.
//  2. Otherwise (unset, or a DEAD owner) write this process's fresh
//     pid-tagged `<tag>:<pid>` claim.
//  3. Re-read it (the confirm-read). The write and the read are two
//     separate tmux commands, not one atomic operation, so a competing
//     writer can land between them. If the confirm-read shows exactly the
//     value just written, the claim is acquired. If it shows something
//     else, this call lost the race to a concurrent writer; loop back to
//     step 1, which will validate THAT writer's liveness in turn -- a live
//     winner is respected on the very next iteration, not thrashed against.
//
// The three returns are: the held ownership (nil unless acquired), whether
// it was acquired at all, and an error only for a genuine tmux/transport
// failure (never for losing the claim to a live owner -- that is a normal,
// error-free stand-down).
func (c Client) ClaimWindowOwnership(ctx context.Context, target string) (*WindowOwnership, bool, error) {
	tag, err := ownershipClaimTag()
	if err != nil {
		return nil, false, err
	}
	mine := formatOwnershipClaim(tag, os.Getpid())
	for attempt := 0; attempt < maxOwnershipClaimAttempts; attempt++ {
		got, err := c.readWindowOwnership(ctx, target)
		if err != nil {
			return nil, false, err
		}
		if got.Set {
			_, competingPID, ok := parseOwnershipClaim(got.Value)
			if ok && pidAlive(competingPID) {
				return nil, false, nil
			}
			// Either unparseable (a bug or a hand-edited option, never
			// something formatOwnershipClaim itself produces) or a
			// confirmed-dead owner: both are stolen from by falling through
			// to the write below.
		}
		if err := c.writeWindowOwnership(ctx, target, mine); err != nil {
			return nil, false, err
		}
		confirm, err := c.readWindowOwnership(ctx, target)
		if err != nil {
			return nil, false, err
		}
		if confirm.Set && confirm.Value == mine {
			return &WindowOwnership{client: c, target: target, claim: mine}, true, nil
		}
		// Lost the confirm-read race to a concurrent writer: loop back to
		// step 1 and let the next iteration's read validate that writer's
		// liveness instead of trusting our own just-issued write.
	}
	return nil, false, fmt.Errorf("claim %s on %q: gave up after %d attempts racing a concurrent writer", OwnershipOption, target, maxOwnershipClaimAttempts)
}

// ForceClaimWindowOwnership implements a single-shot variant of
// ClaimWindowOwnership for a force-attach path (SPEC.md §11.9's force
// override): it never reads the option first and never consults pidAlive
// -- a live owner is not respected here by design, because the caller has
// already decided to override whatever is there. It writes this process's
// fresh pid-tagged `<tag>:<pid>` claim exactly once and confirm-reads it
// exactly once, with no retry loop: if the confirm-read shows exactly the
// value just written, the claim is acquired; if it shows anything else
// (a concurrent writer landed between the write and the confirm-read),
// this call simply lost that single race and returns acquired=false with
// a nil error -- never an error, because losing a race to another writer
// is not a transport failure. An error return is reserved for a genuine
// tmux/transport failure from the write or the confirm-read themselves.
func (c Client) ForceClaimWindowOwnership(ctx context.Context, target string) (*WindowOwnership, bool, error) {
	tag, err := ownershipClaimTag()
	if err != nil {
		return nil, false, err
	}
	mine := formatOwnershipClaim(tag, os.Getpid())
	if err := c.writeWindowOwnership(ctx, target, mine); err != nil {
		return nil, false, err
	}
	confirm, err := c.readWindowOwnership(ctx, target)
	if err != nil {
		return nil, false, err
	}
	if confirm.Set && confirm.Value == mine {
		return &WindowOwnership{client: c, target: target, claim: mine}, true, nil
	}
	return nil, false, nil
}

// ClaimState is the tri-state answer to "what does OwnershipOption read on
// this window RIGHT NOW", from the point of view of a caller who may or
// may not hold a claim there (SPEC.md §11.9's R100 teardown gating, R101
// tick poll and R102 previewFit stand-down all classify a window this way
// before deciding whether to act on it).
type ClaimState int

const (
	// ClaimUnset means OwnershipOption carries no live claim: either it
	// was never set (windowOwnershipState.Set == false, tmux's "invalid
	// option" or an empty successful read) or it names a pid that is no
	// longer alive -- the same "unset or dead owner" grouping
	// ClaimWindowOwnership's own doc comment already treats as
	// equivalent (both fall through to a write there; here, both fall
	// through to this answer).
	ClaimUnset ClaimState = iota
	// ClaimForeignLive means OwnershipOption is set to a value this call
	// does not recognise as its own, naming a pid that answers
	// pidAlive -- some other live process holds the window now (most
	// obviously a ForceClaimWindowOwnership steal, but also an ordinary
	// ClaimWindowOwnership by another process).
	ClaimForeignLive
	// ClaimStillMine means OwnershipOption reads exactly the value a
	// specific WindowOwnership claimed -- nobody has stolen it. Only
	// WindowOwnership.Probe, which knows that value, can ever return
	// this; the target-only Client.ProbeWindowOwnership never does.
	ClaimStillMine
)

// String renders a ClaimState the way test failures and any future log
// line should name it.
func (s ClaimState) String() string {
	switch s {
	case ClaimUnset:
		return "unset"
	case ClaimForeignLive:
		return "foreign-live"
	case ClaimStillMine:
		return "still-mine"
	default:
		return fmt.Sprintf("ClaimState(%d)", int(s))
	}
}

// classifyWindowOwnership reads OwnershipOption on target and classifies
// it against mine (the empty string when the caller holds no claim of its
// own to compare against -- Client.ProbeWindowOwnership's case): unset (or
// a dead owner), a live claim that is not mine, or mine exactly. This is
// the one implementation shared by both exported probes below; neither
// adds a branch of its own.
func (c Client) classifyWindowOwnership(ctx context.Context, target, mine string) (ClaimState, error) {
	got, err := c.readWindowOwnership(ctx, target)
	if err != nil {
		return ClaimUnset, err
	}
	if !got.Set {
		return ClaimUnset, nil
	}
	if mine != "" && got.Value == mine {
		return ClaimStillMine, nil
	}
	_, pid, ok := parseOwnershipClaim(got.Value)
	if !ok || !pidAlive(pid) {
		return ClaimUnset, nil
	}
	return ClaimForeignLive, nil
}

// ProbeWindowOwnership answers whether OwnershipOption on target is unset
// or held by a live foreign pid, for a caller that holds no claim of its
// own on target to compare against (SPEC.md §11.9's R102: previewFit
// checks a session's window it has never claimed). It never returns
// ClaimStillMine -- there is nothing here to be "mine" -- so a caller that
// DOES hold a claim wants WindowOwnership.Probe instead.
func (c Client) ProbeWindowOwnership(ctx context.Context, target string) (ClaimState, error) {
	return c.classifyWindowOwnership(ctx, target, "")
}

// Probe answers, for this WindowOwnership's own target, which of the
// three claim states currently holds: OwnershipOption unset, held by a
// live foreign pid, or still exactly this WindowOwnership's own confirmed
// claim. It is the read-only counterpart to
// ClaimWindowOwnership/ForceClaimWindowOwnership that R100's teardown
// gating and R101's tick poll both need before acting: neither may
// restore geometry, clear @deck_isize_geometry or release ownership
// unless this reads ClaimStillMine first.
func (o *WindowOwnership) Probe(ctx context.Context) (ClaimState, error) {
	return o.client.classifyWindowOwnership(ctx, o.target, o.claim)
}

// Release gives up this ownership, unsetting OwnershipOption on its target
// -- but only if the option still reads exactly the claim this call made.
// If it does not (a later claimant already validated this process as dead
// and stole it, or Release raced a steal in progress), unsetting would
// clobber that later claim instead of merely tidying up this one, so
// Release leaves it alone.
func (o *WindowOwnership) Release(ctx context.Context) error {
	got, err := o.client.readWindowOwnership(ctx, o.target)
	if err != nil {
		return err
	}
	if !got.Set || got.Value != o.claim {
		return nil
	}
	return o.client.unsetWindowOwnership(ctx, o.target)
}
