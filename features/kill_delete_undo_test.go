package features

import (
	"context"
	"time"

	"github.com/cucumber/godog"
)

// registerKillDeleteUndoSteps backs PRD requirement 22: x still kills with
// no confirmation and still refuses an already-stopped row, but a
// successful kill leaves a toast naming undo, actionable for exactly
// DECK_UNDO_MS, that u resumes; once the window is gone (expired, or
// already spent by an earlier u), u does nothing (task 102). Tasks 105/106
// add the dd tombstone/grace scenarios to this same file.
func registerKillDeleteUndoSteps(sc *godog.ScenarioContext) {
	sc.Step(`^deck client "([^"]+)" is started with a short undo window$`, clientStartedWithShortUndoWindow)
	sc.Step(`^deck client "([^"]+)" presses u$`, clientPressesUndo)
	sc.Step(`^(\d+) milliseconds pass$`, millisecondsPass)
}

// clientStartedWithShortUndoWindow sets DECK_UNDO_MS short enough that a
// scenario can wait it out in real wall-clock time (the undo toast's expiry
// is scheduled via a real tea.Tick, not the frozen DECK_CLOCK -- see
// internal/tui.Model's sessionKilled/undoExpired handling) without the
// scenario itself taking anywhere near the real 10s default.
func clientStartedWithShortUndoWindow(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.StartNamedClient(ctx, name, "DECK_UNDO_MS=200")
	if err != nil {
		return err
	}
	return client.WaitForFrame(ctx, false, "deck - sessions")
}

// clientPressesUndo sends u -- requirement 22's undo key, which always
// targets the most recently killed session (internal/tui.Model's
// undoSessionID), never the current selection.
func clientPressesUndo(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	return client.Send("u")
}

// millisecondsPass is a real-wall-clock wait, not a frozen-clock advance:
// the undo toast's expiry is scheduled via a real tea.Tick against
// DECK_UNDO_MS (internal/tui.Model's sessionKilled/undoExpired handling),
// never against m.settings.Clock, so a scenario proving the window has
// expired must wait it out for real.
func millisecondsPass(ctx context.Context, ms int) error {
	timer := time.NewTimer(time.Duration(ms) * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
