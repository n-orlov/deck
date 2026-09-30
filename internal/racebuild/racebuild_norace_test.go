//go:build !race

package racebuild

import "testing"

// TestDisabledOnNormalBuild pins racebuild.Enabled to false on a normal
// (non-race) build.
func TestDisabledOnNormalBuild(t *testing.T) {
	if Enabled {
		t.Fatalf("racebuild.Enabled = %v, want false on a normal build", Enabled)
	}
}
