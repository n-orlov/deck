package service

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/notify"
)

// Prepare claims the pair durably and spawns nothing: the claim is visible to a
// second dispatcher before run is ever called, and run then spawns exactly once.
func TestPrepareClaimsTheOfferBeforeAnySpawnAndRunSpawnsOnce(t *testing.T) {
	f := newHookDispatchFixture(t, "waiting")
	var spawns atomic.Int32
	f.d.Spawn = func(context.Context, notify.Request) (notify.Result, error) {
		spawns.Add(1)
		return notify.Result{}, nil
	}
	ev := HookEvent{SessionID: "s1", StoredKind: "notification", Reason: "permission_prompt", At: time.Now()}
	run, outcome := f.d.Prepare(context.Background(), ev, false)
	if run == nil || outcome.Skip != notify.SkipNone || outcome.Err != nil {
		t.Fatalf("Prepare = run %v outcome %+v, want a runnable claim", run != nil, outcome)
	}
	if spawns.Load() != 0 {
		t.Fatal("Prepare spawned before run was called")
	}
	if again, out := f.d.Prepare(context.Background(), ev, false); again != nil || out.Skip != notify.SkipDeduped {
		t.Fatalf("second Prepare = run %v outcome %+v, want the pair already claimed", again != nil, out)
	}
	if out := run(); !out.Spawned || out.Err != nil || spawns.Load() != 1 {
		t.Fatalf("run = %+v after %d spawns, want one spawn", out, spawns.Load())
	}
}

// A Prepare that skips returns no run, with Dispatch's own reason.
func TestPrepareReturnsNoRunForASkippedOffer(t *testing.T) {
	f := newHookDispatchFixture(t, "running")
	run, outcome := f.d.Prepare(context.Background(), HookEvent{SessionID: "s1", StoredKind: "user_prompt_submitted"}, false)
	if run != nil || outcome.Skip != notify.SkipNotOffer {
		t.Fatalf("Prepare = run %v outcome %+v, want SkipNotOffer", run != nil, outcome)
	}
	run, outcome = f.d.Prepare(context.Background(), HookEvent{SessionID: "missing", StoredKind: "stop"}, false)
	if run != nil || outcome.Err == nil {
		t.Fatalf("Prepare for a missing session = run %v outcome %+v, want an error", run != nil, outcome)
	}
}
