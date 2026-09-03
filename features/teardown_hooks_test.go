package features

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

// registerTeardownHooksSteps backs features/teardown_hooks.feature (tasks
// 018/019, SPEC §9.2): a session's own post_destroy hook, set through the
// launch-inputs editor's "Post-destroy command" field (task 023's `l`
// editor -- the only field this suite drives it through, pending task
// 026's own create-modal field), fires exactly once on A/dd and never on
// x, observed against a real file the hook's own subprocess writes under
// this scenario's DECK_HOME, never a mock.
func registerTeardownHooksSteps(sc *godog.ScenarioContext) {
	sc.Step(`^deck client "([^"]+)" types "([^"]*)" into the post-destroy field$`, clientTypesIntoLaunchInputsPostDestroyField)
	sc.Step(`^the teardown hook artefact file is absent$`, teardownHookArtefactFileIsAbsent)
	sc.Step(`^the teardown hook artefact file contains exactly one line "([^"]+)"$`, teardownHookArtefactFileContainsExactlyOneLine)
}

// teardownHookArtefactPath is the one file every hook line in this suite
// appends to. Fixed rather than parameterised: each scenario owns its own
// DECK_HOME (h.Home) and never shares it with another scenario, so there
// is nothing for a second name to disambiguate.
func teardownHookArtefactPath(h *ScenarioHarness) string {
	return filepath.Join(h.Home, "pd.txt")
}

// clientTypesIntoLaunchInputsPostDestroyField moves focus down one field
// from the launch-inputs editor's default (field 0, Pre-launch command --
// launch_inputs.go's own comment: "the editor always opens with field 0
// focused") onto field 1 (Post-destroy command) via the shared dialog
// contract's "down" (dialog_contract.go, the same key
// clientPressesArrowInOpenDialog sends elsewhere), then types exactly like
// clientTypesIntoLaunchInputsPreLaunchField does for field 0. Only valid
// directly after opening the editor, before any other field navigation.
func clientTypesIntoLaunchInputsPostDestroyField(ctx context.Context, clientName, value string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	if err := client.Send("\x1b[B"); err != nil {
		return err
	}
	time.Sleep(25 * time.Millisecond)
	if err := client.Send(value); err != nil {
		return err
	}
	return client.WaitForFrame(ctx, false, value)
}

// teardownHookArtefactFileIsAbsent proves x never invoked post_destroy: no
// file exists yet at all. Polled rather than a single os.Stat, since x's
// own kill is real tmux/store work that this step is not otherwise
// synchronised with.
func teardownHookArtefactFileIsAbsent(ctx context.Context) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	path := teardownHookArtefactPath(h)
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("teardown hook artefact file %q exists, want absent (x must never run post_destroy)", path)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

// teardownHookArtefactFileContainsExactlyOneLine proves the hook ran
// exactly once -- never zero (the action that should have fired it did)
// and never twice (no double run, and no leftover second run from a later
// action in the same scenario, e.g. an undo) -- and that the one line it
// wrote names the right session via DECK_SESSION_NAME, the same variable
// features/launch_hooks.feature already asserts against a live pane's own
// environment.
func teardownHookArtefactFileContainsExactlyOneLine(ctx context.Context, sessionName string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	path := teardownHookArtefactPath(h)
	deadline := time.Now().Add(5 * time.Second)
	var lastErr error
	for {
		data, err := os.ReadFile(path)
		if err == nil {
			lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
			if len(lines) == 1 && lines[0] == sessionName {
				return nil
			}
			lastErr = fmt.Errorf("teardown hook artefact file %q = %q, want exactly one line %q", path, string(data), sessionName)
		} else {
			lastErr = fmt.Errorf("read teardown hook artefact file %q: %w", path, err)
		}
		if time.Now().After(deadline) {
			return lastErr
		}
		time.Sleep(25 * time.Millisecond)
	}
}
