package features

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

// readPaneHistoryLimit reads the EFFECTIVE history-limit tmux applies to
// target -- what `#{history-limit}` resolves to for that pane once tmux has
// merged whichever scope (window-local override, session, or the
// server-global default) actually governs it. This is deliberately not
// task 028's scope-aware `show-options` read: requirement 15 (II-6) is
// about what a pane's scrollback actually is, not which table it came
// from, and the tmux default of 2000 only shows up at all if the read is
// the effective value -- a scope-only read of an unset window option would
// report "none" and never surface the 2000 that requirement 15's whole
// point is to distinguish from whatever deck sets.
func readPaneHistoryLimit(ctx context.Context, socket, target string) (int, error) {
	commandCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(commandCtx, "tmux", "-L", socket, "display-message", "-p", "-t", target, "#{history-limit}").CombinedOutput()
	trimmed := strings.TrimSpace(string(output))
	if err != nil {
		return 0, fmt.Errorf("tmux -L %s display-message -p -t %s #{history-limit}: %w: %s", socket, target, err, trimmed)
	}
	limit, convErr := strconv.Atoi(trimmed)
	if convErr != nil {
		return 0, fmt.Errorf("tmux -L %s display-message -p -t %s #{history-limit}: non-numeric output %q", socket, target, trimmed)
	}
	return limit, nil
}

// assertPaneHistoryLimit asserts target's effective history-limit is
// exactly want, distinguishing tmux's own default (2000) from whatever
// value deck's own bootstrap sets -- a step that merely checked "is a
// number" or "is non-default" would not catch deck setting the wrong
// non-default value, and a step that checked scope alone (task 028) would
// not catch a value inherited from the session or server-global table
// rather than the window deck actually targeted.
func assertPaneHistoryLimit(ctx context.Context, socket, target string, want int) error {
	got, err := readPaneHistoryLimit(ctx, socket, target)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("pane %q effective history-limit = %d, want %d", target, got, want)
	}
	return nil
}

// registerPaneHistoryLimitSteps wires the requirement-15 (II-6) harness
// prerequisite: a step that can prove a pane created after deck's bootstrap
// carries deck's own history-limit rather than tmux's unmodified default of
// 2000. Task 032 (II-15) is the first consumer, asserting this against a
// pane created through deck's own Client.Bootstrap.
func registerPaneHistoryLimitSteps(sc *godog.ScenarioContext) {
	sc.Step(`^tmux pane "([^"]+)" effective history-limit is (\d+)$`, tmuxPaneEffectiveHistoryLimitIs)
}

func tmuxPaneEffectiveHistoryLimitIs(ctx context.Context, target string, want int) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	return assertPaneHistoryLimit(ctx, h.Socket, target, want)
}
