package tui

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// This file is task 007's own acceptance test obligation (SPEC §5/§8):
// `P` is unbound at the top level entirely, and is reachable only from
// inside the `i` detail dialog, exactly like rename.go's "r"/"l"/"g"
// before it.

// TestTopLevelPIsUnbound proves a bare top-level "P" (the cursor resting
// on a session row, m.detail false) opens nothing and mutates nothing --
// there is no case "P" left anywhere in Model.Update's main list-mode
// switch (only rename.go's own case "P", reachable only once m.detail is
// already true).
func TestTopLevelPIsUnbound(t *testing.T) {
	var called bool
	model := NewWithShellCreatorAttacherKillerResumerAndProfileSwitcher(
		nil, config.Settings{}, "", nil, nil, nil, nil, nil,
		func(_ context.Context, _, _ string) (store.Session, error) {
			called = true
			return store.Session{}, nil
		},
	)
	model.sessions = []store.Session{{ID: "s1", Name: "alpha", Agent: "claude", Status: "running", PermissionProfile: "safe"}}
	model.selected = rowCursor(0)
	before := modelSnapshotForEquality(model)

	got, cmd := model.Update(key("P"))
	after, ok := got.(Model)
	if !ok {
		t.Fatalf("Update(P) returned %T, not tui.Model", got)
	}
	if cmd != nil {
		t.Fatal("a top-level P dispatched a tea.Cmd; it must be a pure no-op")
	}
	if after.profileSwitching {
		t.Fatal("a top-level P opened the profile-switch dialog")
	}
	if after.detail {
		t.Fatal("a top-level P opened the detail dialog")
	}
	if got := modelSnapshotForEquality(after); !reflect.DeepEqual(got, before) {
		t.Fatalf("a top-level P mutated the model: %s", strings.Join(differingModelFields(before, got), "; "))
	}
	if called {
		t.Fatal("a top-level P invoked profileSwitch")
	}
}

// TestDetailPOpensProfilePickerAndReturnsToDetail proves the full `i`
// then `P` route (task 007): the picker opens only once detail is
// already showing, a submitted pick persists through the same durable
// profileSwitch write and the same allow_yolo gating and degradation
// reporting the top-level dialog always had, the model lands back on the
// detail view (not the main list) once it closes -- on both a
// successful submit and an Esc cancel -- and an ineligible session (no
// permission profile at all) is refused with canSwitchProfile's own
// message, still leaving detail open underneath it.
func TestDetailPOpensProfilePickerAndReturnsToDetail(t *testing.T) {
	t.Run("submit persists through profileSwitch and returns to detail", func(t *testing.T) {
		var persistedID, persistedProfile string
		updatedSession := store.Session{ID: "s1", Name: "alpha", Agent: "claude", Status: "running", PermissionProfile: "edits"}
		model := NewWithShellCreatorAttacherKillerResumerAndProfileSwitcher(
			nil, config.Settings{AllowYolo: true}, "", nil, nil, nil, nil, nil,
			func(_ context.Context, id, profile string) (store.Session, error) {
				persistedID, persistedProfile = id, profile
				return updatedSession, nil
			},
		)
		model.sessions = []store.Session{{
			ID: "s1", Name: "alpha", Agent: "claude", Status: "running",
			PermissionProfile:       "safe",
			PermissionProfileReason: `claude does not support permission profile "yolo" without allow_yolo; falling back to safe`,
		}}
		model.selected = rowCursor(0)

		// The degradation reason renders in detail exactly as it always
		// did -- this task moves P's own entry point, never the field
		// detailBody already shows above it.
		got, _ := model.Update(key("i"))
		model = got.(Model)
		if !model.detail {
			t.Fatal("i did not open the detail dialog")
		}
		if !strings.Contains(model.detailBody(), "degraded:") {
			t.Fatalf("detail dropped the permission profile's degradation reason:\n%s", model.detailBody())
		}

		got, _ = model.Update(key("P"))
		model = got.(Model)
		if !model.profileSwitching {
			t.Fatal("P inside detail did not open the profile-switch picker")
		}
		// allow_yolo=true gates yolo INTO the offered cycle, exactly as
		// the top-level dialog always did (createProfileOptionsFor).
		if !strings.Contains(model.View(), "yolo") {
			t.Fatalf("profile-switch picker did not offer yolo with allow_yolo=true:\n%s", model.View())
		}
		got, _ = model.Update(key("right"))
		model = got.(Model)

		got, cmd := model.Update(key("enter"))
		model = got.(Model)
		if cmd == nil {
			t.Fatal("enter on the profile-switch picker did not dispatch a command")
		}
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
		if persistedID != "s1" {
			t.Fatalf("profileSwitch persisted id %q, want s1", persistedID)
		}
		if persistedProfile == "" || persistedProfile == "safe" {
			t.Fatalf("profileSwitch persisted the unchanged value %q", persistedProfile)
		}
	})

	t.Run("esc returns to detail, not the list", func(t *testing.T) {
		model := NewWithShellCreatorAttacherKillerResumerAndProfileSwitcher(
			nil, config.Settings{}, "", nil, nil, nil, nil, nil,
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
		if !model.profileSwitching {
			t.Fatal("P inside detail did not open the profile-switch picker")
		}

		got, _ = model.Update(key("esc"))
		model = got.(Model)
		if model.profileSwitching {
			t.Fatal("esc did not close the profile-switch picker")
		}
		if !model.detail {
			t.Fatal("esc from the profile-switch picker closed detail too -- it must return to detail, not the list")
		}

		// A second esc closes detail itself, proving detail (and only
		// detail) is what the picker's own esc landed on.
		got, _ = model.Update(key("esc"))
		model = got.(Model)
		if model.detail {
			t.Fatal("esc on the detail view itself did not close it")
		}
	})

	t.Run("ineligible session is refused with canSwitchProfile's message", func(t *testing.T) {
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
		got, _ = model.Update(key("P"))
		model = got.(Model)
		if model.profileSwitching {
			t.Fatal("P opened the profile-switch picker for a shell session, which has no permission profile at all")
		}
		if !model.detail {
			t.Fatal("P's refusal closed the detail dialog underneath it")
		}
		if !strings.Contains(model.attachError, "no permission profile") {
			t.Fatalf("attachError = %q, want canSwitchProfile's own refusal wording", model.attachError)
		}
	})
}
