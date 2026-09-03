package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// This file closes task 023's own wiring gap: the launch-inputs editor is
// reachable and themed, but a Model whose launchInputsSetter is nil can
// only report "editing launch inputs is unavailable" on Enter. The shipped
// binary wires that dependency through WithLaunchInputsSetter
// (cmd/deck/main.go passes service.Service.SetLaunchInputs); these tests
// pin both halves of that contract -- with the setter wired a submit
// reaches it with exactly the four typed values, and without it the dialog
// says so instead of silently doing nothing.

// launchInputsWiringModel opens the editor on one live row with the three
// text fields typed and login_shell toggled on, mirroring what `l` prefills
// and a user then edits.
func launchInputsWiringModel() Model {
	m := New(nil, config.Settings{}, "")
	m.width, m.height = 80, 24
	m.sessions = []store.Session{{ID: "s1", Name: "alpha", Agent: "shell", Status: "running", Slug: "alpha"}}
	m.selected = 0
	m.detail = true
	m.launchInputsEditing = true
	m.launchInputsPreLaunch = "echo pre"
	m.launchInputsPostDestroy = "echo post"
	m.launchInputsLaunchArgs = `["--flag","value"]`
	m.launchInputsLoginShell = true
	return m
}

// TestWithLaunchInputsSetterCarriesEveryTypedFieldToTheSetter proves the
// wiring method the shipped binary uses: a Model built by New and handed a
// setter through WithLaunchInputsSetter routes Enter's submit to that
// setter with the session's id and all four typed values, launch_args
// parsed from its JSON text.
func TestWithLaunchInputsSetterCarriesEveryTypedFieldToTheSetter(t *testing.T) {
	var gotID, gotPre, gotPost string
	var gotArgs []string
	var gotLoginShell bool
	calls := 0

	m := launchInputsWiringModel().WithLaunchInputsSetter(func(_ context.Context, id, preLaunch, postDestroy string, launchArgs []string, loginShell bool) (store.Session, error) {
		calls++
		gotID, gotPre, gotPost, gotArgs, gotLoginShell = id, preLaunch, postDestroy, launchArgs, loginShell
		return store.Session{ID: id, Name: "alpha", PreLaunch: preLaunch, PostDestroy: postDestroy, LaunchArgs: launchArgs, LoginShell: loginShell, LaunchDirty: true}, nil
	})

	cmd := m.submitLaunchInputs()
	if m.launchInputsNote != "" {
		t.Fatalf("submit left a note %q; want none with a setter wired", m.launchInputsNote)
	}
	if cmd == nil {
		t.Fatal("submit returned no command; want one that calls the wired setter")
	}
	msg := cmd()
	if calls != 1 {
		t.Fatalf("setter was called %d times, want exactly 1", calls)
	}
	if gotID != "s1" {
		t.Fatalf("setter got session id %q, want %q", gotID, "s1")
	}
	if gotPre != "echo pre" || gotPost != "echo post" {
		t.Fatalf("setter got hooks %q/%q, want %q/%q", gotPre, gotPost, "echo pre", "echo post")
	}
	if len(gotArgs) != 2 || gotArgs[0] != "--flag" || gotArgs[1] != "value" {
		t.Fatalf("setter got launch_args %#v, want [--flag value]", gotArgs)
	}
	if !gotLoginShell {
		t.Fatal("setter got login_shell false, want true")
	}
	saved, ok := msg.(launchInputsSaved)
	if !ok {
		t.Fatalf("submit produced %T, want launchInputsSaved", msg)
	}
	if saved.err != nil {
		t.Fatalf("launchInputsSaved carried an error: %v", saved.err)
	}
	if !saved.session.LaunchDirty {
		t.Fatal("launchInputsSaved's session is not launch_dirty; the editor's write must mark it")
	}
}

// TestLaunchInputsSubmitWithoutASetterSaysSoAndCallsNothing pins the other
// half: the nil-setter path is an explicit on-screen refusal, never a
// silent no-op, and it keeps every typed field so nothing is lost.
func TestLaunchInputsSubmitWithoutASetterSaysSoAndCallsNothing(t *testing.T) {
	m := launchInputsWiringModel()
	if cmd := m.submitLaunchInputs(); cmd != nil {
		t.Fatal("submit without a setter returned a command; want none")
	}
	if !strings.Contains(m.launchInputsNote, "unavailable") {
		t.Fatalf("note = %q, want it to say editing is unavailable", m.launchInputsNote)
	}
	if m.launchInputsPreLaunch != "echo pre" || m.launchInputsPostDestroy != "echo post" || m.launchInputsLaunchArgs != `["--flag","value"]` || !m.launchInputsLoginShell {
		t.Fatal("a refused submit discarded typed fields; validation must retain what the user typed")
	}
	if !m.launchInputsEditing {
		t.Fatal("a refused submit closed the dialog; it must stay open")
	}
}
