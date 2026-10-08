package tmux

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// TestReclaimLeavesAPipeStillBeingArmedByALiveProcess: ArmPipePane creates
// its temp dir and FIFO before enterInteractive writes the claim record, so
// a second deck starting in that window used to find a record-less dir,
// treat it as leaked and remove it, deleting the FIFO the live entry was
// arming (the entry then refused). A record-less dir whose owner marker
// names a live process is still being armed, and the scan must leave it,
// its FIFO and its armed pipe in place.
func TestReclaimLeavesAPipeStillBeingArmedByALiveProcess(t *testing.T) {
	withIsolatedInteractivePipeTempRoot(t)
	socket := reclaimSocket("arming")
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 24)
	defer cleanup()
	client := Client{Socket: socket, Timeout: 5 * time.Second}
	ctx := context.Background()

	pipe, err := client.ArmPipePane(ctx, "s0")
	if err != nil {
		t.Fatalf("ArmPipePane: %v", err)
	}
	defer func() { _ = pipe.Close() }()
	dir := pipe.TempDir()

	reclaimed, err := ReclaimLeakedInteractivePipes(ctx)
	if err != nil {
		t.Fatalf("ReclaimLeakedInteractivePipes: %v", err)
	}
	if len(reclaimed) != 0 {
		t.Fatalf("reclaimed = %v, want nothing: the only dir belongs to this live process", reclaimed)
	}
	if _, err := os.Stat(filepath.Join(dir, "pane.fifo")); err != nil {
		t.Fatalf("the live pipe's FIFO is gone after the reclaim scan: %v", err)
	}
	armed, err := client.run(ctx, "display-message", "-p", "-t", "s0", "#{pane_pipe}")
	if err != nil {
		t.Fatalf("read pane_pipe: %v", err)
	}
	if got := string(armed); got != "1\n" {
		t.Fatalf("pane_pipe = %q after the reclaim scan, want the pipe still armed", got)
	}
}

// TestArmPipePaneDirCarriesItsOwnerFromTheMomentItIsVisible: the dir a scan
// can see (the interactive-pipe prefix) already names its owner, so there
// is no instant at which a scan finds it without one.
func TestArmPipePaneDirCarriesItsOwnerFromTheMomentItIsVisible(t *testing.T) {
	withIsolatedInteractivePipeTempRoot(t)
	dir, err := makeInteractivePipeDir()
	if err != nil {
		t.Fatalf("makeInteractivePipeDir: %v", err)
	}
	if filepath.Dir(dir) != interactivePipeTempRoot {
		t.Fatalf("dir %q is not under the temp root %q", dir, interactivePipeTempRoot)
	}
	if base := filepath.Base(dir); len(base) <= len(interactivePipeTempDirPrefix) || base[:len(interactivePipeTempDirPrefix)] != interactivePipeTempDirPrefix {
		t.Fatalf("dir %q does not carry the interactive-pipe prefix", dir)
	}
	owner, err := os.ReadFile(filepath.Join(dir, interactivePipeOwnerFileName))
	if err != nil {
		t.Fatalf("owner marker missing: %v", err)
	}
	if got, want := string(owner), strconv.Itoa(os.Getpid()); got != want {
		t.Fatalf("owner marker = %q, want this process %q", got, want)
	}
	entries, err := os.ReadDir(interactivePipeTempRoot)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("temp root holds %d entries, want only the renamed dir (no staging dir left behind)", len(entries))
	}
}

// TestReclaimStillRemovesARecordLessDirWhoseOwnerIsDead: the owner marker
// spares only a LIVE owner's dir; a dead owner's record-less dir is leaked
// and removed exactly as before.
func TestReclaimStillRemovesARecordLessDirWhoseOwnerIsDead(t *testing.T) {
	withIsolatedInteractivePipeTempRoot(t)
	deadPID := 999999999
	if pidAlive(deadPID) {
		t.Fatalf("test's chosen dead pid %d is alive; pick another", deadPID)
	}
	dir := plantInteractivePipeEntry(t, 0o700)
	if err := os.WriteFile(filepath.Join(dir, interactivePipeOwnerFileName), []byte(strconv.Itoa(deadPID)), 0o600); err != nil {
		t.Fatalf("plant owner marker: %v", err)
	}

	reclaimed, err := ReclaimLeakedInteractivePipes(context.Background())
	if err != nil {
		t.Fatalf("ReclaimLeakedInteractivePipes: %v", err)
	}
	if len(reclaimed) != 1 || reclaimed[0] != dir {
		t.Fatalf("reclaimed = %v, want exactly [%q]", reclaimed, dir)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("dead owner's dir %q still present (err=%v), want removed", dir, err)
	}
}
