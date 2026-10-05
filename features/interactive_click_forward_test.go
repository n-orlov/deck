package features

import (
	"context"
	"fmt"
	"time"

	"github.com/cucumber/godog"
)

// registerInteractiveClickForwardSteps backs
// features/interactive_click_forward.feature (R202, GH #67).
func registerInteractiveClickForwardSteps(sc *godog.ScenarioContext) {
	sc.Step(`^deck client "([^"]+)" clicks the (left|middle|right) button over the line containing "([^"]+)" in the interactive preview$`, clientClicksInteractivePreview)
}

// The xterm base button codes of the three buttons.
var sgrClickButtons = map[string]int{"left": sgrButtonLeft, "middle": 1, "right": 2}

// clientClicksInteractivePreview sends the SGR press and release a terminal
// delivers for one click of the named button, at the first cell of the first
// preview row containing text.
func clientClicksInteractivePreview(ctx context.Context, name, button, text string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	col, row, err := locatePreviewText(client, text)
	if err != nil {
		return err
	}
	code := sgrClickButtons[button]
	if err := client.Send(sgrPress(code, col, row)); err != nil {
		return fmt.Errorf("%s click press: %w", button, err)
	}
	if err := client.Send(sgrRelease(code, col, row)); err != nil {
		return fmt.Errorf("%s click release: %w", button, err)
	}
	time.Sleep(150 * time.Millisecond)
	return nil
}
