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
//
// With quorum set, every call first waits at a barrier until quorum calls have
// started, so the barrier opens only when all of them are in flight at the
// same time. That is the batch's "one bound" property stated structurally: a
// dispatcher that ran the scripts one after another (or partly so) never
// reaches the quorum, and each call gives up after quorumWait and counts a
// miss. Wall-clock limits such as "one hold plus 300ms" also counted the
// store work around each spawn (row read and claim before it, result after
// it), which serialises on the Store's one pooled connection and, with the
// SQLite engine itself race-instrumented, takes several times longer on a
// loaded -race agent: nightly #55 measured 722ms for 8 deaths, and a 1-death
// batch, with nothing to serialise, measured 886ms under local load.
type slowSpawn struct {
	hold    time.Duration
	outcome func(sessionID string) (notify.Result, error)
	quorum  int
	mu      sync.Mutex
	started map[string]int
	calls   int
	misses  int
	all     chan struct{}
}

// quorumWait is how long a call waits for its siblings to start. Only a
// serialised dispatcher waits this long; a concurrent one opens the barrier as
// soon as the last claim commits.
const quorumWait = 10 * time.Second

func (s *slowSpawn) spawn(_ context.Context, req notify.Request) (notify.Result, error) {
	s.mu.Lock()
	if s.started == nil {
		s.started = map[string]int{}
	}
	if s.all == nil {
		s.all = make(chan struct{})
	}
	s.started[req.Session.ID]++
	s.calls++
	if s.calls == s.quorum {
		close(s.all)
	}
	all := s.all
	s.mu.Unlock()
	if s.quorum > 0 {
		select {
		case <-all:
		case <-time.After(quorumWait):
			s.mu.Lock()
			s.misses++
			s.mu.Unlock()
		}
	}
	time.Sleep(s.hold)
	return s.outcome(req.Session.ID)
}

// requireAllInFlightTogether fails the test unless every spawn reached the
// quorum barrier while all of its siblings were also running.
func (s *slowSpawn) requireAllInFlightTogether(t *testing.T, what string) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.calls != s.quorum || s.misses != 0 {
		t.Fatalf("%s: %d spawns started, %d of them waited %s for the rest without all %d in flight at once: the scripts did not run concurrently",
			what, s.calls, s.misses, quorumWait, s.quorum)
	}
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
			slow := &slowSpawn{hold: 400 * time.Millisecond, quorum: deaths, outcome: func(string) (notify.Result, error) {
				return notify.Result{ExitCode: 0, Output: "slow done\n"}, nil
			}}
			d.Spawn = slow.spawn
			svc.EventHook = func() EventHookDispatcher { return d }
			for i := 0; i < deaths; i++ {
				goneRow(t, db, batchSessionID(i), "claude", "running")
			}

			if err := svc.ReconcileWithin(context.Background(), 5*time.Second); err != nil {
				t.Fatal(err)
			}
			// All scripts in flight at once is what makes the batch cost one hold;
			// a serial dispatch would cost deaths*hold and never reach the quorum.
			slow.requireAllInFlightTogether(t, fmt.Sprintf("%d slow hooks", deaths))
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
	slow := &slowSpawn{hold: 300 * time.Millisecond, quorum: len(outcomes), outcome: func(id string) (notify.Result, error) { return outcomes[id]() }}
	d.Spawn = slow.spawn
	svc.EventHook = func() EventHookDispatcher { return d }
	for i, agent := range []string{"claude", "shell", "codex", "claude"} {
		goneRow(t, db, batchSessionID(i), agent, "running")
	}

	if err := svc.ReconcileWithin(context.Background(), 5*time.Second); err != nil {
		t.Fatal(err)
	}
	slow.requireAllInFlightTogether(t, "mixed batch")
	// The repeat pass below spawns nothing; the barrier has done its job.
	slow.quorum = 0
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

// SPEC §3.1/§10.4: the incoming event's own script (claimed by Prepare, run on
// another goroutine, as `deck _hook` does) overlaps the pass and its death
// batch, so the whole call costs one hold however many deaths the pass records,
// not the incoming hold plus a second one for the batch. Every offer, the
// incoming one and each death, is spawned exactly once and recorded.
func TestPreparedIncomingScriptOverlapsTheDeathBatchWhateverTheNumberOfDeaths(t *testing.T) {
	for _, deaths := range []int{1, 4, 8} {
		t.Run(fmt.Sprintf("%d deaths", deaths), func(t *testing.T) {
			svc, db, _, _ := newAgentTestService(t, nil, fmt.Sprintf("hook-overlap-%d", deaths))
			_, d := withEventHook(&svc, "tmux.session_gone")
			// The incoming script, each death's and the incoming session's own
			// death: all deaths+2 must be in flight together.
			slow := &slowSpawn{hold: 400 * time.Millisecond, quorum: deaths + 2, outcome: func(string) (notify.Result, error) {
				return notify.Result{ExitCode: 0, Output: "slow done\n"}, nil
			}}
			d.Spawn = slow.spawn
			svc.EventHook = func() EventHookDispatcher { return d }
			incoming := batchSessionID(777)
			goneRow(t, db, incoming, "claude", "waiting")
			for i := 0; i < deaths; i++ {
				goneRow(t, db, batchSessionID(i), "claude", "running")
			}

			run, outcome := d.Prepare(context.Background(), HookEvent{
				SessionID: incoming, StoredKind: "notification", Reason: "permission_prompt", At: time.Now(),
			}, false)
			if run == nil {
				t.Fatalf("incoming event not claimed: %+v", outcome)
			}
			done := make(chan struct{})
			go func() { defer close(done); run() }()
			if err := svc.ReconcileWithin(context.Background(), 5*time.Second); err != nil {
				t.Fatal(err)
			}
			<-done
			// The incoming script still running when the last death's starts is
			// what makes the call one hold, not the incoming one then the batch.
			slow.requireAllInFlightTogether(t, fmt.Sprintf("incoming + %d deaths", deaths))
			slow.mu.Lock()
			defer slow.mu.Unlock()
			for i := 0; i < deaths; i++ {
				if n := slow.started[batchSessionID(i)]; n != 1 {
					t.Fatalf("death %d spawned %d times, want once", i, n)
				}
			}
			// The incoming session died in the same pass: its notification offer and
			// its own ended offer are two distinct pairs, each spawned once.
			if n := slow.started[incoming]; n != 2 {
				t.Fatalf("incoming session spawned %d times, want 2 (its notification and its death)", n)
			}
		})
	}
}

// A pair the incoming call already claimed is not claimed again by the death
// batch that follows it: concurrent callers never double-spawn, and the
// session's siblings in the same batch are still offered.
func TestDeathBatchDoesNotRespawnAPairTheIncomingCallAlreadyClaimed(t *testing.T) {
	svc, db, _, _ := newAgentTestService(t, nil, "hook-overlap-claimed")
	_, d := withEventHook(&svc, "tmux.session_gone")
	slow := &slowSpawn{hold: 200 * time.Millisecond, outcome: func(string) (notify.Result, error) {
		return notify.Result{ExitCode: 0}, nil
	}}
	d.Spawn = slow.spawn
	svc.EventHook = func() EventHookDispatcher { return d }
	claimed, sibling := batchSessionID(910), batchSessionID(911)
	goneRow(t, db, claimed, "claude", "running")
	goneRow(t, db, sibling, "claude", "running")

	run, outcome := d.Prepare(context.Background(), HookEvent{
		SessionID: claimed, StoredKind: "tmux.session_gone", Reason: "tmux session disappeared", At: time.Now(),
	}, false)
	if run == nil {
		t.Fatalf("pair not claimed: %+v", outcome)
	}
	done := make(chan struct{})
	go func() { defer close(done); run() }()
	if err := svc.ReconcileWithin(context.Background(), 5*time.Second); err != nil {
		t.Fatal(err)
	}
	<-done
	slow.mu.Lock()
	defer slow.mu.Unlock()
	if slow.started[claimed] != 1 || slow.started[sibling] != 1 {
		t.Fatalf("spawns = %v, want the claimed pair once and its sibling once", slow.started)
	}
}
