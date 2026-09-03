package tui

import (
	"context"
	"testing"

	"github.com/n-orlov/deck/internal/service"
	"github.com/n-orlov/deck/internal/store"
)

// TestCreateModalShellSubmitPassesPreLaunchThrough covers task 038's UI seam:
// the create modal offers (and validateCreateFields validates) the Pre-launch
// field for every agent, `shell` included, and SPEC §6.4's hook "fires on
// create" for every pane deck launches -- so submitting a shell create must
// hand the typed hook to service.CreateShell rather than dropping it, now
// that ShellCreateInput carries a PreLaunch field and CreateShell composes it
// with the global hook.
func TestCreateModalShellSubmitPassesPreLaunchThrough(t *testing.T) {
	before := newCreatingModel(t)
	before.createAgent = "shell"
	before.createPreLaunch = "eval \"$(load-secrets)\""

	var got service.ShellCreateInput
	before.create = func(ctx context.Context, in service.ShellCreateInput) (store.Session, error) {
		got = in
		return store.Session{Name: in.Name}, nil
	}

	updated, cmd := before.Update(key("enter"))
	if after := updated.(Model); after.createError != "" {
		t.Fatalf("createError = %q, want none", after.createError)
	}
	if cmd == nil {
		t.Fatal("enter on a valid shell create issued no command")
	}
	cmd()

	if got.PreLaunch != before.createPreLaunch {
		t.Fatalf("CreateShell received PreLaunch %q, want the modal's own field %q", got.PreLaunch, before.createPreLaunch)
	}
	if got.Name != "my session" || got.CWD == "" {
		t.Fatalf("CreateShell received %#v, want the modal's name and resolved cwd alongside the hook", got)
	}
}
