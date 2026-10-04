package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/agent"
	"github.com/n-orlov/deck/internal/audit"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// preconditionService is a fully wired Service whose tmux binary is a shell
// script chosen per test: the mutators below are exercised through their real
// store, audit log and tmux client, only the tmux server's answers are scripted.
func preconditionService(t *testing.T, tmuxScript string) Service {
	t.Helper()
	home := t.TempDir()
	clock, err := config.NewClock("2025-01-02T03:04:05Z", "")
	if err != nil {
		t.Fatal(err)
	}
	paths := config.Paths{Home: home, LogDir: filepath.Join(home, "log"), StateDB: filepath.Join(home, "state.db")}
	db, err := store.Open(paths)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	logger, err := audit.New(paths, clock)
	if err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(home, "scripted-tmux")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n"+tmuxScript+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	registry := agent.NewRegistry()
	registry.Register(agent.NewClaude())
	registry.Register(agent.NewShell())
	return Service{
		Store: db, TMux: tmux.Client{Binary: script, Socket: "scripted-" + filepath.Base(home)},
		Audit: logger, Clock: clock, Agents: registry, DeckHome: home,
	}
}

const (
	tmuxAnswersEverything = `exit 0`
	tmuxRefusesEverything = `echo "boom: server on fire" >&2; exit 1`
	// has-session succeeds (the session is live) but every other command fails.
	tmuxLiveButRefusesWrites = `for a in "$@"; do case "$a" in has-session) exit 0;; esac; done
echo "boom: write refused" >&2; exit 1`
)

func seedSession(t *testing.T, svc Service, name, agentKind, status string) store.Session {
	t.Helper()
	session, err := svc.Store.CreateSession(context.Background(), store.CreateSessionInput{
		ID: "00000000-0000-4000-8000-0000000000" + strings.Repeat("a", 2), Name: name, CWD: t.TempDir(),
		Agent: agentKind, CapturedPath: "/bin", Status: status, StatusSource: "hook", StatusAt: 1, CreatedAt: 1,
	})
	if err != nil {
		t.Fatalf("seed session: %v", err)
	}
	return session
}

func wantErr(t *testing.T, what string, err error, substr string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: succeeded, want an error containing %q", what, substr)
	}
	if !strings.Contains(err.Error(), substr) {
		t.Fatalf("%s: error = %q, want it to contain %q", what, err, substr)
	}
}

// TestSessionMutatorsRefuseAnUnwiredService: every single-purpose session
// mutator names the missing dependency instead of dereferencing a nil store,
// clock or audit logger -- and does so before anything is written.
func TestSessionMutatorsRefuseAnUnwiredService(t *testing.T) {
	ctx := context.Background()
	var zero Service
	cases := []struct {
		name string
		call func() error
		want string
	}{
		{"SetSessionGroup", func() error { _, err := zero.SetSessionGroup(ctx, "id", 1); return err }, "requires a store and clock"},
		{"SetSessionEnv", func() error { _, err := zero.SetSessionEnv(ctx, "id", "K", "v"); return err }, "requires a store and clock"},
		{"SetLaunchInputs", func() error { _, err := zero.SetLaunchInputs(ctx, "id", "", "", nil, false); return err }, "requires a store and clock"},
		{"PinResume", func() error { _, err := zero.PinResume(ctx, "id"); return err }, "requires a store and clock"},
		{"SetResumeAuto", func() error { _, err := zero.SetResumeAuto(ctx, "id"); return err }, "requires a store and clock"},
		{"ArmFreshOnce", func() error { _, err := zero.ArmFreshOnce(ctx, "id"); return err }, "requires a store and clock"},
		{"Rename", func() error { _, err := zero.Rename(ctx, "id", "n"); return err }, "requires a store and clock"},
		{"SetPermissionProfile", func() error { _, err := zero.SetPermissionProfile(ctx, "id", "edits"); return err }, "requires a store, adapter registry, and clock"},
		{"Unarchive", func() error { _, err := zero.Unarchive(ctx, "id"); return err }, "requires store, audit logger, and clock"},
		{"Archive", func() error { _, err := zero.Archive(ctx, store.Session{ID: "id"}); return err }, "requires store, audit logger, and clock"},
		{"Kill", func() error { return zero.Kill(ctx, store.Session{ID: "id", Slug: "s"}) }, "requires store, audit logger, and clock"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { wantErr(t, tc.name, tc.call(), tc.want) })
	}
}

// TestSessionMutatorsRefuseABlankSessionID: an empty id is refused with the
// mutator's own message, never forwarded to the store as a no-op write.
func TestSessionMutatorsRefuseABlankSessionID(t *testing.T) {
	ctx := context.Background()
	svc := preconditionService(t, tmuxAnswersEverything)
	cases := []struct {
		name string
		call func() error
		want string
	}{
		{"SetSessionGroup", func() error { _, err := svc.SetSessionGroup(ctx, "", 1); return err }, "session id is required"},
		{"SetSessionEnv", func() error { _, err := svc.SetSessionEnv(ctx, "", "K", "v"); return err }, "session id and environment key are required"},
		{"SetSessionEnv blank key", func() error { _, err := svc.SetSessionEnv(ctx, "id", "", "v"); return err }, "session id and environment key are required"},
		{"SetLaunchInputs", func() error { _, err := svc.SetLaunchInputs(ctx, "", "", "", nil, false); return err }, "session id is required"},
		{"PinResume", func() error { _, err := svc.PinResume(ctx, ""); return err }, "session id is required"},
		{"SetResumeAuto", func() error { _, err := svc.SetResumeAuto(ctx, ""); return err }, "session id is required"},
		{"ArmFreshOnce", func() error { _, err := svc.ArmFreshOnce(ctx, ""); return err }, "session id is required"},
		{"Rename", func() error { _, err := svc.Rename(ctx, "", "n"); return err }, "session id is required"},
		{"SetPermissionProfile", func() error { _, err := svc.SetPermissionProfile(ctx, "", "edits"); return err }, "session id and permission profile are required"},
		{"SetPermissionProfile blank profile", func() error { _, err := svc.SetPermissionProfile(ctx, "id", ""); return err }, "session id and permission profile are required"},
		{"Unarchive", func() error { _, err := svc.Unarchive(ctx, ""); return err }, "durable session id"},
		{"Archive", func() error { _, err := svc.Archive(ctx, store.Session{}); return err }, "durable session id"},
		{"Kill", func() error { return svc.Kill(ctx, store.Session{ID: "id"}) }, "durable session id and slug"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { wantErr(t, tc.name, tc.call(), tc.want) })
	}
}

// TestSessionMutatorsReportAnUnknownSession: each mutator that pre-reads the
// row surfaces the store's not-found error wrapped with the id, and leaves the
// event log empty.
func TestSessionMutatorsReportAnUnknownSession(t *testing.T) {
	ctx := context.Background()
	svc := preconditionService(t, tmuxAnswersEverything)
	const id = "no-such-session"
	cases := []struct {
		name string
		call func() error
	}{
		{"SetSessionGroup", func() error { _, err := svc.SetSessionGroup(ctx, id, 1); return err }},
		{"SetSessionEnv", func() error { _, err := svc.SetSessionEnv(ctx, id, "K", "v"); return err }},
		{"SetLaunchInputs", func() error { _, err := svc.SetLaunchInputs(ctx, id, "", "", nil, false); return err }},
		{"PinResume", func() error { _, err := svc.PinResume(ctx, id); return err }},
		{"SetPermissionProfile", func() error { _, err := svc.SetPermissionProfile(ctx, id, "edits"); return err }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			wantErr(t, tc.name, err, `get session "no-such-session"`)
			wantErr(t, tc.name, err, "not found")
		})
	}
}

// TestPinResumeRefusesASessionWithoutAConversation: a row that has no
// conversation id yet has nothing to lock, so the lock is refused by name and
// the row keeps its automatic resume state.
func TestPinResumeRefusesASessionWithoutAConversation(t *testing.T) {
	svc := preconditionService(t, tmuxAnswersEverything)
	session := seedSession(t, svc, "no conversation", "claude", "stopped")

	_, err := svc.PinResume(context.Background(), session.ID)
	wantErr(t, "PinResume", err, `session "no conversation" has no conversation id to lock`)

	got, err := svc.Store.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ResumeState == "pinned" || got.ResumePin != "" {
		t.Fatalf("refused lock still changed the row: state=%q pin=%q", got.ResumeState, got.ResumePin)
	}
}

// TestSetPermissionProfileRefusesWhatTheAgentCannotDo: an unregistered agent
// kind, an adapter that declares no profiles (shell) and a profile the adapter
// does not know are three different refusals, each naming the agent, and none
// writes the row.
func TestSetPermissionProfileRefusesWhatTheAgentCannotDo(t *testing.T) {
	ctx := context.Background()
	svc := preconditionService(t, tmuxAnswersEverything)

	ghost := seedSession(t, svc, "ghost agent", "ghost", "stopped")
	_, err := svc.SetPermissionProfile(ctx, ghost.ID, "edits")
	wantErr(t, "unregistered agent", err, `agent "ghost" is not registered`)

	// The seeded id is fixed, so use a second service/store for the next rows.
	svc2 := preconditionService(t, tmuxAnswersEverything)
	shell := seedSession(t, svc2, "plain shell", "shell", "stopped")
	_, err = svc2.SetPermissionProfile(ctx, shell.ID, "edits")
	wantErr(t, "shell has no profiles", err, `agent "shell" has no permission profiles`)

	svc3 := preconditionService(t, tmuxAnswersEverything)
	claude := seedSession(t, svc3, "claude", "claude", "stopped")
	_, err = svc3.SetPermissionProfile(ctx, claude.ID, "no-such-profile")
	wantErr(t, "unknown profile", err, `agent "claude" does not support permission profile "no-such-profile"`)
	got, err := svc3.Store.GetSession(ctx, claude.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.PermissionProfile == "no-such-profile" {
		t.Fatalf("refused profile was written: %q", got.PermissionProfile)
	}
}

// TestSetSessionGroupMovesTheRowAndReturnsItReread: the returned session is
// the re-read row carrying the new group, a positive id is written verbatim,
// and an id <= 0 returns the row to the structural default (no group).
func TestSetSessionGroupMovesTheRowAndReturnsItReread(t *testing.T) {
	ctx := context.Background()
	svc := preconditionService(t, tmuxAnswersEverything)
	session := seedSession(t, svc, "mover", "claude", "stopped")

	moved, err := svc.SetSessionGroup(ctx, session.ID, 42)
	if err != nil {
		t.Fatalf("SetSessionGroup: %v", err)
	}
	if moved.GroupID == nil || *moved.GroupID != 42 {
		t.Fatalf("returned session group = %v, want 42", moved.GroupID)
	}
	stored, err := svc.Store.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.GroupID == nil || *stored.GroupID != 42 {
		t.Fatalf("stored group = %v, want 42", stored.GroupID)
	}

	back, err := svc.SetSessionGroup(ctx, session.ID, 0)
	if err != nil {
		t.Fatalf("move to default: %v", err)
	}
	if back.GroupID != nil {
		t.Fatalf("group after moving to default = %d, want none", *back.GroupID)
	}
}

// TestSetSessionEnvSurfacesTmuxFailures: the durable edit lands first; a tmux
// that cannot answer has-session is reported as a live-check failure, and a
// live session whose environment table refuses the mirror is reported as a
// mirror failure. Either way the stored value is the user's edit.
func TestSetSessionEnvSurfacesTmuxFailures(t *testing.T) {
	ctx := context.Background()

	svc := preconditionService(t, tmuxRefusesEverything)
	session := seedSession(t, svc, "env check", "claude", "running")
	_, err := svc.SetSessionEnv(ctx, session.ID, "FOO", "bar")
	wantErr(t, "has-session failing", err, `check live session "env check"`)

	live := preconditionService(t, tmuxLiveButRefusesWrites)
	row := seedSession(t, live, "env mirror", "claude", "running")
	_, err = live.SetSessionEnv(ctx, row.ID, "FOO", "bar")
	wantErr(t, "set-environment failing", err, `mirror environment for session "env mirror"`)
	got, err := live.Store.GetSession(ctx, row.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Env["FOO"] != "bar" {
		t.Fatalf("the durable env edit was lost when the mirror failed: %#v", got.Env)
	}
}

// TestKillAndArchiveReportEachFailingStep walks the teardown of a live row
// step by step: a tmux that cannot be asked, a tmux that refuses the kill,
// and a store that can no longer record the result -- each is reported
// naming the session, never swallowed.
func TestKillAndArchiveReportEachFailingStep(t *testing.T) {
	ctx := context.Background()

	t.Run("a stopped row is checked for a retained session first", func(t *testing.T) {
		svc := preconditionService(t, tmuxRefusesEverything)
		session := seedSession(t, svc, "retained", "claude", "stopped")
		wantErr(t, "Kill", svc.Kill(ctx, session), `check for a retained tmux session for "retained"`)
	})
	t.Run("kill refused by tmux", func(t *testing.T) {
		svc := preconditionService(t, tmuxRefusesEverything)
		session := seedSession(t, svc, "live", "claude", "running")
		wantErr(t, "Kill", svc.Kill(ctx, session), `kill tmux session "live"`)
		got, err := svc.Store.GetSession(ctx, session.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status != "running" {
			t.Fatalf("a kill tmux refused still recorded status %q", got.Status)
		}
	})
	t.Run("archive of a live row kill refused", func(t *testing.T) {
		svc := preconditionService(t, tmuxRefusesEverything)
		session := seedSession(t, svc, "live archive", "claude", "running")
		_, err := svc.Archive(ctx, session)
		wantErr(t, "Archive", err, `kill tmux session "live archive"`)
		got, _ := svc.Store.GetSession(ctx, session.ID)
		if got.ArchivedAt != 0 {
			t.Fatalf("an archive whose kill failed still archived the row")
		}
	})
	t.Run("archive of a live row without a slug", func(t *testing.T) {
		svc := preconditionService(t, tmuxAnswersEverything)
		_, err := svc.Archive(ctx, store.Session{ID: "x", Name: "slugless", Status: "running"})
		wantErr(t, "Archive", err, "requires a durable slug to kill a live pane")
	})
	t.Run("store closed after the kill", func(t *testing.T) {
		svc := preconditionService(t, tmuxAnswersEverything)
		session := seedSession(t, svc, "closed store", "claude", "running")
		if err := svc.Store.Close(); err != nil {
			t.Fatal(err)
		}
		wantErr(t, "Kill", svc.Kill(ctx, session), `record killed session "closed store"`)
		_, err := svc.Archive(ctx, session)
		wantErr(t, "Archive live", err, `record killed session "closed store"`)
		session.Status = "stopped"
		_, err = svc.Archive(ctx, session)
		wantErr(t, "Archive stopped", err, `archive session "closed store"`)
		_, err = svc.Unarchive(ctx, session.ID)
		wantErr(t, "Unarchive", err, `unarchive session`)
	})
}
