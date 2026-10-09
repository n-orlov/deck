package service

import (
	"context"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/notify"
	"github.com/n-orlov/deck/internal/store"
)

// goneRow stores a session whose tmux session never existed, so a reconcile
// pass sees a clean disappearance (SPEC §7: any -> stopped).
func goneRow(t *testing.T, db *store.Store, id, agent, status string) store.Session {
	t.Helper()
	session, err := db.CreateSession(context.Background(), store.CreateSessionInput{
		ID: id, Name: "gone-" + id[len(id)-2:], CWD: t.TempDir(), Agent: agent, CapturedPath: "/bin",
		Status: status, StatusSource: "hook", StatusAt: 1, CreatedAt: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	return session
}

// A tmux session that disappeared cleanly is a process death: reconcile
// records `stopped`, and the event hook is offered `ended` after the
// tmux.session_gone row is durable (SPEC §10.4).
func TestReconcileDetectedCleanDisappearanceDispatchesEnded(t *testing.T) {
	for _, agent := range []string{"claude", "shell"} {
		t.Run(agent, func(t *testing.T) {
			svc, db, _, _ := newAgentTestService(t, nil, "hook-gone-"+agent)
			log, _ := withEventHook(&svc, "tmux.session_gone")
			session := goneRow(t, db, "00000000-0000-4000-8000-0000000000a1", agent, "running")

			if err := svc.Reconcile(context.Background()); err != nil {
				t.Fatal(err)
			}
			if got := log.kinds; len(got) != 1 || got[0] != "ended" {
				t.Fatalf("spawned kinds = %v, want one ended", got)
			}
			if !log.rowAtSpawn[0] {
				t.Fatal("the tmux.session_gone event row did not exist when the hook was spawned")
			}
			if log.reasons[0] != "tmux session disappeared" {
				t.Fatalf("reason = %q", log.reasons[0])
			}
			// The stored kind is not renamed, and a repeat pass offers nothing.
			_ = eventSeqOf(t, db, session.ID, "tmux.session_gone")
			if err := svc.Reconcile(context.Background()); err != nil {
				t.Fatal(err)
			}
			if log.count() != 1 {
				t.Fatalf("a repeat pass spawned again: %v", log.kinds)
			}
		})
	}
}

// The script's result is recorded against the tmux.session_gone event, under
// the same rule as every other attached spawn.
func TestCleanDisappearanceResultIsStoredAgainstTheEvent(t *testing.T) {
	svc, db, _, _ := newAgentTestService(t, nil, "hook-gone-result")
	log, _ := withEventHook(&svc, "tmux.session_gone")
	log.result = notify.Result{ExitCode: 4, Output: "bye\n"}
	session := goneRow(t, db, "00000000-0000-4000-8000-0000000000a2", "claude", "idle")

	if err := svc.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, ok, err := db.EventHookResultOf(context.Background(), eventSeqOf(t, db, session.ID, "tmux.session_gone"))
	if err != nil || !ok {
		t.Fatalf("no stored result: ok=%v err=%v", ok, err)
	}
	if got.Kind != "ended" || got.ExitCode != 4 || got.Output != "bye\n" {
		t.Fatalf("stored result = %+v, want ended / exit 4 / bye", got)
	}
}

// Rows that already claim the process is gone, and sessions whose policy
// excludes `ended`, spawn nothing: the same enable/list/dedupe policy.
func TestCleanDisappearanceHonoursThePolicyAndTerminalRows(t *testing.T) {
	t.Run("already stopped row", func(t *testing.T) {
		svc, db, _, _ := newAgentTestService(t, nil, "hook-gone-stopped")
		log, _ := withEventHook(&svc, "tmux.session_gone")
		goneRow(t, db, "00000000-0000-4000-8000-0000000000a3", "claude", "stopped")
		if err := svc.Reconcile(context.Background()); err != nil {
			t.Fatal(err)
		}
		if log.count() != 0 {
			t.Fatalf("a stopped row spawned %v", log.kinds)
		}
	})
	t.Run("ended not in the offered list", func(t *testing.T) {
		svc, db, _, _ := newAgentTestService(t, nil, "hook-gone-list")
		log, d := withEventHook(&svc, "tmux.session_gone")
		d.Policy.Events = []string{"error", "waiting"}
		svc.EventHook = func() EventHookDispatcher { return d }
		session := goneRow(t, db, "00000000-0000-4000-8000-0000000000a4", "claude", "running")
		if err := svc.Reconcile(context.Background()); err != nil {
			t.Fatal(err)
		}
		if log.count() != 0 {
			t.Fatalf("an unlisted ended spawned %v", log.kinds)
		}
		if got, err := db.GetSession(context.Background(), session.ID); err != nil || got.Status != "stopped" {
			t.Fatalf("session after pass = %+v, %v: the stop must still be recorded", got, err)
		}
	})
	t.Run("hook disabled for the session", func(t *testing.T) {
		svc, db, _, _ := newAgentTestService(t, nil, "hook-gone-off")
		log, _ := withEventHook(&svc, "tmux.session_gone")
		session := goneRow(t, db, "00000000-0000-4000-8000-0000000000a5", "claude", "running")
		off := false
		if err := db.SetEventHook(context.Background(), session.ID, &off, nil, "user", 2); err != nil {
			t.Fatal(err)
		}
		if err := svc.Reconcile(context.Background()); err != nil {
			t.Fatal(err)
		}
		if log.count() != 0 {
			t.Fatalf("a session with the hook off spawned %v", log.kinds)
		}
	})
}

// A budgeted pass offers the disappearance after it returns, like a crash.
func TestReconcileWithinDispatchesCleanDisappearanceAfterThePassReturns(t *testing.T) {
	svc, db, _, _ := newAgentTestService(t, nil, "hook-gone-budget")
	log, _ := withEventHook(&svc, "tmux.session_gone")
	d := svc.EventHook()
	inner := log.spawn
	var underDeadline bool
	d.Spawn = func(ctx context.Context, req notify.Request) (notify.Result, error) {
		_, underDeadline = ctx.Deadline()
		return inner(ctx, req)
	}
	svc.EventHook = func() EventHookDispatcher { return d }
	goneRow(t, db, "00000000-0000-4000-8000-0000000000a6", "claude", "running")

	if err := svc.ReconcileWithin(context.Background(), 5*time.Second); err != nil {
		t.Fatal(err)
	}
	if got := log.kinds; len(got) != 1 || got[0] != "ended" {
		t.Fatalf("spawned kinds = %v, want one ended", got)
	}
	if underDeadline {
		t.Fatal("the spawn ran under the pass's budget deadline")
	}
}
