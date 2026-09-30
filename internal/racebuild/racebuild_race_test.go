//go:build race

package racebuild

import "testing"

// TestEnabledOnRaceBuild pins racebuild.Enabled to true when this test
// binary is built with the race detector (`go test -race`).
func TestEnabledOnRaceBuild(t *testing.T) {
	if !Enabled {
		t.Fatalf("racebuild.Enabled = %v, want true on a -race build", Enabled)
	}
}
