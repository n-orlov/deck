package features

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cucumber/godog"

	"github.com/n-orlov/deck/internal/tmux"
)

// registerInteractiveSelectionSteps backs
// features/interactive_selection.feature (steer 017 item 3/task 216, SPEC
// §11.8/§11.9's "Selection and copy"): a mouse drag beginning inside the
// interactive preview's own content box selects text over the grid deck
// itself drew, and releasing writes it to deck's OWN named tmux buffer
// (the load-bearing, tested half of the copy) on deck's own private
// socket -- proven here with a real `tmux show-buffer` against that
// socket, exactly as the task's own successCriteria requires, never by
// re-deriving internal/interactive.Session.SelectedText's arithmetic a
// second time.
func registerInteractiveSelectionSteps(sc *godog.ScenarioContext) {
	sc.Step(`^deck client "([^"]+)" drags to select "([^"]+)" over the line containing it in the interactive pane$`, clientDragsToSelectTextOverInteractivePane)
	sc.Step(`^deck's own selection buffer eventually contains "([^"]+)"$`, deckSelectionBufferEventuallyContains)
	sc.Step(`^deck's own selection buffer is empty$`, deckSelectionBufferIsEmpty)
	sc.Step(`^deck client "([^"]+)" clicks once \(no drag\) on the line containing "([^"]+)" in the interactive pane$`, clientClicksOnceOnInteractivePaneLineContaining)
}

// clientDragsToSelectTextOverInteractivePane locates text in the client's
// OWN current frame (locateText, features/mouse_bindings_test.go -- the
// SAME preview panel content deck itself just rendered while
// m.interactive is true), then synthesizes an SGR drag (Drag,
// features/mouse_synthesis_test.go) from the text's first cell to its
// last, a press-motion-release gesture with the left button held
// throughout -- a plain click, with no motion event at all, is a
// deliberately different gesture (clientClicksOnceOnInteractivePaneLineContaining
// below) precisely because SPEC §11.8 draws that distinction.
func clientDragsToSelectTextOverInteractivePane(ctx context.Context, name, text string) error {
	client, err := mouseSynthesisClient(ctx, name)
	if err != nil {
		return err
	}
	col, row, err := locateText(client, text)
	if err != nil {
		return err
	}
	if err := client.Drag(col, row, col+len(text)-1, row); err != nil {
		return err
	}
	time.Sleep(150 * time.Millisecond)
	return nil
}

// clientClicksOnceOnInteractivePaneLineContaining is Click (press
// immediately followed by release at the SAME cell, no motion event in
// between) at the exact cell text starts at -- proof that a plain click,
// the stated non-exception, commits nothing.
func clientClicksOnceOnInteractivePaneLineContaining(ctx context.Context, name, text string) error {
	client, err := mouseSynthesisClient(ctx, name)
	if err != nil {
		return err
	}
	col, row, err := locateText(client, text)
	if err != nil {
		return err
	}
	if err := client.Click(col, row); err != nil {
		return err
	}
	time.Sleep(150 * time.Millisecond)
	return nil
}

// deckSelectionBufferEventuallyContains polls deck's own named tmux
// buffer (tmux.SelectionBufferName) on the scenario's own private socket
// -- `tmux -L <socket> show-buffer -b deck-selection`, entirely
// independent of any deck client/ScreenDriver -- since the release SGR
// bytes reach deck's own input loop asynchronously with respect to this
// step's own tmux query.
func deckSelectionBufferEventuallyContains(ctx context.Context, want string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(3 * time.Second)
	var lastOutput string
	var lastErr error
	for time.Now().Before(deadline) {
		output, err := tmuxOutput(ctx, h, "show-buffer", "-b", tmux.SelectionBufferName)
		lastOutput, lastErr = strings.TrimRight(string(output), "\n"), err
		if err == nil && strings.Contains(lastOutput, want) {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("deck's own selection buffer (%q on socket %q) never contained %q; last read %q (err=%v)", tmux.SelectionBufferName, h.Socket, want, lastOutput, lastErr)
}

// deckSelectionBufferIsEmpty proves the negative half directly: the
// named buffer does not exist at all on deck's own socket, i.e. nothing
// has ever written to it -- the state a plain click (no drag) must leave
// behind.
func deckSelectionBufferIsEmpty(ctx context.Context) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	output, err := tmuxOutput(ctx, h, "show-buffer", "-b", tmux.SelectionBufferName)
	if err == nil {
		return fmt.Errorf("deck's own selection buffer already contains %q; want it to not exist at all", strings.TrimSpace(string(output)))
	}
	return nil
}
