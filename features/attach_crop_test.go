package features

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

// registerAttachCropSteps backs features/attach_crop.feature: `a`'s full
// attach must never be cropped by deck's own passive preview fit (SPEC §11).
//
// Both steps compare tmux's own window width against the deck client's real
// terminal width, read from the emulator the scenario is driving
// (ScreenDriver.GridSize) rather than from a constant -- a hard-coded 100
// would keep passing if the harness's default client size ever changed,
// while quietly asserting nothing about the crop.
func registerAttachCropSteps(sc *godog.ScenarioContext) {
	sc.Step(`^within one preview tick the private tmux window for session "([^"]+)" is narrower than deck client "([^"]+)" terminal, and left unpinned$`, privateWindowSettlesFittedAndUnpinned)
	sc.Step(`^the private tmux window for session "([^"]+)" is as wide as deck client "([^"]+)" terminal$`, privateWindowIsAsWideAsClientTerminal)
}

// attachCropSettleTimeout bounds both polls below. Passive fit runs off the
// preview tick (DECK_PREVIEW_MS, milliseconds in the scenario config) and an
// attach's own resize is a single tmux round trip, so this is generous by
// orders of magnitude on purpose: it is a stability margin for a loaded CI
// container, not a description of how long either transition takes.
const attachCropSettleTimeout = 5 * time.Second

// privateWindowSettlesFittedAndUnpinned waits for BOTH halves of what a
// passive fit owes, and waits for them together on purpose: the fit resizes
// and then unpins, so a step that polled only one of them could sample the
// instant between the two and pass on a window that is still pinned.
func privateWindowSettlesFittedAndUnpinned(ctx context.Context, name, clientName string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	target, err := privateWindowTarget(ctx, h, name)
	if err != nil {
		return err
	}
	cols, _ := client.GridSize()

	deadline := time.Now().Add(attachCropSettleTimeout)
	var lastWidth int
	var lastPin string
	for {
		lastWidth, err = privateWindowWidth(ctx, h, target)
		if err != nil {
			return err
		}
		lastPin, err = privateWindowSizeOption(ctx, h, target)
		if err != nil {
			return err
		}
		if lastWidth < cols && lastPin == "" {
			return nil
		}
		if !time.Now().Before(deadline) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if lastWidth >= cols {
		return fmt.Errorf("private tmux window %q is %d columns wide against deck client %q's %d-column terminal, want narrower -- the passive fit never landed, so this scenario cannot say anything about the crop it causes (window-local window-size %q)", target, lastWidth, clientName, cols, lastPin)
	}
	return fmt.Errorf("private tmux window %q is fitted to %d columns but window-local window-size reads %q, want unset -- that pin is what crops the next full attach", target, lastWidth, lastPin)
}

// privateWindowIsAsWideAsClientTerminal is the crop assertion itself: with
// deck's client attached, the window must be the terminal's own width, not
// the preview panel's.
func privateWindowIsAsWideAsClientTerminal(ctx context.Context, name, clientName string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	target, err := privateWindowTarget(ctx, h, name)
	if err != nil {
		return err
	}
	cols, _ := client.GridSize()

	deadline := time.Now().Add(attachCropSettleTimeout)
	var lastWidth int
	for {
		lastWidth, err = privateWindowWidth(ctx, h, target)
		if err != nil {
			return err
		}
		if lastWidth == cols {
			return nil
		}
		if !time.Now().Before(deadline) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	pin, pinErr := privateWindowSizeOption(ctx, h, target)
	if pinErr != nil {
		return pinErr
	}
	return fmt.Errorf("private tmux window %q is %d columns wide with deck client %q's %d-column terminal attached, want the full %d -- the attach is cropped to whatever the preview panel last fitted (window-local window-size %q)", target, lastWidth, clientName, cols, cols, pin)
}

// privateWindowTarget resolves a session's name to the tmux window target
// deck gives it, the same "deck_" + slug shape privateWindowGeometry uses.
func privateWindowTarget(ctx context.Context, h *ScenarioHarness, name string) (string, error) {
	slug, err := sessionSlugByName(h, name)
	if err != nil {
		return "", err
	}
	return "deck_" + slug, nil
}

func privateWindowWidth(ctx context.Context, h *ScenarioHarness, target string) (int, error) {
	output, err := tmuxOutput(ctx, h, "display-message", "-p", "-t", target, "#{window_width}")
	if err != nil {
		return 0, fmt.Errorf("read window width for %q: %w", target, err)
	}
	trimmed := strings.TrimSpace(string(output))
	width, convErr := strconv.Atoi(trimmed)
	if convErr != nil {
		return 0, fmt.Errorf("read window width for %q: parse %q: %w", target, trimmed, convErr)
	}
	return width, nil
}

// privateWindowSizeOption reads the WINDOW-local `window-size` and returns
// "" when it is unset. A builtin window option that was never set
// window-locally prints an empty line and exits 0 (unlike a "@"-prefixed
// user option, which errors) -- the same shape internal/tmux's own
// readBuiltinWindowOption relies on.
func privateWindowSizeOption(ctx context.Context, h *ScenarioHarness, target string) (string, error) {
	output, err := tmuxOutput(ctx, h, "show-options", "-wv", "-t", target, "window-size")
	if err != nil {
		return "", fmt.Errorf("read window-local window-size for %q: %w", target, err)
	}
	return strings.TrimSpace(string(output)), nil
}
