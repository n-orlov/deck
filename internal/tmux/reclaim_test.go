// reclaim_test.go proves PRD R89's "the next start reclaims it" half (task
// 030): a leaked interactive pipe -- an armed `pipe-pane`, a claimed window
// ownership option and a temp dir/FIFO left behind by a process that never
// got to run its own exitInteractive teardown (the SIGKILL case named in
// the PRD, which cannot be handled at all) -- is found, disarmed, restored
// and removed by ReclaimLeakedInteractivePipes, driven against a real tmux
// server exactly the way enterInteractive's own entry sequence would have
// left it. A concurrently LIVE claim is proven untouched, and a temp dir
// with no usable metadata is proven removed anyway (it is still a leaked
// FIFO, even with nothing left to restore against).
package tmux

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// withIsolatedInteractivePipeTempRoot points interactivePipeTempRoot (both
// ArmPipePane's own os.MkdirTemp call and ReclaimLeakedInteractivePipes'
// own scan) at an isolated t.TempDir() for the duration of one test, so
// this file's own leaked dirs never mix with the real OS temp directory or
// with any other test/process sharing it.
func withIsolatedInteractivePipeTempRoot(t *testing.T) {
	t.Helper()
	previous := interactivePipeTempRoot
	interactivePipeTempRoot = t.TempDir()
	t.Cleanup(func() { interactivePipeTempRoot = previous })
}

func reclaimSocket(name string) string {
	return fmt.Sprintf("deck-reclaim-%s-%d-%d", name, os.Getpid(), time.Now().UnixNano())
}

func TestReclaimLeakedInteractivePipesDisarmsRestoresAndRemovesAStaleDeadOwnerClaim(t *testing.T) {
	withIsolatedInteractivePipeTempRoot(t)
	socket := reclaimSocket("stale")
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 24)
	defer cleanup()
	client := Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	// Reproduce enterInteractive's own sequence up to the point a SIGKILL
	// would have interrupted it: capture geometry, claim ownership (here
	// with a DEAD pid, standing in for a deck process that has since died
	// without releasing it), fit the window (a bare resize-window --
	// FitWindowToPane's own effect on a single-pane window), arm the pipe,
	// and persist the claim record the way enterInteractive now does.
	geometry, err := client.CaptureWindowGeometry(ctx, "s0")
	if err != nil {
		t.Fatalf("CaptureWindowGeometry: %v", err)
	}
	deadPID := 999999999
	if pidAlive(deadPID) {
		t.Fatalf("test's chosen dead pid %d is alive; pick another", deadPID)
	}
	setWindowOwnershipRaw(t, socket, "s0", formatOwnershipClaim("deadowner", deadPID))
	// R100 (task 111/113): the record's Geometry is the RESOLVED original --
	// what enterInteractive's own ResolveIsizeGeometry would have written to
	// @deck_isize_geometry on first claim -- so a real reclaim pass has the
	// same option present to clear once it actually proceeds.
	if err := client.writeWindowIsizeGeometry(ctx, "s0", geometry); err != nil {
		t.Fatalf("writeWindowIsizeGeometry: %v", err)
	}
	if err := client.resizeWindow(ctx, "s0", 40, 10); err != nil {
		t.Fatalf("simulate FitWindowToPane's own resize: %v", err)
	}
	pipe, err := client.ArmPipePane(ctx, "s0")
	if err != nil {
		t.Fatalf("ArmPipePane: %v", err)
	}
	tempDir := pipe.TempDir()
	if err := SaveInteractiveClaimRecord(tempDir, InteractiveClaimRecord{
		Socket:       socket,
		PaneTarget:   "s0",
		WindowTarget: "s0",
		Geometry:     geometry,
	}); err != nil {
		t.Fatalf("SaveInteractiveClaimRecord: %v", err)
	}
	// The process that armed this pipe is SIGKILLed here, from the pipe's
	// own point of view: no Close, no CloseLocal, nothing -- the FIFO, the
	// armed pipe-pane and the ownership option all leak exactly as they
	// would in the real scenario.

	armedBeforeReclaim, err := client.PanePipe(ctx, "s0")
	if err != nil {
		t.Fatalf("PanePipe before reclaim: %v", err)
	}
	if !armedBeforeReclaim {
		t.Fatalf("test assumption violated: pipe-pane is not armed before reclaim runs")
	}
	if _, err := os.Stat(tempDir); err != nil {
		t.Fatalf("test assumption violated: leaked temp dir %q is not present before reclaim: %v", tempDir, err)
	}

	reclaimed, err := ReclaimLeakedInteractivePipes(ctx)
	if err != nil {
		t.Fatalf("ReclaimLeakedInteractivePipes: %v", err)
	}
	if len(reclaimed) != 1 || reclaimed[0] != tempDir {
		t.Fatalf("reclaimed = %v, want exactly [%q]", reclaimed, tempDir)
	}

	armedAfterReclaim, err := client.PanePipe(ctx, "s0")
	if err != nil {
		t.Fatalf("PanePipe after reclaim: %v", err)
	}
	if armedAfterReclaim {
		t.Fatalf("pipe-pane is still armed after reclaim -- the stale pipe was not disarmed")
	}

	width, height, _, _, err := client.windowAndPaneSize(ctx, "s0")
	if err != nil {
		t.Fatalf("read window size after reclaim: %v", err)
	}
	if width != geometry.Width || height != geometry.Height {
		t.Fatalf("window is %dx%d after reclaim, want the byte-exact original %dx%d", width, height, geometry.Width, geometry.Height)
	}
	_, windowSizeSet, err := client.readBuiltinWindowOption(ctx, "s0", "window-size")
	if err != nil {
		t.Fatalf("read window-size after reclaim: %v", err)
	}
	if windowSizeSet {
		t.Fatalf("window-size is still set window-locally after reclaim, want unset (SPEC \u00a711.9's restore recipe)")
	}
	ownership, err := client.readWindowOwnership(ctx, "s0")
	if err != nil {
		t.Fatalf("readWindowOwnership after reclaim: %v", err)
	}
	if ownership.Set {
		t.Fatalf("ownership option is still set after reclaim (%q), want released", ownership.Value)
	}
	_, isizeSet, err := client.readWindowIsizeGeometry(ctx, "s0")
	if err != nil {
		t.Fatalf("readWindowIsizeGeometry after reclaim: %v", err)
	}
	if isizeSet {
		t.Fatalf("@deck_isize_geometry is still set after reclaim, want unset (task 113: a reclaim that proceeds unsets both options)")
	}
	if _, err := os.Stat(tempDir); !os.IsNotExist(err) {
		t.Fatalf("leaked temp dir %q still present after reclaim (err=%v), want removed", tempDir, err)
	}
}

// TestReclaimLeakedInteractivePipesLeavesALiveOwnersClaimUntouched proves
// the mandatory negative control: a claim naming THIS TEST PROCESS's own
// pid (alive, by construction) is a live owner exactly the way a second,
// still-running deck process's claim would be, and reclaim must stand down
// from it completely -- pipe still armed, window still at the resized
// size, ownership option untouched, temp dir untouched -- the same respect
// ClaimWindowOwnership itself gives a live competing claim.
func TestReclaimLeakedInteractivePipesLeavesALiveOwnersClaimUntouched(t *testing.T) {
	withIsolatedInteractivePipeTempRoot(t)
	socket := reclaimSocket("live")
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 24)
	defer cleanup()
	client := Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	geometry, err := client.CaptureWindowGeometry(ctx, "s0")
	if err != nil {
		t.Fatalf("CaptureWindowGeometry: %v", err)
	}
	liveClaim := formatOwnershipClaim("livecompetitor", os.Getpid())
	setWindowOwnershipRaw(t, socket, "s0", liveClaim)
	// R100 (task 111/113): the live owner's own @deck_isize_geometry record
	// must be just as untouchable as its ownership option -- reclaim must
	// not clear the ONE record of the window's true pre-deck size out from
	// under a claim that is still legitimately live.
	if err := client.writeWindowIsizeGeometry(ctx, "s0", geometry); err != nil {
		t.Fatalf("writeWindowIsizeGeometry: %v", err)
	}
	if err := client.resizeWindow(ctx, "s0", 40, 10); err != nil {
		t.Fatalf("simulate FitWindowToPane's own resize: %v", err)
	}
	pipe, err := client.ArmPipePane(ctx, "s0")
	if err != nil {
		t.Fatalf("ArmPipePane: %v", err)
	}
	tempDir := pipe.TempDir()
	defer func() { _ = pipe.Close() }()
	if err := SaveInteractiveClaimRecord(tempDir, InteractiveClaimRecord{
		Socket:       socket,
		PaneTarget:   "s0",
		WindowTarget: "s0",
		Geometry:     geometry,
	}); err != nil {
		t.Fatalf("SaveInteractiveClaimRecord: %v", err)
	}

	reclaimed, err := ReclaimLeakedInteractivePipes(ctx)
	if err != nil {
		t.Fatalf("ReclaimLeakedInteractivePipes: %v", err)
	}
	if len(reclaimed) != 0 {
		t.Fatalf("reclaimed = %v, want none -- a live owner's claim must be left completely alone", reclaimed)
	}

	armed, err := client.PanePipe(ctx, "s0")
	if err != nil {
		t.Fatalf("PanePipe after reclaim: %v", err)
	}
	if !armed {
		t.Fatalf("pipe-pane was disarmed even though its owner is live")
	}
	width, height, _, _, err := client.windowAndPaneSize(ctx, "s0")
	if err != nil {
		t.Fatalf("read window size after reclaim: %v", err)
	}
	if width != 40 || height != 10 {
		t.Fatalf("window is %dx%d after reclaim, want the untouched resized 40x10", width, height)
	}
	ownership, err := client.readWindowOwnership(ctx, "s0")
	if err != nil {
		t.Fatalf("readWindowOwnership after reclaim: %v", err)
	}
	if !ownership.Set || ownership.Value != liveClaim {
		t.Fatalf("ownership option = (set=%v, value=%q) after reclaim, want the live owner's claim %q left untouched", ownership.Set, ownership.Value, liveClaim)
	}
	isizeGeometry, isizeSet, err := client.readWindowIsizeGeometry(ctx, "s0")
	if err != nil {
		t.Fatalf("readWindowIsizeGeometry after reclaim: %v", err)
	}
	if !isizeSet || isizeGeometry != geometry {
		t.Fatalf("@deck_isize_geometry = (set=%v, value=%+v) after reclaim, want the live owner's original geometry %+v left untouched (task 113)", isizeSet, isizeGeometry, geometry)
	}
	if _, err := os.Stat(tempDir); err != nil {
		t.Fatalf("live owner's own temp dir %q was removed by reclaim: %v", tempDir, err)
	}
}

// TestReclaimLeakedInteractivePipesRemovesADirWithNoUsableMetadata proves
// the fallback for a temp dir this package cannot interpret at all (no
// claim.json, or one that fails to parse -- an older/foreign layout, or a
// SaveInteractiveClaimRecord call that itself failed): there is nothing to
// disarm or restore against, but the leaked directory itself is still real
// and still removed.
func TestReclaimLeakedInteractivePipesRemovesADirWithNoUsableMetadata(t *testing.T) {
	withIsolatedInteractivePipeTempRoot(t)
	orphan, err := os.MkdirTemp(interactivePipeTempRoot, interactivePipeTempDirPrefix)
	if err != nil {
		t.Fatalf("create orphan dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(orphan, "pane.fifo"), []byte("not a real fifo, just a marker"), 0o600); err != nil {
		t.Fatalf("plant marker file: %v", err)
	}

	reclaimed, err := ReclaimLeakedInteractivePipes(context.Background())
	if err != nil {
		t.Fatalf("ReclaimLeakedInteractivePipes: %v", err)
	}
	if len(reclaimed) != 1 || reclaimed[0] != orphan {
		t.Fatalf("reclaimed = %v, want exactly [%q]", reclaimed, orphan)
	}
	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Fatalf("orphan dir %q still present after reclaim (err=%v), want removed", orphan, err)
	}
}

// TestReclaimLeakedInteractivePipesIgnoresDirsWithoutTheInteractivePipePrefix
// proves the scan only ever touches its own kind of temp dir: an unrelated
// directory under the same root, whatever it is named, is never inspected
// or removed.
func TestReclaimLeakedInteractivePipesIgnoresDirsWithoutTheInteractivePipePrefix(t *testing.T) {
	withIsolatedInteractivePipeTempRoot(t)
	unrelated, err := os.MkdirTemp(interactivePipeTempRoot, "some-other-tool-")
	if err != nil {
		t.Fatalf("create unrelated dir: %v", err)
	}

	if _, err := ReclaimLeakedInteractivePipes(context.Background()); err != nil {
		t.Fatalf("ReclaimLeakedInteractivePipes: %v", err)
	}

	if _, err := os.Stat(unrelated); err != nil {
		t.Fatalf("unrelated dir %q was removed by a scan that should never have touched it: %v", unrelated, err)
	}
}
