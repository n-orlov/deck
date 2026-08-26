package features

// TestNavigationHelperNeverSleeps is the mechanical guard for steer
// 3e-002: the merged top-anchored navigation helper behind
// selectSessionByNameThenSend, selectRowByName and
// clientOpensDetailForSession (features/navigation_settle_test.go) must
// settle every keystroke by polling an observable consequence (the
// sidebar's selected-row line actually changing, via
// ScreenDriver.WaitForFrame/WaitForFrameGone) rather than by guessing a
// fixed delay with time.Sleep -- the exact defect class task 324/7ebafce
// fixed for one of the three call sites alone (agent_steps_test.go's
// selectSessionByNameThenSend), which this task collapsed into one helper
// and fixed for all three.
//
// This is scoped narrowly to the navigation helper's own file, not to
// features/*.go generally: many unrelated deadline-polling loops elsewhere
// in this package legitimately still use time.Sleep as their wait step
// (e.g. status_probe_test.go's several `for time.Now().Before(deadline)`
// loops) and are out of this task's scope.
//
// Demonstrate-then-revert: add a line like `time.Sleep(time.Millisecond)`
// anywhere in features/navigation_settle_test.go and rerun this test -- it
// goes red, confirming the check is live -- then remove it again before
// committing.
import (
	"os"
	"strings"
	"testing"
)

func TestNavigationHelperNeverSleeps(t *testing.T) {
	const path = "navigation_settle_test.go"
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	src := string(data)

	for lineNum, line := range strings.Split(src, "\n") {
		if strings.Contains(line, "time.Sleep(") {
			t.Errorf("%s:%d: time.Sleep found in the merged navigation helper -- settle on an observable consequence (WaitForFrame/WaitForFrameGone) instead: %s", path, lineNum+1, strings.TrimSpace(line))
		}
	}

	if !strings.Contains(src, "func navigateToRowByName(") {
		t.Fatalf("%s: navigateToRowByName not found -- guard is not exercising anything", path)
	}
}
