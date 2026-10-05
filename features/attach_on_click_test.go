package features

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

// registerAttachOnClickSteps backs GH #62's scenarios in
// features/attach_on_click.feature: the shared harness leaves
// [ui] attach_on_click at its default (on), so the scenarios about the
// setting turn it off first, and then assert that a key typed after a
// click or double-click never reached the clicked session's pane.
func registerAttachOnClickSteps(sc *godog.ScenarioContext) {
	sc.Step(`^every deck client in this scenario only selects on a sidebar click$`, attachOnClickIsDisabled)
	sc.Step(`^the private tmux pane for session "([^"]+)" did not receive "([^"]+)"$`, privateTMuxPaneForSessionDidNotReceive)
}

// attachOnClickIsDisabled sets DECK_ATTACH_ON_CLICK=0 for every client
// subsequently started in this scenario (the later duplicate key wins, as
// attachOnNewIsEnabled's own does).
func attachOnClickIsDisabled(ctx context.Context) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	h.clientEnv = append(h.clientEnv, "DECK_ATTACH_ON_CLICK=0")
	return nil
}

// privateTMuxPaneForSessionDidNotReceive reads the session's own tmux pane
// after a settle period long enough for a forwarded key to have landed (the
// receive step's own poll finds one within tens of milliseconds) and asserts
// its content does not end in key -- the byte a prior "sends" step wrote
// would sit on the shell's input line, at the end of the capture.
func privateTMuxPaneForSessionDidNotReceive(ctx context.Context, name, key string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	slug, err := sessionSlugByName(h, name)
	if err != nil {
		return err
	}
	time.Sleep(750 * time.Millisecond)
	out, err := tmuxOutput(ctx, h, "capture-pane", "-p", "-t", "deck_"+slug)
	if err != nil {
		return fmt.Errorf("capture private tmux pane for %q: %w", name, err)
	}
	if strings.HasSuffix(strings.TrimRight(string(out), " \n"), key) {
		return fmt.Errorf("private tmux pane for %q received %q, want nothing to reach the pane:\n%s", name, key, out)
	}
	return nil
}
