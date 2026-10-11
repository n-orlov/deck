package service

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

// TestPushSanitisedNameReachesKeystrokePlan: the name the adapter builds its
// plan from is the per-harness sanitised one (R253), never the raw row name.
func TestPushSanitisedNameReachesKeystrokePlan(t *testing.T) {
	rig := newPushRig(t, false)
	row := rig.row(t, "s1", "idle")
	long := strings.Repeat("n", 260)
	for _, tc := range []struct{ typed, want string }{
		{"work\x1b[31m\x07 item", "work item"},
		{long, strings.Repeat("n", 200)},
	} {
		rig.rec.plans = nil
		if _, result, err := rig.svc.RenameAndPush(context.Background(), row.ID, tc.typed); err != nil || !result.Sent {
			t.Fatalf("RenameAndPush(%q): result=%+v err=%v", tc.typed, result, err)
		}
		if want := [][]string{pushPlan(tc.want)}; !reflect.DeepEqual(rig.rec.plans, want) {
			t.Fatalf("plans = %q, want %q", rig.rec.plans, want)
		}
	}
}

// TestPushSanitisedBlankNameIsNeverSent: a name that is only junk once cleaned
// sends nothing and records a refusal; a blank name is never passed on.
func TestPushSanitisedBlankNameIsNeverSent(t *testing.T) {
	rig := newPushRig(t, false)
	row := rig.row(t, "s1", "idle")
	_, result, err := rig.svc.RenameAndPush(context.Background(), row.ID, "\x1b[0m\x07")
	if err != nil {
		t.Fatal(err)
	}
	if result.Sent || len(rig.rec.plans) != 0 || rig.eventCount(t, row.ID, EventHarnessRenameRefused) != 1 {
		t.Fatalf("result=%+v plans=%q refused=%d, want one refusal and no keystrokes", result, rig.rec.plans, rig.eventCount(t, row.ID, EventHarnessRenameRefused))
	}
}
