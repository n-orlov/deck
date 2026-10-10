package features

import (
	"context"
	"fmt"
	"time"

	"github.com/cucumber/godog"
)

// registerHostResizeSteps backs features/host_resize.feature (R239, #75).
func registerHostResizeSteps(sc *godog.ScenarioContext) {
	sc.Step(`^within (\d+) seconds the private tmux window for session "([^"]+)" reports geometry "([^"]+)"$`, privateWindowReportsGeometryWithin)
}

// privateWindowReportsGeometryWithin polls the window's real size until it
// is the wanted one. The only thing between the resize and the read is time:
// the scenario sends no key and no mouse event.
func privateWindowReportsGeometryWithin(ctx context.Context, seconds int, name, want string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(time.Duration(seconds) * time.Second)
	for {
		got, gerr := privateWindowGeometry(ctx, h, name)
		if gerr != nil {
			return gerr
		}
		if got == want {
			return nil
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("private tmux window for session %q reports geometry %q after %d seconds, want %q", name, got, seconds, want)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
