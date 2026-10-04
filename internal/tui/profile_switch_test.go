package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// Task 007 moved every one of these tests off a bare top-level `P` onto
// `i` then `P`: the permission-profile picker is now reachable only from
// inside the `i` detail dialog (there is no top-level "P" case left in
// Model.Update's main switch at all -- see rename.go's own case "P").
// Each test below opens detail first, asserting it actually opened,
// exactly the same way rename_test.go/launch_inputs_test.go already open
// detail before exercising their own nested dialogs.

// TestProfileSwitchPersistsAndStatesRestartToApply proves `i` then `P`
// changes the permission profile of an existing session, persists it
// through the wired profileSwitch function, states the change applies on
// the next launch/restart rather than claiming the live pane changed
// mode, and returns to the detail view once the switch succeeds (task
// 007/020, SPEC §5/§8).
func TestProfileSwitchPersistsAndStatesRestartToApply(t *testing.T) {
	var persistedID, persistedProfile string
	updated := store.Session{ID: "s1", Name: "alpha", Agent: "claude", Status: "running", PermissionProfile: "edits"}
	model := NewWithShellCreatorAttacherKillerResumerAndProfileSwitcher(
		nil, config.Settings{}, "", nil, nil, nil, nil, nil,
		func(_ context.Context, id, profile string) (store.Session, error) {
			persistedID, persistedProfile = id, profile
			return updated, nil
		},
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
		t.Fatal("P did not open the profile-switch dialog from inside detail")
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
	if !model.detail {
		t.Fatal("a successful profile switch did not return to the detail view")
	}
	if persistedID != "s1" {
		t.Fatalf("profileSwitch called with unexpected id %q", persistedID)
	}
	if persistedProfile == "" || persistedProfile == "safe" {
		t.Fatalf("profileSwitch persisted the unchanged value %q", persistedProfile)
	}
}

// TestProfileSwitchEscCancelsWithoutPersisting proves Esc closes the
// dialog without calling profileSwitch at all, and returns to the detail
// view underneath it (task 007), not all the way out to the main list.
func TestProfileSwitchEscCancelsWithoutPersisting(t *testing.T) {
	called := false
	model := NewWithShellCreatorAttacherKillerResumerAndProfileSwitcher(
		nil, config.Settings{}, "", nil, nil, nil, nil, nil,
		func(_ context.Context, _, _ string) (store.Session, error) {
			called = true
			return store.Session{}, nil
		},
	)
	model.sessions = []store.Session{{ID: "s1", Name: "alpha", Agent: "claude", Status: "running", PermissionProfile: "safe"}}
	model.selected = rowCursor(0)

	got, _ := model.Update(key("i"))
	model = got.(Model)
	got, _ = model.Update(key("P"))
	model = got.(Model)
	got, _ = model.Update(key("esc"))
	model = got.(Model)

	if model.profileSwitching {
		t.Fatal("Esc did not close the profile-switch dialog")
	}
	if !model.detail {
		t.Fatal("Esc from the profile-switch dialog did not return to the detail view")
	}
	if called {
		t.Fatal("Esc invoked profileSwitch")
	}
}

// TestProfileSwitchToYoloTakesEffectWithNoConfirm proves switching an
// existing session to yolo persists immediately on enter, with no separate
// confirm keystroke (steer 017 item 2 removed the double-gate task 020's
// original P dialog had; allow_yolo alone still gates yolo's availability).
func TestProfileSwitchToYoloTakesEffectWithNoConfirm(t *testing.T) {
	called := false
	model := NewWithShellCreatorAttacherKillerResumerAndProfileSwitcher(
		nil, config.Settings{AllowYolo: true}, "", nil, nil, nil, nil, nil,
		func(_ context.Context, id, profile string) (store.Session, error) {
			called = true
			return store.Session{ID: id, PermissionProfile: profile}, nil
		},
	)
	model.sessions = []store.Session{{ID: "s1", Name: "alpha", Agent: "claude", Status: "running", PermissionProfile: "safe"}}
	model.selected = rowCursor(0)

	got, _ := model.Update(key("i"))
	model = got.(Model)
	got, _ = model.Update(key("P"))
	model = got.(Model)
	// safe -> plan -> edits -> yolo
	for i := 0; i < 3; i++ {
		got, _ = model.Update(key("right"))
		model = got.(Model)
	}
	if model.profileSwitchValue != "yolo" {
		t.Fatalf("expected candidate value yolo, got %q", model.profileSwitchValue)
	}

	got, cmd := model.Update(key("enter"))
	model = got.(Model)
	if cmd == nil {
		t.Fatal("enter switching to yolo with no confirm did not dispatch a command")
	}
	msg := cmd()
	got, loadCmd := model.Update(msg)
	model = got.(Model)
	if loadCmd == nil {
		t.Fatal("a successful yolo switch did not trigger a reload")
	}
	if !called {
		t.Fatal("enter switching to yolo with no confirm did not invoke profileSwitch")
	}
	if model.profileSwitching {
		t.Fatal("profile-switch dialog remained open after a successful switch to yolo")
	}
}

// TestProfileSwitchAwayFromYoloNeedsNoConfirm proves switching away from
// yolo, or between non-yolo profiles, needs no extra keystroke.
func TestProfileSwitchAwayFromYoloNeedsNoConfirm(t *testing.T) {
	called := false
	model := NewWithShellCreatorAttacherKillerResumerAndProfileSwitcher(
		nil, config.Settings{AllowYolo: true}, "", nil, nil, nil, nil, nil,
		func(_ context.Context, id, profile string) (store.Session, error) {
			called = true
			return store.Session{ID: id, PermissionProfile: profile}, nil
		},
	)
	model.sessions = []store.Session{{ID: "s1", Name: "alpha", Agent: "claude", Status: "running", PermissionProfile: "yolo"}}
	model.selected = rowCursor(0)

	got, _ := model.Update(key("i"))
	model = got.(Model)
	got, _ = model.Update(key("P"))
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
		func(_ context.Context, _, _ string) (store.Session, error) {
			return store.Session{}, nil
		},
	)
	model.sessions = []store.Session{{ID: "s1", Name: "alpha", Agent: "claude", Status: "running", PermissionProfile: "safe"}}
	model.selected = rowCursor(0)

	got, _ := model.Update(key("i"))
	model = got.(Model)
	got, _ = model.Update(key("P"))
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

// TestProfileSwitchNotOfferedForShell proves `P` refuses to open the
// dialog for a shell session, which has no notion of a permission
// profile at all, staying on the detail view underneath it (canSwitchProfile's
// refusal message) rather than closing anything.
func TestProfileSwitchNotOfferedForShell(t *testing.T) {
	model := NewWithShellCreatorAttacherKillerResumerAndProfileSwitcher(
		nil, config.Settings{}, "", nil, nil, nil, nil, nil,
		func(_ context.Context, _, _ string) (store.Session, error) {
			return store.Session{}, nil
		},
	)
	model.sessions = []store.Session{{ID: "s1", Name: "alpha", Agent: "shell", Status: "running"}}
	model.selected = rowCursor(0)

	got, _ := model.Update(key("i"))
	model = got.(Model)
	if !model.detail {
		t.Fatal("i did not open the detail dialog")
	}
	got, _ = model.Update(key("P"))
	model = got.(Model)
	if model.profileSwitching {
		t.Fatal("P opened the profile-switch dialog for a shell session")
	}
	if !model.detail {
		t.Fatal("P's refusal for a shell session closed the detail dialog underneath it")
	}
	if model.attachError == "" {
		t.Fatal("P on a shell session did not explain why nothing happened")
	}
}
