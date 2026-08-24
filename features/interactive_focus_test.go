package features

import (
	"context"
	"time"

	"github.com/cucumber/godog"
)

// registerInteractiveFocusSteps backs features/interactive_focus.feature
// (PRD Part II requirement 44 / task 063): the two steps a real deck
// client needs to drive interactive mode's own keymap through a real pty
// (Enter to enter, Ctrl+Q -- byte 0x11 -- to leave), reused wherever a
// scenario needs to cross that boundary rather than reaching into
// internal/tmux directly the way interactive_sigwinch_budget_test.go's
// steps do.
func registerInteractiveFocusSteps(sc *godog.ScenarioContext) {
	sc.Step(`^deck client "([^"]+)" enters interactive mode$`, clientEntersInteractiveMode)
	sc.Step(`^deck client "([^"]+)" leaves interactive mode$`, clientLeavesInteractiveMode)
}

// clientEntersInteractiveMode sends a bare Enter (SPEC §11.9, task 061:
// Enter's new job once a session is selected and running), then pauses
// briefly for the entry sequence (claim ownership, fit the window, start
// the transport) and the resulting repaint to land before the next step
// reads the frame -- the same pacing gotcha every other keystroke-then-
// assert step in this package already needs (sendClientKeys's own doc
// comment).
func clientEntersInteractiveMode(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	if err := client.Send("\r"); err != nil {
		return err
	}
	time.Sleep(150 * time.Millisecond)
	return nil
}

// clientLeavesInteractiveMode sends Ctrl+Q (byte 0x11), the one bound exit
// chord (task 061/internal/tui/interactive.go's updateInteractive), then
// pauses the same way clientEntersInteractiveMode does for exit's own
// teardown (restore geometry, release ownership) and repaint to land.
func clientLeavesInteractiveMode(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	if err := client.Send("\x11"); err != nil {
		return err
	}
	time.Sleep(150 * time.Millisecond)
	return nil
}
