// Package unit is the covfixture's ordinary Go package for
// ci/suitecheck.TestSuiteScript* (task 002, R187). It deliberately
// contains no non-test source file at all, so -coverpkg=./... instruments
// zero statements for it ("[no statements]") -- which, combined with the
// equally statement-free features/ fixture below, lets
// TestSuiteScriptFailsWhenMergedCoverageProfileHasNoBlocks force a
// genuinely empty coverage-merged.out without needing to fight go test's
// own flags to get there.
//
// Its one test also records whether DECK_SEED_PROBE -- a stand-in for a
// leaked DECK_GODOG_PATHS/DECK_HOME the caller's own shell happened to
// still have set -- was visible inside the go test process, writing
// whatever value it saw (even "", meaning unset) to the path named by
// PROBE_OUT. PROBE_OUT itself is not a DECK_* name, so the scrub this
// task adds to ci/suite.sh must never touch it.
package unit

import (
	"os"
	"testing"
)

func TestFixtureRecordsSeededDeckEnvironment(t *testing.T) {
	out := os.Getenv("PROBE_OUT")
	if out == "" {
		t.Fatal("PROBE_OUT not set")
	}
	seen := os.Getenv("DECK_SEED_PROBE")
	if err := os.WriteFile(out, []byte(seen), 0o644); err != nil {
		t.Fatalf("write %s: %v", out, err)
	}
}
