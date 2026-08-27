package service

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/audit"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// TestReconcileRepairsTerminalRowWithLivePaneShell is SPEC §7's one
// self-healing rule for a shell row: a terminal (stopped/error) status paired
// with a live, non-dead pane is an invariant violation, not evidence a kill or
// relaunch belongs here. deck's own shell-liveness rule is the only signal a
// shell ever has, so the repair promotes straight to running, records the
// correction as an event, and leaves the pane itself untouched -- no kill, no
// respawn, no send-keys against it (proven here by the pane's PID and the
// tmux session both surviving Reconcile unchanged).
func TestReconcileRepairsTerminalRowWithLivePaneShell(t *testing.T) {
	svc, db, logger, cwd := newTerminalRepairService(t, "deck-repair-shell-")

	session, err := db.CreateSession(context.Background(), store.CreateSessionInput{
		ID: "00000000-0000-4000-8000-000000000031", Name: "orphaned shell", CWD: cwd,
		Agent: "shell", CapturedPath: "/bin", Status: "stopped", StatusSource: "tmux", StatusAt: 1, CreatedAt: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.TMux.Create(context.Background(), tmux.Launch{Slug: session.Slug, CWD: cwd, Command: []string{"/bin/sh", "-c", "sleep 30"}}); err != nil {
		t.Fatal(err)
	}
	before := onlyLivePane(t, svc.TMux, session.Slug)

	if err := svc.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	got, err := db.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "running" || got.StatusSource != "tmux" || got.StatusReason != "tmux pane is alive" {
		t.Fatalf("repaired shell row = %#v", got)
	}
	assertSingleEvent(t, db, session.ID, "tmux.shell_live")
	if !auditContains(t, logger.Path(), session.ID, "tmux.shell_live") {
		t.Fatalf("audit lacks the repair transition for %q", session.ID)
	}

	// (c) the pane was not touched: it is the same PID, still alive, in the
	// same still-present tmux session -- no kill, no respawn, no send-keys.
	after := onlyLivePane(t, svc.TMux, session.Slug)
	if after.PID != before.PID || after.Dead {
		t.Fatalf("pane was touched by the repair: before=%#v after=%#v", before, after)
	}

	// Idempotent and unleased: a second pass leaves the row and writes no
	// second correction event, because the row is no longer terminal.
	if err := svc.Reconcile(context.Background()); err != nil {
		t.Fatalf("repeat reconcile: %v", err)
	}
	again, err := db.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if again.Status != got.Status || again.StatusSource != got.StatusSource || again.StatusAt != got.StatusAt {
		t.Fatalf("repeat reconcile changed the repaired row: before=%#v after=%#v", got, again)
	}
	assertSingleEvent(t, db, session.ID, "tmux.shell_live")
}

// TestReconcileRepairsTerminalRowWithLivePaneAgent is the same invariant for
// an agent row (SPEC §7: tmux never fabricates an agent's working state, even
// while repairing this violation). The only correction liveness alone
// supports is the neutral "starting" a fresh pane always begins at; hook or
// probe evidence take it from there on a later pass.
func TestReconcileRepairsTerminalRowWithLivePaneAgent(t *testing.T) {
	svc, db, logger, cwd := newTerminalRepairService(t, "deck-repair-agent-")

	session, err := db.CreateSession(context.Background(), store.CreateSessionInput{
		ID: "00000000-0000-4000-8000-000000000032", Name: "contradicted agent", CWD: cwd,
		Agent: "claude", CapturedPath: "/bin", Status: "stopped", StatusSource: "hook", StatusAt: 1, CreatedAt: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.TMux.Create(context.Background(), tmux.Launch{Slug: session.Slug, CWD: cwd, Command: []string{"/bin/sh", "-c", "sleep 30"}}); err != nil {
		t.Fatal(err)
	}
	before := onlyLivePane(t, svc.TMux, session.Slug)

	if err := svc.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	got, err := db.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "starting" || got.StatusSource != "tmux" || got.PaneExitStatus != nil {
		t.Fatalf("repaired agent row = %#v", got)
	}
	assertSingleEvent(t, db, session.ID, "tmux.terminal_pane_alive")
	if !auditContains(t, logger.Path(), session.ID, "tmux.terminal_pane_alive") {
		t.Fatalf("audit lacks the repair transition for %q", session.ID)
	}

	after := onlyLivePane(t, svc.TMux, session.Slug)
	if after.PID != before.PID || after.Dead {
		t.Fatalf("pane was touched by the repair: before=%#v after=%#v", before, after)
	}

	if err := svc.Reconcile(context.Background()); err != nil {
		t.Fatalf("repeat reconcile: %v", err)
	}
	again, err := db.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if again.Status != got.Status || again.StatusSource != got.StatusSource || again.StatusAt != got.StatusAt {
		t.Fatalf("repeat reconcile changed the repaired row: before=%#v after=%#v", got, again)
	}
	assertSingleEvent(t, db, session.ID, "tmux.terminal_pane_alive")
}

// TestReconcileStillCollectsRetainedCorpseUnderTerminalRow is the ordering
// guard the repair must not disturb: dead-pane collection decides first, so a
// genuinely dead pane under a terminal row is still collected as a crash, not
// mistaken for a live contradiction that only needed its status corrected.
func TestReconcileStillCollectsRetainedCorpseUnderTerminalRow(t *testing.T) {
	svc, db, logger, cwd := newTerminalRepairService(t, "deck-repair-corpse-")

	session, err := db.CreateSession(context.Background(), store.CreateSessionInput{
		ID: "00000000-0000-4000-8000-000000000033", Name: "retained corpse under stopped", CWD: cwd,
		Agent: "claude", CapturedPath: "/bin", Status: "stopped", StatusSource: "hook", StatusAt: 1, CreatedAt: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	const marker = "claude: died anyway"
	command := `printf '` + marker + `\n'; exit 9`
	if _, err := svc.TMux.Create(context.Background(), tmux.Launch{Slug: session.Slug, CWD: cwd, Command: []string{"/bin/sh", "-c", command}}); err != nil {
		t.Fatal(err)
	}
	waitForDeadPane(t, svc.TMux, session.Slug, 9)

	if err := svc.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	got, err := db.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "error" || got.StatusSource != "tmux" || got.PaneExitStatus == nil || *got.PaneExitStatus != 9 {
		t.Fatalf("corpse under a terminal row was not collected as a crash: %#v", got)
	}
	if !strings.Contains(got.CrashTail, marker) {
		t.Fatalf("crash tail missing marker: %q", got.CrashTail)
	}
	if !auditContains(t, logger.Path(), session.ID, "tmux.pane_dead") {
		t.Fatalf("audit lacks the collection transition for %q", session.ID)
	}
	var repairEvents int
	if err := db.DB().QueryRow(`SELECT count(*) FROM events WHERE session_id = ? AND kind = 'tmux.terminal_pane_alive'`, session.ID).Scan(&repairEvents); err != nil {
		t.Fatal(err)
	}
	if repairEvents != 0 {
		t.Fatalf("a retained corpse must never also be treated as a live contradiction, got %d repair events", repairEvents)
	}
	live, err := svc.TMux.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(live) != 0 {
		t.Fatalf("retained corpse was not killed: %#v", live)
	}
}

func newTerminalRepairService(t *testing.T, socketPrefix string) (svc Service, db *store.Store, logger *audit.Logger, cwd string) {
	t.Helper()
	home, cwd := t.TempDir(), t.TempDir()
	clock, err := config.NewClock("2025-01-02T03:04:05Z", "")
	if err != nil {
		t.Fatal(err)
	}
	paths := config.Paths{Home: home, LogDir: filepath.Join(home, "log"), StateDB: filepath.Join(home, "state.db")}
	db, err = store.Open(paths)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	logger, err = audit.New(paths, clock)
	if err != nil {
		t.Fatal(err)
	}
	socket := socketPrefix + strings.ReplaceAll(time.Now().Format("150405.000000000"), ".", "")
	t.Cleanup(func() { _ = exec.Command("tmux", "-L", socket, "kill-server").Run() })
	svc = Service{Store: db, TMux: tmux.Client{Socket: socket}, Audit: logger, Clock: clock}
	return svc, db, logger, cwd
}

// onlyLivePane fails the test unless exactly one live, non-dead pane exists
// for slug, and returns it -- the fixture for "the pane was not touched".
func onlyLivePane(t *testing.T, client tmux.Client, slug string) tmux.Pane {
	t.Helper()
	sessions, err := client.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, session := range sessions {
		if session.Name != "deck_"+slug {
			continue
		}
		if len(session.Panes) != 1 {
			t.Fatalf("session %q has %d panes, want 1", slug, len(session.Panes))
		}
		if session.Panes[0].Dead {
			t.Fatalf("session %q pane is already dead", slug)
		}
		return session.Panes[0]
	}
	t.Fatalf("session %q has no live tmux session", slug)
	return tmux.Pane{}
}

func assertSingleEvent(t *testing.T, db *store.Store, sessionID, kind string) {
	t.Helper()
	var n int
	if err := db.DB().QueryRow(`SELECT count(*) FROM events WHERE session_id = ? AND kind = ?`, sessionID, kind).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("events of kind %q for session %q = %d, want 1", kind, sessionID, n)
	}
}
