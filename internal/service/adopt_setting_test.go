package service

import (
	"context"
	"testing"

	"github.com/n-orlov/deck/internal/store"
)

func boolPtr(v bool) *bool { return &v }

// TestAdoptSettingGlobalAndSessionOverride is R253's setting table: the global
// adopt_harness_names is the default, a session's own override beats it in both
// directions, and off means neither adoption nor push.
func TestAdoptSettingGlobalAndSessionOverride(t *testing.T) {
	for _, tc := range []struct {
		name     string
		global   bool
		override *bool
		want     bool
	}{
		{name: "global on, inherit", global: true, want: true},
		{name: "global off, inherit", global: false, want: false},
		{name: "global off", global: false, override: nil, want: false},
		{name: "per-session off over global on", global: true, override: boolPtr(false), want: false},
		{name: "per-session on over global off", global: false, override: boolPtr(true), want: true},
		{name: "per-session on over global on", global: true, override: boolPtr(true), want: true},
		{name: "per-session off over global off", global: false, override: boolPtr(false), want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			rig := newPushRig(t, true)
			global := tc.global
			rig.svc.AdoptNames = func() bool { return global }

			// Adoption: a derived row whose harness reports a title.
			adopted := rig.row(t, "row-adopt", "idle")
			if _, err := rig.svc.SetHarnessNameAdopt(ctx, adopted.ID, tc.override); err != nil {
				t.Fatal(err)
			}
			*rig.observed = "Harness chose this"
			renamed, err := rig.svc.AdoptName(ctx, adopted.ID)
			if err != nil {
				t.Fatal(err)
			}
			got := mustGet(t, rig.db, adopted.ID)
			if renamed != tc.want || (got.Name == "Harness chose this") != tc.want {
				t.Fatalf("adoption: renamed=%v name=%q, want adopted=%v", renamed, got.Name, tc.want)
			}

			// Push: a deck rename of a running row.
			*rig.observed = ""
			pushed, err := rig.db.CreateSession(ctx, store.CreateSessionInput{
				ID: "row-push", Name: "second row", CWD: t.TempDir(), Agent: "claude", CapturedPath: "/bin",
				Status: "idle", StatusSource: "hook", StatusAt: 1, CreatedAt: 1,
				ConversationID: "conv-row-push", NameSource: "derived",
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := rig.svc.SetHarnessNameAdopt(ctx, pushed.ID, tc.override); err != nil {
				t.Fatal(err)
			}
			after, result, err := rig.svc.RenameAndPush(ctx, pushed.ID, "typed name")
			if err != nil {
				t.Fatal(err)
			}
			if after.Name != "typed name" {
				t.Fatalf("the rename itself must stand, got %q", after.Name)
			}
			if sent := len(rig.rec.plans) == 1; sent != tc.want || result.Sent != tc.want {
				t.Fatalf("push: plans=%v Sent=%v, want pushed=%v", rig.rec.plans, result.Sent, tc.want)
			}
			if tc.want == false && (result.Note != "" || rig.eventCount(t, pushed.ID, EventHarnessRenameRefused) != 0) {
				t.Fatalf("an off row must say nothing about the push: %q", result.Note)
			}
		})
	}
}

// TestAdoptSettingTickHonoursTheSetting: the reconcile tick adopts nothing when
// adoption is off for the row.
func TestAdoptSettingTickHonoursTheSetting(t *testing.T) {
	ctx := context.Background()
	rig := newPushRig(t, true)
	rig.svc.AdoptNames = func() bool { return false }
	row := rig.row(t, "row-tick", "idle")
	*rig.observed = "Harness chose this"
	rig.svc.adoptOnTick(ctx, row)
	if got := mustGet(t, rig.db, row.ID); got.Name != "old name" {
		t.Fatalf("tick adopted %q with adoption off", got.Name)
	}
	if _, err := rig.svc.SetHarnessNameAdopt(ctx, row.ID, boolPtr(true)); err != nil {
		t.Fatal(err)
	}
	rig.svc.adoptOnTick(ctx, mustGet(t, rig.db, row.ID))
	if got := mustGet(t, rig.db, row.ID); got.Name != "Harness chose this" {
		t.Fatalf("tick did not adopt with the per-session override on, name %q", got.Name)
	}
}
