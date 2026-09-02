package tmux

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	data, err := os.ReadFile(filepath.Join(dir, interactiveClaimFileName))
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
// target tmux has already forgotten). The first error encountered, if
// any, is still returned so the caller can report it -- every entry is
// still attempted regardless.
func ReclaimLeakedInteractivePipes(ctx context.Context) ([]string, error) {
	root := interactivePipeTempRoot
	if root == "" {
		root = os.TempDir()
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("scan %q for leaked interactive pipes: %w", root, err)
	}

	var reclaimed []string
	var firstErr error
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), interactivePipeTempDirPrefix) {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		record, err := loadInteractiveClaimRecord(dir)
		if err != nil {
			// No usable metadata -- an older/foreign/corrupt dir, or a
			// SaveInteractiveClaimRecord call that itself failed. There is
			// nothing to disarm or restore against, but the leaked
			// FIFO/dir itself is still real and still worth clearing.
			_ = os.RemoveAll(dir)
			reclaimed = append(reclaimed, dir)
			continue
		}
		did, err := reclaimOne(ctx, Client{Socket: record.Socket}, dir, record)
		if err != nil && firstErr == nil {
			firstErr = err
		}
		if did {
			reclaimed = append(reclaimed, dir)
		}
	}
	return reclaimed, firstErr
}

// reclaimOne reclaims a single leaked interactive pipe, or stands down
// untouched if a live process still legitimately owns its window -- the
// same live-owner respect ClaimWindowOwnership itself gives a concurrent
// claimant, applied here instead to a concurrent process that is still
// genuinely running (this reclaim pass runs at deck START, before this
// process's own tmux client has claimed anything, so the only way another
// claim on the SAME window can be live is a second deck process already
// running against the same tmux server).
func reclaimOne(ctx context.Context, client Client, dir string, record InteractiveClaimRecord) (bool, error) {
	state, err := client.readWindowOwnership(ctx, record.WindowTarget)
	if err != nil {
		// The window (or the whole tmux server/socket) is gone entirely --
		// readWindowOwnership only returns an error for a genuine
		// transport/target failure, never for "unset" (which it reports as
		// Set==false with no error). Nothing is left to disarm or restore
		// against; only the leaked FIFO/dir remains real.
		_ = os.RemoveAll(dir)
		return true, nil
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
			return false, nil
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
	return true, nil
}
