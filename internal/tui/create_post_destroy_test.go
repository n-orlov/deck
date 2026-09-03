package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/n-orlov/deck/internal/service"
	"github.com/n-orlov/deck/internal/store"
)

// TestCreateFieldRowsHasPostDestroyBesidePreLaunch covers task 026: the
// create modal gains a one-line post_destroy field next to pre_launch, with
// field help in the same shape as pre_launch's own row (a short statement
// of what runs and when, not just a label).
func TestCreateFieldRowsHasPostDestroyBesidePreLaunch(t *testing.T) {
	m := newCreatingModel(t)
	rows := m.createFieldRows()

	preIdx, postIdx := -1, -1
	for i, row := range rows {
		if row.label == "Pre-launch command" {
			preIdx = i
		}
		if row.label == "Post-destroy command" {
			postIdx = i
		}
	}
	if preIdx == -1 {
		t.Fatal("createFieldRows() has no Pre-launch command row")
	}
	if postIdx == -1 {
		t.Fatal("createFieldRows() has no Post-destroy command row")
	}
	if postIdx != preIdx+1 {
		t.Fatalf("Post-destroy command row is at index %d, want immediately beside Pre-launch command (index %d) at %d", postIdx, preIdx, preIdx+1)
	}

	help := rows[postIdx].help
	if strings.TrimSpace(help) == "" {
		t.Fatal("Post-destroy command row has empty help")
	}
	for _, phrase := range []string{"run after", "Archive or Delete", "fail-open"} {
		if !strings.Contains(help, phrase) {
			t.Errorf("Post-destroy command help = %q, missing expected phrase %q", help, phrase)
		}
	}
	if len(rows) != createFieldCount {
		t.Fatalf("createFieldRows() returned %d rows, want createFieldCount=%d", len(rows), createFieldCount)
	}
}

// TestCreateModalTypingIntoPostDestroyField covers the field actually
// accepting typed runes at its own field index, exactly as every other
// free-text create field does.
func TestCreateModalTypingIntoPostDestroyField(t *testing.T) {
	m := newCreatingModel(t)
	postIdx := -1
	for i, row := range m.createFieldRows() {
		if row.label == "Post-destroy command" {
			postIdx = i
		}
	}
	if postIdx == -1 {
		t.Fatal("no Post-destroy command field found")
	}
	if !createFieldIsText(postIdx) {
		t.Fatalf("createFieldIsText(%d) = false, want the Post-destroy command field to accept typed text", postIdx)
	}
	m.createField = postIdx

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("rm -rf $DECK_SESSION_DIR")})
	after := updated.(Model)
	if after.createPostDestroy != "rm -rf $DECK_SESSION_DIR" {
		t.Fatalf("createPostDestroy = %q, want the typed text", after.createPostDestroy)
	}

	view := after.createBody()
	if !strings.Contains(view, "rm -rf $DECK_SESSION_DIR") {
		t.Fatalf("createBody() does not render the typed Post-destroy value:\n%s", view)
	}
}

// TestCreateModalShellSubmitPassesPostDestroyThrough mirrors
// TestCreateModalShellSubmitPassesPreLaunchThrough (create_shell_pre_launch_test.go):
// submitting a shell create must hand the typed post_destroy hook to
// service.CreateShell via ShellCreateInput.PostDestroy, not drop it.
func TestCreateModalShellSubmitPassesPostDestroyThrough(t *testing.T) {
	before := newCreatingModel(t)
	before.createAgent = "shell"
	before.createPostDestroy = "echo teardown >> log"

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

	if got.PostDestroy != before.createPostDestroy {
		t.Fatalf("CreateShell received PostDestroy %q, want the modal's own field %q", got.PostDestroy, before.createPostDestroy)
	}
}

// TestCreateModalAgentSubmitPassesPostDestroyThrough covers the non-shell
// create path: submitting a real agent create must hand the typed
// post_destroy hook to service.AgentCreateInput.PostDestroy the same way
// the shell path does.
func TestCreateModalAgentSubmitPassesPostDestroyThrough(t *testing.T) {
	before := newCreatingModel(t)
	before.createAgent = "claude"
	before.createPostDestroy = "echo teardown >> log"

	var got service.AgentCreateInput
	before.createAgentSession = func(ctx context.Context, in service.AgentCreateInput) (store.Session, error) {
		got = in
		return store.Session{Name: in.Name}, nil
	}

	updated, cmd := before.Update(key("enter"))
	if after := updated.(Model); after.createError != "" {
		t.Fatalf("createError = %q, want none", after.createError)
	}
	if cmd == nil {
		t.Fatal("enter on a valid agent create issued no command")
	}
	cmd()

	if got.PostDestroy != before.createPostDestroy {
		t.Fatalf("CreateAgent received PostDestroy %q, want the modal's own field %q", got.PostDestroy, before.createPostDestroy)
	}
}
