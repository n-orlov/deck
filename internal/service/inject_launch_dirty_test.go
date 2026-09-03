package service

import (
	"context"
	"testing"
	"time"
)

// TestInjectEnvClearsEnvDirtyOnlyAndLeavesLaunchDirtySet proves task 021's
// InjectEnv half: on a row that starts with BOTH env_dirty and
// launch_dirty set, a successful injection clears env_dirty (the keys were
// genuinely applied to the live pane's process) but never touches
// launch_dirty, since InjectEnv never relaunches the pane and so never
// carries a pending pre_launch/post_destroy/launch_args/login_shell edit
// anywhere -- only a Restart (`R`) does that.
func TestInjectEnvClearsEnvDirtyOnlyAndLeavesLaunchDirtySet(t *testing.T) {
	cwd := t.TempDir()
	service, db, _, socket := newAgentTestService(t, nil, "inject-launch-dirty-test")

	created, err := service.CreateShell(context.Background(), ShellCreateInput{Name: "inject-both-dirty", CWD: cwd})
	if err != nil {
		t.Fatalf("create shell: %v", err)
	}
	if _, found := waitForPaneProcessEnvironmentKey(t, socket, created.Slug, "PATH", 5*time.Second); !found {
		t.Fatalf("pane process never reported a PATH environment key")
	}

	if _, err := service.SetSessionEnv(context.Background(), created.ID, "INJECT_BOTH_DIRTY_KEY", "v1"); err != nil {
		t.Fatalf("set session env: %v", err)
	}
	if err := db.SetLaunchInputs(context.Background(), created.ID, "echo pre", "", nil, false, "user", 5); err != nil {
		t.Fatalf("set launch inputs: %v", err)
	}

	dirty, err := db.GetSession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if !dirty.EnvDirty || !dirty.LaunchDirty {
		t.Fatalf("row before inject = env_dirty %v launch_dirty %v, want both true", dirty.EnvDirty, dirty.LaunchDirty)
	}

	returned, keys, err := service.InjectEnv(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("inject env: %v", err)
	}
	if len(keys) != 1 || keys[0] != "INJECT_BOTH_DIRTY_KEY" {
		t.Fatalf("injected keys = %#v, want [INJECT_BOTH_DIRTY_KEY]", keys)
	}
	if returned.EnvDirty {
		t.Fatal("InjectEnv's own returned session has env_dirty = true, want false")
	}
	if !returned.LaunchDirty {
		t.Fatal("InjectEnv's own returned session lost launch_dirty, want it to stay true")
	}

	row, err := db.GetSession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if row.EnvDirty {
		t.Fatal("persisted env_dirty remains true after inject")
	}
	if !row.LaunchDirty {
		t.Fatal("persisted launch_dirty was cleared by inject, want it to stay true (only Restart clears it)")
	}
}
