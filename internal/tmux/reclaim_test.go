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
	return fmt.Sprintf("priv-reclaim-%s-%d-%d", name, os.Getpid(), time.Now().UnixNano())
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
	setWindowOwnershipRaw(t, socket, formatOwnershipClaim("deadowner", deadPID))
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
	_, windowSizeSet, err := client.readBuiltinWindowOption(ctx, "s0")
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
	setWindowOwnershipRaw(t, socket, liveClaim)
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

// plantInteractivePipeEntry creates a deck-interactive-pipe-* directory with
// the given mode under the isolated root and returns its path.
func plantInteractivePipeEntry(t *testing.T, mode os.FileMode) string {
	t.Helper()
	dir, err := os.MkdirTemp(interactivePipeTempRoot, interactivePipeTempDirPrefix)
	if err != nil {
		t.Fatalf("create dir: %v", err)
	}
	if err := os.Chmod(dir, mode); err != nil {
		t.Fatalf("chmod %v: %v", mode, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "marker"), []byte("x"), 0o600); err != nil {
		t.Fatalf("plant marker: %v", err)
	}
	return dir
}

func reclaimWithOwnerCheck(t *testing.T, check func(os.FileInfo) bool) []string {
	t.Helper()
	previous := interactivePipeOwnedByCurrentUser
	interactivePipeOwnedByCurrentUser = check
	t.Cleanup(func() { interactivePipeOwnedByCurrentUser = previous })
	reclaimed, err := ReclaimLeakedInteractivePipes(context.Background())
	if err != nil {
		t.Fatalf("ReclaimLeakedInteractivePipes: %v", err)
	}
	return reclaimed
}

func TestReclaimRemovesAnOwned0700Directory(t *testing.T) {
	withIsolatedInteractivePipeTempRoot(t)
	dir := plantInteractivePipeEntry(t, 0o700)
	reclaimed := reclaimWithOwnerCheck(t, func(os.FileInfo) bool { return true })
	if len(reclaimed) != 1 || reclaimed[0] != dir {
		t.Fatalf("reclaimed = %v, want [%q]", reclaimed, dir)
	}
	if _, err := os.Lstat(dir); !os.IsNotExist(err) {
		t.Fatalf("owned 0700 dir still present (err=%v)", err)
	}
}

// TestReclaimLeavesASymlinkAlone plants two deck-interactive-pipe-* symlinks
// to a directory the reclaim must never reach: one already a symlink when the
// temp root is scanned, and one that is a real directory at scan time and is
// swapped for a symlink before the per-entry check runs -- what a local user
// racing the shared temp root can do. The scan's own entry types are stale by
// then, so only a fresh Lstat of each entry keeps both links, and the
// directory behind them, untouched and unreported.
func TestReclaimLeavesASymlinkAlone(t *testing.T) {
	withIsolatedInteractivePipeTempRoot(t)
	target := t.TempDir()
	if err := os.WriteFile(filepath.Join(target, "precious"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	planted := filepath.Join(interactivePipeTempRoot, interactivePipeTempDirPrefix+"link")
	if err := os.Symlink(target, planted); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	swapped := plantInteractivePipeEntry(t, 0o700)

	previous := interactivePipeReadDir
	interactivePipeReadDir = func(root string) ([]os.DirEntry, error) {
		entries, err := os.ReadDir(root)
		if err != nil {
			return nil, err
		}
		if err := os.RemoveAll(swapped); err != nil {
			t.Fatalf("remove scanned dir: %v", err)
		}
		if err := os.Symlink(target, swapped); err != nil {
			t.Fatalf("swap scanned dir for a symlink: %v", err)
		}
		return entries, nil
	}
	t.Cleanup(func() { interactivePipeReadDir = previous })

	reclaimed := reclaimWithOwnerCheck(t, func(os.FileInfo) bool { return true })
	if len(reclaimed) != 0 {
		t.Fatalf("reclaimed = %v, want none", reclaimed)
	}
	for _, link := range []string{planted, swapped} {
		if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("symlink %q was removed or replaced (err=%v)", link, err)
		}
	}
	if _, err := os.Stat(filepath.Join(target, "precious")); err != nil {
		t.Fatalf("symlink target was touched: %v", err)
	}
}

func TestReclaimLeavesAForeignOwnedDirectoryAlone(t *testing.T) {
	withIsolatedInteractivePipeTempRoot(t)
	dir := plantInteractivePipeEntry(t, 0o700)
	reclaimed := reclaimWithOwnerCheck(t, func(os.FileInfo) bool { return false })
	if len(reclaimed) != 0 {
		t.Fatalf("reclaimed = %v, want none", reclaimed)
	}
	if _, err := os.Stat(filepath.Join(dir, "marker")); err != nil {
		t.Fatalf("foreign-owned dir was touched: %v", err)
	}
}

func TestReclaimLeavesAWorldReadableDirectoryAlone(t *testing.T) {
	withIsolatedInteractivePipeTempRoot(t)
	dir := plantInteractivePipeEntry(t, 0o755)
	reclaimed := reclaimWithOwnerCheck(t, func(os.FileInfo) bool { return true })
	if len(reclaimed) != 0 {
		t.Fatalf("reclaimed = %v, want none", reclaimed)
	}
	if _, err := os.Stat(filepath.Join(dir, "marker")); err != nil {
		t.Fatalf("0755 dir was touched: %v", err)
	}
}

func TestInteractivePipeOwnedByCurrentUserMatchesTheRealOwner(t *testing.T) {
	info, err := os.Lstat(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !interactivePipeOwnedByCurrentUser(info) {
		t.Fatalf("a directory this process just created is not reported as owned by the current user")
	}
}

// TestFreshInteractivePipeDirIsNeverAnUntrustedPlantedEntry: the shared temp
// root is writable by every local user, so a deck-interactive-pipe-* entry can
// exist before deck makes its own. Creating an interactive pipe dir must give
// a different, fresh 0700 directory and leave the planted entries (a loose-mode
// directory and a symlink) exactly as they were, and a following reclaim pass
// must not touch them or the live new dir either.
func TestFreshInteractivePipeDirIsNeverAnUntrustedPlantedEntry(t *testing.T) {
	withIsolatedInteractivePipeTempRoot(t)
	loose := plantInteractivePipeEntry(t, 0o755)
	target := t.TempDir()
	link := filepath.Join(interactivePipeTempRoot, interactivePipeTempDirPrefix+"link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	fresh, err := makeInteractivePipeDir()
	if err != nil {
		t.Fatalf("makeInteractivePipeDir: %v", err)
	}
	if fresh == loose || fresh == link {
		t.Fatalf("fresh dir %q reuses a planted entry", fresh)
	}
	info, err := os.Lstat(fresh)
	if err != nil || !info.IsDir() {
		t.Fatalf("fresh dir %q is not a real directory (err=%v)", fresh, err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("fresh dir mode = %v, want 0700", got)
	}
	if !interactivePipeOwnedByCurrentUser(info) {
		t.Fatalf("fresh dir %q is not owned by the current user", fresh)
	}

	reclaimed := reclaimWithOwnerCheck(t, func(os.FileInfo) bool { return true })
	if len(reclaimed) != 0 {
		t.Fatalf("reclaimed = %v, want none: the planted entries are untrusted and the new dir is live", reclaimed)
	}
	if got, err := os.Stat(loose); err != nil || got.Mode().Perm() != 0o755 {
		t.Fatalf("loose-mode entry was changed (err=%v)", err)
	}
	if _, err := os.Stat(filepath.Join(loose, "marker")); err != nil {
		t.Fatalf("loose-mode entry lost its contents: %v", err)
	}
	if got, err := os.Readlink(link); err != nil || got != target {
		t.Fatalf("symlink entry changed (target=%q err=%v)", got, err)
	}
	if _, err := os.Stat(filepath.Join(fresh, interactivePipeOwnerFileName)); err != nil {
		t.Fatalf("fresh dir was touched by the reclaim pass: %v", err)
	}
}

// TestReclaimLeavesARegularFileAlone: a planted deck-interactive-pipe-* entry
// that is a plain file owned by the current user with mode 0600 passes the
// permission and owner checks, so only the is-a-real-directory check keeps it
// (and its contents) from being removed and reported.
func TestReclaimLeavesARegularFileAlone(t *testing.T) {
	withIsolatedInteractivePipeTempRoot(t)
	planted := filepath.Join(interactivePipeTempRoot, interactivePipeTempDirPrefix+"file")
	if err := os.WriteFile(planted, []byte("x"), 0o600); err != nil {
		t.Fatalf("plant file: %v", err)
	}
	reclaimed := reclaimWithOwnerCheck(t, func(os.FileInfo) bool { return true })
	if len(reclaimed) != 0 {
		t.Fatalf("reclaimed = %v, want none", reclaimed)
	}
	if data, err := os.ReadFile(planted); err != nil || string(data) != "x" {
		t.Fatalf("regular-file entry was removed or changed (err=%v)", err)
	}
}
