package main

import (
	"context"
	"io"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/agent"
	"github.com/n-orlov/deck/internal/audit"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/service"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// TestStartupContinuationDrainsTombstoneBacklogThroughProductionWiring is
// task 214's residual of task 202/review finding 3 (R79), carried verbatim by
// operator ruling docs (see /config/amendments/001-202.md): task 202's own
// committed proof (internal/store/tombstone_sweep_test.go's
// TestSweepTombstonesReapsOneBoundedBatchPerCall) calls
// Store.DrainExpiredTombstones directly, so replacing cmd/deck's own drain
// wiring with a single bounded sweep would not fail any committed test. This
// test closes that gap by driving the EXACT two production call sites
// cmd/deck's own run() uses -- preFrameTombstoneSweep (the store-open,
// pre-first-frame call) and newTUIReconcile's returned callback (the exact
// closure passed to the TUI as its reconcile helper) -- never
// Store.DrainExpiredTombstones directly and never a test-owned
// SweepTombstones loop standing in for either one.
//
// A 450-row backlog needs three 200-row batches to clear entirely (200 +
// 200 + 50). The pre-frame call is asserted bounded to exactly the first
// batch; the reconcile callback, called once (the "first startup tick"),
// must then drive DrainExpiredTombstones's own chained batches to a clean
// zero within that SAME call -- not one batch per call -- or this test goes
// red. Swap newTUIReconcile's last line from db.DrainExpiredTombstones back
// to a single bounded db.SweepTombstones call and the reconcile call above
// only removes one more batch (250 -> 50 remaining, not 0), which the
// "zero after the first tick" assertion below catches.
func TestStartupContinuationDrainsTombstoneBacklogThroughProductionWiring(t *testing.T) {
	home, cwd := t.TempDir(), t.TempDir()
	clock, err := config.NewClock("2025-06-01T00:00:00Z", "")
	if err != nil {
		t.Fatal(err)
	}
	paths := config.Paths{Home: home, LogDir: filepath.Join(home, "log"), StateDB: filepath.Join(home, "state.db")}
	db, err := store.Open(paths)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	const deleteGrace = 5 * time.Minute
	now := clock.Now().UnixMilli()
	cutoff := now - deleteGrace.Milliseconds()

	// tombstoneSweepBatchRows (internal/store) is 200; 450 needs three
	// batches (200 + 200 + 50) to drain entirely, exercising
	// DrainExpiredTombstones' own chaining rather than a single pass.
	const expiredCount = 450
	for i := 0; i < expiredCount; i++ {
		id := "startup-continuation-expired-" + strconv.Itoa(i)
		session, err := db.CreateSession(ctx, store.CreateSessionInput{
			ID: id, Name: id, CWD: cwd, Agent: "shell", CapturedPath: "/bin",
			StatusAt: 100, CreatedAt: 100,
		})
		if err != nil {
			t.Fatalf("create session %q: %v", id, err)
		}
		// deleted_at descends with i, so the highest i is the oldest
		// tombstone and must be reaped first.
		if err := db.SoftDeleteSession(ctx, session.ID, cutoff-int64(i)-1); err != nil {
			t.Fatalf("soft delete %q: %v", id, err)
		}
	}
	fresh, err := db.CreateSession(ctx, store.CreateSessionInput{
		ID: "startup-continuation-fresh", Name: "startup-continuation-fresh", CWD: cwd,
		Agent: "shell", CapturedPath: "/bin", StatusAt: 100, CreatedAt: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Stays inside the grace window measured against the SAME now used
	// throughout this test, so one cutoff protects it across both the
	// pre-frame call and the reconcile tick that follows.
	if err := db.SoftDeleteSession(ctx, fresh.ID, cutoff+1); err != nil {
		t.Fatalf("soft delete fresh control row: %v", err)
	}

	countExpiredTombstones := func() int {
		t.Helper()
		var n int
		if err := db.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions WHERE deleted_at != 0 AND id != ?`, fresh.ID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if got := countExpiredTombstones(); got != expiredCount {
		t.Fatalf("seeded expired tombstones = %d, want %d before either call runs", got, expiredCount)
	}

	logger, err := audit.New(paths, clock)
	if err != nil {
		t.Fatal(err)
	}
	// Never bootstrapped: tmux.Client.List treats "no server running" as an
	// empty liveness view (internal/tmux/tmux.go), so ReconcileWithProbes
	// below needs no live tmux session at all -- every row here is
	// tombstoned and therefore already excluded from ListSessions.
	socket := "deck-startup-continuation-" + strconv.Itoa(int(time.Now().UnixNano()))
	registry := agent.NewRegistry()
	registry.Register(agent.NewShell())
	settings := config.Settings{
		Paths: paths, Clock: clock, DeleteGrace: deleteGrace,
		StaleAfter: 45 * time.Second, EventRetentionDays: 30,
		IDs: config.NewIDGenerator(""),
	}
	sessions := service.Service{
		Store: db, TMux: tmux.Client{Socket: socket}, Audit: logger,
		Clock: clock, IDs: settings.IDs, Agents: registry,
	}

	// The EXACT production pre-frame startup sweep (cmd/deck's own run(),
	// right beside R62's EnforceEventRetention call, before the model or
	// tea.Program exist) -- bounded to exactly one batch, never the whole
	// backlog.
	preFrameTombstoneSweep(ctx, db, settings, io.Discard)
	if got, want := countExpiredTombstones(), expiredCount-200; got != want {
		t.Fatalf("expired tombstones after the pre-frame sweep = %d, want %d (exactly one 200-row batch reaped, the rest left for the first startup tick)", got, want)
	}

	// The EXACT reconcile callback/helper cmd/deck passes to the TUI
	// (newTUIReconcile's returned closure) -- called once, standing in for
	// the first startup tick.
	reconcile := newTUIReconcile(db, sessions, settings)
	if err := reconcile(ctx); err != nil {
		t.Fatalf("first startup tick's reconcile callback: %v", err)
	}
	if got := countExpiredTombstones(); got != 0 {
		t.Fatalf("expired tombstones after the first startup tick = %d, want 0 (the whole backlog must drain within the same open/startup cycle, review finding 3/R79)", got)
	}
	var freshDeleted int64
	if err := db.DB().QueryRowContext(ctx, `SELECT deleted_at FROM sessions WHERE id = ?`, fresh.ID).Scan(&freshDeleted); err != nil {
		t.Fatalf("fresh control row missing after drain: %v", err)
	}
	if freshDeleted != cutoff+1 {
		t.Fatalf("fresh control row's deleted_at = %d, want untouched at %d", freshDeleted, cutoff+1)
	}
}
