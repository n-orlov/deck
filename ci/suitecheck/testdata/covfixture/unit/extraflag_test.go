//go:build suiteextraflag

// This file is compiled only when `go test` is given -tags=suiteextraflag,
// which TestSuiteScriptPassesCapturedExtraFlagsToBothPasses supplies
// through ci/suite.sh's own DECK_CI_GO_EXTRA_FLAGS setting. Its test
// writes a marker into EXTRA_PROBE_DIR, proving the unit pass's `go test`
// really received the extra flags even though ci/suite.sh unsets every
// DECK_* variable (DECK_CI_GO_EXTRA_FLAGS included) before launching it.
package unit

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFixtureRecordsExtraFlagsReachedUnitPass(t *testing.T) {
	dir := os.Getenv("EXTRA_PROBE_DIR")
	if dir == "" {
		return
	}
	if err := os.WriteFile(filepath.Join(dir, "unit"), []byte("tagged"), 0o644); err != nil {
		t.Fatalf("write marker: %v", err)
	}
}
