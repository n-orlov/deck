package tmux

import (
	"errors"
	"testing"
)

// TestIsTargetAbsentTreatsAVanishedPaneAsARemovalRace pins GH #36 / R140's
// race fix: a pane that vanished between a pane list and a later
// capture-pane against the pane id that list returned reports "can't find
// pane: <id>", never "can't find session"/"window", because pane ids are
// server-global. IsTargetAbsent must recognize that message as a removal
// race (so callers like the probe capture path can `continue` instead of
// erroring) while still surfacing an unrelated tmux failure as a real error.
func TestIsTargetAbsentTreatsAVanishedPaneAsARemovalRace(t *testing.T) {
	if !IsTargetAbsent(errors.New("can't find pane: %1")) {
		t.Fatal("IsTargetAbsent(can't find pane) = false, want true")
	}
	if IsTargetAbsent(errors.New("unrelated tmux failure: permission denied")) {
		t.Fatal("IsTargetAbsent(unrelated failure) = true, want false")
	}
}
