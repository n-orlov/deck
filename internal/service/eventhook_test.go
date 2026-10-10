package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/notify"
	"github.com/n-orlov/deck/internal/store"
)

type hookDispatchFixture struct {
	db  *store.Store
	d   EventHookDispatcher
	out string
}

func newHookDispatchFixture(t *testing.T, status string) hookDispatchFixture {
	t.Helper()
	home := t.TempDir()
	db, err := store.OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.CreateSession(context.Background(), store.CreateSessionInput{
		ID: "s1", Name: "api", CWD: home, Agent: "claude", CapturedPath: "/bin",
		Status: status, StatusAt: 1, CreatedAt: 1, ConversationID: "c1",
	}); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	script := filepath.Join(out, "hook.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho \"$1\" >> "+filepath.Join(out, "runs")+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return hookDispatchFixture{db: db, out: out, d: EventHookDispatcher{
		Store:   db,
		Policy:  notify.Policy{Command: []string{script}, Default: true, Events: config.EventHookKinds},
		Timeout: 3 * time.Second,
		Deck:    notify.Deck{Host: "box", Version: "test"},
		BaseEnv: []string{"PATH=/usr/bin:/bin"},
	}}
}

func (f hookDispatchFixture) runs() []string {
	raw, _ := os.ReadFile(filepath.Join(f.out, "runs"))
	return strings.Fields(string(raw))
}

func TestEventHookDispatchSpawnsOncePerEpochPair(t *testing.T) {
	f := newHookDispatchFixture(t, "waiting")
	ev := HookEvent{SessionID: "s1", StoredKind: "notification", Reason: "permission_prompt", At: time.Now()}
	first := f.d.Dispatch(context.Background(), ev, false)
	if !first.Spawned || first.Err != nil || first.Result.Failed() {
		t.Fatalf("first dispatch = %+v", first)
	}
	if second := f.d.Dispatch(context.Background(), ev, false); second.Spawned || second.Skip != notify.SkipDeduped {
		t.Fatalf("second dispatch = %+v, want deduped", second)
	}
	ev.Reason = "other_prompt"
	if third := f.d.Dispatch(context.Background(), ev, false); !third.Spawned {
		t.Fatalf("a new reason must spawn: %+v", third)
	}
	if got := f.runs(); len(got) != 2 || got[0] != "waiting" {
		t.Fatalf("runs = %v, want two waiting spawns", got)
	}
}

func TestEventHookDispatchSkipsWhatItShouldNotOffer(t *testing.T) {
	f := newHookDispatchFixture(t, "running")
	ctx := context.Background()
	if out := f.d.Dispatch(ctx, HookEvent{SessionID: "s1", StoredKind: "user_prompt_submitted"}, false); out.Skip != notify.SkipNotOffer {
		t.Errorf("a prompt = %+v", out)
	}
	inert := f.d
	inert.Policy.Command = nil
	if out := inert.Dispatch(ctx, HookEvent{SessionID: "s1", StoredKind: "stop"}, false); out.Skip != notify.SkipInert {
		t.Errorf("no script = %+v", out)
	}
	if out := f.d.Dispatch(ctx, HookEvent{SessionID: "s1", StoredKind: "stop", AppliedStatus: "idle"}, false); out.Skip != notify.SkipNotApplied {
		t.Errorf("a status that did not land = %+v", out)
	}
	if out := f.d.Dispatch(ctx, HookEvent{SessionID: "missing", StoredKind: "stop"}, false); out.Err == nil {
		t.Errorf("a missing session = %+v, want an error", out)
	}
	if got := f.runs(); len(got) != 0 {
		t.Errorf("spawned %v", got)
	}
}

func TestEventHookDispatchReadsThePerSessionFields(t *testing.T) {
	f := newHookDispatchFixture(t, "idle")
	if _, err := f.db.DB().Exec(`UPDATE sessions SET event_hook_enabled = 1, event_hook_events = '["error"]' WHERE id = 's1'`); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if out := f.d.Dispatch(ctx, HookEvent{SessionID: "s1", StoredKind: "stop"}, false); out.Skip != notify.SkipNotListed {
		t.Errorf("idle with an own list of error = %+v", out)
	}
	if _, err := f.db.DB().Exec(`UPDATE sessions SET event_hook_enabled = 0 WHERE id = 's1'`); err != nil {
		t.Fatal(err)
	}
	if out := f.d.Dispatch(ctx, HookEvent{SessionID: "s1", StoredKind: "stop_failure"}, false); out.Skip != notify.SkipDisabled {
		t.Errorf("a session that turned the hook off = %+v", out)
	}
}

func TestEventHookDispatchDetachedStartsWithoutAResultAndReportsAMissingScript(t *testing.T) {
	f := newHookDispatchFixture(t, "stopped")
	out := f.d.Dispatch(context.Background(), HookEvent{SessionID: "s1", StoredKind: "session_end", Reason: "logout"}, true)
	if !out.Spawned || !out.Detached || out.Err != nil || out.Result.Duration != 0 {
		t.Fatalf("detached dispatch = %+v", out)
	}
	deadline := time.Now().Add(10 * time.Second)
	for len(f.runs()) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := f.runs(); len(got) != 1 || got[0] != "ended" {
		t.Fatalf("runs = %v, want one ended", got)
	}
	missing := newHookDispatchFixture(t, "waiting")
	missing.d.Policy.Command = []string{filepath.Join(missing.out, "nope")}
	if out := missing.d.Dispatch(context.Background(), HookEvent{SessionID: "s1", StoredKind: "notification"}, false); out.Err == nil || out.Spawned {
		t.Fatalf("missing script = %+v, want a recordable error", out)
	}
}

func TestEventHookDispatchFromTwoProcessesSpawnsOnce(t *testing.T) {
	f := newHookDispatchFixture(t, "waiting")
	ev := HookEvent{SessionID: "s1", StoredKind: "notification", Reason: "p", At: time.Now()}
	var wg sync.WaitGroup
	spawned := make(chan bool, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			spawned <- f.d.Dispatch(context.Background(), ev, false).Spawned
		}()
	}
	wg.Wait()
	close(spawned)
	count := 0
	for s := range spawned {
		if s {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("%d concurrent dispatches spawned %d times, want 1", count, count)
	}
}

func TestHookLaunchKindFollowsTheLease(t *testing.T) {
	if got := hookLaunchKind(store.Session{}); got != LaunchKindCreate {
		t.Errorf("fresh row = %q", got)
	}
	if got := hookLaunchKind(store.Session{LaunchGeneration: "g1"}); got != LaunchKindResume {
		t.Errorf("leased row = %q", got)
	}
}

// Concurrent offers in one epoch: every distinct (kind, reason) pair spawns
// exactly once and an identical pair spawns for one caller only. A dedupe skip
// is justified only by that pair already being claimed, never by contention
// on the hook_fired column.
func TestEventHookConcurrentDispatchSpawnsEachDistinctPairOnce(t *testing.T) {
	f := newHookDispatchFixture(t, "waiting")
	const reasons, copies = 8, 3
	var mu sync.Mutex
	spawned := map[string]int{}
	var wg sync.WaitGroup
	for r := range reasons {
		for range copies {
			wg.Add(1)
			go func() {
				defer wg.Done()
				reason := fmt.Sprintf("prompt_%d", r)
				out := f.d.Dispatch(context.Background(), HookEvent{SessionID: "s1", StoredKind: "notification", Reason: reason, At: time.Now()}, false)
				mu.Lock()
				defer mu.Unlock()
				if out.Spawned {
					spawned[reason]++
				} else if out.Skip != notify.SkipDeduped {
					t.Errorf("%s: unexpected outcome %+v", reason, out)
				}
			}()
		}
	}
	wg.Wait()
	if len(spawned) != reasons {
		t.Fatalf("spawned reasons = %v, want all %d distinct reasons", spawned, reasons)
	}
	for reason, n := range spawned {
		if n != 1 {
			t.Errorf("reason %s spawned %d times, want 1", reason, n)
		}
	}
	if got := f.runs(); len(got) != reasons {
		t.Fatalf("runs = %v, want %d", got, reasons)
	}
}

func TestEventHookConcurrentDispatchAcrossKindsAndAfterTheFact(t *testing.T) {
	f := newHookDispatchFixture(t, "waiting")
	evs := []HookEvent{
		{SessionID: "s1", StoredKind: "notification", Reason: "permission_prompt"},
		{SessionID: "s1", StoredKind: "notification", Reason: "elicitation"},
		{SessionID: "s1", StoredKind: "stop", Reason: "done"},
		{SessionID: "s1", StoredKind: "tmux.pane_dead", Reason: "crashed"},
		{SessionID: "s1", StoredKind: "tmux.pane_dead", Reason: "oom"},
	}
	outs := make([]HookOutcome, len(evs))
	var wg sync.WaitGroup
	for i, ev := range evs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ev.At = time.Now()
			outs[i] = f.d.Dispatch(context.Background(), ev, false)
		}()
	}
	wg.Wait()
	for i, out := range outs {
		if !out.Spawned {
			t.Errorf("event %d (%s/%s) was suppressed: %+v", i, evs[i].StoredKind, evs[i].Reason, out)
		}
	}
	for _, ev := range evs {
		ev.At = time.Now()
		if again := f.d.Dispatch(context.Background(), ev, false); again.Spawned || again.Skip != notify.SkipDeduped {
			t.Errorf("repeat of %s/%s = %+v, want deduped", ev.StoredKind, ev.Reason, again)
		}
	}
}

// R241 (SPEC §10.1, §10.3): an event whose reason and message carry a NUL is
// delivered, not refused by exec, and its (kind, reason) pair is claimed once:
// a repeat of the same event is deduped and the script runs a single time.
func TestEventHookDispatchClaimsANULCarryingEventOnceAndDeliversIt(t *testing.T) {
	f := newHookDispatchFixture(t, "waiting")
	ev := HookEvent{SessionID: "s1", StoredKind: "notification", Reason: "permission\x00_prompt", Message: "needs\x00approval", At: time.Now()}
	first := f.d.Dispatch(context.Background(), ev, false)
	if !first.Spawned || first.Err != nil || first.Result.Failed() {
		t.Fatalf("first dispatch = %+v, want the script run", first)
	}
	if second := f.d.Dispatch(context.Background(), ev, false); second.Spawned || second.Skip != notify.SkipDeduped {
		t.Fatalf("second dispatch = %+v, want deduped", second)
	}
	if got := f.runs(); len(got) != 1 || got[0] != "waiting" {
		t.Fatalf("runs = %v, want exactly one waiting spawn", got)
	}
	state, err := f.db.EventHookState(context.Background(), "s1")
	if err != nil {
		t.Fatal(err)
	}
	if fired := notify.DecodeFired(state.Fired); len(fired) != 1 || fired[0].Kind != "waiting" {
		t.Fatalf("hook_fired = %+v, want the one claimed pair", fired)
	}
}
