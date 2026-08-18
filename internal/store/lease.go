package store

import (
	"context"
	"database/sql"
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

// LaunchLeaseOutcome distinguishes a successful lease acquisition from one
// held (and not currently breakable) by another owner.
type LaunchLeaseOutcome int

const (
	// LaunchLeaseAcquired means the caller now owns the lease and the row's
	// status has been flipped from stopped to starting in the same
	// transaction.
	LaunchLeaseAcquired LaunchLeaseOutcome = iota
	// LaunchLeaseHeldElsewhere means another (live, in-TTL) owner holds the
	// lease, or the row was not in a leasable (stopped) status at all; the
	// row was not modified.
	LaunchLeaseHeldElsewhere
)

// LaunchLeaseResult reports the outcome of AcquireLaunchLease and, when the
// lease was not acquired, who (if anyone) currently holds it.
type LaunchLeaseResult struct {
	Outcome    LaunchLeaseOutcome
	HeldBy     string
	HeldStatus string
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

// AcquireLaunchLease implements the SPEC §9.3 launch lease: the transaction
// that flips a stopped session to starting also CAS-acquires
// launch_lease_owner/launch_lease_until. A lease is breakable when it is
// unset, its TTL has elapsed, or its owning process is no longer alive (dead
// pid, or a pid from a previous boot). Every outcome — including a lost
// race — leaves the row in a state where a subsequent legitimate acquire can
// still succeed; no case wedges it.
func (s *Store) AcquireLaunchLease(ctx context.Context, sessionID, owner string, ttl time.Duration) (LaunchLeaseResult, error) {
	if sessionID == "" {
		return LaunchLeaseResult{}, errors.New("session id is required")
	}
	if owner == "" {
		return LaunchLeaseResult{}, errors.New("lease owner is required")
	}
	if ttl <= 0 {
		ttl = DefaultLaunchLeaseTTL
	}
	now := time.Now()
	nowMillis := now.UnixMilli()
	until := now.Add(ttl).UnixMilli()

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

	if status != "stopped" {
		return LaunchLeaseResult{Outcome: LaunchLeaseHeldElsewhere, HeldBy: curOwner.String, HeldStatus: status}, nil
	}

	breakable := !curOwner.Valid || curOwner.String == "" ||
		curUntil <= nowMillis || !leaseOwnerAlive(curOwner.String)
	if !breakable {
		return LaunchLeaseResult{Outcome: LaunchLeaseHeldElsewhere, HeldBy: curOwner.String, HeldStatus: status}, nil
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
		 SET status = 'starting', launch_lease_owner = ?, launch_lease_until = ?
		 WHERE id = ? AND status = 'stopped'
		   AND launch_lease_owner IS ?
		   AND launch_lease_until = ?`,
		owner, until, sessionID, ownerMatch, curUntil)
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
		VALUES (?, ?, ?, ?, ?)`, sessionID, nowMillis, "launch_lease_acquired", "user", owner); err != nil {
		return LaunchLeaseResult{}, fmt.Errorf("record launch lease event: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return LaunchLeaseResult{}, fmt.Errorf("commit launch lease acquisition: %w", err)
	}
	return LaunchLeaseResult{Outcome: LaunchLeaseAcquired, HeldBy: owner}, nil
}
