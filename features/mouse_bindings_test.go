package features

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

// registerMouseBindingSteps backs features/mouse.feature (task 029,
// requirements 33-37, 41). Rather than a hand-computed column/row (which
// would silently drift the moment grouping, elision or a mode change
// shifts where a row actually lands — exactly the failure mode task 028's
// own hitTest guards against product-side), these steps locate the target
// text in the client's own current frame and click through the exact
// cell that text occupies.
func registerMouseBindingSteps(sc *godog.ScenarioContext) {
	sc.Step(`^deck client "([^"]+)" clicks on the row containing "([^"]+)"$`, clientClicksOnRowContaining)
	sc.Step(`^deck client "([^"]+)" double-clicks on the row containing "([^"]+)"$`, clientDoubleClicksOnRowContaining)
	sc.Step(`^deck client "([^"]+)" detaches$`, clientDetaches)
}

// locateText finds text's first occurrence in client's current frame,
// returning the 1-based column/row an SGR mouse report must name to land
// on that exact cell (matching ScreenDriver.Click/DoubleClick/Drag's own
// 1-based convention, features/mouse_synthesis_test.go).
func locateText(client *ScreenDriver, text string) (col, row int, err error) {
	frame := client.Frame(false)
	for i, line := range strings.Split(frame, "\n") {
		if idx := strings.Index(line, text); idx >= 0 {
			return idx + 1, i + 1, nil
		}
	}
	return 0, 0, fmt.Errorf("no line of the frame contains %q:\n%s", text, frame)
}

func clientClicksOnRowContaining(ctx context.Context, clientName, text string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	col, row, err := locateText(client, text)
	if err != nil {
		return err
	}
	return client.Click(col, row)
}

func clientDoubleClicksOnRowContaining(ctx context.Context, clientName, text string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	col, row, err := locateText(client, text)
	if err != nil {
		return err
	}
	return client.DoubleClick(col, row)
}

// clientDetaches sends tmux's detach binding (Ctrl-b d), mirroring
// features/assertions_test.go's clientAttachesAndDetaches/
// clientAttachesToSelectedAgentAndDetaches, but as its own step so a
// scenario that attached via a mouse gesture (rather than `\r`) can detach
// without re-deriving that pair of bytes itself.
func clientDetaches(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	time.Sleep(150 * time.Millisecond)
	if err := client.Send("\x02d"); err != nil {
		return err
	}
	return client.WaitForFrame(ctx, false, "deck - sessions")
}
