package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/notify"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// spawnLog is the Spawn seam of a dispatcher under test: it records every
// request and, at the instant of the spawn, whether the event row the spawn is
// for was already committed (SPEC §10.3: the event is written first).
type spawnLog struct {
	mu          sync.Mutex
	db          *store.Store
	storedKind  string
	kinds       []string
	rowAtSpawn  []bool
	reasons     []string
	messages    []string
	result      notify.Result
	spawnReturn error
}

func (l *spawnLog) spawn(ctx context.Context, req notify.Request) (notify.Result, error) {
	var rows int
	_ = l.db.DB().QueryRowContext(ctx, `SELECT count(*) FROM events WHERE session_id = ? AND kind = ?`,
		req.Session.ID, l.storedKind).Scan(&rows)
	l.mu.Lock()
	defer l.mu.Unlock()
	l.kinds = append(l.kinds, req.Event.Kind)
	l.reasons = append(l.reasons, req.Event.Reason)
	l.messages = append(l.messages, req.Event.Message)
	l.rowAtSpawn = append(l.rowAtSpawn, rows > 0)
	return l.result, l.spawnReturn
}

func (l *spawnLog) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.kinds)
}

// withEventHook wires svc's EventHook source to a dispatcher whose spawns go to
// the returned log. storedKind is the events.kind the test expects to exist
// by the time a spawn happens.
func withEventHook(svc *Service, storedKind string) (*spawnLog, EventHookDispatcher) {
	log := &spawnLog{db: svc.Store, storedKind: storedKind}
	d := EventHookDispatcher{
		Store:   svc.Store,
		Policy:  notify.Policy{Command: []string{"/bin/true"}, Default: true, Events: config.EventHookKinds},
		Timeout: time.Second,
		Deck:    notify.Deck{Host: "box", Version: "test"},
		BaseEnv: []string{"PATH=/usr/bin:/bin"},
		Spawn:   log.spawn,
	}
	svc.EventHook = func() EventHookDispatcher { return d }
	return log, d
}

func eventSeqOf(t *testing.T, db *store.Store, sessionID, kind string) int64 {
	t.Helper()
	var seq int64
	if err := db.DB().QueryRow(`SELECT seq FROM events WHERE session_id = ? AND kind = ? ORDER BY seq DESC LIMIT 1`,
		sessionID, kind).Scan(&seq); err != nil {
		t.Fatalf("no %s event for %s: %v", kind, sessionID, err)
	}
	return seq
}

func probeWaitingFixture(t *testing.T, name string) (Service, *store.Store, store.Session, time.Duration) {
	t.Helper()
	cwd := t.TempDir()
	svc, db, _, _ := newAgentTestService(t, nil, name)
	clock, err := config.NewClock("2025-01-02T03:04:05Z", "45s")
	if err != nil {
		t.Fatal(err)
	}
	svc.Clock = clock
	now := clock.Now().UnixMilli()
	session, err := db.CreateSession(context.Background(), store.CreateSessionInput{
		ID: "00000000-0000-4000-8000-0000000000e1", Name: "sampled claude", CWD: cwd,
		Agent: "claude", CapturedPath: "/bin", Status: "running", StatusSource: "hook", StatusAt: now, CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := os.ReadFile(filepath.Join("..", "agent", "testdata", "probes", "claude", "waiting.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.TMux.Create(context.Background(), tmux.Launch{
		Slug: session.Slug, CWD: cwd,
		Command: []string{"/bin/sh", "-c", `printf '%s' "$1"; sleep 30`, "probe-fixture", string(fixture)},
	}); err != nil {
		t.Fatal(err)
	}
	const staleAfter = 45 * time.Second
	clock.Advance()
	return svc, db, session, staleAfter
}

// A probe-classified status change dispatches the event hook (SPEC §10.4).
func TestProbeStatusChangeDispatchesTheEventHook(t *testing.T) {
	svc, db, session, staleAfter := probeWaitingFixture(t, "hook-probe")
	log, _ := withEventHook(&svc, "probe.waiting")

	if err := svc.ReconcileWithProbes(context.Background(), staleAfter); err != nil {
		t.Fatal(err)
	}
	if got := log.kinds; len(got) != 1 || got[0] != "waiting" {
		t.Fatalf("spawned kinds = %v, want one waiting", got)
	}
	if !log.rowAtSpawn[0] {
		t.Fatal("the probe.waiting event row did not exist when the hook was spawned")
	}
	if log.reasons[0] != "permission prompt" {
		t.Fatalf("event reason = %q, want the probe's reason", log.reasons[0])
	}
	seq := eventSeqOf(t, db, session.ID, "probe.waiting")
	// The same verdict on the next pass is the same attention episode: no
	// second event, no second spawn.
	svc.Clock.Advance()
	if err := svc.ReconcileWithProbes(context.Background(), staleAfter); err != nil {
		t.Fatal(err)
	}
	if log.count() != 1 {
		t.Fatalf("a repeated verdict spawned again: %v", log.kinds)
	}
	if _, ok, err := db.EventHookResultOf(context.Background(), seq); err != nil || !ok {
		t.Fatalf("result stored against the probe event = %v, %v", ok, err)
	}
}

// A probe that repeats the status a hook already put on the row is not a new
// event: the hook path owned the ping.
func TestProbeThatRepeatsTheRowsStatusDoesNotDispatch(t *testing.T) {
	svc, db, session, staleAfter := probeWaitingFixture(t, "hook-probe-same")
	if err := db.UpdateSessionStatus(context.Background(), store.StatusUpdateInput{
		SessionID: session.ID, Status: "waiting", Reason: "permission_prompt", Source: "hook",
		At: svc.Clock.Now().UnixMilli(), EventKind: "notification",
	}); err != nil {
		t.Fatal(err)
	}
	svc.Clock.Advance()
	log, _ := withEventHook(&svc, "probe.waiting")
	if err := svc.ReconcileWithProbes(context.Background(), staleAfter); err != nil {
		t.Fatal(err)
	}
	if log.count() != 0 {
		t.Fatalf("a probe repeating the hook's waiting spawned %v", log.kinds)
	}
}

// A reconcile-detected process death dispatches the event hook as `error`.
func TestReconcileDetectedProcessDeathDispatchesTheEventHook(t *testing.T) {
	svc, db, _, _ := newAgentTestService(t, nil, "hook-death")
	cwd := t.TempDir()
	log, _ := withEventHook(&svc, "tmux.pane_dead")
	session, err := db.CreateSession(context.Background(), store.CreateSessionInput{
		ID: "00000000-0000-4000-8000-0000000000e2", Name: "crasher", CWD: cwd,
		Agent: "claude", CapturedPath: "/bin", Status: "running", StatusSource: "hook", StatusAt: 1, CreatedAt: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.TMux.Create(context.Background(), tmux.Launch{Slug: session.Slug, CWD: cwd,
		Command: []string{"/bin/sh", "-c", `echo last words; exit 7`}}); err != nil {
		t.Fatal(err)
	}
	waitForDeadPane(t, svc.TMux, session.Slug, 7)

	if err := svc.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := log.kinds; len(got) != 1 || got[0] != "error" {
		t.Fatalf("spawned kinds = %v, want one error", got)
	}
	if !log.rowAtSpawn[0] {
		t.Fatal("the tmux.pane_dead event row did not exist when the hook was spawned")
	}
	if log.reasons[0] != "tmux pane exited with status 7" || !strings.Contains(log.messages[0], "last words") {
		t.Fatalf("reason %q, message %q: want the exit status and the crash tail", log.reasons[0], log.messages[0])
	}
	// A later pass sees the already-recorded death and offers nothing new.
	if err := svc.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if log.count() != 1 {
		t.Fatalf("a repeat pass spawned again: %v", log.kinds)
	}
}

func killFixture(t *testing.T, name string) (Service, store.Session) {
	t.Helper()
	svc, _, _, _ := newAgentTestService(t, nil, name)
	svc.Shell = "/bin/sh"
	session, err := svc.CreateShell(context.Background(), ShellCreateInput{Name: "to kill", CWD: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	return svc, session
}

// The user's own kill dispatches the event hook as `killed`.
func TestUserKillDispatchesTheEventHook(t *testing.T) {
	svc, session := killFixture(t, "hook-kill")
	log, _ := withEventHook(&svc, "killed")
	if err := svc.Kill(context.Background(), session); err != nil {
		t.Fatal(err)
	}
	if got := log.kinds; len(got) != 1 || got[0] != "killed" {
		t.Fatalf("spawned kinds = %v, want one killed", got)
	}
	if !log.rowAtSpawn[0] {
		t.Fatal("the killed event row did not exist when the hook was spawned")
	}
}

// A script that fails never turns a recorded kill into a failed kill.
func TestAFailingEventHookNeverFailsTheKill(t *testing.T) {
	svc, session := killFixture(t, "hook-kill-fail")
	log, _ := withEventHook(&svc, "killed")
	log.result = notify.Result{ExitCode: 9, Output: "boom"}
	if err := svc.Kill(context.Background(), session); err != nil {
		t.Fatalf("kill failed because the hook did: %v", err)
	}
	got, err := svc.Store.GetSession(context.Background(), session.ID)
	if err != nil || got.Status != "stopped" {
		t.Fatalf("row after kill = %+v, %v", got, err)
	}
}

// An event reaching the dispatcher through both the hook path (cmd/deck's
// dispatchHookEvent, which calls Dispatch with the writer's HookEvent) and the
// service path spawns once: the hook_fired claim is the guard (SPEC §10.4).
func TestOneEventThroughHookAndServicePathsSpawnsOnce(t *testing.T) {
	for _, hookFirst := range []bool{true, false} {
		name := "service-first"
		if hookFirst {
			name = "hook-first"
		}
		t.Run(name, func(t *testing.T) {
			svc, session := killFixture(t, "hook-both-"+name)
			log, d := withEventHook(&svc, "killed")
			hookPath := func() HookOutcome {
				return d.Dispatch(context.Background(), HookEvent{SessionID: session.ID, StoredKind: "killed",
					Reason: "killed by user", At: time.Now()}, false)
			}
			var late HookOutcome
			if hookFirst {
				if first := hookPath(); !first.Spawned {
					t.Fatalf("first dispatch = %+v", first)
				}
			}
			if err := svc.Kill(context.Background(), session); err != nil {
				t.Fatal(err)
			}
			if !hookFirst {
				late = hookPath()
				if late.Spawned || late.Skip != notify.SkipDeduped {
					t.Fatalf("hook path after the service path = %+v, want deduped", late)
				}
			}
			if log.count() != 1 {
				t.Fatalf("one event spawned %d times: %v", log.count(), log.kinds)
			}
		})
	}
}

// The exit status and a capped output tail are stored against the event row,
// for exit 0, a non-zero exit and a timeout, and read back from it.
func TestEventHookResultIsStoredAgainstTheEvent(t *testing.T) {
	cases := []struct {
		name   string
		script string
		want   store.EventHookResult
	}{
		{"exit zero", "echo fine; exit 0", store.EventHookResult{Kind: "waiting", ExitCode: 0, Output: "fine\n"}},
		{"non-zero exit", "echo bad >&2; exit 3", store.EventHookResult{Kind: "waiting", ExitCode: 3, Output: "bad\n"}},
		{"timeout", "echo started; sleep 30", store.EventHookResult{Kind: "waiting", ExitCode: -1, TimedOut: true, Output: "started\n"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newHookDispatchFixture(t, "waiting")
			script := filepath.Join(f.out, "hook.sh")
			if err := os.WriteFile(script, []byte("#!/bin/sh\n"+tc.script+"\n"), 0o755); err != nil {
				t.Fatal(err)
			}
			f.d.Timeout = time.Second
			seq := eventSeqOf2(t, f, "notification")
			out := f.d.Dispatch(context.Background(), HookEvent{SessionID: "s1", StoredKind: "notification",
				Reason: "permission_prompt", At: time.Now(), EventSeq: seq}, false)
			if out.Err != nil || !out.Spawned {
				t.Fatalf("dispatch = %+v", out)
			}
			got, ok, err := f.db.EventHookResultOf(context.Background(), seq)
			if err != nil || !ok {
				t.Fatalf("no stored result: ok=%v err=%v", ok, err)
			}
			if got != tc.want {
				t.Fatalf("stored result = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// eventSeqOf2 writes a notification event for s1 and returns its seq, the
// way the hook path does before it dispatches.
func eventSeqOf2(t *testing.T, f hookDispatchFixture, kind string) int64 {
	t.Helper()
	var seq int64
	if err := f.db.UpdateSessionStatus(context.Background(), store.StatusUpdateInput{
		SessionID: "s1", Status: "waiting", Reason: "permission_prompt", Source: "hook", At: 2,
		EventKind: kind, EventSeq: &seq,
	}); err != nil {
		t.Fatal(err)
	}
	if seq == 0 {
		t.Fatal("UpdateSessionStatus reported no event seq")
	}
	return seq
}

// The stored tail is capped, and a script that cannot start records why.
func TestEventHookResultTailIsCappedAndNotStartedIsRecorded(t *testing.T) {
	f := newHookDispatchFixture(t, "waiting")
	script := filepath.Join(f.out, "hook.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nyes x | head -c 200000\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	seq := eventSeqOf2(t, f, "notification")
	ev := HookEvent{SessionID: "s1", StoredKind: "notification", Reason: "p", At: time.Now(), EventSeq: seq}
	if out := f.d.Dispatch(context.Background(), ev, false); out.Err != nil {
		t.Fatalf("dispatch = %+v", out)
	}
	got, ok, err := f.db.EventHookResultOf(context.Background(), seq)
	if err != nil || !ok || len(got.Output) == 0 || len(got.Output) >= 200000 {
		t.Fatalf("stored tail = %d bytes, ok=%v, err=%v; want a capped non-empty tail", len(got.Output), ok, err)
	}

	missing := newHookDispatchFixture(t, "waiting")
	missing.d.Policy.Command = []string{filepath.Join(missing.out, "nope")}
	seq = eventSeqOf2(t, missing, "notification")
	ev.EventSeq = seq
	if out := missing.d.Dispatch(context.Background(), ev, false); out.Err == nil {
		t.Fatalf("missing script dispatch = %+v, want an error", out)
	}
	got, ok, err = missing.db.EventHookResultOf(context.Background(), seq)
	if err != nil || !ok || got.Error == "" || got.ExitCode != -1 {
		t.Fatalf("not-started record = %+v, ok=%v, err=%v", got, ok, err)
	}
}

// A detached spawn (session end) records nothing: it is never waited for.
func TestDetachedDispatchRecordsNoResult(t *testing.T) {
	f := newHookDispatchFixture(t, "stopped")
	seq := eventSeqOf2(t, f, "session_end")
	out := f.d.Dispatch(context.Background(), HookEvent{SessionID: "s1", StoredKind: "session_end",
		Reason: "logout", At: time.Now(), EventSeq: seq}, true)
	if !out.Spawned {
		t.Fatalf("detached dispatch = %+v", out)
	}
	if _, ok, err := f.db.EventHookResultOf(context.Background(), seq); err != nil || ok {
		t.Fatalf("a detached spawn stored a result: ok=%v err=%v", ok, err)
	}
}

func TestOfferHookIsInertWithoutASource(t *testing.T) {
	svc, session := killFixture(t, "hook-inert")
	if err := svc.Kill(context.Background(), session); err != nil {
		t.Fatalf("kill with no event hook source: %v", err)
	}
}

func TestLiveEventHookFollowsTheSettings(t *testing.T) {
	settings := config.Settings{EventHook: "/opt/hook --flag", EventHookDefault: true,
		EventHookEvents: []string{"error"}, EventHookTimeout: 5 * time.Second}
	live := NewLiveEventHook(settings, notify.Deck{Host: "h", Version: "v"}, []string{"A=1"})
	d := live.Dispatcher()
	if len(d.Policy.Command) != 2 || d.Policy.Command[0] != "/opt/hook" || !d.Policy.Default || d.Timeout != 5*time.Second || d.Deck.Host != "h" {
		t.Fatalf("dispatcher = %+v", d)
	}
	settings.EventHook = ""
	settings.EventHookTimeout = 2 * time.Second
	live.Set(settings)
	if d := live.Dispatcher(); len(d.Policy.Command) != 0 || d.Timeout != 2*time.Second {
		t.Fatalf("after Set = %+v", d)
	}
}

// A budgeted pass (ReconcileWithin, `deck _hook`'s liveness pass) offers the
// deaths it records only after it returns (SPEC §10.4): a script slower than
// the budget neither fails the pass nor keeps it from collecting a second
// crash, and each death still spawns once, in order.
func TestReconcileWithinDispatchesDeathsAfterThePassReturns(t *testing.T) {
	svc, db, _, _ := newAgentTestService(t, nil, "hook-budget")
	log, _ := withEventHook(&svc, "tmux.pane_dead")
	budget := 400 * time.Millisecond
	var inPass []bool
	slow := log.spawn
	d := svc.EventHook()
	d.Spawn = func(ctx context.Context, req notify.Request) (notify.Result, error) {
		_, hasDeadline := ctx.Deadline()
		inPass = append(inPass, hasDeadline)
		time.Sleep(budget + 100*time.Millisecond)
		return slow(ctx, req)
	}
	svc.EventHook = func() EventHookDispatcher { return d }
	var slugs []string
	for i, id := range []string{"00000000-0000-4000-8000-0000000000f1", "00000000-0000-4000-8000-0000000000f2"} {
		cwd := t.TempDir()
		session, err := db.CreateSession(context.Background(), store.CreateSessionInput{
			ID: id, Name: "crasher-" + string(rune('a'+i)), CWD: cwd, Agent: "claude", CapturedPath: "/bin",
			Status: "running", StatusSource: "hook", StatusAt: 1, CreatedAt: int64(1 + i),
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.TMux.Create(context.Background(), tmux.Launch{Slug: session.Slug, CWD: cwd,
			Command: []string{"/bin/sh", "-c", `exit 5`}}); err != nil {
			t.Fatal(err)
		}
		waitForDeadPane(t, svc.TMux, session.Slug, 5)
		slugs = append(slugs, session.Slug)
	}

	if err := svc.ReconcileWithin(context.Background(), budget); err != nil {
		t.Fatalf("a slow event hook failed the budgeted pass: %v", err)
	}
	if got := log.kinds; len(got) != 2 || got[0] != "error" || got[1] != "error" {
		t.Fatalf("spawned kinds = %v, want one error per death", got)
	}
	for i, deadline := range inPass {
		if deadline {
			t.Fatalf("spawn %d ran under the pass's budget deadline", i)
		}
	}
	for _, slug := range slugs {
		live, err := svc.TMux.List(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range live {
			if s.Name == "deck_"+slug {
				t.Fatalf("crashed session %s was not collected by the pass", slug)
			}
		}
	}
}
