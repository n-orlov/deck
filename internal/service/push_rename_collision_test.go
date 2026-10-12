package service

import (
	"context"
	"testing"

	"github.com/n-orlov/deck/internal/store"
)

func (r pushRig) namedRow(t *testing.T, id, name, agentKind, status string) store.Session {
	t.Helper()
	session, err := r.db.CreateSession(context.Background(), store.CreateSessionInput{
		ID: id, Name: name, CWD: t.TempDir(), Agent: agentKind, CapturedPath: "/bin",
		Status: status, StatusSource: "hook", StatusAt: 1, CreatedAt: 1,
		ConversationID: "conv-" + id, NameSource: "derived",
	})
	if err != nil {
		t.Fatal(err)
	}
	return session
}

// TestHarnessRenameCollisionTakesFirstFreeSuffix is the Decisions "Collisions"
// rule (R252, R264) for a deck rename destined for a harness: a name or slug
// another row holds is resolved to the first free -2, -3, ... suffix, the
// resolved name is stored and pushed, and the tmux slug never moves.
func TestHarnessRenameCollisionTakesFirstFreeSuffix(t *testing.T) {
	for _, tc := range []struct {
		name    string
		holders []string
		target  string
		want    string
	}{
		{name: "plain name taken", holders: []string{"alpha"}, target: "alpha", want: "alpha-2"},
		{name: "successive suffixes occupied", holders: []string{"alpha", "alpha-2", "alpha-3"}, target: "alpha", want: "alpha-4"},
		{name: "a gap is filled first", holders: []string{"beta", "beta-3"}, target: "beta", want: "beta-2"},
		{name: "collision by slug only", holders: []string{"Gamma Row"}, target: "gamma row", want: "gamma row-2"},
		{name: "slug collision with an occupied suffix", holders: []string{"Delta Row", "delta row-2"}, target: "DELTA ROW", want: "DELTA ROW-3"},
		{name: "free name is kept as typed", holders: []string{"epsilon"}, target: "zeta", want: "zeta"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			rig := newPushRig(t, true)
			for i, name := range tc.holders {
				rig.namedRow(t, "holder-"+string(rune('a'+i)), name, "claude", "idle")
			}
			row := rig.namedRow(t, "subject", "subject old", "claude", "idle")

			got, result, err := rig.svc.RenameAndPush(ctx, row.ID, "  "+tc.target+"  ")
			if err != nil {
				t.Fatalf("a collision must be resolved, not refused: %v", err)
			}
			if got.Name != tc.want {
				t.Fatalf("stored name = %q, want %q", got.Name, tc.want)
			}
			if got.Slug != row.Slug {
				t.Fatalf("slug changed from %q to %q; the tmux identity must not move", row.Slug, got.Slug)
			}
			if !result.Sent || len(rig.rec.plans) != 1 {
				t.Fatalf("expected one push, got Sent=%v plans=%v", result.Sent, rig.rec.plans)
			}
			if want := pushPlan(tc.want); len(rig.rec.plans[0]) != 3 || rig.rec.plans[0][1] != want[1] {
				t.Fatalf("pushed %v, want the resolved name %q", rig.rec.plans[0], tc.want)
			}
			if stored := mustGet(t, rig.db, row.ID); stored.Name != tc.want || stored.NameSource != "user" {
				t.Fatalf("persisted name=%q source=%q", stored.Name, stored.NameSource)
			}
		})
	}
}

// TestHarnessRenameCollisionRules covers the rows the suffix rule does and
// does not reach: a stopped harness row still takes it (the name goes in at
// the next launch), renaming a row to its own name keeps it, adoption off or a
// shell row keeps the plain refusal.
func TestHarnessRenameCollisionRules(t *testing.T) {
	ctx := context.Background()
	t.Run("stopped harness row is suffixed and sends nothing", func(t *testing.T) {
		rig := newPushRig(t, true)
		rig.namedRow(t, "holder", "omega", "claude", "idle")
		row := rig.namedRow(t, "stopped-row", "stopped old", "claude", "stopped")
		got, result, err := rig.svc.RenameAndPush(ctx, row.ID, "omega")
		if err != nil || got.Name != "omega-2" || result.Sent || len(rig.rec.plans) != 0 {
			t.Fatalf("got %q err=%v sent=%v plans=%v", got.Name, err, result.Sent, rig.rec.plans)
		}
	})
	t.Run("the row's own name is not a collision", func(t *testing.T) {
		rig := newPushRig(t, true)
		row := rig.namedRow(t, "self", "same", "claude", "idle")
		got, _, err := rig.svc.RenameAndPush(ctx, row.ID, "same")
		if err != nil || got.Name != "same" {
			t.Fatalf("got %q err=%v", got.Name, err)
		}
	})
	t.Run("adoption off keeps the refusal", func(t *testing.T) {
		rig := newPushRig(t, true)
		rig.namedRow(t, "holder", "sigma", "claude", "idle")
		row := rig.namedRow(t, "off-row", "off old", "claude", "idle")
		if _, err := rig.svc.SetHarnessNameAdopt(ctx, row.ID, boolPtr(false)); err != nil {
			t.Fatal(err)
		}
		if _, _, err := rig.svc.RenameAndPush(ctx, row.ID, "sigma"); err == nil {
			t.Fatal("a rename that is not destined for a harness must still refuse a taken name")
		}
		if got := mustGet(t, rig.db, row.ID); got.Name != "off old" {
			t.Fatalf("name = %q, want unchanged", got.Name)
		}
	})
	t.Run("shell row keeps the refusal", func(t *testing.T) {
		rig := newPushRig(t, true)
		rig.namedRow(t, "holder", "tau", "claude", "idle")
		row := rig.namedRow(t, "shell-row", "shell old", "shell", "idle")
		if _, _, err := rig.svc.RenameAndPush(ctx, row.ID, "tau"); err == nil {
			t.Fatal("a shell rename must still refuse a taken name")
		}
	})
}
