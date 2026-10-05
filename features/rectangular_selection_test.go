package features

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cucumber/godog"
	"github.com/n-orlov/deck/internal/tmux"
)

// registerRectangularSelectionSteps backs features/rectangular_selection.feature
// (R203, GH #66): a drag over the interactive preview with Alt or Ctrl held,
// anchored to a text the scenario painted, and a read of deck's own tmux
// buffer as one row per line.
func registerRectangularSelectionSteps(sc *godog.ScenarioContext) {
	sc.Step(`^deck client "([^"]+)" (Alt|Ctrl)\+drags from column offset (\d+) row offset (\d+) to column offset (\d+) row offset (\d+) of the text "([^"]+)" in the interactive pane$`, clientModifierDragsFromTextOffsets)
	sc.Step(`^deck client "([^"]+)" drags from column offset (\d+) row offset (\d+) to column offset (\d+) row offset (\d+) of the text "([^"]+)" in the interactive pane$`, clientPlainDragsFromTextOffsets)
	sc.Step(`^deck's own selection buffer holds the rows "([^"]*)"$`, deckSelectionBufferHoldsRows)
}

func clientModifierDragsFromTextOffsets(ctx context.Context, name, key string, c1, r1, c2, r2 int, text string) error {
	mods := sgrModAlt
	if key == "Ctrl" {
		mods = sgrModCtrl
	}
	return dragFromTextOffsets(ctx, name, mods, c1, r1, c2, r2, text)
}

func clientPlainDragsFromTextOffsets(ctx context.Context, name string, c1, r1, c2, r2 int, text string) error {
	return dragFromTextOffsets(ctx, name, 0, c1, r1, c2, r2, text)
}

// dragFromTextOffsets locates text's first cell in the client's own frame and
// drags between cells offset from it.
func dragFromTextOffsets(ctx context.Context, name string, mods, c1, r1, c2, r2 int, text string) error {
	client, err := mouseSynthesisClient(ctx, name)
	if err != nil {
		return err
	}
	col, row, err := locatePreviewText(client, text)
	if err != nil {
		return err
	}
	if err := client.DragWithModifiers(mods, col+c1, row+r1, col+c2, row+r2); err != nil {
		return err
	}
	time.Sleep(150 * time.Millisecond)
	return nil
}

// deckSelectionBufferHoldsRows waits for the buffer to hold the rows, given
// as one string with " | " between rows, joined with newlines.
func deckSelectionBufferHoldsRows(ctx context.Context, rows string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	want := strings.Join(strings.Split(rows, " | "), "\n")
	deadline := time.Now().Add(3 * time.Second)
	var got string
	var lastErr error
	for time.Now().Before(deadline) {
		var output []byte
		output, lastErr = tmuxOutput(ctx, h, "show-buffer", "-b", tmux.SelectionBufferName)
		got = strings.TrimRight(string(output), "\n")
		if lastErr == nil && got == want {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("deck's own selection buffer = %q (err=%w), want %q", got, lastErr, want)
}
