package features

import (
	"context"
	"fmt"
	"testing"

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
	sc.Step(`^the emulator cell at column (\d+) has foreground "(#[0-9a-fA-F]{6})"$`, theEmulatorCellHasForeground)
	sc.Step(`^the emulator cell at column (\d+) does not have foreground "(#[0-9a-fA-F]{6})"$`, theEmulatorCellDoesNotHaveForeground)
	sc.Step(`^the emulator cell at column (\d+) has background "(#[0-9a-fA-F]{6})"$`, theEmulatorCellHasBackground)
	sc.Step(`^the emulator cell at column (\d+) is (bold|dim|reverse)$`, theEmulatorCellHasAttribute)
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

// theEmulatorCellHasForeground and its siblings below exercise requirement
// 1's full attribute set (foreground, background, bold/dim/reverse) against
// a bare vt.Emulator fed raw SGR bytes, independent of anything the product
// currently emits, using the same cellForegroundHex/cellBackgroundHex/
// cellHasAttr helpers a real deck client's cells are read with (see
// cell_attributes_test.go). This is also where requirement 1's negative
// proof lives: "does not have foreground" must fail when pointed at a cell
// that does carry the named colour, which
// TestEmulatorCellForegroundNegativeProofCanFail below demonstrates
// directly.
func theEmulatorCellHasForeground(ctx context.Context, column int, want string) error {
	terminal, err := emulatorFromContext(ctx)
	if err != nil {
		return err
	}
	return theEmulatorCellHasForegroundOnTerminal(terminal, column, want)
}

func theEmulatorCellDoesNotHaveForeground(ctx context.Context, column int, unwanted string) error {
	terminal, err := emulatorFromContext(ctx)
	if err != nil {
		return err
	}
	return theEmulatorCellDoesNotHaveForegroundOnTerminal(terminal, column, unwanted)
}

func theEmulatorCellHasBackground(ctx context.Context, column int, want string) error {
	terminal, err := emulatorFromContext(ctx)
	if err != nil {
		return err
	}
	got, err := cellBackgroundHex(terminal.CellAt(column, 0))
	if err != nil {
		return fmt.Errorf("emulator cell at column %d: %w", column, err)
	}
	if got != want {
		return fmt.Errorf("emulator cell at column %d has background %s, want %s", column, got, want)
	}
	return nil
}

func theEmulatorCellHasAttribute(ctx context.Context, column int, attr string) error {
	terminal, err := emulatorFromContext(ctx)
	if err != nil {
		return err
	}
	has, err := cellHasAttr(terminal.CellAt(column, 0), attr)
	if err != nil {
		return fmt.Errorf("emulator cell at column %d: %w", column, err)
	}
	if !has {
		return fmt.Errorf("emulator cell at column %d is not %s", column, attr)
	}
	return nil
}

// TestCellAttributeAssertionsCanFail is requirement 1's explicit negative
// proof: every per-cell attribute assertion this task adds must be able to
// fail when pointed at a cell of a different colour/attribute, never pass
// by falling back to a fabricated default. features/harness.feature's
// @requirement-1-cell-attributes scenarios cover the passing paths (plus
// the "does not have foreground" form, itself a passing use of the same
// failure path); this test drives the underlying assertion functions
// directly so the failure itself -- not just its absence -- is asserted.
func TestCellAttributeAssertionsCanFail(t *testing.T) {
	term := vt.NewEmulator(10, 1)
	// SGR 1;31 is bold red -- ansiHex index 9 (bright red, #ff0000) is what
	// bold applies on top of the basic 31 (index 1, #800000) in most
	// emulators, but what matters here is only that it is a specific,
	// checkable colour distinct from a plain cell's "no foreground set".
	if _, err := term.Write([]byte("\x1b[1;31mX\x1b[0mY")); err != nil {
		t.Fatalf("write to emulator: %v", err)
	}
	redCell := term.CellAt(0, 0)
	plainCell := term.CellAt(1, 0)

	redHex, err := cellForegroundHex(redCell)
	if err != nil {
		t.Fatalf("coloured cell has no readable foreground: %v", err)
	}

	// The positive assertion fails when pointed at the wrong colour.
	if err := theEmulatorCellHasForegroundOnTerminal(term, 0, "#000001"); err == nil {
		t.Fatal("want failure asserting the wrong foreground on the coloured cell, got nil")
	}

	// The negative assertion ("does not have foreground") fails when the
	// cell actually does carry that colour -- the exact hazard requirement 1
	// calls out: a step that cannot fail is not an assertion.
	if err := theEmulatorCellDoesNotHaveForegroundOnTerminal(term, 0, redHex); err == nil {
		t.Fatalf("want failure asserting cell 0 does not have its own foreground %s, got nil", redHex)
	}

	// A plain cell has no foreground set at all, so the positive assertion
	// must also fail there rather than default to some colour.
	if _, err := cellForegroundHex(plainCell); err == nil {
		t.Fatal("want error reading foreground of a plain cell with none set, got nil")
	}

	// And the same passing cases must genuinely pass, so this is a real
	// discriminating assertion rather than one that always fails.
	if err := theEmulatorCellHasForegroundOnTerminal(term, 0, redHex); err != nil {
		t.Fatalf("want success asserting the coloured cell's own foreground %s: %v", redHex, err)
	}
	if err := theEmulatorCellDoesNotHaveForegroundOnTerminal(term, 1, redHex); err != nil {
		t.Fatalf("want success asserting the plain cell lacks %s: %v", redHex, err)
	}
}

// theEmulatorCellHasForegroundOnTerminal/theEmulatorCellDoesNotHaveForegroundOnTerminal
// factor the column/colour comparison out of the context-threading step
// functions above so TestCellAttributeAssertionsCanFail can drive them
// directly against a terminal it built itself, without godog.
func theEmulatorCellHasForegroundOnTerminal(terminal vt.Terminal, column int, want string) error {
	got, err := cellForegroundHex(terminal.CellAt(column, 0))
	if err != nil {
		return fmt.Errorf("emulator cell at column %d: %w", column, err)
	}
	if got != want {
		return fmt.Errorf("emulator cell at column %d has foreground %s, want %s", column, got, want)
	}
	return nil
}

func theEmulatorCellDoesNotHaveForegroundOnTerminal(terminal vt.Terminal, column int, unwanted string) error {
	got, err := cellForegroundHex(terminal.CellAt(column, 0))
	if err != nil {
		return nil
	}
	if got == unwanted {
		return fmt.Errorf("emulator cell at column %d has foreground %s, want anything else", column, got)
	}
	return nil
}
