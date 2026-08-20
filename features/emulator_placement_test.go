package features

import (
	"context"
	"fmt"

	"github.com/charmbracelet/x/vt"
	"github.com/cucumber/godog"
)

// emulatorScenarioKey threads a bare screen emulator through a scenario. This
// is deliberately independent of ScenarioHarness/ScreenDriver: this scenario
// asserts a property of the emulator library task 002 chose, not of the
// released deck binary, so it must keep working with no deck process at all.
type emulatorScenarioKey struct{}

func emulatorFromContext(ctx context.Context) (vt.Terminal, error) {
	terminal, ok := ctx.Value(emulatorScenarioKey{}).(vt.Terminal)
	if !ok {
		return nil, fmt.Errorf("no fresh terminal emulator started in this scenario")
	}
	return terminal, nil
}

func registerEmulatorPlacementSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a fresh terminal emulator sized (\d+)x(\d+)$`, freshTerminalEmulatorSized)
	sc.Step(`^the emulator receives "([^"]*)"$`, theEmulatorReceives)
	sc.Step(`^the emulator cell at column (\d+) has width (\d+) and content "([^"]*)"$`, theEmulatorCellHasWidthAndContent)
	sc.Step(`^the emulator cell at column (\d+) is a continuation cell$`, theEmulatorCellIsAContinuationCell)
	sc.Step(`^the emulator cell at column (\d+) has content "([^"]*)"$`, theEmulatorCellHasContent)
}

func freshTerminalEmulatorSized(ctx context.Context, width, height int) (context.Context, error) {
	terminal := vt.NewEmulator(width, height)
	return context.WithValue(ctx, emulatorScenarioKey{}, terminal), nil
}

func theEmulatorReceives(ctx context.Context, text string) error {
	terminal, err := emulatorFromContext(ctx)
	if err != nil {
		return err
	}
	_, err = terminal.Write([]byte(text))
	return err
}

func theEmulatorCellHasWidthAndContent(ctx context.Context, column, width int, content string) error {
	terminal, err := emulatorFromContext(ctx)
	if err != nil {
		return err
	}
	cell := terminal.CellAt(column, 0)
	if cell == nil {
		return fmt.Errorf("emulator cell at column %d is nil, want width %d content %q", column, width, content)
	}
	if cell.Width != width || cell.Content != content {
		return fmt.Errorf("emulator cell at column %d = width %d content %q, want width %d content %q", column, cell.Width, cell.Content, width, content)
	}
	return nil
}

func theEmulatorCellIsAContinuationCell(ctx context.Context, column int) error {
	terminal, err := emulatorFromContext(ctx)
	if err != nil {
		return err
	}
	cell := terminal.CellAt(column, 0)
	if cell != nil && (cell.Width != 0 || cell.Content != "") {
		return fmt.Errorf("emulator cell at column %d = width %d content %q, want a continuation placeholder (width 0, empty content)", column, cell.Width, cell.Content)
	}
	return nil
}

func theEmulatorCellHasContent(ctx context.Context, column int, content string) error {
	terminal, err := emulatorFromContext(ctx)
	if err != nil {
		return err
	}
	cell := terminal.CellAt(column, 0)
	if cell == nil {
		return fmt.Errorf("emulator cell at column %d is nil, want content %q", column, content)
	}
	if cell.Content != content {
		return fmt.Errorf("emulator cell at column %d = content %q, want %q", column, cell.Content, content)
	}
	return nil
}
