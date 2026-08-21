package features

import (
	"context"
	"fmt"
	"testing"

	"github.com/charmbracelet/x/vt"
	"github.com/cucumber/godog"
)

// registerFrameBudgetSteps exposes requirement 6's frame-budget assertion:
// the rendered frame of a named client, or of a bare emulator a scenario fed
// raw bytes directly (see registerEmulatorPlacementSteps), occupies no more
// content lines than a stated row budget and no line wider than a stated
// column budget. Both forms read the grid cell by cell -- the client form
// through ScreenDriver.FrameFitsBudget's oversized shadow capture, never the
// raw escape byte stream -- and report the offending line/count on failure
// rather than a bare boolean.
func registerFrameBudgetSteps(sc *godog.ScenarioContext) {
	sc.Step(`^deck client "([^"]+)" frame fits within (\d+) columns? and (\d+) rows?$`, clientFrameFitsBudget)
	sc.Step(`^deck client "([^"]+)" frame does not fit within (\d+) columns? and (\d+) rows?$`, clientFrameDoesNotFitBudget)
	sc.Step(`^the emulator frame fits within (\d+) columns? and (\d+) rows?$`, emulatorFrameFitsBudget)
	sc.Step(`^the emulator frame does not fit within (\d+) columns? and (\d+) rows?$`, emulatorFrameDoesNotFitBudget)
}

func clientFrameFitsBudget(ctx context.Context, name string, cols, rows int) error {
	client, err := assertionClient(ctx, name)
	if err != nil {
		return err
	}
	if err := client.FrameFitsBudget(cols, rows); err != nil {
		return fmt.Errorf("client %q: %w", name, err)
	}
	return nil
}

// clientFrameDoesNotFitBudget is the negative form: it exists so a scenario
// can prove the check discriminates (a budget too small for even the
// minimum-size notice genuinely fails it) without the scenario itself
// having to fail, mirroring requirement 1's "does not have foreground" form.
func clientFrameDoesNotFitBudget(ctx context.Context, name string, cols, rows int) error {
	client, err := assertionClient(ctx, name)
	if err != nil {
		return err
	}
	if err := client.FrameFitsBudget(cols, rows); err == nil {
		return fmt.Errorf("client %q frame fits within %d columns and %d rows, want it not to", name, cols, rows)
	}
	return nil
}

func emulatorFrameFitsBudget(ctx context.Context, cols, rows int) error {
	terminal, err := emulatorFromContext(ctx)
	if err != nil {
		return err
	}
	return frameFitsBudget(terminal, cols, rows)
}

func emulatorFrameDoesNotFitBudget(ctx context.Context, cols, rows int) error {
	terminal, err := emulatorFromContext(ctx)
	if err != nil {
		return err
	}
	if err := frameFitsBudget(terminal, cols, rows); err == nil {
		return fmt.Errorf("emulator frame fits within %d columns and %d rows, want it not to", cols, rows)
	}
	return nil
}

// frameFitsBudget scans terminal's own grid -- cell by cell, never the raw
// escape stream -- and returns an error naming the offending line/extent
// when content exists beyond a cols x rows budget. A cell holding only a
// blank space does not count as content, matching NormalizeFrame's own
// trailing-blank trim, so a frame padded out to its allocated grid size is
// never mistaken for one that actually overflowed.
func frameFitsBudget(terminal vt.Terminal, cols, rows int) error {
	width, height := terminal.Width(), terminal.Height()
	lastContentRow := -1
	maxLineWidth := 0
	widestRow := -1
	for y := 0; y < height; y++ {
		lineWidth := 0
		for x := 0; x < width; x++ {
			cell := terminal.CellAt(x, y)
			if cell == nil || cell.Content == "" || cell.Content == " " {
				continue
			}
			if end := x + cell.Width; end > lineWidth {
				lineWidth = end
			}
		}
		if lineWidth > 0 {
			lastContentRow = y
		}
		if lineWidth > maxLineWidth {
			maxLineWidth = lineWidth
			widestRow = y
		}
	}
	lineCount := lastContentRow + 1
	if lineCount > rows {
		return fmt.Errorf("frame has %d content lines (last content on row %d), exceeding the budget of %d rows", lineCount, lastContentRow, rows)
	}
	if maxLineWidth > cols {
		return fmt.Errorf("frame's widest line is row %d at %d columns, exceeding the budget of %d columns", widestRow, maxLineWidth, cols)
	}
	return nil
}

// TestFrameBudgetCanFail is requirement 6's explicit negative proof, in the
// same shape as task 001's TestCellAttributeAssertionsCanFail: the check
// must fail when content genuinely overflows a budget, both by height (too
// many content lines) and by width (a line wider than the budget), and it
// must still pass for content that honestly fits -- otherwise it is not a
// discriminating assertion, and a features scenario built on it would go
// green for the wrong reason.
func TestFrameBudgetCanFail(t *testing.T) {
	// A tall emulator receiving five lines of content overflows a
	// three-row budget, and the too-many-lines message names both the
	// observed count and the budget.
	tall := vt.NewEmulator(10, 10)
	if _, err := tall.Write([]byte("one\r\ntwo\r\nthree\r\nfour\r\nfive")); err != nil {
		t.Fatalf("write to emulator: %v", err)
	}
	if err := frameFitsBudget(tall, 10, 3); err == nil {
		t.Fatal("want failure: 5 content lines exceed a budget of 3 rows, got nil")
	}

	// A wide emulator receiving one line longer than the budget overflows
	// by width, even though it easily fits the row budget.
	wide := vt.NewEmulator(20, 3)
	if _, err := wide.Write([]byte("0123456789ABCDEFGHIJ")); err != nil {
		t.Fatalf("write to emulator: %v", err)
	}
	if err := frameFitsBudget(wide, 10, 3); err == nil {
		t.Fatal("want failure: a 20-column line exceeds a budget of 10 columns, got nil")
	}

	// The same wide emulator's content genuinely fits its own real size,
	// so the check must pass there -- proving the failures above are
	// discriminating rather than the function always failing.
	if err := frameFitsBudget(wide, 20, 3); err != nil {
		t.Fatalf("want success: content fits its own real size 20x3: %v", err)
	}

	// A frame padded with trailing blank rows/columns out to the
	// emulator's allocated size must not be mistaken for one that
	// overflowed: only actual content counts.
	padded := vt.NewEmulator(40, 40)
	if _, err := padded.Write([]byte("hello")); err != nil {
		t.Fatalf("write to emulator: %v", err)
	}
	if err := frameFitsBudget(padded, 10, 1); err != nil {
		t.Fatalf("want success: a single short line in a much larger allocated grid fits a 10x1 budget: %v", err)
	}
}
