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
	sc.Step(`^the teardown hook artefact file records each of "([^"]+)" and "([^"]+)"'s own DECK_SESSION_ID exactly once$`, teardownHookIDArtefactFileRecordsEachSessionsOwnIDExactlyOnce)
}

// teardownHookArtefactPath is the one file every hook line in this suite
// appends to. Fixed rather than parameterised: each scenario owns its own
// DECK_HOME (h.Home) and never shares it with another scenario, so there
// is nothing for a second name to disambiguate.
func teardownHookArtefactPath(h *ScenarioHarness) string {
	return filepath.Join(h.Home, "pd.txt")
}

// teardownHookIDArtefactPath is the file the bulk-dd scenario's two
// session hooks each append their own DECK_SESSION_ID to (task 019, SPEC
// §9.2's teardown env). Kept separate from teardownHookArtefactPath's
// session-NAME file on purpose: that step already asserts a file holds
// exactly one line naming one session, and reusing it for two lines from
// two sessions would silently change what it proves for the earlier
// scenarios that also open a fresh DECK_HOME of their own.
func teardownHookIDArtefactPath(h *ScenarioHarness) string {
	return filepath.Join(h.Home, "pd_ids.txt")
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

// teardownHookIDArtefactFileRecordsEachSessionsOwnIDExactlyOnce proves a
// bulk dd over the two named marked rows ran each row's own post_destroy
// hook exactly once, tagged with that row's own DECK_SESSION_ID (SPEC
// §6.1's session context, carried into the teardown env by
// Service.teardownEnv) -- never a shared id between the two rows, and
// never zero or two runs for either one. It reads each session's actual
// id from the state database itself (sessionIDByName), never a value this
// step invents, so a bug that wrote the wrong session's id would fail
// this exactly as loudly as writing no id at all.
func teardownHookIDArtefactFileRecordsEachSessionsOwnIDExactlyOnce(ctx context.Context, name1, name2 string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	id1, err := sessionIDByName(h, name1)
	if err != nil {
		return err
	}
	id2, err := sessionIDByName(h, name2)
	if err != nil {
		return err
	}
	if id1 == "" || id2 == "" || id1 == id2 {
		return fmt.Errorf("session ids for %q/%q must be distinct and non-empty, got %q/%q", name1, name2, id1, id2)
	}
	path := teardownHookIDArtefactPath(h)
	deadline := time.Now().Add(5 * time.Second)
	var lastErr error
	for {
		data, err := os.ReadFile(path)
		if err == nil {
			lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
			counts := map[string]int{}
			for _, line := range lines {
				if line != "" {
					counts[line]++
				}
			}
			if len(lines) == 2 && counts[id1] == 1 && counts[id2] == 1 {
				return nil
			}
			lastErr = fmt.Errorf("teardown hook id artefact file %q = %q, want exactly one line each for %q's id %q and %q's id %q", path, string(data), name1, id1, name2, id2)
		} else {
			lastErr = fmt.Errorf("read teardown hook id artefact file %q: %w", path, err)
		}
		if time.Now().After(deadline) {
			return lastErr
		}
		time.Sleep(25 * time.Millisecond)
	}
}
