package features

// TestMouseFeatureNeverRegainsMillisecondsPass is task 403's mechanical
// guard: the R54 no-op scenario in features/mouse.feature must never go
// back to a fixed-duration "N milliseconds pass" step (mouse.go's own
// discriminator, docs/reports/phase3e-403-*/README.md) -- every one of
// this file's three former waits was replaced by a step that polls the
// specific observable consequence it stood in for
// (clientPreviewTopBorderContains/clientHasSessionSelected's own
// WaitForFrame(Func) idiom for the two real changes, and
// privateTMuxWindowOwnershipClaimForSessionStillMatches's bounded
// poll-across-the-window for the one genuine no-op) instead of guessing a
// delay.
//
// This is scoped to features/mouse.feature alone, not to features/*.feature
// generally: kill_delete_undo.feature, interactive_sigwinch_budget.feature
// and preview.feature all still use "milliseconds pass" for its
// legitimate job -- waiting out a REAL wall-clock timeout deck itself
// schedules against a live clock (the undo toast's DECU_UNDO_MS expiry,
// io/os signal-coalescing pacing, and a genuine pre-baseline settle) --
// and millisecondsPass (features/kill_delete_undo_test.go) stays available
// for exactly those callers.
//
// Demonstrate-then-revert: add a line like
// `And 200 milliseconds pass` back into
// features/mouse.feature's @requirement-54-sidebar-click-on-interactive-row-is-a-no-op
// scenario and rerun this test -- it goes red, confirming the check is
// live -- then remove it again before committing.
import (
	"os"
	"strings"
	"testing"
)

func TestMouseFeatureNeverRegainsMillisecondsPass(t *testing.T) {
	const path = "mouse.feature"
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	src := string(data)

	for lineNum, line := range strings.Split(src, "\n") {
		if strings.Contains(line, "milliseconds pass") {
			t.Errorf("%s:%d: a fixed-duration \"milliseconds pass\" step reappeared in mouse.feature -- settle on an observable consequence instead (see this test's own doc comment): %s", path, lineNum+1, strings.TrimSpace(line))
		}
	}

	if !strings.Contains(src, "@requirement-54-sidebar-click-on-interactive-row-is-a-no-op") {
		t.Fatalf("%s: the R54 no-op scenario's tag not found -- guard is not exercising anything", path)
	}
}
