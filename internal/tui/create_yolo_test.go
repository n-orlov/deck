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
// config.Settings.AllowYolo, needed because yolo's single-gate config check
// (task 017, de-gated further by steer 017 item 2) reads that setting
// directly rather than a hard-coded default.
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
	// Task 030: framedDialog no longer grows to fit content, so a
	// wide-enough viewport keeps this sentence off a word-wrap boundary.
	m.width = 100

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

// TestCreateModalYoloTakesEffectImmediatelyWithNoConfirm proves that, with
// allow_yolo=true, selecting yolo and pressing Enter creates the session
// directly -- no separate confirm keystroke is required (steer 017 item 2
// removed the double-gate; allow_yolo alone still gates availability).
func TestCreateModalYoloTakesEffectImmediatelyWithNoConfirm(t *testing.T) {
	m := newCreatingModelWithSettings(t, config.Settings{AllowYolo: true})
	var called bool
	m.create = func(ctx context.Context, in service.ShellCreateInput) (store.Session, error) {
		called = true
		return store.Session{Name: in.Name}, nil
	}
	m.createProfile = "yolo"

	updated, cmd := m.Update(key("enter"))
	after := updated.(Model)
	if after.createError != "" {
		t.Fatalf("unexpected validation error creating yolo with no confirm: %q", after.createError)
	}
	if cmd == nil {
		t.Fatal("expected a create command to be issued for yolo with no confirm")
	}
	_ = cmd()
	if !called {
		t.Fatal("create was not invoked for yolo with no confirm")
	}
}

// TestCreateModalYCharacterStillTypesIntoTextFields proves plain "y"
// characters still type normally into text fields (e.g. a name starting
// with "y") now that field 3's "y" confirm keystroke is gone entirely.
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

// TestCreateModalDefaultsToSafeWhenYoloDefaultFalse proves the create
// modal's Permission profile field still opens on "safe" (options[0]) when
// yolo_default is false (the schema default), regardless of allow_yolo.
func TestCreateModalDefaultsToSafeWhenYoloDefaultFalse(t *testing.T) {
	m := New(nil, config.Settings{AllowYolo: true, YoloDefault: false}, "")
	if got := m.defaultCreateProfile("claude"); got != "safe" {
		t.Fatalf("defaultCreateProfile = %q, want %q", got, "safe")
	}
}

// TestCreateModalDefaultsToYoloWhenYoloDefaultAndAllowYoloAreBothTrue proves
// steer 017 item 2's yolo_default: the create modal's Permission profile
// field opens already on "yolo" when BOTH yolo_default and allow_yolo are
// true, for an adapter that declares yolo support.
func TestCreateModalDefaultsToYoloWhenYoloDefaultAndAllowYoloAreBothTrue(t *testing.T) {
	m := New(nil, config.Settings{AllowYolo: true, YoloDefault: true}, "")
	if got := m.defaultCreateProfile("claude"); got != "yolo" {
		t.Fatalf("defaultCreateProfile = %q, want %q", got, "yolo")
	}
}

// TestCreateModalYoloDefaultIsInertWithoutAllowYolo proves yolo_default is
// genuinely inert (never silently overrides allow_yolo) when allow_yolo is
// false: the modal still opens on "safe", not "yolo".
func TestCreateModalYoloDefaultIsInertWithoutAllowYolo(t *testing.T) {
	m := New(nil, config.Settings{AllowYolo: false, YoloDefault: true}, "")
	if got := m.defaultCreateProfile("claude"); got != "safe" {
		t.Fatalf("defaultCreateProfile = %q, want %q (yolo_default must be inert with allow_yolo=false)", got, "safe")
	}
}

// TestCreateModalYoloDefaultSkippedForAnAdapterThatDoesNotOfferYolo proves
// yolo_default never picks a profile the adapter itself does not declare
// (shell has no permission-profile notion at all): defaultCreateProfile
// falls back to createProfileOptionsFor's own narrowed list instead of
// forcing "yolo" onto an adapter that cannot offer it.
func TestCreateModalYoloDefaultSkippedForAnAdapterThatDoesNotOfferYolo(t *testing.T) {
	m := New(nil, config.Settings{AllowYolo: true, YoloDefault: true}, "")
	if got := m.defaultCreateProfile("shell"); got != "safe" {
		t.Fatalf("defaultCreateProfile(shell) = %q, want %q", got, "safe")
	}
}
