package features

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cucumber/godog"
)

// registerCreateCWDCaretSteps backs requirement 179 (§11.7 under a caret): the
// cwd field's offered value is edited in place, and its ghost and tab
// completion exist only with the caret at the end of the text.
func registerCreateCWDCaretSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a directory named "([^"]+)" exists under the scenario home labelled "([^"]+)"$`, directoryExistsUnderScenarioHomeLabelled)
	sc.Step(`^deck client "([^"]+)" presses "(left|backspace)" in the cwd field$`, clientPressesCaretOrEditKeyInCWDField)
	sc.Step(`^deck client "([^"]+)" cwd field comes to show no ghost text$`, clientCWDFieldComesToShowNoGhostText)
}

// directoryExistsUnderScenarioHomeLabelled creates the directory under the
// scenario's DECK_HOME and registers it under label, so a later step can compare
// a session's stored cwd with it.
func directoryExistsUnderScenarioHomeLabelled(ctx context.Context, name, label string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	dir := filepath.Join(h.Home, name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create directory %q under scenario home: %w", name, err)
	}
	registerNamedDirectory(h, label, dir)
	return nil
}

// clientPressesCaretOrEditKeyInCWDField sends one real left (\x1b[D) or
// backspace (\x7f) to the focused cwd field. There is nothing on screen to
// wait on for a caret move alone, so the scenario's next step reads the frame
// that follows the keys, which the pty delivers in order.
func clientPressesCaretOrEditKeyInCWDField(ctx context.Context, name, key string) error {
	client, err := assertionClient(ctx, name)
	if err != nil {
		return err
	}
	seq := map[string]string{"left": "\x1b[D", "backspace": "\x7f"}[key]
	return client.Send(seq)
}

// clientCWDFieldComesToShowNoGhostText waits until the cwd field's rows hold no
// `dimmed`-token cell. The scenario has shown a ghost there first, so the wait
// ends on the frame the caret move repainted, rather than passing on the
// frame that still carries the ghost.
func clientCWDFieldComesToShowNoGhostText(ctx context.Context, name string) error {
	client, err := assertionClient(ctx, name)
	if err != nil {
		return err
	}
	dimmed, err := resolveScenarioTokenHex(ctx, "dimmed")
	if err != nil {
		return err
	}
	var last string
	_, err = client.WaitForFrameFunc(ctx, false, func(string) bool {
		found, row, col, content, _, _, ferr := cwdFieldDimmedCell(client, dimmed)
		if ferr != nil {
			return false
		}
		if found {
			last = fmt.Sprintf("a dimmed-token cell %q at row %d column %d", content, row, col)
			return false
		}
		return true
	})
	if err != nil {
		return fmt.Errorf("client %q: want no ghost text in the cwd field once the caret has left its end; last read: %s: %w", name, last, err)
	}
	return nil
}
