package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/audit"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// TestReconcileLosesInterleavedStatusWriteDuringShellPromotion pins F31: the
// shell-liveness promotion at reconcile.go's `EventKind: "tmux.shell_live"`
// site takes its status verdict from the in-memory row `Store.ListSessions`
// snapshotted *before* `TMux.List` ran, and writes "running"/"tmux"
// unconditionally -- with no `AllowedCurrentStatuses` guard -- however stale
// that snapshot has become by the time the write actually lands. This test
// forces a concrete writer (an "error" status, as a hook might report a
// crash) to land in the gap between the snapshot and the promotion write,
// using only a test-side seam: a fake tmux script whose `list-sessions`
// blocks on a rendezvous file until released, which brackets exactly that
// gap because `Reconcile` calls `Store.ListSessions` and only then
// `TMux.List` (which shells out to this fake). The interleaved "error"
// verdict must survive; today it does not -- the promotion clobbers it back
// to "running"/"tmux", which is the lost update this test pins red until
// task 007 lands the guard.
func TestReconcileLosesInterleavedStatusWriteDuringShellPromotion(t *testing.T) {
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

	started := filepath.Join(home, "list-sessions.started")
	release := filepath.Join(home, "list-sessions.release")
	logPath := filepath.Join(home, "tmux-calls.log")
	script := filepath.Join(home, "fake-tmux")

	// The row is created with status "starting" sourced from "tmux" -- not
	// "user" -- so the reconcile loop's own `terminal` short-circuit (a
	// `starting`/`user` row is a resume-in-flight window it deliberately
	// skips) does not intercept it before reaching the shell-liveness
	// promotion branch this test targets (see reconcile.go's
	// `session.Agent == "shell" && session.Status == "starting"` branch).
	session, err := db.CreateSession(context.Background(), store.CreateSessionInput{
		ID: "00000000-0000-4000-8000-000000000051", Name: "shell about to be promoted", CWD: t.TempDir(),
		Agent: "shell", CapturedPath: "/bin/sh", Status: "starting", StatusSource: "tmux", StatusAt: 1, CreatedAt: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	// The fake tmux `list-sessions` blocks on `release` so the test can write
	// its own interleaved status update while `Reconcile` is provably
	// blocked between its `Store.ListSessions` snapshot (which already ran,
	// by construction: `TMux.List` is the very next call the reconcile pass
	// makes) and the promotion write further down. `started` proves this
	// test did not race ahead of the block.
	body := strings.NewReplacer(
		"@LOG@", logPath, "@SLUG@", session.Slug, "@STARTED@", started, "@RELEASE@", release,
	).Replace(`#!/bin/sh
printf '%s\n' "$*" >> '@LOG@'
for arg in "$@"; do
	case "$arg" in
	list-sessions)
		: > '@STARTED@'
		i=0
		while [ ! -f '@RELEASE@' ]; do
			i=$((i + 1))
			if [ "$i" -gt 200 ]; then
				exit 1
			fi
			sleep 0.05
		done
		printf 'deck_@SLUG@\n'
		exit 0
		;;
	list-panes)
		printf '%s\n' '%1|/tmp|4242|0|||sh|80|24'
		exit 0
		;;
	esac
done
exit 0
`)
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}

	svc := Service{Store: db, TMux: tmux.Client{Binary: script, Socket: "lost-update"}, Audit: logger, Clock: clock}

	reconcileErr := make(chan error, 1)
	go func() {
		reconcileErr <- svc.Reconcile(context.Background())
	}()

	if !waitForFile(t, started, 5*time.Second) {
		t.Fatalf("fake tmux list-sessions was never invoked; the test never armed its rendezvous")
	}

	// The interleaved writer: a hook (or probe) reporting a crash while
	// Reconcile is provably parked inside TMux.List, after its own
	// Store.ListSessions snapshot already ran.
	if err := db.UpdateSessionStatus(context.Background(), store.StatusUpdateInput{
		SessionID: session.ID, Status: "error", Reason: "agent reported a crash mid-reconcile", Source: "hook",
		At: 2, EventKind: "hook.error",
	}); err != nil {
		t.Fatalf("interleaved status write: %v", err)
	}

	if err := os.WriteFile(release, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-reconcileErr:
		if err != nil {
			t.Fatalf("reconcile: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("reconcile did not return after the rendezvous was released")
	}

	got, err := db.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	// The interleaved "error" verdict must survive the promotion write that
	// raced it. Today it does not: the shell-liveness promotion has no
	// AllowedCurrentStatuses guard, so it clobbers the interleaved write back
	// to "running"/"tmux" regardless of what landed in between.
	if got.Status != "error" || got.StatusSource != "hook" {
		t.Fatalf("lost update: interleaved verdict status=%q source=%q was overwritten by the shell-liveness promotion, got row %#v", "error", "hook", got)
	}
}
