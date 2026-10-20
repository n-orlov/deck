package features

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

// registerActivityResizeInertSteps backs features/activity_resize_inert.feature
// (R412). The pane's program is a real process in a real tmux window: it draws
// a wrapped transcript line, a composer frame and a footer once, and from then
// on only rewrites its footer line when a line arrives on its stdin. It
// ignores SIGWINCH and redraws nothing when the window is resized, so the
// reflow tmux does to the wrapped line is the only thing a resize changes.
func registerActivityResizeInertSteps(sc *godog.ScenarioContext) {
	sc.Step(`^the private tmux pane for session "([^"]+)" draws a wrapped transcript, a composer frame and the footer "([^"]+)"$`, paneDrawsComposerFrame)
	sc.Step(`^the private tmux pane for session "([^"]+)" churns its footer to "([^"]+)" in a later (\d+)-second output bucket$`, paneChurnsFooterInLaterBucket)
	sc.Step(`^the private tmux window for session "([^"]+)" is resized by another process to (\d+)x(\d+)$`, privateWindowResizedByAnotherProcess)
	sc.Step(`^the private tmux window for session "([^"]+)" no longer matches "([^"]+)"$`, privateWindowNoLongerMatches)
	sc.Step(`^after (\d+) configured reconcile intervals deck client "([^"]+)" screen shows sessions in this order:$`, afterIntervalsSessionsInOrder)
}

// activityPaneScript draws the pane and then rewrites only its footer. The
// transcript line is far wider than any pane the scenario produces, so it is
// wrapped by the terminal, and tmux re-wraps it at the new width on a resize.
const activityPaneScript = `#!/bin/sh
stty -echo
rule='────────────────────'
printf '\033[2J\033[H'
printf '%s\n' "● $1"
printf '%s\n%s\n%s\n%s\n' "$rule" '❯ ' "$rule" "$2"
while IFS= read -r footer; do
	printf '\033[1A\033[2K\r%s\n' "$footer"
done
`

const activityPaneTranscript = "Reading the reconcile loop, the output clock and the sidebar's sort comparator to see how " +
	"a pane that is resized by a click reflows the transcript it already holds before any output arrives"

func paneDrawsComposerFrame(ctx context.Context, name, footer string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	slug, err := sessionSlugByName(h, name)
	if err != nil {
		return err
	}
	script := filepath.Join(h.Home, "activity-pane.sh")
	if err := os.WriteFile(script, []byte(activityPaneScript), 0o700); err != nil {
		return fmt.Errorf("write activity pane script: %w", err)
	}
	// Every draw starts in a bucket later than any pane's last output, so the
	// sessions' last_output_at differ by a whole bucket and the order is total.
	if err := waitForLaterOutputBucket(ctx, h, 2); err != nil {
		return err
	}
	target := "deck_" + slug
	line := fmt.Sprintf("exec sh %s %s %s", script, shellQuote(activityPaneTranscript), shellQuote(footer))
	if _, err := tmuxOutput(ctx, h, "send-keys", "-t", target, "-l", line); err != nil {
		return fmt.Errorf("type the pane program into %q: %w", target, err)
	}
	if _, err := tmuxOutput(ctx, h, "send-keys", "-t", target, "Enter"); err != nil {
		return fmt.Errorf("start the pane program in %q: %w", target, err)
	}
	return waitForPaneText(ctx, h, target, footer)
}

func paneChurnsFooterInLaterBucket(ctx context.Context, name, footer string, bucketSeconds int) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	slug, err := sessionSlugByName(h, name)
	if err != nil {
		return err
	}
	if err := waitForLaterOutputBucket(ctx, h, bucketSeconds); err != nil {
		return err
	}
	target := "deck_" + slug
	before, err := windowActivity(ctx, h, target)
	if err != nil {
		return err
	}
	if _, err := tmuxOutput(ctx, h, "send-keys", "-t", target, "-l", footer); err != nil {
		return fmt.Errorf("type the footer into %q: %w", target, err)
	}
	if _, err := tmuxOutput(ctx, h, "send-keys", "-t", target, "Enter"); err != nil {
		return fmt.Errorf("submit the footer into %q: %w", target, err)
	}
	if err := waitForPaneText(ctx, h, target, footer); err != nil {
		return err
	}
	// The churn must have moved tmux's window_activity into a later bucket, or
	// the clock would have nothing to examine and the scenario would prove
	// nothing.
	return pollUntil(5*time.Second, func() error {
		after, err := windowActivity(ctx, h, target)
		if err != nil {
			return err
		}
		if after/int64(bucketSeconds) <= before/int64(bucketSeconds) {
			return fmt.Errorf("window_activity of %q stayed in bucket %d (%d -> %d)", target, before/int64(bucketSeconds), before, after)
		}
		return nil
	})
}

func privateWindowNoLongerMatches(ctx context.Context, name, label string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	was, ok := h.windowGeometrySnapshots[label]
	if !ok {
		return fmt.Errorf("no private tmux window geometry was captured as %q", label)
	}
	return pollUntil(5*time.Second, func() error {
		now, err := privateWindowGeometry(ctx, h, name)
		if err != nil {
			return err
		}
		if now == was {
			return fmt.Errorf("private tmux window for session %q is still %s: the click did not resize it", name, now)
		}
		return nil
	})
}

// afterIntervalsSessionsInOrder lets the clock take n more readings and then
// reads the order once; it does not poll, so a row that moved and moved back
// is not forgiven by the retry.
func afterIntervalsSessionsInOrder(ctx context.Context, intervals int, clientName string, table *godog.Table) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	timer := time.NewTimer(time.Duration(intervals)*scenarioReconcileInterval + 100*time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
	}
	return sidebarShowsSessionsInOrder(client.Frame(false), table)
}

// sidebarShowsSessionsInOrder checks the order of the sidebar's own rows only.
// Once a click has entered interactive mode the preview panel's title names
// the clicked session too, above every row, so the whole-frame search the
// other order steps use would find that title first.
func sidebarShowsSessionsInOrder(frame string, table *godog.Table) error {
	var sidebar []string
	for _, line := range strings.Split(frame, "\n") {
		if !strings.HasPrefix(line, "|") {
			continue
		}
		rest := line[1:]
		if end := strings.Index(rest, "|"); end >= 0 {
			rest = rest[:end]
		}
		sidebar = append(sidebar, rest)
	}
	previous, previousName := -1, ""
	for _, row := range table.Rows {
		name := row.Cells[0].Value
		index := -1
		for i, line := range sidebar {
			if strings.Contains(line, name) {
				index = i
				break
			}
		}
		if index < 0 {
			return fmt.Errorf("sidebar shows no row for %q:\n%s", name, frame)
		}
		if index <= previous {
			return fmt.Errorf("sidebar shows %q at row %d, want it strictly after %q at row %d:\n%s", name, index, previousName, previous, frame)
		}
		previous, previousName = index, name
	}
	return nil
}

func windowActivity(ctx context.Context, h *ScenarioHarness, target string) (int64, error) {
	out, err := tmuxOutput(ctx, h, "display-message", "-p", "-t", target, "#{window_activity}")
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
}

// waitForLaterOutputBucket blocks until the wall clock is in a bucket later
// than the newest window_activity on the private server.
func waitForLaterOutputBucket(ctx context.Context, h *ScenarioHarness, bucketSeconds int) error {
	width := int64(bucketSeconds)
	return pollUntil(time.Duration(bucketSeconds)*3*time.Second, func() error {
		out, err := tmuxOutput(ctx, h, "list-windows", "-a", "-F", "#{window_activity}")
		if err != nil {
			return err
		}
		var newest int64
		for _, field := range strings.Fields(string(out)) {
			at, err := strconv.ParseInt(field, 10, 64)
			if err != nil {
				return fmt.Errorf("window_activity %q: %w", field, err)
			}
			if at > newest {
				newest = at
			}
		}
		if now := time.Now().Unix(); now/width <= newest/width {
			return fmt.Errorf("still in output bucket %d (newest window_activity %d)", now/width, newest)
		}
		return nil
	})
}

func waitForPaneText(ctx context.Context, h *ScenarioHarness, target, want string) error {
	return pollUntil(5*time.Second, func() error {
		out, err := tmuxOutput(ctx, h, "capture-pane", "-p", "-t", target)
		if err != nil {
			return err
		}
		if !strings.Contains(string(out), want) {
			return fmt.Errorf("pane %q does not show %q yet:\n%s", target, want, out)
		}
		return nil
	})
}

// privateWindowResizedByAnotherProcess resizes the session's window with tmux
// itself, the way a second deck client or a real tmux client would. The
// session is not the one the list has selected, so deck does not re-fit it.
func privateWindowResizedByAnotherProcess(ctx context.Context, name string, width, height int) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	slug, err := sessionSlugByName(h, name)
	if err != nil {
		return err
	}
	target := "deck_" + slug
	if _, err := tmuxOutput(ctx, h, "resize-window", "-t", target, "-x", strconv.Itoa(width), "-y", strconv.Itoa(height)); err != nil {
		return fmt.Errorf("resize %q to %dx%d: %w", target, width, height, err)
	}
	want := fmt.Sprintf("%dx%d", width, height)
	return pollUntil(5*time.Second, func() error {
		got, err := privateWindowGeometry(ctx, h, name)
		if err != nil {
			return err
		}
		if got != want {
			return fmt.Errorf("private tmux window for session %q is %s, want %s", name, got, want)
		}
		return nil
	})
}
