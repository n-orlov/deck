package service

import (
	"context"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// TestCreateRefusesIncompleteInputsBeforeWritingAnything: an unwired service,
// a blank name or cwd, an unregistered agent kind, a relative user shell and
// an empty PATH are each refused by name, and none leaves a row behind.
func TestCreateRefusesIncompleteInputsBeforeWritingAnything(t *testing.T) {
	ctx := context.Background()

	var zero Service
	_, err := zero.CreateAgent(ctx, AgentCreateInput{Name: "n", CWD: "/", Agent: "claude"})
	wantErr(t, "CreateAgent unwired", err, "agent creation requires store, audit logger, clock, id generator, and adapter registry")
	_, err = zero.CreateShell(ctx, ShellCreateInput{Name: "n", CWD: "/"})
	wantErr(t, "CreateShell unwired", err, "shell creation requires store, audit logger, clock, and id generator")

	svc := preconditionService(t, tmuxAnswersEverything)
	svc.IDs = config.NewIDGenerator("create-errors")
	svc.Shell = "/bin/sh"

	_, err = svc.CreateAgent(ctx, AgentCreateInput{Name: "", CWD: "/tmp", Agent: "claude"})
	wantErr(t, "CreateAgent blank name", err, "agent session name and working directory are required")
	_, err = svc.CreateAgent(ctx, AgentCreateInput{Name: "n", CWD: "", Agent: "claude"})
	wantErr(t, "CreateAgent blank cwd", err, "agent session name and working directory are required")
	_, err = svc.CreateAgent(ctx, AgentCreateInput{Name: "n", CWD: "/tmp", Agent: "ghost"})
	wantErr(t, "CreateAgent unknown kind", err, `unknown agent kind "ghost"`)

	_, err = svc.CreateShell(ctx, ShellCreateInput{Name: "", CWD: "/tmp"})
	wantErr(t, "CreateShell blank name", err, "shell session name and working directory are required")
	_, err = svc.CreateShell(ctx, ShellCreateInput{Name: "n", CWD: ""})
	wantErr(t, "CreateShell blank cwd", err, "shell session name and working directory are required")

	relative := svc
	relative.Shell = "bash"
	_, err = relative.CreateShell(ctx, ShellCreateInput{Name: "n", CWD: "/tmp"})
	wantErr(t, "CreateShell relative shell", err, `user shell "bash" must be an absolute path`)

	t.Setenv("PATH", "")
	_, err = svc.CreateShell(ctx, ShellCreateInput{Name: "n", CWD: "/tmp"})
	wantErr(t, "CreateShell empty PATH", err, "PATH is required to create a shell session")
	_, err = svc.CreateAgent(ctx, AgentCreateInput{Name: "n", CWD: "/tmp", Agent: "claude"})
	wantErr(t, "CreateAgent empty PATH", err, "PATH is required to create an agent session")

	rows, err := svc.Store.ListSessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("refused creates left %d row(s) behind: %#v", len(rows), rows)
	}
}

// TestCreateWhoseTmuxRefusesToLaunchLeavesAnErrorRow: when tmux cannot create
// the pane, the durable row is kept (the user can see and delete it) in
// status "error" with the launch failure as its reason, and the error names
// the session -- for both the shell and the agent launch paths.
func TestCreateWhoseTmuxRefusesToLaunchLeavesAnErrorRow(t *testing.T) {
	ctx := context.Background()
	stubExecutableOnPath(t, "claude")
	svc := preconditionService(t, tmuxRefusesEverything)
	svc.IDs = config.NewIDGenerator("launch-refused")
	svc.Shell = "/bin/sh"

	// Instrumentation needs an absolute deck executable and a deck home; a
	// service missing either fails the launch with a row left in error.
	noExe := svc
	noHome := svc
	noHome.DeckExecutable = "/usr/local/bin/deck"
	noHome.DeckHome = ""
	for name, tc := range map[string]struct {
		svc  Service
		want string
	}{
		"relative deck executable": {noExe, "deck executable for instrumentation must be absolute"},
		"no deck home":             {noHome, "deck home for instrumentation is required"},
	} {
		session, err := tc.svc.CreateAgent(ctx, AgentCreateInput{Name: "instr " + name, CWD: t.TempDir(), Agent: "claude", PermissionProfile: "edits"})
		wantErr(t, name, err, `instrument agent session "instr `+name+`"`)
		wantErr(t, name, err, tc.want)
		if stored, getErr := svc.Store.GetSession(ctx, session.ID); getErr != nil || stored.Status != "error" {
			t.Fatalf("%s: stored row = %+v, %v; want status error", name, stored.Status, getErr)
		}
	}
	svc.DeckExecutable = "/usr/local/bin/deck"

	cases := []struct {
		name string
		want string
		call func() (store.Session, error)
	}{
		{"shell", `launch shell session "no pane shell"`, func() (store.Session, error) {
			return svc.CreateShell(ctx, ShellCreateInput{Name: "no pane shell", CWD: t.TempDir()})
		}},
		{"agent", `launch agent session "no pane agent"`, func() (store.Session, error) {
			return svc.CreateAgent(ctx, AgentCreateInput{Name: "no pane agent", CWD: t.TempDir(), Agent: "claude", PermissionProfile: "edits"})
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			session, err := tc.call()
			wantErr(t, "create", err, tc.want)
			if session.ID == "" {
				t.Fatalf("the failed launch returned no row to point the user at")
			}
			stored, getErr := svc.Store.GetSession(ctx, session.ID)
			if getErr != nil {
				t.Fatalf("the failed launch's row was not kept: %v", getErr)
			}
			if stored.Status != "error" {
				t.Fatalf("status after a refused launch = %q, want error", stored.Status)
			}
		})
	}
}
