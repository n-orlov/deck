package features

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/cucumber/godog"
	"github.com/n-orlov/deck/internal/tmux"
)

// registerInteractiveGeometrySteps wires PRD phase3b II-7/II-8's scenario:
// a bare split window created directly on the scenario's own private
// socket (there is no consumer inside deck itself yet -- task 034 is the
// internal/tmux primitives commit; a later task in the 034-061 range wires
// Enter itself to FitWindowToPane), a step that runs
// internal/tmux.Client.FitWindowToPane against it and records how many
// resizes it took, and the PRD-named negative control: the naive
// pane-targeting alternative that never compensates for chrome, run to its
// own bound so a scenario can assert it never converges.
func registerInteractiveGeometrySteps(sc *godog.ScenarioContext) {
	sc.Step(`^tmux session "([^"]+)" is a bare (\d+)x(\d+) window split with a (\d+)-row sibling pane$`, tmuxSessionIsABareSplitWindow)
	sc.Step(`^deck fits window "([^"]+)" pane "([^"]+)" to (\d+)x(\d+)$`, deckFitsWindowPaneTo)
	sc.Step(`^the fit converged in at most (\d+) resizes$`, theFitConvergedInAtMostResizes)
	sc.Step(`^a naive pane-targeting loop targets window "([^"]+)" pane "([^"]+)" at (\d+)x(\d+) for at most (\d+) attempts$`, naivePaneTargetingLoopTargets)
	sc.Step(`^the naive loop never converges$`, theNaiveLoopNeverConverges)
	sc.Step(`^tmux pane "([^"]+)" is (\d+)x(\d+)$`, tmuxPaneIs)
}

func tmuxSessionIsABareSplitWindow(ctx context.Context, session string, width, height, siblingRows int) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	commandCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if output, err := exec.CommandContext(commandCtx, "tmux", "-L", h.Socket, "new-session", "-d", "-s", session,
		"-x", strconv.Itoa(width), "-y", strconv.Itoa(height)).CombinedOutput(); err != nil {
		return fmt.Errorf("tmux -L %s new-session -d -s %s -x %d -y %d: %w: %s", h.Socket, session, width, height, err, output)
	}
	splitCtx, splitCancel := context.WithTimeout(ctx, 5*time.Second)
	defer splitCancel()
	if output, err := exec.CommandContext(splitCtx, "tmux", "-L", h.Socket, "split-window", "-v", "-t", session,
		"-l", strconv.Itoa(siblingRows)).CombinedOutput(); err != nil {
		return fmt.Errorf("tmux -L %s split-window -v -t %s -l %d: %w: %s", h.Socket, session, siblingRows, err, output)
	}
	return nil
}

func deckFitsWindowPaneTo(ctx context.Context, windowTarget, paneTarget string, width, height int) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	client := tmux.Client{Socket: h.Socket, Timeout: 5 * time.Second}
	resizes, fitErr := client.FitWindowToPane(ctx, windowTarget, paneTarget, width, height)
	h.lastGeometryFitResizes = resizes
	h.lastGeometryFitErr = fitErr
	return nil
}

func theFitConvergedInAtMostResizes(ctx context.Context, max int) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	if h.lastGeometryFitErr != nil {
		return fmt.Errorf("fit did not converge: %w", h.lastGeometryFitErr)
	}
	if h.lastGeometryFitResizes < 1 || h.lastGeometryFitResizes > max {
		return fmt.Errorf("fit converged in %d resizes, want between 1 and %d", h.lastGeometryFitResizes, max)
	}
	return nil
}

// naivePaneTargetingLoop is the alternative PRD II-8 explicitly rejects,
// reproduced here ONLY to demonstrate why it must be rejected: it treats
// the WANTED pane height as if it were the window height to request,
// never compensating for the chrome a sibling pane and its border consume.
// It is not shipped code -- internal/tmux/geometry.go's FitWindowToPane is
// the real, chrome-compensated implementation this negative control exists
// to justify.
func naivePaneTargetingLoop(ctx context.Context, socket, target, paneTarget string, wantWidth, wantHeight, maxAttempts int) (finalHeight int, converged bool, err error) {
	for attempt := 0; attempt < maxAttempts; attempt++ {
		width, height, err := paneSize(ctx, socket, paneTarget)
		if err != nil {
			return 0, false, err
		}
		if width == wantWidth && height == wantHeight {
			return height, true, nil
		}
		// The naive mistake: request the WANTED pane size as the window
		// size directly, never adding the chrome the sibling pane and its
		// border are consuming -- PRD II-8's "resize the window, never the
		// pane" is honoured (this issues resize-window, not the pane
		// command), but the SIZE it requests is wrong.
		if err := resizeWindowRaw(ctx, socket, target, wantWidth, wantHeight); err != nil {
			return 0, false, err
		}
	}
	_, height, err := paneSize(ctx, socket, paneTarget)
	if err != nil {
		return 0, false, err
	}
	return height, false, nil
}

func resizeWindowRaw(ctx context.Context, socket, target string, width, height int) error {
	commandCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(commandCtx, "tmux", "-L", socket, "resize-window", "-t", target,
		"-x", strconv.Itoa(width), "-y", strconv.Itoa(height)).CombinedOutput()
	if err != nil {
		return fmt.Errorf("tmux -L %s resize-window -t %s -x %d -y %d: %w: %s", socket, target, width, height, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func paneSize(ctx context.Context, socket, target string) (width, height int, err error) {
	commandCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(commandCtx, "tmux", "-L", socket, "display-message", "-p", "-t", target,
		"#{pane_width} #{pane_height}").CombinedOutput()
	if err != nil {
		return 0, 0, fmt.Errorf("tmux -L %s display-message -p -t %s pane size: %w: %s", socket, target, err, strings.TrimSpace(string(output)))
	}
	fields := strings.Fields(string(output))
	if len(fields) != 2 {
		return 0, 0, fmt.Errorf("tmux -L %s display-message -p -t %s pane size: unexpected output %q", socket, target, string(output))
	}
	width, err = strconv.Atoi(fields[0])
	if err != nil {
		return 0, 0, err
	}
	height, err = strconv.Atoi(fields[1])
	if err != nil {
		return 0, 0, err
	}
	return width, height, nil
}

func naivePaneTargetingLoopTargets(ctx context.Context, windowTarget, paneTarget string, width, height, maxAttempts int) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	finalHeight, converged, loopErr := naivePaneTargetingLoop(ctx, h.Socket, windowTarget, paneTarget, width, height, maxAttempts)
	if loopErr != nil {
		return loopErr
	}
	h.lastNaiveLoopConverged = converged
	h.lastNaiveLoopFinalHeight = finalHeight
	return nil
}

func theNaiveLoopNeverConverges(ctx context.Context) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	if h.lastNaiveLoopConverged {
		return fmt.Errorf("naive pane-targeting loop converged at pane_height %d; want it to remain stuck, proving the window-targeted chrome-compensated loop is necessary", h.lastNaiveLoopFinalHeight)
	}
	return nil
}

func tmuxPaneIs(ctx context.Context, target string, width, height int) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	gotWidth, gotHeight, err := paneSize(ctx, h.Socket, target)
	if err != nil {
		return err
	}
	if gotWidth != width || gotHeight != height {
		return fmt.Errorf("pane %q is %dx%d, want %dx%d", target, gotWidth, gotHeight, width, height)
	}
	return nil
}
