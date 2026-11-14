package features

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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

// tmuxSpawnLines is every tmux invocation the shim has seen so far, one
// argument line per process.
func (h *ScenarioHarness) tmuxSpawnLines() ([]string, error) {
	data, err := os.ReadFile(h.tmuxSpawnLog())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read tmux spawn log: %w", err)
	}
	lines := strings.SplitAfter(string(data), "\n")
	// A line still being appended has no newline yet; it is counted on the
	// next read, once it is whole.
	if n := len(lines); n > 0 && !strings.HasSuffix(lines[n-1], "\n") {
		lines = lines[:n-1]
	}
	for i := range lines {
		lines[i] = strings.TrimSuffix(lines[i], "\n")
	}
	return lines, nil
}

// tmuxSpawnCommand is a shim line without its leading "-L <socket>": the
// tmux command deck asked for.
func tmuxSpawnCommand(line string) string {
	if rest, ok := strings.CutPrefix(line, "-L "); ok {
		if _, cmd, ok := strings.Cut(rest, " "); ok {
			return cmd
		}
	}
	return line
}

// isPerTickTmuxSpawn reports whether a shim line is one of R184/R185's
// steady-state per-tick reads: the reconcile tick's one `list-panes -a`, or
// a preview tick's one `display-message -p -t <target> #{pane_dead}|...`
// (passive: the capture-pane rides in the same process; interactive: the
// liveness/attach/owner read). Everything else is one-off work.
func isPerTickTmuxSpawn(line string) bool {
	cmd := tmuxSpawnCommand(line)
	if strings.HasPrefix(cmd, "list-panes -a ") {
		return true
	}
	if rest, ok := strings.CutPrefix(cmd, "display-message -p -t "); ok {
		_, format, _ := strings.Cut(rest, " ")
		return strings.HasPrefix(format, "#{pane_dead}|")
	}
	return false
}

// tmuxSpawnSettleSpan is how long the shim must see nothing but per-tick
// reads before the window opens: two of the product's default 500 ms
// reconcile intervals, so reconcile and preview ticks both land inside it.
// tmuxSpawnSettleDeadline bounds the wait.
const (
	tmuxSpawnSettleSpan     = time.Second
	tmuxSpawnSettleDeadline = 20 * time.Second
)

// waitForTmuxSpawnsToSettle waits until the client's spawns have been only
// per-tick reads for tmuxSpawnSettleSpan, and returns the log length at that
// moment, where the window opens.
//
// The steps before the window (creating the sessions, selecting one,
// entering interactive mode) leave one-off tmux work behind that no frame
// shows has finished: the new selection's passive fit (show-options,
// display-message, resize-window, set-option), and the output clock's first
// capture of each new pane plus its re-baseline after a fit resize
// (capture-pane). Painting the last row proves none of that is over, and
// under load it runs later. A window opened on the frame alone therefore
// counted a load-dependent slice of setup as per-tick cost (GH #48, #52:
// 77 > 75, with every extra spawn in the window's first second).
//
// A client that never settles fails here and names what it keeps spawning:
// one-off work that keeps recurring is itself the cost R185 forbids.
func (h *ScenarioHarness) waitForTmuxSpawnsToSettle(ctx context.Context) (int, error) {
	lines, err := h.tmuxSpawnLines()
	if err != nil {
		return 0, err
	}
	seen := len(lines)
	quietSince := time.Now()
	deadline := quietSince.Add(tmuxSpawnSettleDeadline)
	var recent []string
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		if time.Since(quietSince) >= tmuxSpawnSettleSpan {
			return seen, nil
		}
		if time.Now().After(deadline) {
			return 0, fmt.Errorf("deck's tmux spawns did not settle to per-tick reads for %s within %s; one-off spawns kept arriving, latest: %q", tmuxSpawnSettleSpan, tmuxSpawnSettleDeadline, recent)
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return 0, ctx.Err()
		}
		if lines, err = h.tmuxSpawnLines(); err != nil {
			return 0, err
		}
		for _, line := range lines[seen:] {
			if isPerTickTmuxSpawn(line) {
				continue
			}
			quietSince = time.Now()
			recent = append(recent, tmuxSpawnCommand(line))
			if len(recent) > 5 {
				recent = recent[1:]
			}
		}
		seen = len(lines)
	}
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

// tmuxSpawnsAreCountedOverAWindow is the measurement itself: wait for the
// client's spawns to settle to per-tick reads, then count the shim's lines
// across the window. The window is the quantity being measured, not a wait
// for some state to arrive.
func tmuxSpawnsAreCountedOverAWindow(ctx context.Context, clientName string, seconds int) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	if _, err := h.Client(clientName); err != nil {
		return err
	}
	before, err := h.waitForTmuxSpawnsToSettle(ctx)
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
	lines, err := h.tmuxSpawnLines()
	if err != nil {
		return err
	}
	h.tmuxSpawnWindowLines = lines[before:]
	h.tmuxSpawnWindowCount = len(h.tmuxSpawnWindowLines)
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
		return fmt.Errorf("deck spawned %d tmux processes in %d s, want at most %d; by command:\n%s", h.tmuxSpawnWindowCount, h.tmuxSpawnWindowSeconds, limit, tmuxSpawnBreakdown(h.tmuxSpawnWindowLines))
	}
	return nil
}

// tmuxSpawnBreakdown counts a window's spawns by command (the first word,
// plus the format for display-message), most frequent first, so an
// over-budget failure says what was spawned and not only how much.
func tmuxSpawnBreakdown(lines []string) string {
	counts := map[string]int{}
	for _, line := range lines {
		fields := strings.Fields(tmuxSpawnCommand(line))
		key := ""
		switch {
		case len(fields) >= 5 && fields[0] == "display-message":
			key = "display-message " + fields[4]
		case len(fields) > 0:
			key = fields[0]
		}
		counts[key]++
	}
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if counts[keys[i]] != counts[keys[j]] {
			return counts[keys[i]] > counts[keys[j]]
		}
		return keys[i] < keys[j]
	})
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "  %3d %s\n", counts[k], k)
	}
	return b.String()
}
