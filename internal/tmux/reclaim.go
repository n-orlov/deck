package tmux

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// interactiveClaimFileName is the metadata file SaveInteractiveClaimRecord
// writes inside a PanePipe's own temp dir (PanePipe.TempDir), and the one
// loadInteractiveClaimRecord reads back on a later process's reclaim pass.
// It lives INSIDE the same temp dir the FIFO does, so the ordinary happy
// path (PanePipe.Close's closeLocal removing the whole tempDir) already
// deletes it for free -- there is no second place that ever needs its own
// cleanup.
const interactiveClaimFileName = "claim.json"

// InteractiveClaimRecord is everything SPEC.md §11.9's entry path (task
// 061's enterInteractive, internal/tui/interactive.go) knows about one
// interactive-mode claim at the moment it arms the pipe, and everything a
// LATER process needs to undo that claim if the process that made it never
// gets to (PRD R89: SIGKILL cannot be handled, so "the next start reclaims
// it" is the only guarantee available). PaneTarget is the target
// ArmPipePane itself was given (a pane id) -- what a bare `pipe-pane -t`
// disarms; WindowTarget and Geometry are exactly what
// Client.ClaimWindowOwnership/CaptureWindowGeometry captured at entry --
// what Client.RestoreWindowGeometry and the ownership release need.
type InteractiveClaimRecord struct {
	Socket       string         `json:"socket"`
	PaneTarget   string         `json:"pane_target"`
	WindowTarget string         `json:"window_target"`
	Geometry     WindowGeometry `json:"geometry"`
}

// interactivePipeOwnerFileName is the marker every interactive-pipe temp dir
// carries from the moment the scan can see it: the pid of the process
// arming it. The claim record (claim.json) is written only once the whole
// entry has succeeded, so between ArmPipePane creating the dir and that
// write the marker is the only thing telling a reclaim pass in another
// deck process that the dir is in use rather than leaked.
const interactivePipeOwnerFileName = "owner.pid"

// interactivePipeStagingPrefix names a pipe dir before its owner marker is
// in place. It deliberately does not start with interactivePipeTempDirPrefix,
// so ReclaimLeakedInteractivePipes never sees a dir without its marker.
const interactivePipeStagingPrefix = "deck-pipe-staging-"

// makeInteractivePipeDir creates ArmPipePane's temp dir with this process's
// owner marker already inside it: the dir is made under the staging prefix,
// the marker written, and only then renamed (atomically, within the same
// parent) to its interactive-pipe name.
func makeInteractivePipeDir() (string, error) {
	staging, err := os.MkdirTemp(interactivePipeTempRoot, interactivePipeStagingPrefix)
	if err != nil {
		return "", err
	}
	owner := []byte(strconv.Itoa(os.Getpid()))
	if err := os.WriteFile(filepath.Join(staging, interactivePipeOwnerFileName), owner, 0o600); err != nil {
		_ = os.RemoveAll(staging)
		return "", fmt.Errorf("write pipe dir owner marker: %w", err)
	}
	final := filepath.Join(filepath.Dir(staging), interactivePipeTempDirPrefix+strings.TrimPrefix(filepath.Base(staging), interactivePipeStagingPrefix))
	if err := os.Rename(staging, final); err != nil {
		_ = os.RemoveAll(staging)
		return "", fmt.Errorf("publish pipe dir: %w", err)
	}
	return final, nil
}

// interactivePipeOwnerAlive reports whether dir's owner marker names a live
// process: a dir still being armed by it, not a leaked one.
func interactivePipeOwnerAlive(dir string) bool {
	data, err := os.ReadFile(filepath.Join(dir, interactivePipeOwnerFileName)) //nolint:gosec // G304: dir passed isReclaimableInteractivePipeDir; the file name is a package constant
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	return err == nil && pid > 0 && pidAlive(pid)
}

// SaveInteractiveClaimRecord writes record as JSON into dir/claim.json.
// dir is a PanePipe's own TempDir(); the caller (enterInteractive) calls
// this once, right after StartWithTransport's ArmPipePane has succeeded,
// so a later ReclaimLeakedInteractivePipes pass has everything it needs
// even if this process is SIGKILLed the very next instant. Best-effort by
// design at the call site (a write failure here must never fail entering
// interactive mode itself) -- but the failure is still returned rather
// than swallowed here, so the caller decides how loudly to ignore it.
func SaveInteractiveClaimRecord(dir string, record InteractiveClaimRecord) error {
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal interactive claim record: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, interactiveClaimFileName), data, 0o600); err != nil {
		return fmt.Errorf("write interactive claim record into %q: %w", dir, err)
	}
	return nil
}

// loadInteractiveClaimRecord reads dir/claim.json back. A missing or
// unparseable file is reported as an error (not a zero record) so the
// caller can tell "no metadata to reclaim against, just remove the leaked
// dir" apart from "a genuine record was found".
func loadInteractiveClaimRecord(dir string) (InteractiveClaimRecord, error) {
	data, err := os.ReadFile(filepath.Join(dir, interactiveClaimFileName)) //nolint:gosec // G304: dir is a deck-created interactive-claim directory; the file name is a package constant
	if err != nil {
		return InteractiveClaimRecord{}, err
	}
	var record InteractiveClaimRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return InteractiveClaimRecord{}, fmt.Errorf("parse interactive claim record in %q: %w", dir, err)
	}
	if record.Socket == "" || record.PaneTarget == "" || record.WindowTarget == "" {
		return InteractiveClaimRecord{}, fmt.Errorf("interactive claim record in %q is missing a required field", dir)
	}
	return record, nil
}

// ReclaimLeakedInteractivePipes implements PRD R89's "the next start
// reclaims it" guarantee: every abnormal exit this package cannot itself
// clean up (SIGKILL, above all -- it cannot be handled at all, which is
// exactly why the guarantee is stated at the NEXT start rather than at the
// exit that leaked) leaves behind a temp dir under interactivePipeTempRoot
// (normally the real OS temp dir) named interactivePipeTempDirPrefix-*,
// holding a FIFO, an armed `pipe-pane -IO` still writing into it, and a
// claimed window-ownership option -- this walks every such dir, and for
// each one whose claim is genuinely stale (no LIVE process -- confirmed by
// the same kill(pid, 0) liveness check ClaimWindowOwnership itself uses,
// never by mere presence of the option -- still holds the window's
// ownership), disarms the pipe, restores the window's geometry byte-exact
// (SPEC §11.9) the same way Model.exitInteractive itself would have, and
// releases the ownership option -- then, regardless of whether a live
// owner was found, removes the leaked temp dir (a dir a live owner still
// legitimately holds is skipped entirely instead: see reclaimOne).
//
// This is deliberately best-effort and bounded to exactly one pass over
// whatever it finds right now, the same shape as store.SweepTombstones'
// own call site in cmd/deck/main.go: a failure reclaiming one leaked pipe
// must never stop deck from starting, and must never block starting on
// something that cannot converge (a tmux server that is itself gone, a
// target tmux has already forgotten). A failed scan of the temp
// root is still returned so the caller can report it; it is the only error
// this returns: every per-dir step (the tmux
// disarm/restore/release calls and the removal) is best-effort and never
// stops the pass, so every entry is attempted regardless.
func ReclaimLeakedInteractivePipes(ctx context.Context) ([]string, error) {
	root := interactivePipeTempRoot
	if root == "" {
		root = os.TempDir()
	}
	entries, err := interactivePipeReadDir(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("scan %q for leaked interactive pipes: %w", root, err)
	}

	var reclaimed []string
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), interactivePipeTempDirPrefix) {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		if !isReclaimableInteractivePipeDir(dir) {
			continue
		}
		if reclaimInteractivePipeDir(ctx, dir) {
			reclaimed = append(reclaimed, dir)
		}
	}
	return reclaimed, nil
}

// reclaimInteractivePipeDir is ReclaimLeakedInteractivePipes' step for one
// trusted dir, reporting whether it was reclaimed.
func reclaimInteractivePipeDir(ctx context.Context, dir string) bool {
	record, err := loadInteractiveClaimRecord(dir)
	if err == nil {
		return reclaimOne(ctx, Client{Socket: record.Socket}, dir, record)
	}
	if interactivePipeOwnerAlive(dir) {
		// Not leaked: a live process is still arming this pipe and has
		// not written its claim record yet.
		return false
	}
	// No usable metadata -- an older/foreign/corrupt dir, or a
	// SaveInteractiveClaimRecord call that itself failed. There is
	// nothing to disarm or restore against, but the leaked FIFO/dir
	// itself is still real and still worth clearing.
	_ = os.RemoveAll(dir)
	return true
}

// interactivePipeReadDir lists the temp root for ReclaimLeakedInteractivePipes.
// A listing's entry types are only what the root held at scan time, so the
// trust check re-reads each entry with Lstat rather than trusting them; it is
// a variable so tests can swap an entry for a symlink between the scan and
// that check.
var interactivePipeReadDir = os.ReadDir

// interactivePipeOwnedByCurrentUser reports whether info's owner is the
// current uid. It is a variable so tests can report a directory as
// foreign-owned without needing a second uid on the machine.
var interactivePipeOwnedByCurrentUser = func(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && int64(stat.Uid) == int64(os.Getuid())
}

// isReclaimableInteractivePipeDir is the trust check a scan result must
// pass before anything under it is touched: the entry is a real directory
// (Lstat, so a symlink is never followed or removed), owned by the current
// user, and grants no permission bit beyond 0700. The shared temp root is
// world-writable, so any other local user can plant a deck-interactive-pipe-*
// entry; only what deck's own os.MkdirTemp (0700, this uid) could have made
// is reclaimed, and everything else stays in place and unreported.
func isReclaimableInteractivePipeDir(dir string) bool {
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return false
	}
	if info.Mode().Perm()&^0o700 != 0 {
		return false
	}
	return interactivePipeOwnedByCurrentUser(info)
}

// reclaimOne reclaims a single leaked interactive pipe, or stands down
// untouched if a live process still legitimately owns its window -- the
// same live-owner respect ClaimWindowOwnership itself gives a concurrent
// claimant, applied here instead to a concurrent process that is still
// genuinely running (this reclaim pass runs at deck START, before this
// process's own tmux client has claimed anything, so the only way another
// claim on the SAME window can be live is a second deck process already
// running against the same tmux server).
func reclaimOne(ctx context.Context, client Client, dir string, record InteractiveClaimRecord) bool {
	state, err := client.readWindowOwnership(ctx, record.WindowTarget)
	if err != nil {
		// The window (or the whole tmux server/socket) is gone entirely --
		// readWindowOwnership only returns an error for a genuine
		// transport/target failure, never for "unset" (which it reports as
		// Set==false with no error). Nothing is left to disarm or restore
		// against; only the leaked FIFO/dir remains real.
		_ = os.RemoveAll(dir)
		return true
	}
	if state.Set {
		_, pid, ok := parseOwnershipClaim(state.Value)
		if ok && pidAlive(pid) {
			// A live process holds this window's ownership right now --
			// stand down untouched, exactly as ClaimWindowOwnership itself
			// would. Leaving the temp dir in place is deliberate too: it is
			// that live process's own pipe, still in active use. Neither
			// option is touched: the ownership option (state.Value, already
			// read above) and @deck_isize_geometry both stay exactly as the
			// live owner left them -- R100's original-geometry record must
			// outlive every steal until the LAST holder lets go legitimately,
			// and a live owner has not.
			return false
		}
	}
	// Stale (a dead owner's claim, or already unset with the pipe left
	// armed by a crash between release and disarm): reclaim in the same
	// order Model.exitInteractive's own still-mine-gated teardown uses
	// (task 112's teardownInteractiveClaim in internal/tui) -- disarm the
	// transport, restore geometry, clear @deck_isize_geometry, THEN release
	// ownership -- so a concurrent claimant racing this reclaim pass never
	// observes a window resized by an owner that has already let go of it,
	// and R100's original-geometry record never outlives the claim it was
	// kept for.
	_, _ = client.run(ctx, "pipe-pane", "-t", record.PaneTarget)
	_ = client.RestoreWindowGeometry(ctx, record.WindowTarget, record.Geometry)
	_ = client.ClearIsizeGeometry(ctx, record.WindowTarget)
	if state.Set {
		_ = client.unsetWindowOwnership(ctx, record.WindowTarget)
	}
	_ = os.RemoveAll(dir)
	return true
}
