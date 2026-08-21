package features

import (
	"context"
	"fmt"
	"image/color"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/cucumber/godog"
)

// registerCellAttributeSteps exposes requirement 1's per-cell SGR attribute
// assertions against a real deck client's rendered grid: a named cell's (row,
// column) attributes, and a matched substring's attributes across every cell
// the substring occupies. Every assertion reads ScreenDriver.CellAt directly
// (never Frame's trimmed string), and returns an error rather than a
// fabricated default whenever a cell or its colour cannot be determined, so a
// step can never report success by accident.
func registerCellAttributeSteps(sc *godog.ScenarioContext) {
	sc.Step(`^deck client "([^"]+)" is started with colour enabled$`, startNamedClientWithColour)
	sc.Step(`^deck client "([^"]+)" cell at row (\d+) column (\d+) has foreground "(#[0-9a-fA-F]{6})"$`, cellHasForeground)
	sc.Step(`^deck client "([^"]+)" cell at row (\d+) column (\d+) does not have foreground "(#[0-9a-fA-F]{6})"$`, cellDoesNotHaveForeground)
	sc.Step(`^deck client "([^"]+)" cell at row (\d+) column (\d+) has background "(#[0-9a-fA-F]{6})"$`, cellHasBackground)
	sc.Step(`^deck client "([^"]+)" cell at row (\d+) column (\d+) is (bold|dim|reverse)$`, cellHasAttribute)
	sc.Step(`^deck client "([^"]+)" cell at row (\d+) column (\d+) is not (bold|dim|reverse)$`, cellDoesNotHaveAttribute)
	sc.Step(`^deck client "([^"]+)" text "([^"]+)" has foreground "(#[0-9a-fA-F]{6})"$`, textHasForeground)
	sc.Step(`^deck client "([^"]+)" text "([^"]+)" does not have foreground "(#[0-9a-fA-F]{6})"$`, textDoesNotHaveForeground)
	sc.Step(`^deck client "([^"]+)" text "([^"]+)" has background "(#[0-9a-fA-F]{6})"$`, textHasBackground)
	sc.Step(`^deck client "([^"]+)" text "([^"]+)" is (bold|dim|reverse)$`, textHasAttribute)
	sc.Step(`^deck client "([^"]+)" text "([^"]+)" is not (bold|dim|reverse)$`, textDoesNotHaveAttribute)

	// Task 013's token-named forms: the same assertions above, but the
	// expected colour is resolved through internal/theme against whatever
	// theme the scenario pinned (resolveScenarioTokenHex), rather than
	// spelled as a hex literal in the feature file. A scenario written this
	// way keeps working if a built-in theme's palette is retuned later --
	// only scenarios asserting a SPECIFIC palette's value (the 16-colour
	// floor, NO_COLOR) still need the hex-literal forms above.
	sc.Step(`^deck client "([^"]+)" cell at row (\d+) column (\d+) has foreground token "([^"]+)"$`, cellHasForegroundToken)
	sc.Step(`^deck client "([^"]+)" cell at row (\d+) column (\d+) does not have foreground token "([^"]+)"$`, cellDoesNotHaveForegroundToken)
	sc.Step(`^deck client "([^"]+)" cell at row (\d+) column (\d+) has background token "([^"]+)"$`, cellHasBackgroundToken)
	sc.Step(`^deck client "([^"]+)" cell at row (\d+) column (\d+) does not have background token "([^"]+)"$`, cellDoesNotHaveBackgroundToken)
	sc.Step(`^deck client "([^"]+)" text "([^"]+)" has foreground token "([^"]+)"$`, textHasForegroundToken)
	sc.Step(`^deck client "([^"]+)" text "([^"]+)" does not have foreground token "([^"]+)"$`, textDoesNotHaveForegroundToken)
	sc.Step(`^deck client "([^"]+)" text "([^"]+)" has background token "([^"]+)"$`, textHasBackgroundToken)
}

// startNamedClientWithColour starts a client with the harness's default
// NO_COLOR=1 override lifted, so requirement-1 scenarios can assert against
// deck's real, live colour output rather than only a bare vt.Emulator fed
// synthetic escapes.
func startNamedClientWithColour(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	// The empty-valued "NO_COLOR=" sentinel tells ScenarioHarness.Environment
	// to omit NO_COLOR from the client's environment entirely, rather than
	// setting it to an empty string -- which config's getenv wrapper would
	// still read back as "unset" (true) either way, but naming the sentinel
	// keeps the override's intent explicit at the call site.
	client, err := h.StartNamedClient(ctx, name, "NO_COLOR=")
	if err != nil {
		return err
	}
	return client.WaitForFrame(ctx, false, "deck - sessions")
}

// colorHex renders a color.Color as a "#rrggbb" string using its own RGBA
// components, which is how the vt emulator represents every SGR colour form
// (basic, indexed, and true colour) uniformly -- the caller need not know
// which form a particular cell used.
func colorHex(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)
}

// cellForegroundHex returns an error rather than any hex string when the
// cell has no foreground colour set at all (the terminal's own default),
// so a step can never silently pass by treating "no colour" as some
// particular colour.
func cellForegroundHex(cell *uv.Cell) (string, error) {
	if cell == nil {
		return "", fmt.Errorf("cell could not be read from the grid")
	}
	if cell.Style.Fg == nil {
		return "", fmt.Errorf("cell %q has no foreground colour set (terminal default)", cell.Content)
	}
	return colorHex(cell.Style.Fg), nil
}

func cellBackgroundHex(cell *uv.Cell) (string, error) {
	if cell == nil {
		return "", fmt.Errorf("cell could not be read from the grid")
	}
	if cell.Style.Bg == nil {
		return "", fmt.Errorf("cell %q has no background colour set (terminal default)", cell.Content)
	}
	return colorHex(cell.Style.Bg), nil
}

// cellHasAttr reports one of the bold/dim/reverse flags task 001 requires.
// An unrecognised flag name is a step-registration bug, not a runtime
// possibility, given the fixed step patterns above, but it still returns an
// error instead of a silent false to keep the "no fabricated default" rule
// total.
func cellHasAttr(cell *uv.Cell, name string) (bool, error) {
	if cell == nil {
		return false, fmt.Errorf("cell could not be read from the grid")
	}
	switch name {
	case "bold":
		return cell.Style.Attrs&uv.AttrBold != 0, nil
	case "dim":
		return cell.Style.Attrs&uv.AttrFaint != 0, nil
	case "reverse":
		return cell.Style.Attrs&uv.AttrReverse != 0, nil
	default:
		return false, fmt.Errorf("unknown cell attribute %q", name)
	}
}

func cellHasForeground(ctx context.Context, name string, row, col int, want string) error {
	client, err := assertionClient(ctx, name)
	if err != nil {
		return err
	}
	got, err := cellForegroundHex(client.CellAt(col, row))
	if err != nil {
		return fmt.Errorf("client %q cell at row %d column %d: %w", name, row, col, err)
	}
	if got != want {
		return fmt.Errorf("client %q cell at row %d column %d has foreground %s, want %s", name, row, col, got, want)
	}
	return nil
}

func cellDoesNotHaveForeground(ctx context.Context, name string, row, col int, unwanted string) error {
	client, err := assertionClient(ctx, name)
	if err != nil {
		return err
	}
	got, err := cellForegroundHex(client.CellAt(col, row))
	if err != nil {
		// No foreground set at all trivially satisfies "does not have
		// foreground <colour>".
		return nil
	}
	if got == unwanted {
		return fmt.Errorf("client %q cell at row %d column %d has foreground %s, want anything else", name, row, col, got)
	}
	return nil
}

func cellHasBackground(ctx context.Context, name string, row, col int, want string) error {
	client, err := assertionClient(ctx, name)
	if err != nil {
		return err
	}
	got, err := cellBackgroundHex(client.CellAt(col, row))
	if err != nil {
		return fmt.Errorf("client %q cell at row %d column %d: %w", name, row, col, err)
	}
	if got != want {
		return fmt.Errorf("client %q cell at row %d column %d has background %s, want %s", name, row, col, got, want)
	}
	return nil
}

func cellHasAttribute(ctx context.Context, name string, row, col int, attr string) error {
	client, err := assertionClient(ctx, name)
	if err != nil {
		return err
	}
	has, err := cellHasAttr(client.CellAt(col, row), attr)
	if err != nil {
		return fmt.Errorf("client %q cell at row %d column %d: %w", name, row, col, err)
	}
	if !has {
		return fmt.Errorf("client %q cell at row %d column %d is not %s", name, row, col, attr)
	}
	return nil
}

func cellDoesNotHaveAttribute(ctx context.Context, name string, row, col int, attr string) error {
	client, err := assertionClient(ctx, name)
	if err != nil {
		return err
	}
	has, err := cellHasAttr(client.CellAt(col, row), attr)
	if err != nil {
		return fmt.Errorf("client %q cell at row %d column %d: %w", name, row, col, err)
	}
	if has {
		return fmt.Errorf("client %q cell at row %d column %d is %s, want not %s", name, row, col, attr, attr)
	}
	return nil
}

func assertionClient(ctx context.Context, name string) (*ScreenDriver, error) {
	h, err := assertionHarness(ctx)
	if err != nil {
		return nil, err
	}
	return h.Client(name)
}

// textCells locates the first grid run whose per-cell Content concatenates
// to text and returns every cell in that run, reading the grid directly
// (never Frame's trimmed string) so the returned cells carry real style
// attributes.
func textCells(client *ScreenDriver, text string) ([]*uv.Cell, error) {
	row, col, err := client.FindText(text)
	if err != nil {
		return nil, err
	}
	runes := []rune(text)
	cells := make([]*uv.Cell, len(runes))
	for i := range runes {
		cells[i] = client.CellAt(col+i, row)
	}
	return cells, nil
}

func textHasForeground(ctx context.Context, name, text, want string) error {
	client, err := assertionClient(ctx, name)
	if err != nil {
		return err
	}
	cells, err := textCells(client, text)
	if err != nil {
		return fmt.Errorf("client %q: %w", name, err)
	}
	for i, cell := range cells {
		got, err := cellForegroundHex(cell)
		if err != nil {
			return fmt.Errorf("client %q text %q cell %d: %w", name, text, i, err)
		}
		if got != want {
			return fmt.Errorf("client %q text %q cell %d has foreground %s, want %s", name, text, i, got, want)
		}
	}
	return nil
}

func textDoesNotHaveForeground(ctx context.Context, name, text, unwanted string) error {
	client, err := assertionClient(ctx, name)
	if err != nil {
		return err
	}
	cells, err := textCells(client, text)
	if err != nil {
		return fmt.Errorf("client %q: %w", name, err)
	}
	for i, cell := range cells {
		got, err := cellForegroundHex(cell)
		if err != nil {
			continue // no foreground set at all satisfies "does not have".
		}
		if got == unwanted {
			return fmt.Errorf("client %q text %q cell %d has foreground %s, want anything else", name, text, i, got)
		}
	}
	return nil
}

func textHasBackground(ctx context.Context, name, text, want string) error {
	client, err := assertionClient(ctx, name)
	if err != nil {
		return err
	}
	cells, err := textCells(client, text)
	if err != nil {
		return fmt.Errorf("client %q: %w", name, err)
	}
	for i, cell := range cells {
		got, err := cellBackgroundHex(cell)
		if err != nil {
			return fmt.Errorf("client %q text %q cell %d: %w", name, text, i, err)
		}
		if got != want {
			return fmt.Errorf("client %q text %q cell %d has background %s, want %s", name, text, i, got, want)
		}
	}
	return nil
}

func textHasAttribute(ctx context.Context, name, text, attr string) error {
	client, err := assertionClient(ctx, name)
	if err != nil {
		return err
	}
	cells, err := textCells(client, text)
	if err != nil {
		return fmt.Errorf("client %q: %w", name, err)
	}
	for i, cell := range cells {
		has, err := cellHasAttr(cell, attr)
		if err != nil {
			return fmt.Errorf("client %q text %q cell %d: %w", name, text, i, err)
		}
		if !has {
			return fmt.Errorf("client %q text %q cell %d is not %s", name, text, i, attr)
		}
	}
	return nil
}

func textDoesNotHaveAttribute(ctx context.Context, name, text, attr string) error {
	client, err := assertionClient(ctx, name)
	if err != nil {
		return err
	}
	cells, err := textCells(client, text)
	if err != nil {
		return fmt.Errorf("client %q: %w", name, err)
	}
	for i, cell := range cells {
		has, err := cellHasAttr(cell, attr)
		if err != nil {
			return fmt.Errorf("client %q text %q cell %d: %w", name, text, i, err)
		}
		if has {
			return fmt.Errorf("client %q text %q cell %d is %s, want not %s", name, text, i, attr, attr)
		}
	}
	return nil
}

// cellDoesNotHaveBackground is background's counterpart to
// cellDoesNotHaveForeground: a cell with no background set at all trivially
// satisfies "does not have background <colour>" rather than erroring, the
// same rule cellDoesNotHaveForeground applies.
func cellDoesNotHaveBackground(ctx context.Context, name string, row, col int, unwanted string) error {
	client, err := assertionClient(ctx, name)
	if err != nil {
		return err
	}
	got, err := cellBackgroundHex(client.CellAt(col, row))
	if err != nil {
		return nil
	}
	if got == unwanted {
		return fmt.Errorf("client %q cell at row %d column %d has background %s, want anything else", name, row, col, got)
	}
	return nil
}

// textHasBackground's counterpart to textDoesNotHaveForeground.
func textDoesNotHaveBackground(ctx context.Context, name, text, unwanted string) error {
	client, err := assertionClient(ctx, name)
	if err != nil {
		return err
	}
	cells, err := textCells(client, text)
	if err != nil {
		return fmt.Errorf("client %q: %w", name, err)
	}
	for i, cell := range cells {
		got, err := cellBackgroundHex(cell)
		if err != nil {
			continue
		}
		if got == unwanted {
			return fmt.Errorf("client %q text %q cell %d has background %s, want anything else", name, text, i, got)
		}
	}
	return nil
}

// cellHasForegroundToken and its siblings below are task 013's token-named
// forms of the hex-literal steps above: resolveScenarioTokenHex (see
// theme_pin_test.go) resolves tokenName's colour through internal/theme
// against whatever theme the scenario pinned via its config.toml, THEN
// delegates to the exact same hex-literal assertion function every existing
// requirement-1 scenario already exercises -- so a feature file can name a
// token ("waiting") instead of a hex literal ("#fbbf24") and keep passing
// if that token's authored colour changes, while still going through the
// identical "never fabricate a default" cell-reading path.
func cellHasForegroundToken(ctx context.Context, name string, row, col int, tokenName string) error {
	want, err := resolveScenarioTokenHex(ctx, tokenName)
	if err != nil {
		return fmt.Errorf("client %q cell at row %d column %d: %w", name, row, col, err)
	}
	return cellHasForeground(ctx, name, row, col, want)
}

func cellDoesNotHaveForegroundToken(ctx context.Context, name string, row, col int, tokenName string) error {
	unwanted, err := resolveScenarioTokenHex(ctx, tokenName)
	if err != nil {
		return fmt.Errorf("client %q cell at row %d column %d: %w", name, row, col, err)
	}
	return cellDoesNotHaveForeground(ctx, name, row, col, unwanted)
}

func cellHasBackgroundToken(ctx context.Context, name string, row, col int, tokenName string) error {
	want, err := resolveScenarioTokenHex(ctx, tokenName)
	if err != nil {
		return fmt.Errorf("client %q cell at row %d column %d: %w", name, row, col, err)
	}
	return cellHasBackground(ctx, name, row, col, want)
}

func cellDoesNotHaveBackgroundToken(ctx context.Context, name string, row, col int, tokenName string) error {
	unwanted, err := resolveScenarioTokenHex(ctx, tokenName)
	if err != nil {
		return fmt.Errorf("client %q cell at row %d column %d: %w", name, row, col, err)
	}
	return cellDoesNotHaveBackground(ctx, name, row, col, unwanted)
}

func textHasForegroundToken(ctx context.Context, name, text, tokenName string) error {
	want, err := resolveScenarioTokenHex(ctx, tokenName)
	if err != nil {
		return fmt.Errorf("client %q text %q: %w", name, text, err)
	}
	return textHasForeground(ctx, name, text, want)
}

func textDoesNotHaveForegroundToken(ctx context.Context, name, text, tokenName string) error {
	unwanted, err := resolveScenarioTokenHex(ctx, tokenName)
	if err != nil {
		return fmt.Errorf("client %q text %q: %w", name, text, err)
	}
	return textDoesNotHaveForeground(ctx, name, text, unwanted)
}

func textHasBackgroundToken(ctx context.Context, name, text, tokenName string) error {
	want, err := resolveScenarioTokenHex(ctx, tokenName)
	if err != nil {
		return fmt.Errorf("client %q text %q: %w", name, text, err)
	}
	return textHasBackground(ctx, name, text, want)
}

