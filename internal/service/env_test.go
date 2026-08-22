package service

import (
	"context"
	"testing"
)

// TestSetSessionEnvPersistsMarksDirtyAndMirrorsToTmux proves task 021's
// write path end to end at the service level: editing a running session's
// env writes the session's own env map, sets env_dirty, and mirrors the
// key into tmux's own environment table for the session (asserted against
// real tmux via show-environment, the same helper agent_test.go already
// uses) -- never into config.toml, never into any other session's row.
func TestSetSessionEnvPersistsMarksDirtyAndMirrorsToTmux(t *testing.T) {
	cwd := t.TempDir()
	service, db, _, socket := newAgentTestService(t, nil, "set-session-env-test")

	session, err := service.CreateAgent(context.Background(), AgentCreateInput{
		Name: "env target", CWD: cwd, Agent: "claude", PermissionProfile: "safe",
		Env: map[string]string{"ENV_EDIT_KEY": "before"},
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if session.EnvDirty {
		t.Fatalf("freshly created session is env_dirty; want false")
	}

	updated, err := service.SetSessionEnv(context.Background(), session.ID, "ENV_EDIT_KEY", "after")
	if err != nil {
		t.Fatalf("set session env: %v", err)
	}
	if updated.Env["ENV_EDIT_KEY"] != "after" {
		t.Fatalf("returned session env[ENV_EDIT_KEY] = %q, want %q", updated.Env["ENV_EDIT_KEY"], "after")
	}
	if !updated.EnvDirty {
		t.Fatalf("returned session env_dirty = false, want true after an edit")
	}

	reread, err := db.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if reread.Env["ENV_EDIT_KEY"] != "after" || !reread.EnvDirty {
		t.Fatalf("reread session = %+v, want env[ENV_EDIT_KEY]=after and env_dirty=true", reread)
	}

	assertTMuxEnvironment(t, socket, session.Slug, "ENV_EDIT_KEY", "after")
}

// TestSetSessionEnvOnANewKeyAddsItRatherThanRefusing proves an edit is not
// limited to keys the session was created with: a key that exists only in
// config.toml's [env] table (never the session's own env map) still ends
// up in the session's own highest-priority layer once edited here.
func TestSetSessionEnvOnANewKeyAddsItRatherThanRefusing(t *testing.T) {
	cwd := t.TempDir()
	service, _, _, _ := newAgentTestService(t, map[string]string{"CONFIG_ONLY_KEY": "config-value"}, "set-session-env-new-key")

	session, err := service.CreateAgent(context.Background(), AgentCreateInput{
		Name: "env new key", CWD: cwd, Agent: "claude", PermissionProfile: "safe",
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if _, present := session.Env["CONFIG_ONLY_KEY"]; present {
		t.Fatalf("session env unexpectedly already has CONFIG_ONLY_KEY before any edit")
	}

	updated, err := service.SetSessionEnv(context.Background(), session.ID, "CONFIG_ONLY_KEY", "session-override")
	if err != nil {
		t.Fatalf("set session env: %v", err)
	}
	if updated.Env["CONFIG_ONLY_KEY"] != "session-override" {
		t.Fatalf("session env[CONFIG_ONLY_KEY] = %q, want %q", updated.Env["CONFIG_ONLY_KEY"], "session-override")
	}
}
