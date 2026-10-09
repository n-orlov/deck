package features

import (
	"context"
	"strings"
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
	sc.Step(`^deck client "([^"]+)" forces entry into interactive mode$`, clientForcesEntryIntoInteractiveMode)
}

// interactiveSettleBound is the fixed pause the enter/leave steps used to
// take after their key, and is now only the longest they wait for the mode
// change to show: SendAwaitingInteractive returns the moment the frame does.
const interactiveSettleBound = 150 * time.Millisecond

// interactiveFrameMarker is the text only interactive mode's frame carries
// (the preview title's "Ctrl+Q to leave" and the footer's "Ctrl+Q leave
// interactive mode").
const interactiveFrameMarker = "Ctrl+Q"

// SendAwaitingInteractive writes keys, then waits until the frame shows
// interactive mode (wantInteractive) or the list view again (!wantInteractive),
// for at most interactiveSettleBound. deck's Update claims the window, arms
// the transport and flips its interactive flag (or tears all of that down)
// synchronously before the next View, so a frame that already shows the new
// mode proves the whole entry or exit sequence has finished -- what the
// fixed pause only hoped for. When the frame never shows the change (a
// refused entry, a cropped title) the wait runs out the same bound the fixed
// pause had, and a frame that already showed the target mode before the
// write proves nothing, so it is paced for the full bound as before.
func (d *ScreenDriver) SendAwaitingInteractive(ctx context.Context, keys string, wantInteractive bool) error {
	showing := func() bool { return strings.Contains(d.Frame(false), interactiveFrameMarker) }
	already := showing() == wantInteractive
	if err := d.Send(keys); err != nil {
		return err
	}
	if already {
		time.Sleep(interactiveSettleBound)
		return nil
	}
	waitCtx, cancel := context.WithTimeout(ctx, interactiveSettleBound)
	defer cancel()
	for showing() != wantInteractive {
		select {
		case <-d.updated:
		case <-d.done:
			return nil
		case <-waitCtx.Done():
			return nil
		}
	}
	return nil
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
	return client.SendAwaitingInteractive(ctx, "\r", true)
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
	return client.SendAwaitingInteractive(ctx, "\x11", false)
}

// clientForcesEntryIntoInteractiveMode sends a bare `F` (task 105's list-mode
// binding to enterInteractiveBody(true), SPEC "F forces entry over whoever
// holds the window"), the one entry path that skips the attached-client
// refusal and steals a live ownership claim instead of standing down for it
// (internal/tui/force_enter_test.go's TestForceEntersDespiteAnAttachedClient
// proves the same claim-stealing at the model level; this is its real-tmux,
// real-pty counterpart). Paced identically to clientEntersInteractiveMode:
// the claim steal, window resize and transport start all need to land
// before the next step reads the frame.
func clientForcesEntryIntoInteractiveMode(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	return client.SendAwaitingInteractive(ctx, "F", true)
}
