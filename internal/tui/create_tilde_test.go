package tui

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/n-orlov/deck/internal/service"
	"github.com/n-orlov/deck/internal/store"
)

// TestExpandCreateCWDBareTilde asserts a bare "~" resolves to the user's
// home directory, absolute and unchanged from a tilde.
func TestExpandCreateCWDBareTilde(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	got, err := expandCreateCWD("~")
	if err != nil {
		t.Fatalf("expandCreateCWD(~) error: %v", err)
	}
	want, _ := filepath.Abs(home)
	if got != want {
		t.Fatalf("expandCreateCWD(~) = %q, want %q", got, want)
	}
}

// TestExpandCreateCWDTildeSlash asserts "~/sub/dir" expands to the resolved
// absolute path under the home directory, per SPEC §11.7's tilde rule.
func TestExpandCreateCWDTildeSlash(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	got, err := expandCreateCWD("~/Projects/thing")
	if err != nil {
		t.Fatalf("expandCreateCWD error: %v", err)
	}
	want := filepath.Join(home, "Projects", "thing")
	if got != want {
		t.Fatalf("expandCreateCWD(~/Projects/thing) = %q, want %q", got, want)
	}
}

// TestExpandCreateCWDOtherUserRejected asserts "~otheruser" is rejected with
// a stated reason rather than half-expanded (requirement 39 explicitly puts
// other-user home resolution out of scope).
func TestExpandCreateCWDOtherUserRejected(t *testing.T) {
	got, err := expandCreateCWD("~otheruser/work")
	if err == nil {
		t.Fatalf("expandCreateCWD(~otheruser/work) = %q, want error", got)
	}
	if got != "" {
		t.Fatalf("expandCreateCWD(~otheruser/work) returned a half-expanded path %q", got)
	}
}

// TestExpandCreateCWDNoTildeUnchanged asserts inputs without a leading ~ are
// returned exactly as given, preserving every existing relative/absolute-path
// behaviour this task does not touch.
func TestExpandCreateCWDNoTildeUnchanged(t *testing.T) {
	for _, raw := range []string{"/abs/path", "relative/path", ""} {
		got, err := expandCreateCWD(raw)
		if err != nil {
			t.Fatalf("expandCreateCWD(%q) error: %v", raw, err)
		}
		if got != raw {
			t.Fatalf("expandCreateCWD(%q) = %q, want unchanged", raw, got)
		}
	}
}

// TestCreateModalTildeCWDValidatesAndSubmitsResolvedPath drives the create
// modal through the pty-free Update path with a "~"-prefixed cwd and asserts
// the resolved absolute path (never the tilde) both passes validation and is
// what the store receives on submit.
func TestCreateModalTildeCWDValidatesAndSubmitsResolvedPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	sub := filepath.Join(home, "Projects", "invp-ops-dev-agents")
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatal(err)
	}

	before := newCreatingModel(t)
	before.createCWD = "~/Projects/invp-ops-dev-agents"

	if msg := before.validateCreateFields(); msg != "" {
		t.Fatalf("validateCreateFields() = %q, want no error for an existing tilde-expanded dir", msg)
	}

	var gotCWD string
	before.create = func(ctx context.Context, in service.ShellCreateInput) (store.Session, error) {
		gotCWD = in.CWD
		return store.Session{}, nil
	}

	updated, cmd := before.Update(key("enter"))
	after := updated.(Model)
	if after.createError != "" {
		t.Fatalf("createError = %q, want none", after.createError)
	}
	if cmd == nil {
		t.Fatal("enter on a valid tilde cwd issued no command")
	}
	cmd()

	if gotCWD != sub {
		t.Fatalf("store received CWD %q, want resolved absolute path %q (not the tilde)", gotCWD, sub)
	}
	// The typed field itself is untouched: only the value handed to the
	// store is resolved, matching every other validated-field's retain rule.
	if after.createCWD != "~/Projects/invp-ops-dev-agents" {
		t.Fatalf("createCWD field mutated: %q", after.createCWD)
	}
}

// TestCreateModalOtherUserTildeRejectedNotHalfExpanded asserts "~otheruser"
// is rejected with a stated reason and the field is retained verbatim, never
// half-expanded into some other path.
func TestCreateModalOtherUserTildeRejectedNotHalfExpanded(t *testing.T) {
	before := newCreatingModel(t)
	before.createCWD = "~otheruser/work"

	updated, cmd := before.Update(key("enter"))
	after := updated.(Model)
	if cmd != nil {
		t.Fatal("enter on ~otheruser issued a create command")
	}
	if after.createError == "" {
		t.Fatal("createError empty, want a stated rejection reason")
	}
	if after.createCWD != "~otheruser/work" {
		t.Fatalf("createCWD changed: %q", after.createCWD)
	}
}
