package tui

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/n-orlov/deck/internal/agent"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/service"
	"github.com/n-orlov/deck/internal/store"
)

// This file is cure-01-03's own review-cure obligation (R161): the
// committed acceptance tests for tasks 007/008
// (profile_switch_detail_test.go's TestDetailPOpensProfilePickerAndReturnsToDetail,
// pin_detail_test.go's TestDetailCLocksConversation) construct a nil
// *store.Store and only capture the profileSwitcher/resumeModer callback's
// own arguments -- they never wire the PRODUCTION service.Service methods
// those callbacks stand in for in real deck, and never read the value
// those methods actually persisted back out of a store. Review found that
// suppressing the durable write inside service.SetPermissionProfile or
// service.PinResume left every one of those committed nodes green.
//
// The two tests below close that gap: they open a real *store.Store
// (store.OpenPath against a temp file, exactly like
// group_shared_state_db_test.go), build a real agent.Registry and
// service.Service around it, and wire THAT service's own
// SetPermissionProfile/ResumeMode methods as the model's
// profileSwitcher/resumeModer -- so `i` -> `P`/`c` -> choose -> submit
// drives the exact function bodies internal/service/profile.go and
// internal/service/pin.go ship in production. After the submitted Cmd
// runs, each test reads the session back out of the SAME store handle
// (never the callback's own return value, and never a prewritten
// constant) and asserts the persisted permission_profile / resume_state /
// resume_pin, and that the model landed back on the detail view.

// newDurableChooserStore opens a fresh temp-file store and seeds one
// eligible claude session (a real permission profile and conversation id,
// so both the P and the c route have something real to act on).
func newDurableChooserStore(t *testing.T) (*store.Store, service.Service) {
	t.Helper()
	home := t.TempDir()
	db, err := store.OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	clock, err := config.NewClock("2025-01-02T03:04:05Z", "")
	if err != nil {
		t.Fatalf("NewClock: %v", err)
	}
	registry := agent.NewRegistry()
	registry.Register(agent.NewClaude())

	ctx := context.Background()
	if _, err := db.CreateSession(ctx, store.CreateSessionInput{
		ID: "s1", Name: "alpha", CWD: t.TempDir(), Agent: "claude",
		CapturedPath: "/bin/true", Status: "running", StatusAt: 1, CreatedAt: 1,
		PermissionProfile: "safe", ConversationID: "conv-1", ResumeState: "auto",
	}); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	svc := service.Service{Store: db, Agents: registry, Clock: clock}
	return db, svc
}

// TestDetailPProfileSwitchWritesRealStore drives i -> P -> choose ->
// submit through service.Service.SetPermissionProfile's own production
// body and reads the persisted permission_profile back from the store
// that call actually wrote to.
func TestDetailPProfileSwitchWritesRealStore(t *testing.T) {
	db, svc := newDurableChooserStore(t)
	ctx := context.Background()

	model := NewWithShellCreatorAttacherKillerResumerAndProfileSwitcher(
		db, config.Settings{AllowYolo: true}, "", nil, nil, nil, nil, nil,
		svc.SetPermissionProfile,
	)
	model.sessions = []store.Session{{ID: "s1", Name: "alpha", Agent: "claude", Status: "running", PermissionProfile: "safe"}}
	model.selected = rowCursor(0)

	got, _ := model.Update(key("i"))
	model = got.(Model)
	if !model.detail {
		t.Fatal("i did not open the detail dialog")
	}

	got, _ = model.Update(key("P"))
	model = got.(Model)
	if !model.profileSwitching {
		t.Fatal("P inside detail did not open the profile-switch picker")
	}

	got, _ = model.Update(key("right"))
	model = got.(Model)

	got, cmd := model.Update(key("enter"))
	model = got.(Model)
	if cmd == nil {
		t.Fatal("enter on the profile-switch picker did not dispatch a command")
	}
	// Running the returned Cmd is what actually invokes
	// service.Service.SetPermissionProfile -- the production write this
	// task exists to observe, not a fake capturing its own arguments.
	msg := cmd()
	got, loadCmd := model.Update(msg)
	model = got.(Model)
	if loadCmd == nil {
		t.Fatal("a successful profile switch did not trigger a reload")
	}
	if model.profileSwitching {
		t.Fatal("the profile-switch picker stayed open after a successful submit")
	}
	if !model.detail {
		t.Fatal("a successful profile switch did not land back on the detail view")
	}

	persisted, err := db.GetSession(ctx, "s1")
	if err != nil {
		t.Fatalf("GetSession after submit: %v", err)
	}
	if persisted.PermissionProfile == "" || persisted.PermissionProfile == "safe" {
		t.Fatalf("store's own persisted permission_profile = %q, want it changed from safe by the durable write", persisted.PermissionProfile)
	}
}

// TestDetailCLockWritesRealStore drives i -> c -> lock -> submit through
// service.Service.ResumeMode (which dispatches "pinned" to
// service.Service.PinResume)'s own production body and reads the
// persisted resume_state/resume_pin back from the store those calls
// actually wrote to.
func TestDetailCLockWritesRealStore(t *testing.T) {
	db, svc := newDurableChooserStore(t)
	ctx := context.Background()

	model := NewWithShellCreatorAttacherKillerResumerProfileSwitcherAndResumeModer(
		db, config.Settings{}, "", nil, nil, nil, nil, nil,
		svc.SetPermissionProfile, svc.ResumeMode,
	)
	model.sessions = []store.Session{{ID: "s1", Name: "alpha", Agent: "claude", Status: "running", ConversationID: "conv-1", ResumeState: "auto"}}
	model.selected = rowCursor(0)

	got, _ := model.Update(key("i"))
	model = got.(Model)
	if !model.detail {
		t.Fatal("i did not open the detail dialog")
	}

	got, _ = model.Update(key("c"))
	model = got.(Model)
	if !model.pinning {
		t.Fatal("c inside detail did not open the lock chooser")
	}

	// auto -> pinned
	got, _ = model.Update(key("right"))
	model = got.(Model)
	if model.pinValue != "pinned" {
		t.Fatalf("candidate value after cycling = %q, want pinned", model.pinValue)
	}

	got, cmd := model.Update(key("enter"))
	model = got.(Model)
	if cmd == nil {
		t.Fatal("enter on the lock chooser did not dispatch a command")
	}
	// Running the returned Cmd is what actually invokes
	// service.Service.ResumeMode -> PinResume -- the production write
	// this task exists to observe, not a prewritten store.Session
	// constant standing in for it.
	msg := cmd()
	got, loadCmd := model.Update(msg)
	model = got.(Model)
	if loadCmd == nil {
		t.Fatal("a successful lock did not trigger a reload")
	}
	if model.pinning {
		t.Fatal("the lock chooser stayed open after a successful submit")
	}
	if !model.detail {
		t.Fatal("a successful lock did not land back on the detail view")
	}

	persisted, err := db.GetSession(ctx, "s1")
	if err != nil {
		t.Fatalf("GetSession after submit: %v", err)
	}
	if persisted.ResumeState != "pinned" {
		t.Fatalf("store's own persisted resume_state = %q, want pinned", persisted.ResumeState)
	}
	if persisted.ResumePin != "conv-1" {
		t.Fatalf("store's own persisted resume_pin = %q, want conv-1 (the session's own conversation id)", persisted.ResumePin)
	}
}
