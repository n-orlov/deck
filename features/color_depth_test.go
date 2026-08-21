package features

import (
	"context"
	"fmt"

	"github.com/cucumber/godog"

	"github.com/n-orlov/deck/internal/theme"
)

// registerColorDepthSteps exposes requirements 2, 3, 29 and 31's colour-depth
// and NO_COLOR pty coverage: forcing DECK_COLOR_DEPTH from a released
// client, and asserting a monochrome NO_COLOR frame carries no colour at
// all -- proving §11.6's "the glyphs are load-bearing, never decorative"
// claim from a real pty rather than only from unit tests over
// internal/theme/internal/tui in isolation.
func registerColorDepthSteps(sc *godog.ScenarioContext) {
	sc.Step(`^deck client "([^"]+)" is started with colour enabled and colour depth "([^"]+)"$`, startNamedClientWithColourDepth)
	sc.Step(`^deck client "([^"]+)" frame has no colour anywhere$`, clientFrameHasNoColourAnywhere)
	sc.Step(`^deck client "([^"]+)" text "([^"]+)" has the quantised ANSI colour for token "([^"]+)"$`, textHasQuantisedANSIColourForToken)
}

// startNamedClientWithColourDepth mirrors startNamedClientWithColour (task
// 001's colour-enabled client) but also forces DECK_COLOR_DEPTH, so a
// scenario can pin either render path (requirement 2's truecolour path,
// requirement 29's 16-colour floor) deterministically regardless of what
// the harness's own pty advertises -- exactly what §13.1's
// DECK_COLOR_DEPTH override exists for.
func startNamedClientWithColourDepth(ctx context.Context, name, depth string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.StartNamedClient(ctx, name, "NO_COLOR=", "DECK_COLOR_DEPTH="+depth)
	if err != nil {
		return err
	}
	return client.WaitForFrame(ctx, false, "deck - sessions")
}

// clientFrameHasNoColourAnywhere walks every cell of the client's entire
// grid -- not just a named row or a matched substring -- so requirement
// 3/31's NO_COLOR proof is total: nothing anywhere in the frame carries a
// foreground or background colour. Whatever distinguishes a session's
// status under NO_COLOR must therefore be doing so through its glyphs and
// words alone, which is the property this step exists to make assertable
// rather than merely claimed in prose.
func clientFrameHasNoColourAnywhere(ctx context.Context, name string) error {
	client, err := assertionClient(ctx, name)
	if err != nil {
		return err
	}
	cols, rows := client.GridSize()
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			cell := client.CellAt(x, y)
			if cell == nil {
				continue
			}
			if hex, err := cellForegroundHex(cell); err == nil {
				return fmt.Errorf("client %q cell at row %d column %d has foreground %s under NO_COLOR, want none", name, y, x, hex)
			}
			if hex, err := cellBackgroundHex(cell); err == nil {
				return fmt.Errorf("client %q cell at row %d column %d has background %s under NO_COLOR, want none", name, y, x, hex)
			}
		}
	}
	return nil
}

// ansiCodeToRenderedHex mirrors ultraviolet/vt's OWN basic-16 SGR rendering
// table (github.com/charmbracelet/x/ansi's ansiHex[0..15], reached via
// ansi.BasicColor.RGBA -- see ultraviolet's ReadStyle handling SGR codes
// 30-37/90-97) -- the ACTUAL colour this harness's pty terminal paints back
// for a basic ANSI colour code. This is deliberately NOT deck's own
// theme.ReferencePalette hex values: SPEC §11.6 states plainly that
// terminals do not agree on what their 16 ANSI slots render, which is
// exactly why deck fixes its OWN reference palette for the *quantisation*
// maths rather than for what a pty is asserted to paint back for a given
// code. A requirement-29 per-cell assertion must check what the terminal
// actually rendered for the emitted SGR code, not deck's declared
// reference hex, which a real terminal is never claimed to reproduce
// exactly (see features/emulator_placement_test.go's
// TestCellAttributeAssertionsCanFail, which independently pins SGR 31 to
// this same #800000, not deck's #cd0000).
var ansiCodeToRenderedHex = map[int]string{
	30: "#000000", 31: "#800000", 32: "#008000", 33: "#808000",
	34: "#000080", 35: "#800080", 36: "#008080", 37: "#c0c0c0",
	90: "#808080", 91: "#ff0000", 92: "#00ff00", 93: "#ffff00",
	94: "#0000ff", 95: "#ff00ff", 96: "#00ffff", 97: "#ffffff",
}

// textHasQuantisedANSIColourForToken resolves the scenario's currently
// pinned theme (the default, absent a config.toml selecting one -- see
// resolveScenarioTheme), quantises tokenName's colour exactly as
// internal/tui/theme_color.go's sgrForToken does for DECK_COLOR_DEPTH=16,
// and asserts the matched text's cells render the pty's OWN colour for
// that SGR code (ansiCodeToRenderedHex) -- proving requirement 29's
// 16-colour floor renders from a real released binary through a pty,
// rather than only from the loader's own unit tests.
func textHasQuantisedANSIColourForToken(ctx context.Context, name, text, tokenName string) error {
	client, err := assertionClient(ctx, name)
	if err != nil {
		return err
	}
	resolved, err := resolveScenarioTheme(ctx)
	if err != nil {
		return err
	}
	quantized, err := resolved.QuantizedColor(theme.Token(tokenName))
	if err != nil {
		return fmt.Errorf("quantise token %q against pinned theme %q: %w", tokenName, resolved.Name, err)
	}
	code, ok := theme.ANSI16Code(quantized)
	if !ok {
		return fmt.Errorf("token %q quantised to %q, which is not one of theme.ReferencePalette's 16 entries", tokenName, quantized)
	}
	want, ok := ansiCodeToRenderedHex[code]
	if !ok {
		return fmt.Errorf("no known rendered colour for ANSI SGR code %d", code)
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
			return fmt.Errorf("client %q text %q cell %d has foreground %s, want %s (ANSI code %d for quantised token %q)", name, text, i, got, want, code, tokenName)
		}
	}
	return nil
}
