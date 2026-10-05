// Package features is the covfixture's stand-in for the real Godog entry
// point (task 002, R187): a no-op TestFeatures, fast enough that
// ci/suite.sh's second pass contributes nothing to this fixture's
// coverage data either -- which is the point, see unit/unit_test.go's own
// header. Test files are never instrumented, so the marker below keeps the
// fixture statement-free.
package features

import (
	"os"
	"path/filepath"
	"testing"
)

// extraFlagMarker is set by extraflag_test.go, which only builds under
// -tags=suiteextraflag.
var extraFlagMarker string

func TestFeatures(t *testing.T) {
	dir := os.Getenv("EXTRA_PROBE_DIR")
	if dir != "" {
		// What ci/suite.sh told this pass about the unit pass's covermode.
		if err := os.WriteFile(filepath.Join(dir, "features-covermode"), []byte(os.Getenv("DECK_FEATURES_COVERMODE")), 0o644); err != nil {
			t.Fatalf("write covermode marker: %v", err)
		}
	}
	if dir == "" || extraFlagMarker == "" {
		return
	}
	if err := os.WriteFile(filepath.Join(dir, "features"), []byte(extraFlagMarker), 0o644); err != nil {
		t.Fatalf("write marker: %v", err)
	}
}
