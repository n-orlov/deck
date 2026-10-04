package features

import (
	"context"
	"fmt"
	"math"

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

// terminalSize validates a scenario-supplied column and row count and narrows
// them to the uint16 the PTY winsize takes: a step that names a size outside
// 1..65535 is a feature-file mistake and fails the step instead of wrapping.
func terminalSize(cols, rows int) (width, height uint16, err error) {
	if cols < 1 || cols > math.MaxUint16 || rows < 1 || rows > math.MaxUint16 {
		return 0, 0, fmt.Errorf("terminal size %dx%d is outside 1..%d", cols, rows, math.MaxUint16)
	}
	return uint16(cols), uint16(rows), nil
}

func startNamedClientWithSize(ctx context.Context, name string, cols, rows int) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	width, height, err := terminalSize(cols, rows)
	if err != nil {
		return err
	}
	client, err := h.StartNamedClientWithSize(ctx, name, width, height)
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
	width, height, err := terminalSize(cols, rows)
	if err != nil {
		return err
	}
	return client.ResizeAndAwaitRender(ctx, width, height)
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
