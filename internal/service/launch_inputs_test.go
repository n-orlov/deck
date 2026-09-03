package service

import (
	"context"
	"testing"
)

// TestSetLaunchInputsPersistsAllFourAndMarksLaunchDirty proves task 023's
// service half -- the one dependency the `i` dialog's launch-inputs editor
// needs to be able to edit anything at all in the shipped binary (SPEC
// §6.2/§11.4, PRD R108). One call writes all four editable launch inputs
// and marks the row launch_dirty; the returned session and the re-read row
// agree; and none of the four inputs the editor must never touch (agent,
// cwd, slug, captured_path) moves.
func TestSetLaunchInputsPersistsAllFourAndMarksLaunchDirty(t *testing.T) {
	svc := newArchiveTestService(t)
	cwd := t.TempDir()
	created, err := svc.CreateShell(context.Background(), ShellCreateInput{Name: "launch-inputs-edit", CWD: cwd})
	if err != nil {
		t.Fatalf("create shell: %v", err)
	}
	if created.LaunchDirty {
		t.Fatal("a freshly created session is already launch_dirty; it must not be")
	}
	// Compare identity columns against the persisted row as it stands just
	// before the edit, not against CreateShell's own return value:
	// captured_path is written by creation itself after that value is built,
	// so the returned struct legitimately does not carry it yet.
	before, err := svc.Store.GetSession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get session before edit: %v", err)
	}

	updated, err := svc.SetLaunchInputs(context.Background(), created.ID, "echo pre-launch", "echo post-destroy", []string{"--flag", "value"}, true)
	if err != nil {
		t.Fatalf("set launch inputs: %v", err)
	}
	if updated.PreLaunch != "echo pre-launch" {
		t.Fatalf("pre_launch = %q, want %q", updated.PreLaunch, "echo pre-launch")
	}
	if updated.PostDestroy != "echo post-destroy" {
		t.Fatalf("post_destroy = %q, want %q", updated.PostDestroy, "echo post-destroy")
	}
	if len(updated.LaunchArgs) != 2 || updated.LaunchArgs[0] != "--flag" || updated.LaunchArgs[1] != "value" {
		t.Fatalf("launch_args = %#v, want [--flag value]", updated.LaunchArgs)
	}
	if !updated.LoginShell {
		t.Fatal("login_shell = false, want true")
	}
	if !updated.LaunchDirty {
		t.Fatal("launch_dirty = false after an edit; only a relaunch may clear it")
	}

	row, err := svc.Store.GetSession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if row.PreLaunch != updated.PreLaunch || row.PostDestroy != updated.PostDestroy || row.LoginShell != updated.LoginShell || !row.LaunchDirty {
		t.Fatalf("re-read row disagrees with the returned session: %+v vs %+v", row, updated)
	}
	if row.Agent != before.Agent || row.CWD != before.CWD || row.Slug != before.Slug || row.CapturedPath != before.CapturedPath {
		t.Fatalf("an identity column moved: agent %q/%q cwd %q/%q slug %q/%q captured_path %q/%q",
			row.Agent, before.Agent, row.CWD, before.CWD, row.Slug, before.Slug, row.CapturedPath, before.CapturedPath)
	}
}

// TestSetLaunchInputsRejectsBlankAndUnknownSession proves the guard
// clauses: a blank id and an id no row holds are both refused, and the
// refusal happens before anything is written.
func TestSetLaunchInputsRejectsBlankAndUnknownSession(t *testing.T) {
	svc := newArchiveTestService(t)
	if _, err := svc.SetLaunchInputs(context.Background(), "", "echo pre", "", nil, false); err == nil {
		t.Fatal("a blank session id was accepted; want an error")
	}
	if _, err := svc.SetLaunchInputs(context.Background(), "no-such-session", "echo pre", "", nil, false); err == nil {
		t.Fatal("an unknown session id was accepted; want an error")
	}
}
