package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/service"
	"github.com/n-orlov/deck/internal/store"
)

// newCreatingModel returns a model with the create modal open and every
// field populated with a distinguishable value, so each test can flip just
// the one field under test and assert every OTHER field is still exactly
// what was typed after the error is rendered.
func newCreatingModel(t *testing.T) Model {
	t.Helper()
	m := New(nil, config.Settings{}, "")
	m.creating = true
	m.setCreateText(createFieldName, "my session")
	m.setCreateText(createFieldCWD, t.TempDir())
	m.createAgent = "shell"
	m.createProfile = "safe"
	m.setCreateText(createFieldLaunchArgs, "")
	m.setCreateText(createFieldEnv, "")
	m.setCreateText(createFieldPreLaunch, "echo hi")
	m.createLoginShell = false
	m.createField = 0
	return m
}

func assertFieldsRetained(t *testing.T, before, after Model) {
	t.Helper()
	if after.createText(createFieldName) != before.createText(createFieldName) {
		t.Errorf("createName changed: %q -> %q", before.createText(createFieldName), after.createText(createFieldName))
	}
	if after.createAgent != before.createAgent {
		t.Errorf("createAgent changed: %q -> %q", before.createAgent, after.createAgent)
	}
	if after.createText(createFieldPreLaunch) != before.createText(createFieldPreLaunch) {
		t.Errorf("createPreLaunch changed: %q -> %q", before.createText(createFieldPreLaunch), after.createText(createFieldPreLaunch))
	}
}

func TestCreateModalMissingCWDMessage(t *testing.T) {
	before := newCreatingModel(t)
	before.setCreateText(createFieldCWD, "")
	updated, _ := before.Update(key("enter"))
	after := updated.(Model)
	if after.createError != "working directory is required" {
		t.Fatalf("createError = %q", after.createError)
	}
	if !after.creating {
		t.Fatal("modal closed on validation error")
	}
	assertFieldsRetained(t, before, after)
}

func TestCreateModalCWDNotDirectoryMessage(t *testing.T) {
	before := newCreatingModel(t)
	file := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	before.setCreateText(createFieldCWD, file)
	updated, _ := before.Update(key("enter"))
	after := updated.(Model)
	if !strings.Contains(after.createError, "is not a directory") {
		t.Fatalf("createError = %q, want mention of 'is not a directory'", after.createError)
	}
	if after.createText(createFieldCWD) != file {
		t.Errorf("createCWD changed: %q -> %q", file, after.createText(createFieldCWD))
	}
	assertFieldsRetained(t, before, after)
}

func TestCreateModalMalformedEnvKeyMessage(t *testing.T) {
	before := newCreatingModel(t)
	before.setCreateText(createFieldEnv, "novalue,GOOD=1")
	updated, _ := before.Update(key("enter"))
	after := updated.(Model)
	if !strings.Contains(after.createError, "key=value") {
		t.Fatalf("createError = %q, want mention of 'key=value'", after.createError)
	}
	if !strings.Contains(after.createError, "novalue") {
		t.Fatalf("createError = %q, want the offending entry named", after.createError)
	}
	if after.createText(createFieldEnv) != "novalue,GOOD=1" {
		t.Errorf("createEnv changed: %q", after.createText(createFieldEnv))
	}
	assertFieldsRetained(t, before, after)
}

func TestCreateModalMalformedLaunchArgsJSONMessage(t *testing.T) {
	before := newCreatingModel(t)
	before.setCreateText(createFieldLaunchArgs, "{not json")
	updated, _ := before.Update(key("enter"))
	after := updated.(Model)
	if !strings.Contains(after.createError, "launch_args must be a JSON array") {
		t.Fatalf("createError = %q, want mention of launch_args JSON", after.createError)
	}
	if after.createText(createFieldLaunchArgs) != "{not json" {
		t.Errorf("createLaunchArgs changed: %q", after.createText(createFieldLaunchArgs))
	}
	assertFieldsRetained(t, before, after)
}

func TestCreateModalUnsupportedProfileMessage(t *testing.T) {
	before := newCreatingModel(t)
	before.createAgent = "pi"
	before.createProfile = "plan"
	updated, _ := before.Update(key("enter"))
	after := updated.(Model)
	if !strings.Contains(after.createError, "does not support permission profile") {
		t.Fatalf("createError = %q, want a declared-capability degradation reason", after.createError)
	}
	if !strings.Contains(after.createError, "plan") {
		t.Fatalf("createError = %q, want the requested profile named", after.createError)
	}
	if after.createProfile != "plan" {
		t.Errorf("createProfile changed: %q", after.createProfile)
	}
	assertFieldsRetained(t, before, after)
}

// TestCreateModalShellIgnoresProfileValidation proves the fix above does not
// regress ordinary shell creation: shell declares no permission profiles at
// all (SPEC §5/§8), so its default "safe" selection must never be treated
// as an unsupported profile.
func TestCreateModalShellIgnoresProfileValidation(t *testing.T) {
	before := newCreatingModel(t)
	var called bool
	before.create = func(ctx context.Context, in service.ShellCreateInput) (store.Session, error) {
		called = true
		return store.Session{Name: in.Name}, nil
	}
	updated, cmd := before.Update(key("enter"))
	after := updated.(Model)
	if after.createError != "" {
		t.Fatalf("unexpected validation error for shell/safe: %q", after.createError)
	}
	if cmd == nil {
		t.Fatal("expected a create command to be issued")
	}
	_ = cmd()
	if !called {
		t.Fatal("create was not invoked")
	}
}

func TestCreateModalDuplicateNameMessage(t *testing.T) {
	before := newCreatingModel(t)
	updated, _ := before.Update(shellCreated{err: errors.New(`session name "my session" already exists`)})
	after := updated.(Model)
	if !strings.Contains(after.createError, "already exists") {
		t.Fatalf("createError = %q, want 'already exists'", after.createError)
	}
	if !after.creating {
		t.Fatal("modal closed on duplicate-name error")
	}
	assertFieldsRetained(t, before, after)
}

func TestCreateModalSlugCollisionMessage(t *testing.T) {
	before := newCreatingModel(t)
	updated, _ := before.Update(shellCreated{err: errors.New(`session name "my session" collides with existing slug "my-session"`)})
	after := updated.(Model)
	if !strings.Contains(after.createError, "collides with existing slug") {
		t.Fatalf("createError = %q, want 'collides with existing slug'", after.createError)
	}
	view := after.createView()
	if !strings.Contains(view, "name collides with existing slug") {
		t.Fatalf("createView missing rendered collision message:\n%s", view)
	}
	if !after.creating {
		t.Fatal("modal closed on slug-collision error")
	}
	assertFieldsRetained(t, before, after)
}

func TestCreateModalEscapeCreatesNothing(t *testing.T) {
	before := newCreatingModel(t)
	var called bool
	before.create = func(ctx context.Context, in service.ShellCreateInput) (store.Session, error) {
		called = true
		return store.Session{}, nil
	}
	updated, cmd := before.Update(key("esc"))
	after := updated.(Model)
	if after.creating {
		t.Fatal("esc did not close the create modal")
	}
	if cmd != nil {
		t.Fatal("esc issued a command")
	}
	if called {
		t.Fatal("esc invoked create")
	}
}
