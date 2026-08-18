package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// TestProfileSwitchPersistsAndStatesRestartToApply proves `P` changes the
// permission profile of an existing session, persists it through the wired
// profileSwitch function, and that the UI states the change applies on the
// next launch/restart rather than claiming the live pane changed mode
// (task 020, SPEC §5/§8).
func TestProfileSwitchPersistsAndStatesRestartToApply(t *testing.T) {
	var persistedID, persistedProfile string
	updated := store.Session{ID: "s1", Name: "alpha", Agent: "claude", Status: "running", PermissionProfile: "edits"}
	model := NewWithShellCreatorAttacherKillerResumerAndProfileSwitcher(
		nil, config.Settings{}, "", nil, nil, nil, nil, nil,
		func(ctx context.Context, id, profile string) (store.Session, error) {
			persistedID, persistedProfile = id, profile
			return updated, nil
		},
	)
	model.sessions = []store.Session{{ID: "s1", Name: "alpha", Agent: "claude", Status: "running", PermissionProfile: "safe"}}
	model.selected = 0

	got, _ := model.Update(key("P"))
	model = got.(Model)
	if !model.profileSwitching {
		t.Fatal("P did not open the profile-switch dialog")
	}
	view := model.View()
	if !strings.Contains(view, "next launch/restart") {
		t.Fatalf("profile-switch dialog did not state restart-to-apply wording:\n%s", view)
	}

	// Cycle right at least once so the candidate value actually differs
	// from the session's current persisted profile ("safe").
	got, _ = model.Update(key("right"))
	model = got.(Model)

	got, cmd := model.Update(key("enter"))
	model = got.(Model)
	if cmd == nil {
		t.Fatal("enter on the profile-switch dialog did not dispatch a command")
	}
	msg := cmd()
	got, loadCmd := model.Update(msg)
	model = got.(Model)
	if loadCmd == nil {
		t.Fatal("a successful profile switch did not trigger a reload")
	}
	if model.profileSwitching {
		t.Fatal("profile-switch dialog remained open after a successful switch")
	}
	if persistedID != "s1" {
		t.Fatalf("profileSwitch called with unexpected id %q", persistedID)
	}
	if persistedProfile == "" || persistedProfile == "safe" {
		t.Fatalf("profileSwitch persisted the unchanged value %q", persistedProfile)
	}
}

// TestProfileSwitchEscCancelsWithoutPersisting proves Esc closes the dialog
// without calling profileSwitch at all.
func TestProfileSwitchEscCancelsWithoutPersisting(t *testing.T) {
	called := false
	model := NewWithShellCreatorAttacherKillerResumerAndProfileSwitcher(
		nil, config.Settings{}, "", nil, nil, nil, nil, nil,
		func(ctx context.Context, id, profile string) (store.Session, error) {
			called = true
			return store.Session{}, nil
		},
	)
	model.sessions = []store.Session{{ID: "s1", Name: "alpha", Agent: "claude", Status: "running", PermissionProfile: "safe"}}
	model.selected = 0

	got, _ := model.Update(key("P"))
	model = got.(Model)
	got, _ = model.Update(key("esc"))
	model = got.(Model)

	if model.profileSwitching {
		t.Fatal("Esc did not close the profile-switch dialog")
	}
	if called {
		t.Fatal("Esc invoked profileSwitch")
	}
}

// TestProfileSwitchYoloRequiresConfirm proves switching an existing session
// to yolo requires the explicit "y" confirm keystroke before enter persists
// it, mirroring the create modal's double-gate (SPEC §5; operator steering
// 002: task 020 only single-gated this path via allow_yolo).
func TestProfileSwitchYoloRequiresConfirm(t *testing.T) {
	called := false
	model := NewWithShellCreatorAttacherKillerResumerAndProfileSwitcher(
		nil, config.Settings{AllowYolo: true}, "", nil, nil, nil, nil, nil,
		func(ctx context.Context, id, profile string) (store.Session, error) {
			called = true
			return store.Session{ID: id, PermissionProfile: profile}, nil
		},
	)
	model.sessions = []store.Session{{ID: "s1", Name: "alpha", Agent: "claude", Status: "running", PermissionProfile: "safe"}}
	model.selected = 0

	got, _ := model.Update(key("P"))
	model = got.(Model)
	// safe -> plan -> edits -> yolo
	for i := 0; i < 3; i++ {
		got, _ = model.Update(key("right"))
		model = got.(Model)
	}
	if model.profileSwitchValue != "yolo" {
		t.Fatalf("expected candidate value yolo, got %q", model.profileSwitchValue)
	}

	// Enter without confirming must state why and change nothing.
	got, cmd := model.Update(key("enter"))
	model = got.(Model)
	if cmd != nil {
		t.Fatal("enter without yolo confirm dispatched a command")
	}
	if called {
		t.Fatal("enter without yolo confirm invoked profileSwitch")
	}
	if model.profileSwitchNote == "" {
		t.Fatal("enter without yolo confirm did not state why nothing happened")
	}
	if !model.profileSwitching {
		t.Fatal("enter without yolo confirm closed the dialog")
	}
	view := model.View()
	if !strings.Contains(view, "requires confirmation") {
		t.Fatalf("profile-switch dialog did not state the yolo confirm requirement:\n%s", view)
	}

	// Now confirm with "y" and enter: it must persist.
	got, _ = model.Update(key("y"))
	model = got.(Model)
	got, cmd = model.Update(key("enter"))
	model = got.(Model)
	if cmd == nil {
		t.Fatal("enter after yolo confirm did not dispatch a command")
	}
	cmd()
	if !called {
		t.Fatal("enter after yolo confirm did not invoke profileSwitch")
	}
}

// TestProfileSwitchAwayFromYoloNeedsNoConfirm proves switching away from
// yolo, or between non-yolo profiles, needs no extra keystroke.
func TestProfileSwitchAwayFromYoloNeedsNoConfirm(t *testing.T) {
	called := false
	model := NewWithShellCreatorAttacherKillerResumerAndProfileSwitcher(
		nil, config.Settings{AllowYolo: true}, "", nil, nil, nil, nil, nil,
		func(ctx context.Context, id, profile string) (store.Session, error) {
			called = true
			return store.Session{ID: id, PermissionProfile: profile}, nil
		},
	)
	model.sessions = []store.Session{{ID: "s1", Name: "alpha", Agent: "claude", Status: "running", PermissionProfile: "yolo"}}
	model.selected = 0

	got, _ := model.Update(key("P"))
	model = got.(Model)
	if model.profileSwitchValue != "yolo" {
		t.Fatalf("expected initial candidate value yolo, got %q", model.profileSwitchValue)
	}
	got, _ = model.Update(key("left")) // yolo -> edits
	model = got.(Model)
	if model.profileSwitchValue != "edits" {
		t.Fatalf("expected candidate value edits, got %q", model.profileSwitchValue)
	}

	got, cmd := model.Update(key("enter"))
	model = got.(Model)
	if cmd == nil {
		t.Fatal("enter switching away from yolo did not dispatch a command without any confirm")
	}
	cmd()
	if !called {
		t.Fatal("enter switching away from yolo did not invoke profileSwitch")
	}
}

// TestProfileSwitchYoloAbsentWhenNotAllowed proves yolo is not among the
// offered options at all when allow_yolo=false, matching the create modal's
// config gate.
func TestProfileSwitchYoloAbsentWhenNotAllowed(t *testing.T) {
	model := NewWithShellCreatorAttacherKillerResumerAndProfileSwitcher(
		nil, config.Settings{AllowYolo: false}, "", nil, nil, nil, nil, nil,
		func(ctx context.Context, id, profile string) (store.Session, error) {
			return store.Session{}, nil
		},
	)
	model.sessions = []store.Session{{ID: "s1", Name: "alpha", Agent: "claude", Status: "running", PermissionProfile: "safe"}}
	model.selected = 0

	got, _ := model.Update(key("P"))
	model = got.(Model)
	view := model.View()
	if strings.Contains(view, "yolo") {
		t.Fatalf("yolo offered in profile-switch dialog despite allow_yolo=false:\n%s", view)
	}
	for i := 0; i < 4; i++ {
		got, _ = model.Update(key("right"))
		model = got.(Model)
		if model.profileSwitchValue == "yolo" {
			t.Fatal("cycling reached yolo despite allow_yolo=false")
		}
	}
}

// TestProfileSwitchNotOfferedForShell proves `P` refuses to open the dialog
// for a shell session, which has no notion of a permission profile at all.
func TestProfileSwitchNotOfferedForShell(t *testing.T) {
	model := NewWithShellCreatorAttacherKillerResumerAndProfileSwitcher(
		nil, config.Settings{}, "", nil, nil, nil, nil, nil,
		func(ctx context.Context, id, profile string) (store.Session, error) {
			return store.Session{}, nil
		},
	)
	model.sessions = []store.Session{{ID: "s1", Name: "alpha", Agent: "shell", Status: "running"}}
	model.selected = 0

	got, _ := model.Update(key("P"))
	model = got.(Model)
	if model.profileSwitching {
		t.Fatal("P opened the profile-switch dialog for a shell session")
	}
	if model.attachError == "" {
		t.Fatal("P on a shell session did not explain why nothing happened")
	}
}
