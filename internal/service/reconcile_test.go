package service

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/agent"
	"github.com/n-orlov/deck/internal/audit"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

func TestReconcilerStopsDisappearedSessionAndDoesNotRelaunchServer(t *testing.T) {
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
	socket := "deck-reconcile-" + strings.ReplaceAll(time.Now().Format("150405.000000000"), ".", "")
	t.Cleanup(func() { _ = exec.Command("tmux", "-L", socket, "kill-server").Run() })
	service := Service{Store: db, TMux: tmux.Client{Socket: socket}, Audit: logger, Clock: clock}

	alpha := reconcileSession(t, db, "00000000-0000-4000-8000-000000000021", "alpha", cwd, "shell", "running", "user")
	beta := reconcileSession(t, db, "00000000-0000-4000-8000-000000000022", "beta", cwd, "claude", "waiting", "hook")
	for _, session := range []store.Session{alpha, beta} {
		if _, err := service.TMux.Create(context.Background(), tmux.Launch{Slug: session.Slug, CWD: cwd, Command: []string{"/bin/sh", "-c", "sleep 30"}}); err != nil {
			t.Fatalf("create live tmux session %q: %v", session.Name, err)
		}
	}

	const reconcileInterval = 250 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		err := service.RunReconciler(ctx, reconcileInterval)
		if err != nil {
			t.Errorf("reconcile loop exited early: %v", err)
		}
		done <- err
	}()
	if err := service.TMux.Kill(context.Background(), alpha.Slug); err != nil {
		t.Fatal(err)
	}
	// The disappearance must be visible within the configured cadence, not
	// merely eventually after an arbitrary multi-interval grace period.
	waitForStatus(t, db, alpha.ID, "stopped", reconcileInterval)
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("reconcile loop: %v", err)
	}

	rows, err := db.ListSessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].Status != "stopped" || rows[0].StatusSource != "tmux" || rows[1].Status != "waiting" || rows[1].StatusSource != "hook" {
		t.Fatalf("reconciled rows = %#v", rows)
	}
	var events int
	if err := db.DB().QueryRow(`SELECT count(*) FROM events WHERE session_id = ? AND kind = 'tmux.session_gone'`, alpha.ID).Scan(&events); err != nil || events != 1 {
		t.Fatalf("disappearance events = %d, %v", events, err)
	}
	if !auditContains(t, logger.Path(), alpha.ID, "tmux.session_gone") {
		t.Fatalf("audit lacks disappearance transition for %q", alpha.ID)
	}

	if err := exec.Command("tmux", "-L", socket, "kill-server").Run(); err != nil {
		t.Fatalf("kill private tmux server: %v", err)
	}
	if err := service.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile killed server: %v", err)
	}
	waitForStatus(t, db, beta.ID, "stopped", 100*time.Millisecond)
	if err := exec.Command("tmux", "-L", socket, "list-sessions").Run(); err == nil {
		t.Fatal("reconciliation recreated the killed private tmux server")
	}
}

func TestReconcilePromotesOnlyLiveStartingShell(t *testing.T) {
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
	socket := "deck-shell-live-" + strings.ReplaceAll(time.Now().Format("150405.000000000"), ".", "")
	t.Cleanup(func() { _ = exec.Command("tmux", "-L", socket, "kill-server").Run() })
	svc := Service{Store: db, TMux: tmux.Client{Socket: socket}, Audit: logger, Clock: clock}

	shell := reconcileSession(t, db, "00000000-0000-4000-8000-000000000024", "live shell", cwd, "shell", "starting", "tmux")
	claude := reconcileSession(t, db, "00000000-0000-4000-8000-000000000025", "unsignalled agent", cwd, "claude", "starting", "tmux")
	for _, session := range []store.Session{shell, claude} {
		if _, err := svc.TMux.Create(context.Background(), tmux.Launch{Slug: session.Slug, CWD: cwd, Command: []string{"/bin/sh", "-c", "sleep 30"}}); err != nil {
			t.Fatalf("create live tmux session %q: %v", session.Name, err)
		}
	}

	if err := svc.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	gotShell, err := db.GetSession(context.Background(), shell.ID)
	if err != nil {
		t.Fatal(err)
	}
	gotClaude, err := db.GetSession(context.Background(), claude.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotShell.Status != "running" || gotShell.StatusSource != "tmux" || gotShell.StatusReason != "tmux pane is alive" {
		t.Fatalf("live shell verdict = %#v", gotShell)
	}
	if gotClaude.Status != "starting" || gotClaude.StatusSource != "tmux" {
		t.Fatalf("live unsignalled agent verdict = %#v", gotClaude)
	}
	var shellEvents, agentEvents int
	if err := db.DB().QueryRow(`SELECT count(*) FROM events WHERE session_id = ? AND kind = 'tmux.shell_live'`, shell.ID).Scan(&shellEvents); err != nil {
		t.Fatal(err)
	}
	if err := db.DB().QueryRow(`SELECT count(*) FROM events WHERE session_id = ? AND kind = 'tmux.shell_live'`, claude.ID).Scan(&agentEvents); err != nil {
		t.Fatal(err)
	}
	if shellEvents != 1 || agentEvents != 0 || !auditContains(t, logger.Path(), shell.ID, "tmux.shell_live") {
		t.Fatalf("shell promotion evidence: shell events=%d agent events=%d", shellEvents, agentEvents)
	}
}

func TestProbeEligibilityIncludesOnlySignalStatesAtWallClockBoundary(t *testing.T) {
	now := time.UnixMilli(100_000)
	for _, status := range []string{"starting", "running", "waiting"} {
		t.Run(status, func(t *testing.T) {
			row := store.Session{Status: status, StatusAt: now.Add(-45 * time.Second).UnixMilli()}
			if !probeEligible(row, now, 45*time.Second) {
				t.Fatalf("%s row was not eligible at stale_after boundary", status)
			}
			row.StatusAt++
			if probeEligible(row, now, 45*time.Second) {
				t.Fatalf("%s row became eligible one millisecond early", status)
			}
		})
	}
	for _, status := range []string{"idle", "error", "stopped"} {
		if probeEligible(store.Session{Status: status, StatusAt: 1}, now, time.Millisecond) {
			t.Fatalf("%s row must never be probe-eligible", status)
		}
	}
}

func TestTUIProbeReconcileUsesWallClockStalenessAndHookPrecedence(t *testing.T) {
	cwd := t.TempDir()
	svc, db, _, _ := newAgentTestService(t, nil, "probe-reconcile")
	clock, err := config.NewClock("2025-01-02T03:04:05Z", "45s")
	if err != nil {
		t.Fatal(err)
	}
	svc.Clock = clock
	now := svc.Clock.Now().UnixMilli()
	session, err := db.CreateSession(context.Background(), store.CreateSessionInput{
		ID: "00000000-0000-4000-8000-000000000026", Name: "sampled claude", CWD: cwd,
		Agent: "claude", CapturedPath: "/bin", Status: "starting", StatusSource: "tmux", StatusAt: now, CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := os.ReadFile(filepath.Join("..", "agent", "testdata", "probes", "claude", "waiting.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.TMux.Create(context.Background(), tmux.Launch{
		Slug: session.Slug, CWD: cwd,
		Command: []string{"/bin/sh", "-c", `printf '%s' "$1"; sleep 30`, "probe-fixture", string(fixture)},
	}); err != nil {
		t.Fatal(err)
	}

	const staleAfter = 45 * time.Second
	if err := svc.ReconcileWithProbes(context.Background(), staleAfter); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "starting" || got.StatusSource != "tmux" {
		t.Fatalf("early probe changed row = %#v", got)
	}
	var probeEvents int
	if err := db.DB().QueryRow(`SELECT count(*) FROM events WHERE session_id = ? AND kind LIKE 'probe.%'`, session.ID).Scan(&probeEvents); err != nil || probeEvents != 0 {
		t.Fatalf("early probe events = %d, %v; want 0", probeEvents, err)
	}

	// Advance the frozen wall clock on demand instead of sleeping 45 seconds.
	// The liveness-only path used by deck _hook must still decline pane sampling
	// even when an adapter and a classifiable fixture are present.
	if err := db.UpdateSessionStatus(context.Background(), store.StatusUpdateInput{
		SessionID: session.ID, Status: "running", Source: "hook", At: now, EventKind: "test.hook",
	}); err != nil {
		t.Fatal(err)
	}
	if got := svc.Clock.Advance().UnixMilli(); got != now+staleAfter.Milliseconds() {
		t.Fatalf("advanced wall clock = %d, want %d", got, now+staleAfter.Milliseconds())
	}
	if err := svc.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, err = db.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "running" || got.StatusSource != "hook" {
		t.Fatalf("liveness-only reconcile probed = %#v", got)
	}

	if err := svc.ReconcileWithProbes(context.Background(), staleAfter); err != nil {
		t.Fatal(err)
	}
	got, err = db.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "waiting" || got.StatusSource != "probe" || got.StatusReason != "permission prompt" {
		t.Fatalf("stale hook correction = %#v", got)
	}
	if err := db.DB().QueryRow(`SELECT count(*) FROM events WHERE session_id = ? AND kind = 'probe.waiting'`, session.ID).Scan(&probeEvents); err != nil || probeEvents != 1 {
		t.Fatalf("probe evidence = %d, %v; want 1", probeEvents, err)
	}
}

type racingProbeAdapter struct {
	agent.Adapter
	probe func() (string, string)
}

func (a racingProbeAdapter) Probe(string) (string, string) { return a.probe() }

func TestRecordedProbeLosesToHookThatBecomesFreshDuringClassification(t *testing.T) {
	cwd := t.TempDir()
	svc, db, _, _ := newAgentTestService(t, nil, "probe-race")
	now := svc.Clock.Now().UnixMilli()
	session, err := db.CreateSession(context.Background(), store.CreateSessionInput{
		ID: "00000000-0000-4000-8000-000000000027", Name: "raced probe", CWD: cwd,
		Agent: "claude", CapturedPath: "/bin", Status: "running", StatusSource: "hook", StatusAt: now - 60_000, CreatedAt: now - 60_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.TMux.Create(context.Background(), tmux.Launch{Slug: session.Slug, CWD: cwd, Command: []string{"/bin/sh", "-c", "echo pane; sleep 30"}}); err != nil {
		t.Fatal(err)
	}
	svc.Agents.Register(racingProbeAdapter{Adapter: agent.NewClaude(), probe: func() (string, string) {
		if err := db.UpdateSessionStatus(context.Background(), store.StatusUpdateInput{
			SessionID: session.ID, Status: "idle", Source: "hook", At: now, EventKind: "test.fresh_hook",
		}); err != nil {
			t.Fatal(err)
		}
		return "waiting", "permission prompt"
	}})
	if err := svc.ReconcileWithProbes(context.Background(), 45*time.Second); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "idle" || got.StatusSource != "hook" || got.StatusAt != now {
		t.Fatalf("fresh hook lost precedence = %#v", got)
	}
	var events int
	if err := db.DB().QueryRow(`SELECT count(*) FROM events WHERE session_id = ? AND kind = 'probe.waiting'`, session.ID).Scan(&events); err != nil || events != 1 {
		t.Fatalf("losing probe evidence = %d, %v; want 1", events, err)
	}
}

func TestProbeReconcileNeverProbesShell(t *testing.T) {
	cwd := t.TempDir()
	svc, db, _, _ := newAgentTestService(t, nil, "shell-no-probe")
	now := svc.Clock.Now().UnixMilli()
	session, err := db.CreateSession(context.Background(), store.CreateSessionInput{
		ID: "00000000-0000-4000-8000-000000000028", Name: "unprobeable shell", CWD: cwd,
		Agent: "shell", CapturedPath: "/bin", Status: "running", StatusSource: "tmux", StatusAt: now - 60_000, CreatedAt: now - 60_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.TMux.Create(context.Background(), tmux.Launch{Slug: session.Slug, CWD: cwd, Command: []string{"/bin/sh", "-c", "echo 'API Error:'; sleep 30"}}); err != nil {
		t.Fatal(err)
	}
	calls := 0
	svc.Agents.Register(racingProbeAdapter{Adapter: agent.NewShell(), probe: func() (string, string) {
		calls++
		return "error", "must not run"
	}})
	if err := svc.ReconcileWithProbes(context.Background(), 45*time.Second); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatalf("shell Probe called %d times, want 0", calls)
	}
	got, err := db.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "running" || got.StatusSource != "tmux" {
		t.Fatalf("shell changed by probe reconcile = %#v", got)
	}
}

func TestReconcilerCapturesAndCollectsCrashFirstWriterOnly(t *testing.T) {
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
	socket := "deck-crash-" + strings.ReplaceAll(time.Now().Format("150405.000000000"), ".", "")
	t.Cleanup(func() { _ = exec.Command("tmux", "-L", socket, "kill-server").Run() })
	svc := Service{Store: db, TMux: tmux.Client{Socket: socket}, Audit: logger, Clock: clock}

	session, err := db.CreateSession(context.Background(), store.CreateSessionInput{
		ID: "00000000-0000-4000-8000-000000000023", Name: "crasher", CWD: cwd,
		Agent: "claude", CapturedPath: "/bin", Status: "running", StatusSource: "hook", StatusAt: 1, CreatedAt: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	// More than 200 uniquely numbered lines pin both tail direction and cap.
	// SGR is interpreted by tmux and must not survive the plain crash capture.
	command := `i=1; while [ "$i" -le 207 ]; do printf 'line-%03d\n' "$i"; i=$((i+1)); done; printf '\033[31mdanger\033[0m\n'; exit 23`
	if _, err := svc.TMux.Create(context.Background(), tmux.Launch{Slug: session.Slug, CWD: cwd, Command: []string{"/bin/sh", "-c", command}}); err != nil {
		t.Fatal(err)
	}
	waitForDeadPane(t, svc.TMux, session.Slug, 23)

	// Independent clients race without a collection lease. Either may win the
	// durable artifact and either may find that the other's idempotent kill won.
	secondDB, err := store.Open(paths)
	if err != nil {
		t.Fatal(err)
	}
	defer secondDB.Close()
	second := svc
	second.Store = secondDB
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, reconciler := range []Service{svc, second} {
		wg.Add(1)
		go func(reconciler Service) {
			defer wg.Done()
			errs <- reconciler.Reconcile(context.Background())
		}(reconciler)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent reconcile: %v", err)
		}
	}

	got, err := db.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "error" || got.StatusSource != "tmux" || got.PaneExitStatus == nil || *got.PaneExitStatus != 23 {
		t.Fatalf("crash verdict = %#v", got)
	}
	lines := strings.Split(got.CrashTail, "\n")
	if len(lines) != 200 || lines[0] != "line-011" || !strings.Contains(got.CrashTail, "danger") || !strings.Contains(lines[len(lines)-1], "Pane is dead (status 23") {
		t.Fatalf("crash tail has %d lines, first=%q last=%q", len(lines), lines[0], lines[len(lines)-1])
	}
	if strings.ContainsRune(got.CrashTail, '\x1b') {
		t.Fatalf("crash tail contains ANSI escape: %q", got.CrashTail)
	}
	live, err := svc.TMux.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(live) != 0 {
		t.Fatalf("dead tmux session was not collected: %#v", live)
	}

	// A later pass sees the deliberately absent tmux session but preserves the
	// terminal crash verdict and first writer's artifact.
	if err := svc.Reconcile(context.Background()); err != nil {
		t.Fatalf("repeat reconcile: %v", err)
	}
	after, err := db.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Status != "error" || after.PaneExitStatus == nil || *after.PaneExitStatus != 23 || after.CrashTail != got.CrashTail {
		t.Fatalf("repeat reconcile changed crash artifact: %#v", after)
	}
}

func TestReconcileWithinBoundsAStalledTmuxCommand(t *testing.T) {
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
	defer db.Close()
	logger, err := audit.New(paths, clock)
	if err != nil {
		t.Fatal(err)
	}
	stalled := filepath.Join(home, "stalled-tmux")
	if err := os.WriteFile(stalled, []byte("#!/bin/sh\nexec sleep 10\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	svc := Service{Store: db, TMux: tmux.Client{Binary: stalled, Socket: "stalled", Timeout: time.Hour}, Audit: logger, Clock: clock}

	started := time.Now()
	err = svc.ReconcileWithin(context.Background(), 30*time.Millisecond)
	elapsed := time.Since(started)
	if err == nil {
		t.Fatal("bounded reconcile unexpectedly succeeded")
	}
	if elapsed >= time.Second {
		t.Fatalf("stalled tmux held liveness pass for %s, want < 1s", elapsed)
	}
}

func TestCrashTailStripsControlsAndKeepsLast200Lines(t *testing.T) {
	var captured strings.Builder
	for i := 1; i <= 202; i++ {
		fmt.Fprintf(&captured, "line-%03d\n", i)
	}
	captured.WriteString("\x1b[31mred\x1b[0m\x00\n\x1b]0;hostile title\x07safe\x7f\n")
	got := crashTail([]byte(captured.String()), 200)
	lines := strings.Split(got, "\n")
	if len(lines) != 200 || lines[0] != "line-005" || lines[198] != "red" || lines[199] != "safe" {
		t.Fatalf("sanitized tail (%d lines): first=%q final=%q", len(lines), lines[0], lines[len(lines)-2:])
	}
	if strings.ContainsAny(got, "\x00\x1b\x7f") {
		t.Fatalf("sanitized tail retained controls: %q", got)
	}
}

func waitForDeadPane(t *testing.T, client tmux.Client, slug string, status int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		sessions, err := client.List(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		for _, session := range sessions {
			if session.Name != "deck_"+slug {
				continue
			}
			for _, pane := range session.Panes {
				if pane.Dead && pane.DeadStatus != nil && *pane.DeadStatus == status {
					return
				}
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("session %q did not retain dead pane with status %d", slug, status)
}

func reconcileSession(t *testing.T, db *store.Store, id, name, cwd, agent, status, source string) store.Session {
	t.Helper()
	session, err := db.CreateSession(context.Background(), store.CreateSessionInput{ID: id, Name: name, CWD: cwd, Agent: agent, CapturedPath: "/bin", Status: status, StatusSource: source, StatusAt: 1, CreatedAt: 1})
	if err != nil {
		t.Fatal(err)
	}
	return session
}

func waitForStatus(t *testing.T, db *store.Store, id, want string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		rows, err := db.ListSessions(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range rows {
			if row.ID == id && row.Status == want {
				return
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("session %q did not become %q within %s", id, want, timeout)
}

func auditContains(t *testing.T, path, sessionID, event string) bool {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), `"session_id":"`+sessionID+`"`) && strings.Contains(scanner.Text(), `"event":"`+event+`"`) {
			return true
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return false
}
