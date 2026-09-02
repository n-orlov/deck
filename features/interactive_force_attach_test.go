package features

import (
	"context"
	"time"

	"github.com/cucumber/godog"
)

// registerInteractiveForceAttachSteps backs
// features/interactive_force_attach.feature's geometry-survives-the-chain
// scenario (task 115): the one step that scenario needs and no existing
// file provides -- dismissing task 116's lost-attach dialog once it has
// already raised on the displaced client. The dialog "dismisses on Enter"
// literally (lost_attach.go's updateLostAttachView), never on esc, so this
// sends the same bare "\r" clientEntersInteractiveMode does; unlike that
// step this one must run only once the dialog is actually up (the
// scenario's own "screen contains "Lost attach: ..."" step establishes
// that before this one is reached), otherwise a bare Enter would instead
// re-enter interactive mode on whatever row happens to be selected.
func registerInteractiveForceAttachSteps(sc *godog.ScenarioContext) {
	sc.Step(`^deck client "([^"]+)" dismisses the lost-attach dialog$`, clientDismissesLostAttachDialog)
}

func clientDismissesLostAttachDialog(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	if err := client.Send("\r"); err != nil {
		return err
	}
	time.Sleep(150 * time.Millisecond)
	return nil
}
