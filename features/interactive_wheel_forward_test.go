package features

import (
	"context"
	"fmt"
	"time"

	"github.com/cucumber/godog"
)

// registerInteractiveWheelForwardSteps backs
// features/interactive_wheel_forward.feature (R183, GH #59).
func registerInteractiveWheelForwardSteps(sc *godog.ScenarioContext) {
	sc.Step(`^deck client "([^"]+)" sends Ctrl\+C$`, clientSendsCtrlC)
	sc.Step(`^deck client "([^"]+)" scrolls the interactive wheel (up|down) (\d+) times? holding Shift over the line containing "([^"]+)"$`, clientScrollsInteractiveWheelHoldingShift)
}

func clientSendsCtrlC(ctx context.Context, name string) error {
	return sendClientKeys(ctx, name, "\x03")
}

// SGR button bit 4 is the Shift modifier, so a Shift+wheel-up is 64+4 and a
// Shift+wheel-down 65+4.
const sgrShiftBit = 4

func clientScrollsInteractiveWheelHoldingShift(ctx context.Context, name, dir string, times int, text string) error {
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
	button := sgrButtonWheelDown + sgrShiftBit
	if dir == "up" {
		button = sgrButtonWheelUp + sgrShiftBit
	}
	for i := 0; i < times; i++ {
		if err := client.Send(sgrPress(button, col, row)); err != nil {
			return fmt.Errorf("shift wheel notch %d/%d: %w", i+1, times, err)
		}
	}
	time.Sleep(150 * time.Millisecond)
	return nil
}
