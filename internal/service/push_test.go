package service

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/agent"
	"github.com/n-orlov/deck/internal/store"
)

// pushPlan is the keystroke plan the fake adapter builds for a name.
func pushPlan(name string) []string { return []string{"\x15", "/rename " + name, "Enter"} }

// pushFakeAdapter is Claude declaring PushRename=sendkeys and an observation
// source, with a scripted observed name and dialog state.
type pushFakeAdapter struct {
	agent.Claude
	observeOK bool
	observe   *string
	dialog    *bool
}

func (a pushFakeAdapter) Capabilities() agent.Caps {
	caps := a.Claude.Capabilities()
	caps.Name = agent.NameCaps{PushRename: agent.NamePushSendKeys}
	if a.observeOK {
		caps.Name.ObserveName = []string{agent.ObserveHook}
	}
	return caps
}

func (a pushFakeAdapter) PushName(in agent.NamePushInput) ([]string, bool) {
	return pushPlan(in.Name), true
}

func (a pushFakeAdapter) ReadName(agent.NameProbeInput) (string, bool, bool) {
	return *a.observe, false, *a.observe != ""
}

func (a pushFakeAdapter) DialogFocused(string) bool { return *a.dialog }

// sendRecorder is the fake tmux send recorder: every keystroke plan sent.
type sendRecorder struct {
	plans [][]string
	err   error
}

func (r *sendRecorder) Screen(context.Context, string) (string, error) { return "screen", nil }

func (r *sendRecorder) SendPlan(_ context.Context, _ string, plan []string) error {
	r.plans = append(r.plans, plan)
	return r.err
}

type pushRig struct {
	svc      Service
	db       *store.Store
	rec      *sendRecorder
	observed *string
	dialog   *bool
}

func newPushRig(t *testing.T, observeOK bool) pushRig {
	t.Helper()
	svc, db, _, _ := newAgentTestService(t, nil, "push")
	rig := pushRig{svc: svc, db: db, rec: &sendRecorder{}, observed: new(string), dialog: new(bool)}
	rig.svc.Agents.Register(pushFakeAdapter{observeOK: observeOK, observe: rig.observed, dialog: rig.dialog})
	rig.svc.NamePane = rig.rec
	return rig
}

func (r pushRig) row(t *testing.T, id, status string) store.Session {
	t.Helper()
	session, err := r.db.CreateSession(context.Background(), store.CreateSessionInput{
		ID: id, Name: "old name", CWD: t.TempDir(), Agent: "claude", CapturedPath: "/bin",
		Status: status, StatusSource: "hook", StatusAt: 1, CreatedAt: 1,
		ConversationID: "conv-" + id, NameSource: "derived",
	})
	if err != nil {
		t.Fatal(err)
	}
	return session
}

func (r pushRig) eventCount(t *testing.T, id, kind string) int {
	t.Helper()
	var n int
	if err := r.db.DB().QueryRow(`SELECT count(*) FROM events WHERE session_id = ? AND kind = ?`, id, kind).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// TestPushGate is R252's gate table: a rename is pushed only on idle or
// running with no focused dialog; waiting, starting and error are refused
// with an event and a note naming the reason; a stopped row sends nothing.
func TestPushGate(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     string
		dialog     bool
		wantSent   bool
		wantEvent  int
		wantReason string
	}{
		{name: "idle sends", status: "idle", wantSent: true},
		{name: "running sends", status: "running", wantSent: true},
		{name: "waiting is refused", status: "waiting", wantEvent: 1, wantReason: "status is waiting"},
		{name: "starting is refused", status: "starting", wantEvent: 1, wantReason: "status is starting"},
		{name: "error is refused", status: "error", wantEvent: 1, wantReason: "status is error"},
		{name: "a focused dialog is refused", status: "idle", dialog: true, wantEvent: 1, wantReason: "dialog is focused"},
		{name: "a stopped row sends nothing", status: "stopped"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rig := newPushRig(t, true)
			row := rig.row(t, "row-a", tc.status)
			*rig.dialog = tc.dialog

			got, result, err := rig.svc.RenameAndPush(context.Background(), row.ID, "new name")
			if err != nil {
				t.Fatal(err)
			}
			if got.Name != "new name" || got.NameSource != "user" || got.Slug != row.Slug {
				t.Fatalf("rename did not stand: %#v", got)
			}
			if sent := len(rig.rec.plans) == 1; sent != tc.wantSent || len(rig.rec.plans) > 1 {
				t.Fatalf("sent plans = %v, want sent=%v", rig.rec.plans, tc.wantSent)
			}
			if tc.wantSent && !reflect.DeepEqual(rig.rec.plans[0], pushPlan("new name")) {
				t.Fatalf("plan = %q", rig.rec.plans[0])
			}
			if n := rig.eventCount(t, row.ID, EventHarnessRenameRefused); n != tc.wantEvent {
				t.Fatalf("refused events = %d, want %d", n, tc.wantEvent)
			}
			if tc.wantReason != "" && !strings.Contains(result.Note, tc.wantReason) {
				t.Fatalf("note %q does not name %q", result.Note, tc.wantReason)
			}
			if result.Sent != tc.wantSent {
				t.Fatalf("result.Sent = %v", result.Sent)
			}
		})
	}
}

// TestPushGateIgnoresAdaptersWithoutSendKeys: a shell row and an adapter whose
// caps do not claim sendkeys never receive a plan and never log a refusal.
func TestPushGateIgnoresAdaptersWithoutSendKeys(t *testing.T) {
	svc, db, _, _ := newAgentTestService(t, nil, "push-none")
	// Claude claims sendkeys now (R256), so stand in for an adapter that does not.
	svc.Agents.Register(adoptFakeAdapter{claims: false, asked: new(int)})
	rec := &sendRecorder{}
	svc.NamePane = rec
	row := adoptRow(t, db, "row-a", "old name", "derived")
	if _, _, err := svc.RenameAndPush(context.Background(), row.ID, "new name"); err != nil {
		t.Fatal(err)
	}
	if len(rec.plans) != 0 {
		t.Fatalf("an adapter with no PushRename claim was sent %v", rec.plans)
	}
}

// TestPushEcho: the pushed name is held in pending_harness_name, its observed
// echo is not adopted (name and name_source unchanged), and the field clears
// on confirmation and on timeout.
func TestPushEcho(t *testing.T) {
	ctx := context.Background()
	t.Run("echo confirms and clears without adopting", func(t *testing.T) {
		rig := newPushRig(t, true)
		row := rig.row(t, "row-a", "idle")
		if _, _, err := rig.svc.RenameAndPush(ctx, row.ID, "pushed name"); err != nil {
			t.Fatal(err)
		}
		pending, _ := rig.db.GetSession(ctx, row.ID)
		if pending.PendingHarnessName != "pushed name" || pending.PendingHarnessNameAt == 0 {
			t.Fatalf("pending = %q at %d", pending.PendingHarnessName, pending.PendingHarnessNameAt)
		}
		*rig.observed = "pushed name"
		if _, err := rig.svc.AdoptName(ctx, row.ID); err != nil {
			t.Fatal(err)
		}
		got, _ := rig.db.GetSession(ctx, row.ID)
		if got.PendingHarnessName != "" || got.Name != "pushed name" || got.NameSource != "user" {
			t.Fatalf("after echo: %#v", got)
		}
		if n := rig.eventCount(t, row.ID, EventHarnessRenameFailed); n != 0 {
			t.Fatalf("a confirmed push emitted %d failed events", n)
		}
		if n := rig.eventCount(t, row.ID, "renamed"); n != 1 {
			t.Fatalf("renamed events = %d, want only the user's own", n)
		}
	})
	t.Run("echo on a harness row does not change name_source", func(t *testing.T) {
		rig := newPushRig(t, true)
		row := rig.row(t, "row-b", "idle")
		if _, err := rig.db.AdoptSessionName(ctx, row.ID, "pushed name", "harness:claude", "harness", 5); err != nil {
			t.Fatal(err)
		}
		if err := rig.db.SetPendingHarnessName(ctx, row.ID, "pushed name", rig.svc.Clock.Now().UnixMilli()); err != nil {
			t.Fatal(err)
		}
		*rig.observed = "pushed name"
		rig.svc.adoptOnTick(ctx, mustGet(t, rig.db, row.ID))
		got := mustGet(t, rig.db, row.ID)
		if got.PendingHarnessName != "" || got.NameSource != "harness" {
			t.Fatalf("echo changed the row: pending=%q source=%q", got.PendingHarnessName, got.NameSource)
		}
	})
	t.Run("timeout clears", func(t *testing.T) {
		rig := newPushRig(t, true)
		row := rig.row(t, "row-c", "idle")
		old := rig.svc.Clock.Now().UnixMilli() - pushConfirmTimeout.Milliseconds() - 1
		if err := rig.db.SetPendingHarnessName(ctx, row.ID, "never echoed", old); err != nil {
			t.Fatal(err)
		}
		rig.svc.adoptOnTick(ctx, mustGet(t, rig.db, row.ID))
		if got := mustGet(t, rig.db, row.ID); got.PendingHarnessName != "" || got.NameSource != "derived" {
			t.Fatalf("after timeout: pending=%q source=%q", got.PendingHarnessName, got.NameSource)
		}
	})
	t.Run("an unconfirmed push inside the timeout stays pending", func(t *testing.T) {
		rig := newPushRig(t, true)
		row := rig.row(t, "row-d", "idle")
		if err := rig.db.SetPendingHarnessName(ctx, row.ID, "waiting", rig.svc.Clock.Now().UnixMilli()); err != nil {
			t.Fatal(err)
		}
		*rig.observed = "something else"
		rig.svc.adoptOnTick(ctx, mustGet(t, rig.db, row.ID))
		if got := mustGet(t, rig.db, row.ID); got.PendingHarnessName != "waiting" || got.NameSource != "derived" || got.Name != "old name" {
			t.Fatalf("pending row changed early: %#v", got)
		}
	})
}

func mustGet(t *testing.T, db *store.Store, id string) store.Session {
	t.Helper()
	session, err := db.GetSession(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return session
}

// TestPushFailureEmitsOneEventAndNeverRetries: a confirmation that never
// arrives yields exactly one harness_rename_failed event across repeated
// ticks, and the fake send recorder sees the one original send only.
func TestPushFailureEmitsOneEventAndNeverRetries(t *testing.T) {
	ctx := context.Background()
	rig := newPushRig(t, true)
	row := rig.row(t, "row-a", "running")
	if _, _, err := rig.svc.RenameAndPush(ctx, row.ID, "lost name"); err != nil {
		t.Fatal(err)
	}
	old := rig.svc.Clock.Now().UnixMilli() - pushConfirmTimeout.Milliseconds() - 1
	if err := rig.db.SetPendingHarnessName(ctx, row.ID, "lost name", old); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		rig.svc.adoptOnTick(ctx, mustGet(t, rig.db, row.ID))
		if _, err := rig.svc.AdoptName(ctx, row.ID); err != nil {
			t.Fatal(err)
		}
	}
	if n := rig.eventCount(t, row.ID, EventHarnessRenameFailed); n != 1 {
		t.Fatalf("harness_rename_failed events = %d, want exactly 1", n)
	}
	if len(rig.rec.plans) != 1 {
		t.Fatalf("send recorder saw %d plans, want the single original send: %v", len(rig.rec.plans), rig.rec.plans)
	}
}

// TestPushSendErrorEmitsOneFailureAndNoRetry: a send that errors is one failed
// event and one attempt, with nothing left pending.
func TestPushSendErrorEmitsOneFailureAndNoRetry(t *testing.T) {
	ctx := context.Background()
	rig := newPushRig(t, true)
	rig.rec.err = errors.New("pane vanished")
	row := rig.row(t, "row-a", "idle")
	_, result, err := rig.svc.RenameAndPush(ctx, row.ID, "new name")
	if err != nil {
		t.Fatal(err)
	}
	rig.svc.adoptOnTick(ctx, mustGet(t, rig.db, row.ID))
	if n := rig.eventCount(t, row.ID, EventHarnessRenameFailed); n != 1 || len(rig.rec.plans) != 1 {
		t.Fatalf("failed events = %d, sends = %d; want 1 and 1", n, len(rig.rec.plans))
	}
	if !strings.Contains(result.Note, "pane vanished") || mustGet(t, rig.db, row.ID).PendingHarnessName != "" {
		t.Fatalf("note %q, pending %q", result.Note, mustGet(t, rig.db, row.ID).PendingHarnessName)
	}
}

// TestPushHoldsNothingWhenTheAdapterCannotObserve: with no observation source
// a confirmation can never arrive, so the plan is sent and nothing is pending.
func TestPushHoldsNothingWhenTheAdapterCannotObserve(t *testing.T) {
	rig := newPushRig(t, false)
	row := rig.row(t, "row-a", "idle")
	if _, _, err := rig.svc.RenameAndPush(context.Background(), row.ID, "new name"); err != nil {
		t.Fatal(err)
	}
	if len(rig.rec.plans) != 1 || mustGet(t, rig.db, row.ID).PendingHarnessName != "" {
		t.Fatalf("plans=%v pending=%q", rig.rec.plans, mustGet(t, rig.db, row.ID).PendingHarnessName)
	}
}
