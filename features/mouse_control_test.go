package features

import (
	"context"
	"fmt"
	"strings"

	"github.com/cucumber/godog"
)

// registerMouseControlSteps covers requirement 3: DECK_MOUSE is a boolean
// runtime control that overrides [ui] mouse (defaulting to true) for SGR
// mouse reporting. Rather than re-parsing config.toml, these steps observe
// the one externally visible effect of the setting on the released binary:
// bubbletea's mouse-cell-motion enable sequence ("\x1b[?1002h...\x1b[?1006h")
// appears in the raw pty stream on start only when reporting is on.
const sgrMouseEnableSequence = "\x1b[?1002h"

func registerMouseControlSteps(sc *godog.ScenarioContext) {
	sc.Step(`^the deck config disables mouse reporting$`, deckConfigDisablesMouse)
	sc.Step(`^deck client "([^"]+)" raw output enabled SGR mouse reporting$`, clientRawOutputEnabledMouseReporting)
	sc.Step(`^deck client "([^"]+)" raw output did not enable SGR mouse reporting$`, clientRawOutputDidNotEnableMouseReporting)
}

// deckConfigDisablesMouse sets DECK_MOUSE=0 for every client subsequently
// started in this scenario, mirroring deckConfigAllowsYolo's pattern of
// mutating scenario-scoped state before the client under test is started.
func deckConfigDisablesMouse(ctx context.Context) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	h.clientEnv = append(h.clientEnv, "DECK_MOUSE=0")
	return nil
}

func clientRawOutputEnabledMouseReporting(ctx context.Context, name string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	if !strings.Contains(client.Raw(), sgrMouseEnableSequence) {
		return fmt.Errorf("deck client %q raw output did not enable SGR mouse reporting: %q", name, client.Raw())
	}
	return nil
}

func clientRawOutputDidNotEnableMouseReporting(ctx context.Context, name string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	if strings.Contains(client.Raw(), sgrMouseEnableSequence) {
		return fmt.Errorf("deck client %q raw output unexpectedly enabled SGR mouse reporting: %q", name, client.Raw())
	}
	return nil
}
