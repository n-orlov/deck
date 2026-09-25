package features

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/cucumber/godog"
)

// registerCIVerifyFlakySteps wires the throwaway task-019 (C.7) probe step:
// it fails on its first invocation and passes on the immediate rerun
// (whichever process runs it next -- the full features/ pass, or
// ci/suite.sh's own per-scenario solo rerun via DECK_GODOG_PATHS), by
// keeping a marker file in the process's temp dir. The marker lives in the
// sibling container's own /tmp, which persists across ci/suite.sh's own
// rerun (same container, same ci/suite.sh invocation) but starts fresh on
// the next CI run. This step and its .feature file exist only on this
// throwaway branch -- never on main.
func registerCIVerifyFlakySteps(sc *godog.ScenarioContext) {
	sc.Step(`^the ci-verify flaky marker is toggled$`, func(context.Context) error {
		marker := filepath.Join(os.TempDir(), "deck-ci-verify-flaky-features.marker")
		if _, err := os.Stat(marker); err == nil {
			return nil
		}
		if err := os.WriteFile(marker, []byte("x"), 0o644); err != nil {
			return err
		}
		return errors.New("ci-verify: intentional first-run failure (task 019, R145/R146)")
	})
}
