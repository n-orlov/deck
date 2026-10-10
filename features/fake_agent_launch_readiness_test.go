package features

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func newReadinessHarness(t *testing.T) (context.Context, *ScenarioHarness) {
	t.Helper()
	h, err := newScenarioHarness("")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := h.Close(); err != nil {
			t.Error(err)
		}
	})
	for _, k := range []string{"HOME", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "DECK_HOME"} {
		t.Setenv(k, h.Home)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return context.WithValue(ctx, scenarioHarnessKey{}, h), h
}

func bootstrapReadinessServer(ctx context.Context, t *testing.T, h *ScenarioHarness) {
	t.Helper()
	if _, err := tmuxOutput(ctx, h,
		"start-server", ";",
		"set-option", "-s", "exit-empty", "off", ";",
		"set-option", "-g", "remain-on-exit", "failed"); err != nil {
		t.Fatal(err)
	}
}

func paneFacts(ctx context.Context, t *testing.T, h *ScenarioHarness, session string) (pid int, command, dead string) {
	t.Helper()
	out, err := tmuxOutput(ctx, h, "list-panes", "-t", session, "-F", "#{pane_pid}|#{pane_current_command}|#{pane_dead}")
	if err != nil {
		t.Fatal(err)
	}
	fields := strings.Split(strings.TrimSpace(string(out)), "|")
	if len(fields) != 3 {
		t.Fatalf("pane facts for %q = %q", session, out)
	}
	pid, err = strconv.Atoi(fields[0])
	if err != nil {
		t.Fatal(err)
	}
	return pid, fields[1], fields[2]
}

func processAlive(pid int) bool { return syscall.Kill(pid, 0) == nil }

// holdPaneLauncher puts a gated `env` first on PATH: when it is the pane
// launcher of the long-running fake Claude it writes the returned ready file and
// waits, without exec'ing the agent, until release is called.
func holdPaneLauncher(t *testing.T, h *ScenarioHarness) (ready string, release func()) {
	t.Helper()
	realEnv, err := exec.LookPath("env")
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(h.Home, "launcher")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	ready, gate := filepath.Join(h.Home, "env-ready"), filepath.Join(h.Home, "allow-exec")
	script := fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = FAKE_CLAUDE_COMMANDS=1 ]; then\n printf ready > %q\n while [ ! -e %q ]; do /bin/sleep 0.01; done\nfi\nexec %q \"$@\"\n", ready, gate, realEnv)
	if err := os.WriteFile(filepath.Join(dir, "env"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return ready, func() { _ = os.WriteFile(gate, []byte("release"), 0o600) }
}

// TestFakeAgentLongRunningLaunchWaitsForFakeClaudeBeforeSIGKILL holds the pane
// launcher (a gated `env` first on PATH) before it execs fake-claude. The
// launch step must not return, so the SIGKILL step cannot read the launcher's
// name as the pane command. After release the chain SIGKILLs the verified
// fake-claude process and the session keeps a nonzero dead pane.
func TestFakeAgentLongRunningLaunchWaitsForFakeClaudeBeforeSIGKILL(t *testing.T) {
	ctx, h := newReadinessHarness(t)
	s := &fakeAgentScenario{}
	if err := s.buildFixture(ctx); err != nil {
		t.Fatal(err)
	}
	ready, releaseOnce := holdPaneLauncher(t, h)
	defer releaseOnce() // runs before the harness cleanup so the launcher never outlives it

	const session = "deck_fake-agent-readiness"
	done := make(chan error, 1)
	go func() {
		err := s.launchLongRunning(ctx, session)
		if err == nil {
			err = agentProcessInPrivateSessionIsKilledWithSIGKILL(ctx, "fake-claude", session)
		}
		done <- err
	}()
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("env launcher never reached the gate")
		case <-time.After(time.Millisecond):
		}
	}
	select {
	case err := <-done:
		t.Fatalf("launch/SIGKILL chain completed while the launcher was held before exec'ing fake-claude: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	releaseOnce()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("launch/SIGKILL chain after release: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("chain never completed after the launcher was released")
	}
	if err := privateSessionRetainsNonzeroDeadPane(ctx, session); err != nil {
		t.Fatal(err)
	}
}

// TestFakeAgentLongRunningLaunchReadinessIsBoundedWithoutACallerDeadline holds
// the launcher forever and launches under a context that has no deadline. The
// readiness wait must give up within the launch budget, with an error carrying
// the last pane facts (the held launcher's shell, not fake-claude).
func TestFakeAgentLongRunningLaunchReadinessIsBoundedWithoutACallerDeadline(t *testing.T) {
	ctx, h := newReadinessHarness(t)
	s := &fakeAgentScenario{}
	if err := s.buildFixture(ctx); err != nil {
		t.Fatal(err)
	}
	_, release := holdPaneLauncher(t, h)
	defer release() // frees the held launcher before the harness cleanup

	// Same harness, but no deadline: only the readiness wait's own bound can end it.
	noDeadline := context.WithValue(context.Background(), scenarioHarnessKey{}, h)
	if _, ok := noDeadline.Deadline(); ok {
		t.Fatal("test context unexpectedly carries a deadline")
	}
	done := make(chan error, 1)
	start := time.Now()
	go func() { done <- s.launchLongRunning(noDeadline, "deck_fake-agent-held") }()
	select {
	case err := <-done:
		if elapsed := time.Since(start); elapsed > fakeAgentLaunchBudget+2*time.Second {
			t.Fatalf("readiness wait took %s, want within the %s launch budget", elapsed, fakeAgentLaunchBudget)
		}
		if err == nil {
			t.Fatal("launch of a held launcher succeeded, want a readiness error")
		}
		for _, want := range []string{"never ran \"fake-claude\"", "last facts \"sh|0\""} {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("error %q does not carry %q", err, want)
			}
		}
	case <-time.After(fakeAgentLaunchBudget + 2*time.Second):
		t.Fatalf("readiness wait outlived the %s launch budget under a context with no deadline", fakeAgentLaunchBudget)
	}
}

// TestSIGKILLStepRefusesBadTargetsWithoutSignalling pins that the SIGKILL step
// still verifies its target: a pane that never becomes the wanted agent, a dead
// pane and a missing session are all refused, and no process is signalled.
func TestSIGKILLStepRefusesBadTargetsWithoutSignalling(t *testing.T) {
	ctx, h := newReadinessHarness(t)
	bootstrapReadinessServer(ctx, t, h)

	t.Run("pane never becomes the wanted agent", func(t *testing.T) {
		const session = "deck_wrong-agent"
		if _, err := tmuxOutput(ctx, h, "new-session", "-d", "-s", session, "--", "sleep", "300"); err != nil {
			t.Fatal(err)
		}
		pid, command, dead := paneFacts(ctx, t, h, session)
		t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
		if command != "sleep" || dead != "0" {
			t.Fatalf("setup facts = %q|%q, want sleep|0", command, dead)
		}
		err := agentProcessInPrivateSessionIsKilledWithSIGKILL(ctx, "fake-claude", session)
		if err == nil || !strings.Contains(err.Error(), "want agent") {
			t.Fatalf("step error = %v, want a wrong-command refusal", err)
		}
		time.Sleep(100 * time.Millisecond)
		if !processAlive(pid) {
			t.Fatalf("process %d running %q was signalled by a refused step", pid, command)
		}
		if _, _, dead := paneFacts(ctx, t, h, session); dead != "0" {
			t.Fatalf("pane died after a refused step")
		}
	})

	t.Run("dead pane", func(t *testing.T) {
		const session = "deck_dead-agent"
		if _, err := tmuxOutput(ctx, h, "new-session", "-d", "-s", session, "--", "sh", "-c", "exit 3"); err != nil {
			t.Fatal(err)
		}
		var command string
		for {
			var dead string
			_, command, dead = paneFacts(ctx, t, h, session)
			if dead == "1" {
				break
			}
			select {
			case <-ctx.Done():
				t.Fatal("pane never died")
			case <-time.After(10 * time.Millisecond):
			}
		}
		// Ask for the command the dead pane reports, so only the liveness check can refuse.
		err := agentProcessInPrivateSessionIsKilledWithSIGKILL(ctx, command, session)
		if err == nil || !strings.Contains(err.Error(), "already dead") {
			t.Fatalf("step error = %v, want a dead-pane refusal", err)
		}
	})

	t.Run("missing session", func(t *testing.T) {
		bystander := exec.Command("sleep", "300")
		if err := bystander.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = bystander.Process.Kill(); _ = bystander.Wait() })
		err := agentProcessInPrivateSessionIsKilledWithSIGKILL(ctx, "fake-claude", "deck_no-such-session")
		if err == nil || !strings.Contains(err.Error(), "locate agent process") {
			t.Fatalf("step error = %v, want a missing-session refusal", err)
		}
		if !processAlive(bystander.Process.Pid) {
			t.Fatal("bystander process was signalled by a refused step")
		}
	})
}
