package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/service"
	"github.com/n-orlov/deck/internal/store"
)

// newCreatingModelWithSettings is newCreatingModel with control over
// config.Settings.AllowYolo, needed because the yolo double-gate (task 017)
// reads that setting directly rather than a hard-coded default.
func newCreatingModelWithSettings(t *testing.T, settings config.Settings) Model {
	t.Helper()
	m := New(nil, settings, "")
	m.creating = true
	m.createName = "my session"
	m.createCWD = t.TempDir()
	m.createAgent = "shell"
	m.createProfile = "safe"
	m.createField = 0
	return m
}

// TestCreateProfileOptionsForOffersOnlyDeclaredProfiles proves the modal
// narrows the cycled set to exactly what each adapter's Caps declares
// (SPEC §5), never the static full list.
func TestCreateProfileOptionsForOffersOnlyDeclaredProfiles(t *testing.T) {
	cases := []struct {
		kind      string
		allowYolo bool
		want      []string
	}{
		{"claude", true, []string{"safe", "plan", "edits", "yolo"}},
		{"claude", false, []string{"safe", "plan", "edits"}},
		{"pi", true, []string{"safe", "edits", "yolo"}},
		{"pi", false, []string{"safe", "edits"}},
		{"shell", true, []string{"safe"}},
		{"shell", false, []string{"safe"}},
	}
	for _, tc := range cases {
		m := New(nil, config.Settings{}, "")
		got := m.createProfileOptionsFor(tc.kind, tc.allowYolo)
		if strings.Join(got, ",") != strings.Join(tc.want, ",") {
			t.Errorf("createProfileOptionsFor(%q, %v) = %v, want %v", tc.kind, tc.allowYolo, got, tc.want)
		}
	}
}

// TestCreateModalNoYoloOfferedWithAllowYoloFalse proves yolo is not merely
// hidden but explicitly explained as unavailable when the config disallows
// it, satisfying task 017's "states why rather than hiding it silently".
func TestCreateModalNoYoloOfferedWithAllowYoloFalse(t *testing.T) {
	m := newCreatingModelWithSettings(t, config.Settings{AllowYolo: false})
	m.createAgent = "claude"
	m.createField = 3

	for i := 0; i < 5; i++ {
		updated, _ := m.Update(key("right"))
		m = updated.(Model)
		if m.createProfile == "yolo" {
			t.Fatalf("yolo was offered while cycling with allow_yolo=false (landed on %q)", m.createProfile)
		}
	}

	view := m.createView()
	if !strings.Contains(view, "yolo is not offered because allow_yolo is not enabled") {
		t.Fatalf("createView did not explain why yolo is unavailable:\n%s", view)
	}
}

// TestCreateModalYoloRequiresConfirmMessage proves that, even with
// allow_yolo=true, selecting yolo and pressing Enter without the explicit
// confirm keystroke is refused with a specific message and creates nothing.
func TestCreateModalYoloRequiresConfirmMessage(t *testing.T) {
	m := newCreatingModelWithSettings(t, config.Settings{AllowYolo: true})
	var called bool
	m.create = func(ctx context.Context, in service.ShellCreateInput) (store.Session, error) {
		called = true
		return store.Session{}, nil
	}
	m.createProfile = "yolo"

	updated, _ := m.Update(key("enter"))
	after := updated.(Model)
	if !strings.Contains(after.createError, "yolo requires confirmation") {
		t.Fatalf("createError = %q, want mention of yolo confirmation", after.createError)
	}
	if !after.creating {
		t.Fatal("modal closed on missing yolo confirmation")
	}
	if after.createProfile != "yolo" {
		t.Errorf("createProfile changed: %q", after.createProfile)
	}
	if called {
		t.Fatal("create was invoked without the yolo confirm")
	}
}

// TestCreateModalYoloConfirmThenCreateSucceeds proves the explicit confirm
// keystroke ("y" on the Permission profile field) unblocks creation.
func TestCreateModalYoloConfirmThenCreateSucceeds(t *testing.T) {
	m := newCreatingModelWithSettings(t, config.Settings{AllowYolo: true})
	var called bool
	m.create = func(ctx context.Context, in service.ShellCreateInput) (store.Session, error) {
		called = true
		return store.Session{Name: in.Name}, nil
	}
	m.createProfile = "yolo"
	m.createField = 3

	updated, _ := m.Update(key("y"))
	m = updated.(Model)
	if !m.createYoloConfirmed {
		t.Fatal("y keystroke on the profile field did not set createYoloConfirmed")
	}

	updated, cmd := m.Update(key("enter"))
	after := updated.(Model)
	if after.createError != "" {
		t.Fatalf("unexpected validation error after yolo confirm: %q", after.createError)
	}
	if cmd == nil {
		t.Fatal("expected a create command to be issued after the yolo confirm")
	}
	_ = cmd()
	if !called {
		t.Fatal("create was not invoked after the yolo confirm")
	}
}

// TestCreateModalYCharacterStillTypesIntoTextFields proves the yolo confirm
// keystroke does not swallow ordinary "y" characters typed into text
// fields (e.g. a name starting with "y").
func TestCreateModalYCharacterStillTypesIntoTextFields(t *testing.T) {
	m := newCreatingModelWithSettings(t, config.Settings{AllowYolo: true})
	m.createField = 0
	m.createName = ""

	updated, _ := m.Update(key("y"))
	after := updated.(Model)
	if after.createName != "y" {
		t.Fatalf("createName = %q, want %q", after.createName, "y")
	}
}
