package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/notify"
)

// slowSpawn is a Spawn seam that holds every call for hold and then answers
// with the result chosen for the session by outcome.
type slowSpawn struct {
	hold    time.Duration
	outcome func(sessionID string) (notify.Result, error)
	mu      sync.Mutex
	started map[string]int
}

func (s *slowSpawn) spawn(_ context.Context, req notify.Request) (notify.Result, error) {
	s.mu.Lock()
	if s.started == nil {
		s.started = map[string]int{}
	}
	s.started[req.Session.ID]++
	s.mu.Unlock()
	time.Sleep(s.hold)
	return s.outcome(req.Session.ID)
}

func batchSessionID(n int) string { return fmt.Sprintf("00000000-0000-4000-8000-00000000b%03d", n) }

// SPEC §3.1/§10.4: the process deaths one liveness pass records are offered
// after the pass, and the whole batch costs one event_hook_timeout, not one per
// death. Each offer is still claimed, spawned and recorded on its own event.
func TestDeferredHookBatchCostsOneBoundWhateverTheNumberOfDeaths(t *testing.T) {
	for _, deaths := range []int{1, 3, 8} {
		t.Run(fmt.Sprintf("%d deaths", deaths), func(t *testing.T) {
			svc, db, _, _ := newAgentTestService(t, nil, fmt.Sprintf("hook-batch-%d", deaths))
			_, d := withEventHook(&svc, "tmux.session_gone")
			slow := &slowSpawn{hold: 400 * time.Millisecond, outcome: func(string) (notify.Result, error) {
				return notify.Result{ExitCode: 0, Output: "slow done\n"}, nil
			}}
			d.Spawn = slow.spawn
			svc.EventHook = func() EventHookDispatcher { return d }
			for i := 0; i < deaths; i++ {
				goneRow(t, db, batchSessionID(i), "claude", "running")
			}

			start := time.Now()
			if err := svc.ReconcileWithin(context.Background(), 5*time.Second); err != nil {
				t.Fatal(err)
			}
			// One hold plus slack; a serial dispatch would take deaths*hold.
			if elapsed := time.Since(start); elapsed > slow.hold+300*time.Millisecond {
				t.Fatalf("%d slow hooks held the pass %s, want about one hold (%s)", deaths, elapsed, slow.hold)
			}
			for i := 0; i < deaths; i++ {
				id := batchSessionID(i)
				if slow.started[id] != 1 {
					t.Fatalf("session %d spawned %d times, want once (an offer was lost or doubled)", i, slow.started[id])
				}
				got, ok, err := db.EventHookResultOf(context.Background(), eventSeqOf(t, db, id, "tmux.session_gone"))
				if err != nil || !ok || got.Kind != "ended" || got.Output != "slow done\n" {
					t.Fatalf("session %d stored result = %+v ok=%v err=%v", i, got, ok, err)
				}
			}
		})
	}
}

// Slow, failing, timed-out and healthy scripts in one pass each leave their own
// recorded outcome and each is offered exactly once, and a repeat pass offers
// nothing: no offer is dropped because a sibling was slow or failed.
func TestDeferredHookBatchRecordsEveryOutcomeWhenSomeFail(t *testing.T) {
	svc, db, _, _ := newAgentTestService(t, nil, "hook-batch-mixed")
	_, d := withEventHook(&svc, "tmux.session_gone")
	outcomes := map[string]func() (notify.Result, error){
		batchSessionID(0): func() (notify.Result, error) { return notify.Result{ExitCode: 7, Output: "boom\n"}, nil },
		batchSessionID(1): func() (notify.Result, error) { return notify.Result{ExitCode: -1, TimedOut: true}, nil },
		batchSessionID(2): func() (notify.Result, error) { return notify.Result{}, errors.New("cannot start script") },
		batchSessionID(3): func() (notify.Result, error) { return notify.Result{ExitCode: 0, Output: "fine\n"}, nil },
	}
	slow := &slowSpawn{hold: 300 * time.Millisecond, outcome: func(id string) (notify.Result, error) { return outcomes[id]() }}
	d.Spawn = slow.spawn
	svc.EventHook = func() EventHookDispatcher { return d }
	for i, agent := range []string{"claude", "shell", "codex", "claude"} {
		goneRow(t, db, batchSessionID(i), agent, "running")
	}

	start := time.Now()
	if err := svc.ReconcileWithin(context.Background(), 5*time.Second); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > slow.hold+300*time.Millisecond {
		t.Fatalf("mixed batch held the pass %s, want about one hold", elapsed)
	}
	want := map[string]struct {
		exit     int
		timedOut bool
		errText  string
	}{
		batchSessionID(0): {exit: 7},
		batchSessionID(1): {exit: -1, timedOut: true},
		batchSessionID(2): {exit: -1, errText: "cannot start script"},
		batchSessionID(3): {exit: 0},
	}
	for id, w := range want {
		if slow.started[id] != 1 {
			t.Fatalf("%s spawned %d times, want once", id, slow.started[id])
		}
		got, ok, err := db.EventHookResultOf(context.Background(), eventSeqOf(t, db, id, "tmux.session_gone"))
		if err != nil || !ok {
			t.Fatalf("%s: no stored result: ok=%v err=%v", id, ok, err)
		}
		if got.ExitCode != w.exit || got.TimedOut != w.timedOut || got.Error != w.errText {
			t.Fatalf("%s stored %+v, want exit %d timedOut %v error %q", id, got, w.exit, w.timedOut, w.errText)
		}
	}
	if err := svc.ReconcileWithin(context.Background(), 5*time.Second); err != nil {
		t.Fatal(err)
	}
	for id, n := range slow.started {
		if n != 1 {
			t.Fatalf("a repeat pass respawned %s (%d)", id, n)
		}
	}
}
