package features

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

// registerTmuxSpawnBudgetSteps wires features/tmux_spawn_budget.feature
// (R184/R185, GH #49): the number of tmux processes a deck client spawns
// over a fixed window, counted by a fixture `tmux` first on the client's
// PATH that appends one line to a log file and execs the next tmux (under
// ci/run.sh that is the guard, so the guard still sees every call). The
// client runs at the product's own default reconcile and preview cadence:
// the harness's usual 250 ms / 50 ms ticks are a test accelerator and would
// make the count a function of the accelerator, not of deck.
func registerTmuxSpawnBudgetSteps(sc *godog.ScenarioContext) {
	sc.Step(`^deck client "([^"]+)" is started counting its tmux spawns at the product's own tick cadence$`, startClientCountingTmuxSpawns)
	sc.Step(`^deck client "([^"]+)" creates (\d+) shell sessions named "([^"]*NN[^"]*)"$`, clientCreatesNumberedShellSessions)
	sc.Step(`^the tmux spawns of deck client "([^"]+)" are counted over a window of (\d+) seconds$`, tmuxSpawnsAreCountedOverAWindow)
	sc.Step(`^at most (\d+) tmux processes were spawned in that window$`, atMostNTmuxProcessesWereSpawned)
}

func (h *ScenarioHarness) tmuxSpawnLog() string {
	return filepath.Join(h.Home, "tmux-spawn-shim", "spawns.log")
}

// tmuxSpawnCount is the number of tmux processes the shim has seen so far.
func (h *ScenarioHarness) tmuxSpawnCount() (int, error) {
	data, err := os.ReadFile(h.tmuxSpawnLog())
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read tmux spawn log: %w", err)
	}
	return strings.Count(string(data), "\n"), nil
}

// ensureTmuxSpawnShim writes the counting tmux (once per scenario) and
// returns its directory. Plain /bin/sh and a builtin printf: the shim's own
// start-up cost stays as small as it can be.
func (h *ScenarioHarness) ensureTmuxSpawnShim() (string, error) {
	dir := filepath.Dir(h.tmuxSpawnLog())
	shim := filepath.Join(dir, "tmux")
	if _, err := os.Stat(shim); err == nil {
		return dir, nil
	}
	next, err := exec.LookPath("tmux")
	if err != nil {
		return "", fmt.Errorf("resolve the tmux the spawn-counting fixture delegates to: %w", err)
	}
	if next, err = filepath.Abs(next); err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create tmux spawn shim dir: %w", err)
	}
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >>" + shellQuote(h.tmuxSpawnLog()) + "\nexec " + shellQuote(next) + " \"$@\"\n"
	if err := os.WriteFile(shim, []byte(script), 0o700); err != nil {
		return "", fmt.Errorf("write tmux spawn shim: %w", err)
	}
	return dir, nil
}

func startClientCountingTmuxSpawns(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	dir, err := h.ensureTmuxSpawnShim()
	if err != nil {
		return err
	}
	path := dir + string(os.PathListSeparator)
	if h.agentPATHDir != "" {
		path += h.agentPATHDir + string(os.PathListSeparator)
	}
	path += os.Getenv("PATH")
	// An empty DECK_RECONCILE_MS/DECK_PREVIEW_MS is unset to config (the
	// later duplicate wins in os/exec), so both fall back to the defaults.
	client, err := h.StartNamedClient(ctx, name, "PATH="+path, "DECK_RECONCILE_MS=", "DECK_PREVIEW_MS=")
	if err != nil {
		return err
	}
	return client.WaitForFrame(ctx, false, "deck - sessions")
}

// clientCreatesNumberedShellSessions creates count shell sessions through
// the create modal, one at a time, named by replacing NN in pattern with
// 01..count. Each creation waits on its own row settling, as every other
// create step does.
func clientCreatesNumberedShellSessions(ctx context.Context, clientName string, count int, pattern string) error {
	for i := 1; i <= count; i++ {
		name := strings.Replace(pattern, "NN", fmt.Sprintf("%02d", i), 1)
		if err := clientCreatesShellSession(ctx, clientName, name); err != nil {
			return fmt.Errorf("create shell session %d of %d (%q): %w", i, count, name, err)
		}
	}
	return nil
}

// tmuxSpawnsAreCountedOverAWindow is the measurement itself: read the
// shim's count, let the window elapse, read it again. The window is the
// quantity being measured, not a wait for some state to arrive.
func tmuxSpawnsAreCountedOverAWindow(ctx context.Context, clientName string, seconds int) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	if _, err := h.Client(clientName); err != nil {
		return err
	}
	before, err := h.tmuxSpawnCount()
	if err != nil {
		return err
	}
	timer := time.NewTimer(time.Duration(seconds) * time.Second)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
		return ctx.Err()
	}
	after, err := h.tmuxSpawnCount()
	if err != nil {
		return err
	}
	h.tmuxSpawnWindowCount = after - before
	h.tmuxSpawnWindowSeconds = seconds
	return nil
}

func atMostNTmuxProcessesWereSpawned(ctx context.Context, limit int) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	if h.tmuxSpawnWindowSeconds == 0 {
		return fmt.Errorf("no tmux spawn window was measured; the counting step must run first")
	}
	if h.tmuxSpawnWindowCount > limit {
		return fmt.Errorf("deck spawned %d tmux processes in %d s, want at most %d", h.tmuxSpawnWindowCount, h.tmuxSpawnWindowSeconds, limit)
	}
	return nil
}
