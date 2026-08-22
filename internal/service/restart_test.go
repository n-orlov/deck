package service

import (
	"context"
	"strings"
	"testing"
)

// TestRestartKillsRelaunchesWithResumeArgvSameConversationIDAndClearsEnvDirty
// proves task 022's `R` end to end at the service level: it kills the live
// pane, relaunches with the adapter's RESUME argv (never Launch's), reuses
// the exact same conversation id (never a fresh one), carries whatever
// environment the row currently has persisted (including an edit made
// while the OLD pane was still running), and clears env_dirty back to
// false only once that relaunch has actually happened.
func TestRestartKillsRelaunchesWithResumeArgvSameConversationIDAndClearsEnvDirty(t *testing.T) {
	cwd := t.TempDir()
	stubExecutableOnPath(t, "claude")
	service, db, logger, socket := newAgentTestService(t, nil, "restart-test")

	created, err := service.CreateAgent(context.Background(), AgentCreateInput{
		Name: "Claude: restart", CWD: cwd, Agent: "claude", PermissionProfile: "edits",
		Env: map[string]string{"RESTART_ENV_KEY": "before-restart"},
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if created.Status == "stopped" {
		t.Fatalf("freshly created session status = %q, want non-stopped so it can be restarted", created.Status)
	}

	updated, err := service.SetSessionEnv(context.Background(), created.ID, "RESTART_ENV_KEY", "after-restart")
	if err != nil {
		t.Fatalf("set session env: %v", err)
	}
	if !updated.EnvDirty {
		t.Fatalf("env_dirty = false after an edit, want true before restart")
	}

	restarted, outcome, err := service.Restart(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("restart: %v", err)
	}
	if outcome != ResumeStarted {
		t.Fatalf("outcome = %v, want ResumeStarted", outcome)
	}
	if restarted.Status != "starting" {
		t.Fatalf("row status = %q, want starting", restarted.Status)
	}
	if restarted.ConversationID != created.ConversationID {
		t.Fatalf("restarted conversation id = %q, want the same as before restart %q", restarted.ConversationID, created.ConversationID)
	}
	if restarted.EnvDirty {
		t.Fatalf("env_dirty = true after a successful restart, want false")
	}

	row, err := db.GetSession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if row.EnvDirty {
		t.Fatal("persisted env_dirty remains true after restart")
	}
	if row.ConversationID != created.ConversationID {
		t.Fatalf("persisted conversation id = %q, want %q", row.ConversationID, created.ConversationID)
	}
	if row.Env["RESTART_ENV_KEY"] != "after-restart" {
		t.Fatalf("persisted env[RESTART_ENV_KEY] = %q, want after-restart", row.Env["RESTART_ENV_KEY"])
	}

	live, err := service.TMux.List(context.Background())
	if err != nil || len(live) != 1 {
		t.Fatalf("live tmux sessions after restart = %#v, %v", live, err)
	}

	records := auditRecords(t, logger.Path())
	var argv []string
	launches := 0
	for _, record := range records {
		if record["event"] == "launch" {
			launches++
			argv = jsonStrings(record["argv"])
		}
	}
	if launches != 2 {
		t.Fatalf("launch records = %d, want 2 (create + restart)", launches)
	}
	wantPrefix := []string{"claude", "--resume", created.ConversationID}
	if len(argv) < len(wantPrefix) || strings.Join(argv[:len(wantPrefix)], "\x00") != strings.Join(wantPrefix, "\x00") {
		t.Fatalf("restart argv = %#v, want to start with %#v (resume, never a fresh conversation)", argv, wantPrefix)
	}
	for _, token := range argv {
		if strings.Contains(token, "--session-id") || strings.Contains(token, "--continue") {
			t.Fatalf("restart argv = %#v must not reuse launch/--continue forms", argv)
		}
	}
	assertTMuxEnvironment(t, socket, restarted.Slug, "RESTART_ENV_KEY", "after-restart")
}

// TestRestartRefusesAnAlreadyStoppedSession proves Restart never
// masquerades as a first launch: a row with no live pane to kill must be
// resumed via Resume/`r` instead.
func TestRestartRefusesAnAlreadyStoppedSession(t *testing.T) {
	cwd := t.TempDir()
	stubExecutableOnPath(t, "claude")
	service, db, _, _ := newAgentTestService(t, nil, "restart-stopped-test")

	created, err := service.CreateAgent(context.Background(), AgentCreateInput{
		Name: "Claude: restart stopped", CWD: cwd, Agent: "claude", PermissionProfile: "edits",
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	stopSession(t, db, created.ID)

	_, outcome, err := service.Restart(context.Background(), created.ID)
	if err == nil {
		t.Fatalf("expected an error restarting an already-stopped session")
	}
	if outcome != ResumeNotLeasable {
		t.Fatalf("outcome = %v, want ResumeNotLeasable", outcome)
	}
}
