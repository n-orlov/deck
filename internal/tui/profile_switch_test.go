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
