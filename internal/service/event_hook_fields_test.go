package service

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

// R233a: the session's own event-hook controls are stored by the create calls
// and by SetEventHook, and SetEventHook sets neither dirty flag.
func TestCreateShellAndAgentStoreTheEventHookControls(t *testing.T) {
	svc := newArchiveTestService(t)
	on := true
	created, err := svc.CreateShell(context.Background(), ShellCreateInput{
		Name: "hooked", CWD: t.TempDir(), EventHookEnabled: &on, EventHookEvents: []string{"idle", "ended"},
	})
	if err != nil {
		t.Fatalf("create shell: %v", err)
	}
	row, err := svc.Store.GetSession(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if row.EventHookEnabled == nil || !*row.EventHookEnabled || !reflect.DeepEqual(row.EventHookEvents, []string{"idle", "ended"}) {
		t.Fatalf("stored %v / %#v, want on / [idle ended]", row.EventHookEnabled, row.EventHookEvents)
	}

	plain, err := svc.CreateShell(context.Background(), ShellCreateInput{Name: "plain", CWD: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	row, _ = svc.Store.GetSession(context.Background(), plain.ID)
	if row.EventHookEnabled != nil || row.EventHookEvents != nil {
		t.Fatalf("a create without the fields stored %v / %#v, want inherit", row.EventHookEnabled, row.EventHookEvents)
	}
}

func TestSetEventHookAppliesAtOnceAndSetsNoDirtyFlag(t *testing.T) {
	svc := newArchiveTestService(t)
	created, err := svc.CreateShell(context.Background(), ShellCreateInput{Name: "set-hook", CWD: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	off := false
	updated, err := svc.SetEventHook(context.Background(), created.ID, &off, []string{})
	if err != nil {
		t.Fatalf("set event hook: %v", err)
	}
	if updated.EventHookEnabled == nil || *updated.EventHookEnabled || updated.EventHookEvents == nil || len(updated.EventHookEvents) != 0 {
		t.Fatalf("returned %v / %#v, want off / empty list", updated.EventHookEnabled, updated.EventHookEvents)
	}
	if updated.EnvDirty || updated.LaunchDirty {
		t.Fatalf("set env_dirty=%v launch_dirty=%v, want neither", updated.EnvDirty, updated.LaunchDirty)
	}
	// The dispatcher's own read sees it too.
	state, err := svc.Store.EventHookState(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.Enabled == nil || *state.Enabled || state.Events == nil || len(state.Events) != 0 {
		t.Fatalf("EventHookState = %v / %#v, want off / empty list", state.Enabled, state.Events)
	}
}

func TestSetEventHookRefusals(t *testing.T) {
	svc := newArchiveTestService(t)
	created, err := svc.CreateShell(context.Background(), ShellCreateInput{Name: "refuse-hook", CWD: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetEventHook(context.Background(), created.ID, nil, []string{"loud"}); err == nil || !strings.Contains(err.Error(), "loud") {
		t.Fatalf("unoffered kind: err = %v, want a refusal naming it", err)
	}
	if _, err := svc.SetEventHook(context.Background(), "", nil, nil); err == nil {
		t.Fatal("empty id accepted")
	}
	if _, err := svc.SetEventHook(context.Background(), "no-such-id", nil, nil); err == nil {
		t.Fatal("unknown id accepted")
	}
	if _, err := (Service{}).SetEventHook(context.Background(), created.ID, nil, nil); err == nil {
		t.Fatal("a service without a store accepted the edit")
	}
}
