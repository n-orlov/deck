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

// TestReconcileCollectsRetainedDeadPaneWhenRowAlreadyReadsStopped is the
// discriminating fixture for issue #6: the row reads stopped from a hook WHILE
// tmux still retains the dead pane. deck's server runs `remain-on-exit failed`,
// so a non-zero exit retains the pane and its session; Claude's SessionEnd hook
// (reason `other`) wrote status=stopped in the same millisecond, before any pass
// observed the corpse. The old status-first short-circuit in reconcile then
// skipped the row before tmux was ever consulted, so the corpse was never
// captured, never killed, and held the session name against every later resume.
//
// A test that merely reconciles a stopped row with no tmux session, or a crashed
// row that is still `running`, passes on the unfixed code — the hook write is
// what makes this fixture discriminating.
func TestReconcileCollectsRetainedDeadPaneWhenRowAlreadyReadsStopped(t *testing.T) {
	home, cwd := t.TempDir(), t.TempDir()
	clock, err := config.NewClock("2025-01-02T03:04:05Z", "")
	if err != nil {
		t.Fatal(err)
	}
	paths := config.Paths{Home: home, LogDir: filepath.Join(home, "log"), StateDB: filepath.Join(home, "state.db")}
	db, err := store.Open(paths)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	logger, err := audit.New(paths, clock)
	if err != nil {
		t.Fatal(err)
	}
	socket := "deck-corpse-" + strings.ReplaceAll(time.Now().Format("150405.000000000"), ".", "")
	t.Cleanup(func() { _ = exec.Command("tmux", "-L", socket, "kill-server").Run() })
	svc := Service{Store: db, TMux: tmux.Client{Socket: socket}, Audit: logger, Clock: clock}

	ctx := context.Background()
	session, err := db.CreateSession(ctx, store.CreateSessionInput{
		ID: "00000000-0000-4000-8000-000000000069", Name: "backlog-ideas", CWD: cwd,
		Agent: "claude", CapturedPath: "/bin", Status: "running", StatusSource: "hook", StatusAt: 1, CreatedAt: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	const marker = "claude: resume failed"
	command := `printf '` + marker + `\n'; exit 7`
	if _, err := svc.TMux.Create(ctx, tmux.Launch{Slug: session.Slug, CWD: cwd, Command: []string{"/bin/sh", "-c", command}}); err != nil {
		t.Fatal(err)
	}
	waitForDeadPane(t, svc.TMux, session.Slug, 7)

	// The hook half of the fixture: SessionEnd lands stopped before any pass
	// observes the retained corpse.
	if err := db.UpdateSessionStatus(ctx, store.StatusUpdateInput{
		SessionID: session.ID, Status: "stopped", Reason: "SessionEnd (other)",
		Source: "hook", At: clock.Now().UnixMilli(), EventKind: "hook.SessionEnd",
	}); err != nil {
		t.Fatal(err)
	}
	fixture, err := db.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if fixture.Status != "stopped" || fixture.StatusSource != "hook" || fixture.PaneExitStatus != nil {
		t.Fatalf("fixture is not discriminating: status=%q source=%q pane_exit_status=%v",
			fixture.Status, fixture.StatusSource, fixture.PaneExitStatus)
	}
	// ...and tmux still retains the dead pane at the moment the pass runs.
	waitForDeadPane(t, svc.TMux, session.Slug, 7)

	if err := svc.Reconcile(ctx); err != nil {
		t.Fatalf("reconcile retained corpse: %v", err)
	}

	got, err := db.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "error" {
		t.Fatalf("collected row reads %q (source %q), want error: the crash was not collected", got.Status, got.StatusSource)
	}
	if got.StatusSource != "tmux" || got.PaneExitStatus == nil || *got.PaneExitStatus != 7 {
		t.Fatalf("crash verdict = status %q source %q pane_exit_status %v", got.Status, got.StatusSource, got.PaneExitStatus)
	}
	if !strings.Contains(got.CrashTail, marker) {
		t.Fatalf("crash tail did not capture the pane output: %q", got.CrashTail)
	}
	if !strings.Contains(got.CrashTail, "Pane is dead (status 7") {
		t.Fatalf("crash tail lacks the dead-pane banner: %q", got.CrashTail)
	}
	if !auditContains(t, logger.Path(), session.ID, "tmux.pane_dead") {
		t.Fatalf("audit lacks the collection transition for %q", session.ID)
	}
	live, err := svc.TMux.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(live) != 0 {
		t.Fatalf("retained corpse was not killed: %#v", live)
	}

	// The verdict is first-writer-wins and terminal: a later pass sees the
	// deliberately absent tmux session and must not rewrite the crash as a
	// clean stop.
	if err := svc.Reconcile(ctx); err != nil {
		t.Fatalf("repeat reconcile: %v", err)
	}
	after, err := db.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Status != "error" || after.PaneExitStatus == nil || *after.PaneExitStatus != 7 || after.CrashTail != got.CrashTail {
		t.Fatalf("repeat reconcile changed the crash artifact: %#v", after)
	}
}
