package tui

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/tmux"
)

// TestInteractivePreviewTickDetectsDeadPane: a pane that dies while
// interactive is detected by the very next preview tick, with no poll of
// the Session's own.
func TestInteractivePreviewTickDetectsDeadPane(t *testing.T) {
	m, client, socket, _ := interactiveTickModel(t, "tickdead")
	if out, err := exec.Command("tmux", "-L", socket, "set-option", "-g", "remain-on-exit", "failed").CombinedOutput(); err != nil {
		t.Fatalf("set remain-on-exit: %v: %s", err, out)
	}
	pidOut, err := exec.Command("tmux", "-L", socket, "display-message", "-p", "-t", m.interactiveWindowTarget, "#{pane_pid}").Output()
	if err != nil {
		t.Fatalf("read pane_pid: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(pidOut)))
	if err != nil || pid <= 1 {
		t.Fatalf("pane_pid %q: %v", pidOut, err)
	}
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		t.Fatalf("kill pane process %d: %v", pid, err)
	}
	if !waitForDisplacementTest(t, 3*time.Second, func() bool {
		tick, err := client.InteractiveTickRead(context.Background(), m.interactiveWindowTarget)
		return err == nil && tick.PaneDead
	}) {
		t.Fatal("tmux never reported the killed pane dead")
	}

	select {
	case <-m.interactiveGrid.Dead():
		t.Fatal("Dead() was already closed before any preview tick ran")
	default:
	}
	m = runTick(t, m)
	select {
	case <-m.interactiveGrid.Dead():
	default:
		t.Fatal("one preview tick after the pane died did not report it through Dead()")
	}
}

// TestInteractivePreviewTickDetectsDisplacement: a claim stolen without
// any competing pipe (so the transport's fast path stays silent) is caught
// by the very next preview tick's one read, leaving interactive mode with
// the lost-attach dialog.
func TestInteractivePreviewTickDetectsDisplacement(t *testing.T) {
	m, _, socket, _ := interactiveTickModel(t, "tickdisp")
	if out, err := exec.Command("tmux", "-L", socket, "set-option", "-w", "-t", m.interactiveWindowTarget, tmux.OwnershipOption, "0123456789abcdef:"+strconv.Itoa(os.Getpid())).CombinedOutput(); err != nil {
		t.Fatalf("overwrite the ownership claim: %v: %s", err, out)
	}
	if !m.interactive || m.lostAttach {
		t.Fatal("test setup: expected interactive mode and no dialog before the tick")
	}
	m = runTick(t, m)
	if m.interactive {
		t.Fatal("one preview tick after the claim was stolen did not leave interactive mode")
	}
	if !m.lostAttach || m.lostAttachSession != "tickdisp" {
		t.Fatalf("lostAttach=%v session=%q, want the lost-attach dialog naming tickdisp", m.lostAttach, m.lostAttachSession)
	}
}
