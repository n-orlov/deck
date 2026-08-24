package features

import (
	"context"
	"fmt"

	"github.com/cucumber/godog"
)

// registerResizeSteps wires the mid-scenario pty-resize surface (requirement
// 1): a scenario can start a client at an explicit geometry, resize it later
// via ScreenDriver.Resize (TIOCSWINSZ, with SIGWINCH left to the kernel), and
// assert the emulator grid the frame is read from actually changed size.
func registerResizeSteps(sc *godog.ScenarioContext) {
	sc.Step(`^deck client "([^"]+)" is started with terminal size (\d+)x(\d+)$`, startNamedClientWithSize)
	sc.Step(`^deck client "([^"]+)" terminal is resized to (\d+)x(\d+)$`, resizeNamedClient)
	sc.Step(`^deck client "([^"]+)" frame width is (\d+)$`, clientFrameWidthIs)
	sc.Step(`^deck client "([^"]+)" frame height is (\d+)$`, clientFrameHeightIs)
}

func startNamedClientWithSize(ctx context.Context, name string, cols, rows int) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.StartNamedClientWithSize(ctx, name, uint16(cols), uint16(rows))
	if err != nil {
		return err
	}
	return client.WaitForFrame(ctx, false, "deck - sessions")
}

// resizeNamedClient blocks until deck has observably re-rendered at the new
// size (task 202/F3): ScreenDriver.ResizeAndAwaitRender changes the kernel
// and emulator geometry exactly as Resize did, then polls the driver's own
// raw output for the render marker a resize-triggered flush writes, so the
// very next step (for example requirement 48's Enter into interactive mode)
// never races a frame that is still shaped for the pre-resize size.
func resizeNamedClient(ctx context.Context, name string, cols, rows int) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	return client.ResizeAndAwaitRender(ctx, uint16(cols), uint16(rows))
}

func clientFrameWidthIs(ctx context.Context, name string, want int) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	cols, _ := client.GridSize()
	if cols != want {
		return fmt.Errorf("client %q frame width = %d, want %d", name, cols, want)
	}
	return nil
}

func clientFrameHeightIs(ctx context.Context, name string, want int) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	_, rows := client.GridSize()
	if rows != want {
		return fmt.Errorf("client %q frame height = %d, want %d", name, rows, want)
	}
	return nil
}
