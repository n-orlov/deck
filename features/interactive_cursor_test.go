package features

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cucumber/godog"
)

// registerInteractiveCursorSteps backs features/interactive_cursor.feature
// (R182, GH #60): the interactive preview draws the pane's own cursor as a
// reverse-video cell, hides it with the pane's DECTCEM state, and draws none
// while scrolled back. Every assertion reads real cells off the deck
// client's rendered grid (ScreenDriver.CellAt) and polls until the cell is
// in the wanted state: the cursor is painted by a coalesced repaint after
// the pane's bytes reach the grid, so the wait is on that durable fact,
// never on a sleep.
func registerInteractiveCursorSteps(sc *godog.ScenarioContext) {
	sc.Step(`^deck client "([^"]+)" types "([^"]*)" without Enter into the interactive pane$`, clientTypesWithoutEnterIntoInteractivePane)
	sc.Step(`^deck client "([^"]+)" presses left (\d+) times in the interactive pane$`, clientPressesLeftInInteractivePane)
	sc.Step(`^deck client "([^"]+)" clears the interactive pane's input line$`, clientClearsInteractiveInputLine)
	sc.Step(`^deck client "([^"]+)" shows the pane cursor on the cell holding "([^"]+)" in "([^"]+)"$`, clientShowsCursorOnCellHolding)
	sc.Step(`^deck client "([^"]+)" shows no pane cursor on the cell holding "([^"]+)" in "([^"]+)"$`, clientShowsNoCursorOnCellHolding)
	sc.Step(`^deck client "([^"]+)" shows the pane cursor right after "([^"]+)"$`, clientShowsCursorRightAfter)
	sc.Step(`^deck client "([^"]+)" shows no pane cursor right after "([^"]+)"$`, clientShowsNoCursorRightAfter)
	sc.Step(`^deck client "([^"]+)" shows no reverse-video cell in the preview panel$`, clientShowsNoReverseCellInPreview)
	sc.Step(`^deck client "([^"]+)" is started with colour enabled and preview paint "(fit|nofit|bg|off)"$`, startNamedClientWithColourAndPaint)
}

func interactiveCursorClient(ctx context.Context, name string) (*ScreenDriver, error) {
	h, err := assertionHarness(ctx)
	if err != nil {
		return nil, err
	}
	return h.Client(name)
}

func clientTypesWithoutEnterIntoInteractivePane(ctx context.Context, name, text string) error {
	client, err := interactiveCursorClient(ctx, name)
	if err != nil {
		return err
	}
	if err := client.Send(text); err != nil {
		return err
	}
	// The typed text reaches the screen only through the pane and back, so
	// seeing it is the proof the forwarded keys landed.
	if visible := strings.TrimSpace(text); visible != "" && !strings.ContainsAny(visible, "'\\") {
		return client.WaitForFrame(ctx, false, visible)
	}
	return nil
}

func clientPressesLeftInInteractivePane(ctx context.Context, name string, n int) error {
	return clientSendsRawSequenceNTimes(ctx, name, "\x1b[D", n)
}

// clientClearsInteractiveInputLine sends Ctrl+U, which readline binds to
// "kill the whole line before the cursor".
func clientClearsInteractiveInputLine(ctx context.Context, name string) error {
	client, err := interactiveCursorClient(ctx, name)
	if err != nil {
		return err
	}
	return client.Send("\x15")
}

// cellReverseAt reports whether the cell at (col,row) carries SGR 7.
func cellReverseAt(client *ScreenDriver, col, row int) (bool, error) {
	cell := client.CellAt(col, row)
	if cell == nil {
		return false, fmt.Errorf("no cell at column %d row %d", col, row)
	}
	return cellHasAttr(cell, "reverse")
}

// waitCellReverse polls until locate's cell has the wanted reverse state.
func waitCellReverse(ctx context.Context, name, what string, want bool, locate func(*ScreenDriver) (col, row int, err error)) error {
	client, err := interactiveCursorClient(ctx, name)
	if err != nil {
		return err
	}
	last := "never located"
	_, werr := client.WaitForFrameFunc(ctx, false, func(string) bool {
		col, row, lerr := locate(client)
		if lerr != nil {
			last = lerr.Error()
			return false
		}
		got, rerr := cellReverseAt(client, col, row)
		if rerr != nil {
			last = rerr.Error()
			return false
		}
		last = fmt.Sprintf("cell (col %d, row %d) reverse=%v", col, row, got)
		return got == want
	})
	if werr != nil {
		return fmt.Errorf("client %q: want %s reverse=%v; last read: %s: %w\n%s", name, what, want, last, werr, client.Frame(false))
	}
	return nil
}

// settleReverse is the "stays not reverse" half of a negative check: the
// cell must already be in the wanted state, and a short quiet window later
// still is, so a cursor drawn one repaint late cannot slip past.
func requireNotReverseSteadily(ctx context.Context, name, what string, locate func(*ScreenDriver) (int, int, error)) error {
	client, err := interactiveCursorClient(ctx, name)
	if err != nil {
		return err
	}
	if _, err := client.WaitForQuiescence(ctx, false, 300*time.Millisecond); err != nil {
		return fmt.Errorf("client %q never went quiet: %w", name, err)
	}
	col, row, err := locate(client)
	if err != nil {
		return fmt.Errorf("client %q: %s: %w\n%s", name, what, err, client.Frame(false))
	}
	got, err := cellReverseAt(client, col, row)
	if err != nil {
		return err
	}
	if got {
		return fmt.Errorf("client %q: %s (col %d, row %d) is reverse video but must not be:\n%s", name, what, col, row, client.Frame(false))
	}
	return nil
}

func cellHoldingLocator(letter, within string) func(*ScreenDriver) (int, int, error) {
	return func(c *ScreenDriver) (int, int, error) {
		row, col, err := c.FindText(within)
		if err != nil {
			return 0, 0, err
		}
		idx := strings.Index(within, letter)
		if idx < 0 {
			return 0, 0, fmt.Errorf("%q does not contain %q", within, letter)
		}
		return col + utf8.RuneCountInString(within[:idx]), row, nil
	}
}

func afterTextLocator(text string) func(*ScreenDriver) (int, int, error) {
	return func(c *ScreenDriver) (int, int, error) {
		row, col, err := c.FindText(text)
		if err != nil {
			return 0, 0, err
		}
		return col + utf8.RuneCountInString(text), row, nil
	}
}

func clientShowsCursorOnCellHolding(ctx context.Context, name, letter, within string) error {
	return waitCellReverse(ctx, name, fmt.Sprintf("the cell holding %q in %q", letter, within), true, cellHoldingLocator(letter, within))
}

func clientShowsNoCursorOnCellHolding(ctx context.Context, name, letter, within string) error {
	return requireNotReverseSteadily(ctx, name, fmt.Sprintf("the cell holding %q in %q", letter, within), cellHoldingLocator(letter, within))
}

func clientShowsCursorRightAfter(ctx context.Context, name, text string) error {
	return waitCellReverse(ctx, name, fmt.Sprintf("the cell right after %q", text), true, afterTextLocator(text))
}

func clientShowsNoCursorRightAfter(ctx context.Context, name, text string) error {
	return requireNotReverseSteadily(ctx, name, fmt.Sprintf("the cell right after %q", text), afterTextLocator(text))
}

// clientShowsNoReverseCellInPreview scans every cell right of the sidebar
// seam, below the top border and above the footer, once the client is quiet.
func clientShowsNoReverseCellInPreview(ctx context.Context, name string) error {
	client, err := interactiveCursorClient(ctx, name)
	if err != nil {
		return err
	}
	if _, err := client.WaitForQuiescence(ctx, false, 300*time.Millisecond); err != nil {
		return fmt.Errorf("client %q never went quiet: %w", name, err)
	}
	frame := client.Frame(false)
	seam, err := seamColumn(frame)
	if err != nil {
		return err
	}
	cols, rows := client.GridSize()
	for row := 1; row < rows-1; row++ {
		for col := seam + 1; col < cols-1; col++ {
			got, rerr := cellReverseAt(client, col, row)
			if rerr != nil {
				continue
			}
			if got {
				return fmt.Errorf("client %q: preview cell (col %d, row %d) is reverse video while no cursor may be drawn:\n%s", name, col, row, frame)
			}
		}
	}
	return nil
}

func startNamedClientWithColourAndPaint(ctx context.Context, name, mode string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.StartNamedClient(ctx, name, "NO_COLOR=", "DECK_PREVIEW_PAINT="+mode)
	if err != nil {
		return err
	}
	return client.WaitForFrame(ctx, false, "deck - sessions")
}
