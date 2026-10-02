package features

import (
	"context"
	"fmt"
	"strings"

	"github.com/cucumber/godog"
	"github.com/n-orlov/deck/internal/tmux"
)

// This file is task 018's (R180) real-tmux contract for alt+w: the focused
// field's whole text lands in deck's own selection buffer on the scenario's
// private socket.

func registerFieldCopySteps(sc *godog.ScenarioContext) {
	sc.Step(`^deck client "([^"]+)" presses alt\+w in the create name field$`, clientPressesAltWInCreateNameField)
	sc.Step(`^deck's own selection buffer holds exactly "([^"]*)"$`, deckSelectionBufferHoldsExactly)
}

// clientPressesAltWInCreateNameField sends the real terminal bytes for alt+w
// (ESC then w, in one write so they reach the input loop as one key).
func clientPressesAltWInCreateNameField(ctx context.Context, clientName string) error {
	client, err := assertionClient(ctx, clientName)
	if err != nil {
		return err
	}
	return client.Send("\x1bw")
}

// deckSelectionBufferHoldsExactly reads `tmux -L <socket> show-buffer -b
// deck-selection` once. The scenario waits on the on-screen confirmation
// first, which deck draws only after the buffer write returned, so one read is
// the whole postcondition: no polling.
func deckSelectionBufferHoldsExactly(ctx context.Context, want string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	output, err := tmuxOutput(ctx, h, "show-buffer", "-b", tmux.SelectionBufferName)
	if err != nil {
		return fmt.Errorf("show-buffer -b %s on socket %q: %w", tmux.SelectionBufferName, h.Socket, err)
	}
	if got := strings.TrimRight(string(output), "\n"); got != want {
		return fmt.Errorf("deck's own selection buffer = %q, want exactly %q", got, want)
	}
	return nil
}
