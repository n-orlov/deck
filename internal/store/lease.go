package store

import (
	"context"
	cryptorand "crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// DefaultLaunchLeaseTTL is the SPEC §9.3 launch-lease lifetime: long enough
// to cover a slow agent launch, short enough that a crashed launcher does not
// wedge the row for long.
const DefaultLaunchLeaseTTL = 30 * time.Second

// LaunchLeaseOutcome distinguishes successful acquisition, a lease genuinely
// held by another live launcher, and a row whose status is not leasable.
type LaunchLeaseOutcome int

const (
	// LaunchLeaseAcquired means the caller now owns the lease and the row's
	// status has been flipped from stopped to starting in the same
	// transaction.
	LaunchLeaseAcquired LaunchLeaseOutcome = iota
	// LaunchLeaseHeldElsewhere means another live, in-TTL owner holds the
	// lease; the row was not modified.
	LaunchLeaseHeldElsewhere
	// LaunchLeaseNotLeasable means the row's status is not stopped. HeldStatus
	// reports the status observed in the acquisition transaction.
	LaunchLeaseNotLeasable
)

// LaunchLeaseResult reports the outcome of AcquireLaunchLease and, when the
// lease was not acquired, who (if anyone) currently holds it.
type LaunchLeaseResult struct {
	Outcome    LaunchLeaseOutcome
	HeldBy     string
	HeldStatus string
	// LaunchGeneration is the per-launch discriminator minted by THIS
	// acquisition (issue #11, R74); empty unless Outcome is
	// LaunchLeaseAcquired. It is persisted as the generation half of
	// launch_lease_owner, so the row always names the launch whose pane is
	// current, and the launcher passes it into the agent's environment so a
	// hook can say which launch it came from.
	LaunchGeneration string
}

// launchGenerationSep separates the "pid@boot_id" launcher identity SPEC
// §9.3 pins from the per-launch generation token R74 appends to it inside
// launch_lease_owner. The composite lives in the existing column on purpose:
// SPEC.md:243-284 pins the sessions DDL, so a discriminator needing a column
// of its own would need a spec change first.
const launchGenerationSep = "#"

// newLaunchGeneration mints a per-launch generation token: 8 bytes of
// crypto/rand as hex. Deliberately NOT a timestamp and not a counter -- two
// launches of one row from the same process at the same clock reading (deck's
// clock is injectable, and a test clock does not advance at all) must still
// get different tokens, which is the whole point of the discriminator.
func newLaunchGeneration() (string, error) {
	var raw [8]byte
	if _, err := cryptorand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("mint launch generation: %w", err)
	}
	return hex.EncodeToString(raw[:]), nil
}

// splitOwnerGeneration splits a stored launch_lease_owner into launcher
// identity and per-launch generation. An owner written before R74 (or by a
// test fixture) carries no generation and yields an empty one, so every
// identity-based check keeps working unchanged.
func splitOwnerGeneration(stored string) (identity, generation string) {
	identity, generation, found := strings.Cut(stored, launchGenerationSep)
	if !found {
		return stored, ""
	}
	return identity, generation
}

// composeLeaseOwner joins a launcher identity and a generation into the value
// written to launch_lease_owner.
func composeLeaseOwner(identity, generation string) string {
	if generation == "" {
		return identity
	}
	return identity + launchGenerationSep + generation
}

// CurrentLaunchLeaseOwner formats this process's launch-lease owner string
// per SPEC §9.3: "pid@boot_id". boot_id lets a lease from a previous boot be
// recognized as stale even if pid numbers happen to be reused after a
// restart; when the boot id cannot be determined the component is left empty
// (still internally consistent for comparisons within one boot).
func CurrentLaunchLeaseOwner() string {
	return fmt.Sprintf("%d@%s", os.Getpid(), bootID())
}

func bootID() string {
	data, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// parseLeaseOwner splits a "pid@boot_id" owner string. An owner that does not
// parse cleanly is treated as unparseable-and-therefore-stale by the caller.
func parseLeaseOwner(owner string) (pid int, boot string, ok bool) {
	// The generation suffix R74 appends is not part of the identity: left on,
	// it would make every boot id compare unequal, so every live lease would
	// look like one from a previous boot and be breakable -- exactly the
	// double-launch §9.3 exists to prevent.
	owner, _ = splitOwnerGeneration(owner)
	at := strings.LastIndex(owner, "@")
	if at < 0 {
		return 0, "", false
	}
	pidPart, bootPart := owner[:at], owner[at+1:]
	parsed, err := strconv.Atoi(pidPart)
	if err != nil || parsed <= 0 {
		return 0, "", false
	}
	return parsed, bootPart, true
}

// leaseOwnerAlive reports whether the process named by owner is plausibly
// still the one that acquired the lease: same boot (a pid from a previous
// boot cannot be the current live process, however small the number) and a
// pid that answers a signal-0 probe.
func leaseOwnerAlive(owner string) bool {
	pid, boot, ok := parseLeaseOwner(owner)
	if !ok {
		return false
	}
	if current := bootID(); current != "" && boot != current {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Unix, FindProcess never fails merely because the pid is dead; the
	// liveness check is the signal-0 probe below. Signal(syscall.Signal(0))
	// asks the kernel whether the process exists and is ours to see, without
	// actually delivering a signal.
	err = proc.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	if errors.Is(err, os.ErrProcessDone) {
		return false
	}
	// EPERM means the pid exists but is owned by someone else; deck's own
	// launcher processes always run as the current user, so a pid we cannot
	// signal is not one of ours and the lease is stale.
	return false
}

// ReleaseLaunchLease ends a launch's hold on the row's launch lease once that
// launch attempt has concluded (issue #11, R75). It clears
// launch_lease_until -- and ONLY launch_lease_until.
//
// Why the hold has to end at all: SPEC §9.3's lease exists so two clients
// cannot double-start one row, i.e. so a SECOND launcher is kept out while a
// launch is in flight. Once the pane is up (or the attempt has failed) nothing
// is in flight any more, but the ~30 s TTL keeps the row "held by <owner>" for
// the rest of the window. Any resume in that window -- including one by the
// very process that holds the lease, on a row that is legitimately stopped
// again -- was answered *starting elsewhere*, which §9.3 pins as "a claim about
// another client, so it is only made when one is actually there". There was no
// other client: the row's own last launcher was reported as one.
//
// Why launch_lease_owner is deliberately KEPT: its generation half is R74's
// per-launch discriminator (see composeLeaseOwner and
// store.Session.LaunchGeneration). The row must go on naming which launch is
// current for as long as the pane exists, because that is what lets a late hook
// write from a superseded launch be recognized -- and a superseded pane can
// chatter long after 30 s. Blanking the owner here would silently un-fix R74:
// with no token on the row, every hook is "nothing to discriminate, therefore
// apply" again. So the released state is exactly "owner names the current
// launch, nobody is mid-launch": launch_lease_until = 0 is what AcquireLaunchLease
// already reads as breakable (an unset/elapsed TTL), so no acquisition logic
// changes and no guard is weakened -- a lease held by a *different* live owner
// still has its own non-zero until and still blocks.
//
// heldOwner is the full stored owner string this launch acquired
// (LaunchLeaseResult.HeldBy, generation included) and the write CASes on it, so
// a release that arrives after some other launcher has already taken the row
// over releases nothing: it cannot cut short a lease it does not own. The
// boolean reports whether a held lease was actually released.
func (s *Store) ReleaseLaunchLease(ctx context.Context, sessionID, heldOwner string) (bool, error) {
	if sessionID == "" {
		return false, errors.New("session id is required")
	}
	if heldOwner == "" {
		return false, errors.New("lease owner is required")
	}
	result, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET launch_lease_until = 0
		 WHERE id = ? AND launch_lease_owner = ? AND launch_lease_until != 0`,
		sessionID, heldOwner)
	if err != nil {
		return false, fmt.Errorf("release launch lease: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("check launch lease release: %w", err)
	}
	return affected == 1, nil
}

// AcquireLaunchLease implements the SPEC §9.3 launch lease: the transaction
// that flips a stopped session to starting also CAS-acquires
// launch_lease_owner/launch_lease_until and clears killed_by_user: an explicit
// resume is the user action that releases the terminal kill guard. It clears
// pane_exit_status and crash_tail on exactly the same rationale (SPEC.md:728,
// issue #9): both describe a pane that resume is about to replace, and both
// carry control-flow meaning rather than mere diagnostics -- the reconciler
// takes no liveness verdict from a row whose pane_exit_status is set
// (internal/service/reconcile.go), and UpdateSessionStatus drops a hook write
// of `running` while it is set (store.go's crash-verdict guard). Left behind
// across a resume they froze three of the operator's live sessions out of
// reconciliation permanently, one for 30+ hours.
//
// crash_tail is cleared, not retained: it is the text of a pane that no longer
// exists, so keeping it would caption a NEW pane with the last words of the old
// one, and the column's mere presence has control-flow meaning. The crash
// itself stays on the record where forensics belong -- the `tmux.pane_dead`
// event names the exit status in its reason -- rather than in a session column
// the reconciler reads.
//
// A lease is breakable when it is unset, its TTL has elapsed, or its owning
// process is no longer alive (dead
// pid, or a pid from a previous boot). Every outcome — including a lost
// race — leaves the row in a state where a subsequent legitimate acquire can
// still succeed; no case wedges it.
//
// Each acquisition also mints a fresh per-launch generation token (issue #11,
// R74) and stores it as the generation half of launch_lease_owner
// ("pid@boot_id#generation"), returning it in
// LaunchLeaseResult.LaunchGeneration. The row therefore always names the
// launch whose pane is the current one, which is what lets a late hook write
// from a superseded launch be recognized as superseded: the launcher hands
// the token to the agent's environment, the hook hands it back, and a token
// that no longer matches the row belongs to a pane deck has already replaced.
// The token is random, not a timestamp: two launches of one row can share a
// clock reading (deck's clock is injectable) but must never share a token.
func (s *Store) AcquireLaunchLease(ctx context.Context, sessionID, owner string, ttl time.Duration, at int64) (LaunchLeaseResult, error) {
	if sessionID == "" {
		return LaunchLeaseResult{}, errors.New("session id is required")
	}
	if owner == "" {
		return LaunchLeaseResult{}, errors.New("lease owner is required")
	}
	if at == 0 {
		return LaunchLeaseResult{}, errors.New("launch lease timestamp is required")
	}
	if ttl <= 0 {
		ttl = DefaultLaunchLeaseTTL
	}
	until := at + ttl.Milliseconds()

	// Mint the per-launch generation before the transaction: it identifies
	// THIS launch attempt, and a failure to produce one must not leave a
	// half-acquired lease behind (issue #11, R74).
	generation, err := newLaunchGeneration()
	if err != nil {
		return LaunchLeaseResult{}, err
	}
	storedOwner := composeLeaseOwner(owner, generation)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return LaunchLeaseResult{}, fmt.Errorf("begin acquire launch lease: %w", err)
	}
	defer tx.Rollback()

	var status string
	var curOwner sql.NullString
	var curUntil int64
	row := tx.QueryRowContext(ctx,
		`SELECT status, launch_lease_owner, launch_lease_until FROM sessions WHERE id = ?`, sessionID)
	if err := row.Scan(&status, &curOwner, &curUntil); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return LaunchLeaseResult{}, fmt.Errorf("session %q not found", sessionID)
		}
		return LaunchLeaseResult{}, fmt.Errorf("read launch lease: %w", err)
	}

	leaseHeld := curOwner.Valid && curOwner.String != "" &&
		curUntil > at && leaseOwnerAlive(curOwner.String)
	if leaseHeld {
		return LaunchLeaseResult{Outcome: LaunchLeaseHeldElsewhere, HeldBy: curOwner.String, HeldStatus: status}, nil
	}
	if status != "stopped" {
		return LaunchLeaseResult{Outcome: LaunchLeaseNotLeasable, HeldBy: curOwner.String, HeldStatus: status}, nil
	}

	// CAS on the exact previously-observed owner/until pair: within this one
	// transaction nothing else can have changed them (SQLite serializes
	// writers), and a mismatch here can only mean the row was no longer
	// "stopped" by the time we tried to write it, which the status clause
	// below already covers directly.
	var ownerMatch any
	if curOwner.Valid {
		ownerMatch = curOwner.String
	} else {
		ownerMatch = nil
	}
	result, err := tx.ExecContext(ctx,
		`UPDATE sessions
		 SET status = 'starting', killed_by_user = 0,
		     pane_exit_status = NULL, crash_tail = NULL,
		     launch_lease_owner = ?, launch_lease_until = ?
		 WHERE id = ? AND status = 'stopped'
		   AND launch_lease_owner IS ?
		   AND launch_lease_until = ?`,
		storedOwner, until, sessionID, ownerMatch, curUntil)
	if err != nil {
		return LaunchLeaseResult{}, fmt.Errorf("acquire launch lease: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return LaunchLeaseResult{}, fmt.Errorf("check launch lease acquisition: %w", err)
	}
	if affected != 1 {
		// Lost a race with a concurrent acquirer between our read and our
		// write; the row is untouched by us and remains usable by whoever
		// won, or by a later legitimate acquire.
		return LaunchLeaseResult{Outcome: LaunchLeaseHeldElsewhere}, nil
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO events (session_id, at, kind, reason, payload)
		VALUES (?, ?, ?, ?, ?)`, sessionID, at, "launch_lease_acquired", "user", storedOwner); err != nil {
		return LaunchLeaseResult{}, fmt.Errorf("record launch lease event: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return LaunchLeaseResult{}, fmt.Errorf("commit launch lease acquisition: %w", err)
	}
	return LaunchLeaseResult{Outcome: LaunchLeaseAcquired, HeldBy: storedOwner, LaunchGeneration: generation}, nil
}
