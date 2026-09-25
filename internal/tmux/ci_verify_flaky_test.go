package tmux

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCIVerifyFlakyGo is a throwaway probe for task 019 (C.7): it fails on
// its first invocation and passes on the immediate rerun, exercising
// ci/suite.sh's fail-once-retry-once policy and the resulting flaky list in
// the job summary. The marker lives in the sibling container's own /tmp,
// which persists across gotestsum's own rerun (same container, same
// ci/suite.sh invocation) but starts fresh on the next CI run. Never merged
// to main -- this file lives only on this throwaway branch and is removed
// before the PR closes.
func TestCIVerifyFlakyGo(t *testing.T) {
	marker := filepath.Join(os.TempDir(), "deck-ci-verify-flaky-go.marker")
	if _, err := os.Stat(marker); err == nil {
		return
	}
	if err := os.WriteFile(marker, []byte("x"), 0o644); err != nil {
		t.Fatalf("ci-verify: could not write marker: %v", err)
	}
	t.Fatal("ci-verify: intentional first-run failure (task 019, R145/R146)")
}
