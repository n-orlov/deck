package features

import (
	"context"
	"time"

	"github.com/cucumber/godog"
)

// registerAutoEnterSteps backs GH #52's scenario in
// features/interactive_focus.feature (SPEC §11's newly-created bullet,
// §11.9): the shared harness turns [ui] attach_on_new off for every
// client (ScenarioHarness.Environment), so the one scenario about it needs
// a way to turn it back on, plus a way to type a whole line into the pane
// it lands in without leaving a raw carriage return in the feature file.
func registerAutoEnterSteps(sc *godog.ScenarioContext) {
	sc.Step(`^every deck client in this scenario enters the interactive preview on create$`, attachOnNewIsEnabled)
	sc.Step(`^deck client "([^"]+)" types the line "([^"]+)" into the interactive preview$`, clientTypesLineIntoInteractivePreview)
}

// attachOnNewIsEnabled sets DECK_ATTACH_ON_NEW=1 for every client
// subsequently started in this scenario. clientEnv follows the harness's
// own base environment, so this later entry is the one exec keeps.
func attachOnNewIsEnabled(ctx context.Context) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	h.clientEnv = append(h.clientEnv, "DECK_ATTACH_ON_NEW=1")
	return nil
}

// clientTypesLineIntoInteractivePreview sends text and then Enter as two
// writes, paced like clientCreatesShellSession's own name-then-submit, so
// the pane's shell receives the whole line before the carriage return
// that runs it.
func clientTypesLineIntoInteractivePreview(ctx context.Context, name, text string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	if err := client.Send(text); err != nil {
		return err
	}
	time.Sleep(75 * time.Millisecond)
	return client.Send("\r")
}
