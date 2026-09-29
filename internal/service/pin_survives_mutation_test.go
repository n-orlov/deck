package service

import (
	"context"
	"testing"

	"github.com/n-orlov/deck/internal/store"
)

// TestPinSurvivesEveryMutation proves task 004 (phase 4f sidebar pins,
// SPEC §11.3): pinned_at is UI-only sidebar-ordering state, set ONLY by
// SetSessionsPinned (task 003), and every OTHER row mutation -- whether it
// lives in the service layer (kill/resume/restart) or is a plain store
// mutator (rename, group move, archive/unarchive, the `dd` soft-delete/
// restore pair, and a status transition) -- must leave it byte-for-byte
// unchanged. Each sub-test builds its own fresh session so the mutations
// under test never interact with each other's fixture state.
func TestPinSurvivesEveryMutation(t *testing.T) {
	const pinnedAt = int64(424242)

	// newPinnedFixture creates a brand-new shell session (no external agent
	// binary required, so it works uniformly for every sub-test including
	// the ones that relaunch a pane) and pins it, asserting the pin itself
	// took before the sub-test's own mutation runs.
	newPinnedFixture := func(t *testing.T, seed string) (Service, *store.Store, store.Session) {
		t.Helper()
		cwd := t.TempDir()
		service, db, _, _ := newAgentTestService(t, nil, "pin-survives-"+seed)
		created, err := service.CreateShell(context.Background(), ShellCreateInput{Name: seed, CWD: cwd})
		if err != nil {
			t.Fatalf("create shell: %v", err)
		}
		if err := db.SetSessionsPinned(context.Background(), []string{created.ID}, true, pinnedAt); err != nil {
			t.Fatalf("pin session: %v", err)
		}
		row, err := db.GetSession(context.Background(), created.ID)
		if err != nil {
			t.Fatalf("get session after pin: %v", err)
		}
		if row.PinnedAt != pinnedAt {
			t.Fatalf("pinned_at right after pinning = %d, want %d", row.PinnedAt, pinnedAt)
		}
		return service, db, created
	}

	assertStillPinned := func(t *testing.T, db *store.Store, sessionID string) {
		t.Helper()
		row, err := db.GetSession(context.Background(), sessionID)
		if err != nil {
			t.Fatalf("get session: %v", err)
		}
		if row.PinnedAt != pinnedAt {
			t.Fatalf("pinned_at after mutation = %d, want unchanged %d", row.PinnedAt, pinnedAt)
		}
	}

	t.Run("kill", func(t *testing.T) {
		service, db, created := newPinnedFixture(t, "kill")
		if err := service.Kill(context.Background(), created); err != nil {
			t.Fatalf("kill: %v", err)
		}
		assertStillPinned(t, db, created.ID)
	})

	t.Run("resume", func(t *testing.T) {
		service, db, created := newPinnedFixture(t, "resume")
		if err := service.TMux.Kill(context.Background(), created.Slug); err != nil {
			t.Fatalf("kill pane before resume: %v", err)
		}
		stopSession(t, db, created.ID)
		_, outcome, err := service.Resume(context.Background(), created.ID)
		if err != nil {
			t.Fatalf("resume: %v", err)
		}
		if outcome != ResumeStarted {
			t.Fatalf("resume outcome = %v, want ResumeStarted", outcome)
		}
		assertStillPinned(t, db, created.ID)
	})

	t.Run("restart", func(t *testing.T) {
		service, db, created := newPinnedFixture(t, "restart")
		_, outcome, err := service.Restart(context.Background(), created.ID)
		if err != nil {
			t.Fatalf("restart: %v", err)
		}
		if outcome != ResumeStarted {
			t.Fatalf("restart outcome = %v, want ResumeStarted", outcome)
		}
		assertStillPinned(t, db, created.ID)
	})

	t.Run("rename", func(t *testing.T) {
		_, db, created := newPinnedFixture(t, "rename")
		if err := db.RenameSession(context.Background(), created.ID, "renamed-while-pinned", "user", 100); err != nil {
			t.Fatalf("rename session: %v", err)
		}
		assertStillPinned(t, db, created.ID)
	})

	t.Run("group_move", func(t *testing.T) {
		_, db, created := newPinnedFixture(t, "group-move")
		group, err := db.CreateGroup(context.Background(), "pin-survives-group")
		if err != nil {
			t.Fatalf("create group: %v", err)
		}
		if err := db.SetSessionGroup(context.Background(), created.ID, group.ID, "user", 100); err != nil {
			t.Fatalf("set session group: %v", err)
		}
		assertStillPinned(t, db, created.ID)
	})

	t.Run("archive", func(t *testing.T) {
		service, db, created := newPinnedFixture(t, "archive")
		if err := service.Kill(context.Background(), created); err != nil {
			t.Fatalf("kill before archive: %v", err)
		}
		if err := db.ArchiveSession(context.Background(), created.ID, 100); err != nil {
			t.Fatalf("archive session: %v", err)
		}
		assertStillPinned(t, db, created.ID)
	})

	t.Run("unarchive", func(t *testing.T) {
		service, db, created := newPinnedFixture(t, "unarchive")
		if err := service.Kill(context.Background(), created); err != nil {
			t.Fatalf("kill before archive: %v", err)
		}
		if err := db.ArchiveSession(context.Background(), created.ID, 100); err != nil {
			t.Fatalf("archive session: %v", err)
		}
		if err := db.UnarchiveSession(context.Background(), created.ID, 200); err != nil {
			t.Fatalf("unarchive session: %v", err)
		}
		assertStillPinned(t, db, created.ID)
	})

	t.Run("dd_soft_delete", func(t *testing.T) {
		_, db, created := newPinnedFixture(t, "soft-delete")
		if err := db.SoftDeleteSession(context.Background(), created.ID, 100); err != nil {
			t.Fatalf("soft delete session: %v", err)
		}
		assertStillPinned(t, db, created.ID)
	})

	t.Run("restore_after_dd", func(t *testing.T) {
		_, db, created := newPinnedFixture(t, "restore")
		if err := db.SoftDeleteSession(context.Background(), created.ID, 100); err != nil {
			t.Fatalf("soft delete session before restore: %v", err)
		}
		if err := db.RestoreSession(context.Background(), created.ID, 200); err != nil {
			t.Fatalf("restore session: %v", err)
		}
		assertStillPinned(t, db, created.ID)
	})

	t.Run("status_transitions", func(t *testing.T) {
		_, db, created := newPinnedFixture(t, "status")
		at := int64(100)
		for _, status := range []string{"waiting", "running", "idle", "stopped", "error"} {
			at++
			if err := db.UpdateSessionStatus(context.Background(), store.StatusUpdateInput{
				SessionID: created.ID, Status: status, Source: "user", At: at,
			}); err != nil {
				t.Fatalf("update session status to %q: %v", status, err)
			}
			assertStillPinned(t, db, created.ID)
		}
	})
}
