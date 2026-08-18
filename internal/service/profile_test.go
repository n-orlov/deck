package service

import (
	"context"
	"testing"
)

// TestSetPermissionProfilePersistsAndDoesNotRelaunch proves changing a
// session's permission profile persists the new value without touching the
// still-running pane: no additional "launch" audit record is written for it
// (task 020, SPEC §5/§8 — the change only ever applies on the session's
// next launch/restart).
func TestSetPermissionProfilePersistsAndDoesNotRelaunch(t *testing.T) {
	cwd := t.TempDir()
	stubExecutableOnPath(t, "claude")
	service, db, logger, _ := newAgentTestService(t, nil, "profile-switch-test")

	created, err := service.CreateAgent(context.Background(), AgentCreateInput{
		Name: "Claude: profile switch", CWD: cwd, Agent: "claude", PermissionProfile: "safe",
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	assertOneLaunchRecorded(t, logger.Path())

	updated, err := service.SetPermissionProfile(context.Background(), created.ID, "edits")
	if err != nil {
		t.Fatalf("set permission profile: %v", err)
	}
	if updated.PermissionProfile != "edits" {
		t.Fatalf("permission profile = %q, want edits", updated.PermissionProfile)
	}

	reloaded, err := db.GetSession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if reloaded.PermissionProfile != "edits" {
		t.Fatalf("persisted permission profile = %q, want edits", reloaded.PermissionProfile)
	}

	// The still-running pane must not have been re-launched: exactly the
	// one launch record from creation, nothing added by the profile switch.
	assertOneLaunchRecorded(t, logger.Path())
}

// TestSetPermissionProfileRefusesUnsupportedProfile proves an unsupported
// profile is refused with a specific error rather than silently degraded or
// persisted.
func TestSetPermissionProfileRefusesUnsupportedProfile(t *testing.T) {
	cwd := t.TempDir()
	stubExecutableOnPath(t, "pi")
	service, db, _, _ := newAgentTestService(t, nil, "profile-switch-unsupported")

	created, err := service.CreateAgent(context.Background(), AgentCreateInput{
		Name: "Pi: profile switch", CWD: cwd, Agent: "pi", PermissionProfile: "safe",
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	if _, err := service.SetPermissionProfile(context.Background(), created.ID, "plan"); err == nil {
		t.Fatal("SetPermissionProfile did not refuse a profile pi does not support")
	}

	reloaded, err := db.GetSession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if reloaded.PermissionProfile != "safe" {
		t.Fatalf("persisted permission profile changed to %q despite the refusal", reloaded.PermissionProfile)
	}
}
