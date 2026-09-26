// Package features is the deterministic timeout fixture for
// ci/suitecheck.TestSuiteScriptTimeoutFixtureRetainsScenarioResultsAndAbort
// (cure-01-08, R146). Its TestFeatures stands in for the real Godog entry
// point: it leaves DECK_GODOG_JUNIT as the empty file Godog's JUnit
// formatter leaves behind when the process is killed before it flushes,
// finishes one scenario subtest, then blocks in a second one until go
// test's own -timeout alarm kills the binary -- the exact shape of final-sha
// nightly 36222292303.
package features

import (
	"os"
	"testing"
	"time"
)

func TestFeatures(t *testing.T) {
	if path := os.Getenv("DECK_GODOG_JUNIT"); path != "" {
		f, err := os.Create(path)
		if err != nil {
			t.Fatalf("create %s: %v", path, err)
		}
		f.Close()
	}
	t.Run("completed_scenario", func(t *testing.T) {})
	t.Run("hanging_scenario", func(t *testing.T) { time.Sleep(time.Hour) })
}
