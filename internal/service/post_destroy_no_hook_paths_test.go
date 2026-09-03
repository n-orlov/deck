package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestKillReapSweepAndNameReuseReapNeverRunPostDestroy proves the other
// half of SPEC §9.2's teardown contract task 013-015 already exercise for
// `A`/`dd`: "post_destroy never runs on x or on any reap path" (the
// standing rule this plan carries forward from the PRD). Each of the four
// subtests below configures the SAME kind of session-scoped PostDestroy
// hook task 014 proves DOES fire on `dd` -- a shell line that appends a
// line to a file this test owns -- and then drives one of the four named
// non-hook paths, asserting the hook's own artefact file is still absent
// afterwards. A hook that fired would leave the file behind; there is no
// other way for the file to appear, so its absence is direct evidence the
// path never ran it, not merely that the test never looked.
func TestKillReapSweepAndNameReuseReapNeverRunPostDestroy(t *testing.T) {
	t.Run("Kill", testKillRunsNoPostDestroy)
	t.Run("Reap", testReapRunsNoPostDestroy)
	t.Run("StoreOpenTombstoneSweep", testStoreOpenTombstoneSweepRunsNoPostDestroy)
	t.Run("CreateReapsATombstoneToReuseItsName", testNameReuseReapRunsNoPostDestroy)
}

// hookArtefactLine is the same "append a line naming the teardown kind and
// session id" hook body task 014's tests use to prove a hook DID run --
// reused here unchanged so a hook that fired here would look exactly as
// unmistakable as it does there.
func hookArtefactLine(artefact string) string {
	return "echo \"$DECK_TEARDOWN_KIND:$DECK_SESSION_ID\" >> " + artefact
}

// assertNoPostDestroyArtefact fails the test if path exists at all -- the
// hook line above never does anything else, so any existing file (let
// alone a non-empty one) means the hook ran.
func assertNoPostDestroyArtefact(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("post_destroy artefact %q: stat = %v, want IsNotExist (post_destroy must never run on this path)", path, err)
	}
}

func testKillRunsNoPostDestroy(t *testing.T) {
	svc := newTombstoneTestService(t)
	artefact := filepath.Join(t.TempDir(), "kill-hook.out")
	session, err := svc.CreateShell(context.Background(), ShellCreateInput{
		Name: "kill-runs-no-hook", CWD: t.TempDir(),
		PostDestroy: hookArtefactLine(artefact),
	})
	if err != nil {
		t.Fatalf("create shell: %v", err)
	}
	if err := svc.Kill(context.Background(), session); err != nil {
		t.Fatalf("kill: %v", err)
	}
	assertNoPostDestroyArtefact(t, artefact)
}

func testReapRunsNoPostDestroy(t *testing.T) {
	svc := newTombstoneTestService(t)
	artefact := filepath.Join(t.TempDir(), "reap-hook.out")
	session, err := svc.CreateShell(context.Background(), ShellCreateInput{
		Name: "reap-runs-no-hook", CWD: t.TempDir(),
		PostDestroy: hookArtefactLine(artefact),
	})
	if err != nil {
		t.Fatalf("create shell: %v", err)
	}
	// Delete's own post_destroy run (task 013) is expected to fire once,
	// exactly as task 014 proves on `dd`; it is Reap below -- the grace-
	// window expiry that follows a delete, not the delete itself -- whose
	// silence this subtest is about, so the artefact is deliberately
	// cleared right back to absent before Reap runs.
	if _, err := svc.Delete(context.Background(), session); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := os.Remove(artefact); err != nil {
		t.Fatalf("clear delete's own post_destroy artefact before reaping: %v", err)
	}
	if err := svc.Reap(context.Background(), session.ID); err != nil {
		t.Fatalf("reap: %v", err)
	}
	assertNoPostDestroyArtefact(t, artefact)
}

func testStoreOpenTombstoneSweepRunsNoPostDestroy(t *testing.T) {
	svc := newTombstoneTestService(t)
	artefact := filepath.Join(t.TempDir(), "sweep-hook.out")
	session, err := svc.CreateShell(context.Background(), ShellCreateInput{
		Name: "sweep-runs-no-hook", CWD: t.TempDir(),
		PostDestroy: hookArtefactLine(artefact),
	})
	if err != nil {
		t.Fatalf("create shell: %v", err)
	}
	if _, err := svc.Delete(context.Background(), session); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := os.Remove(artefact); err != nil {
		t.Fatalf("clear delete's own post_destroy artefact before sweeping: %v", err)
	}
	// deck's own pre-first-frame store-open call site drives exactly this
	// store method (cmd/deck/main.go); deleteGrace=0 and a far-future now
	// puts the just-tombstoned row past cutoff on this, its very first
	// (unthrottled) call.
	now := time.Now().Add(365 * 24 * time.Hour).UnixMilli()
	if _, err := svc.Store.SweepTombstones(context.Background(), 0, now); err != nil {
		t.Fatalf("SweepTombstones: %v", err)
	}
	var count int
	if err := svc.Store.DB().QueryRowContext(context.Background(),
		`SELECT count(*) FROM sessions WHERE id = ?`, session.ID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("session row count after sweep = %d, %v, want 0 (sweep must have actually reaped it for this subtest to mean anything)", count, err)
	}
	assertNoPostDestroyArtefact(t, artefact)
}

func testNameReuseReapRunsNoPostDestroy(t *testing.T) {
	svc := newTombstoneTestService(t)
	ctx := context.Background()
	artefact := filepath.Join(t.TempDir(), "name-reuse-hook.out")
	first, err := svc.CreateShell(ctx, ShellCreateInput{
		Name: "reused-name-runs-no-hook", CWD: t.TempDir(),
		PostDestroy: hookArtefactLine(artefact),
	})
	if err != nil {
		t.Fatalf("create first shell: %v", err)
	}
	if _, err := svc.Delete(ctx, first); err != nil {
		t.Fatalf("delete first: %v", err)
	}
	if err := os.Remove(artefact); err != nil {
		t.Fatalf("clear delete's own post_destroy artefact before reusing the name: %v", err)
	}
	second, err := svc.CreateShell(ctx, ShellCreateInput{Name: "reused-name-runs-no-hook", CWD: t.TempDir()})
	if err != nil {
		t.Fatalf("CreateShell reusing a tombstoned name: %v, want success", err)
	}
	if second.ID == first.ID {
		t.Fatalf("reuse returned the old row: id %q", second.ID)
	}
	exists, err := svc.Store.SessionRowExists(ctx, first.ID)
	if err != nil || exists {
		t.Fatalf("tombstoned holder row after reuse: exists = %v, %v, want gone (name reuse must have actually reaped it for this subtest to mean anything)", exists, err)
	}
	assertNoPostDestroyArtefact(t, artefact)
}
