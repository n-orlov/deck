package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/audit"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// paneMutatingCommands are the tmux commands SPEC §7's repair rule forbids
// against the pane it repairs around: it "touches the pane not at all".
var paneMutatingCommands = []string{"kill-session", "kill-pane", "kill-server", "respawn-pane", "respawn-window", "send-keys"}

// TestReconcileRepairsUserKilledRowWithLivePane is the invariant violation
// SPEC §7:573 names outright -- "deck being killed between a kill and its
// status write reaches the same state". The row carries the full user-kill
// verdict (stopped + killed_by_user) while the pane the kill was supposed to
// destroy is demonstrably alive. Repairing only the status would leave the
// spent flag outranking every later hook forever (§9.1), so the pass corrects
// the row, spends the flag, records the correction, and issues nothing at all
// against the pane -- proven here by a fake tmux client that records every
// invocation it receives.
func TestReconcileRepairsUserKilledRowWithLivePane(t *testing.T) {
	svc, db, logger, calls := newFakeTMuxRepairService(t, "repair-killed")

	session, err := db.CreateSession(context.Background(), store.CreateSessionInput{
		ID: "00000000-0000-4000-8000-000000000041", Name: "killed but alive", CWD: t.TempDir(),
		Agent: "claude", CapturedPath: "/bin", Status: "running", StatusSource: "hook", StatusAt: 1, CreatedAt: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	// The `x` kill's own durable write, without the tmux teardown it is
	// supposed to accompany.
	if err := db.UpdateSessionStatus(context.Background(), store.StatusUpdateInput{
		SessionID: session.ID, Status: "stopped", Reason: "killed by user", Source: "user",
		At: 2, EventKind: "user.kill", KilledByUser: true,
	}); err != nil {
		t.Fatal(err)
	}
	fakeTMuxLivePane(t, svc.TMux, session.Slug)

	if err := svc.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	got, err := db.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	// (a) the corrected status: an agent row gets the neutral starting a fresh
	// pane begins at, never a fabricated working state.
	if got.Status != "starting" || got.StatusSource != "tmux" {
		t.Fatalf("row was not repaired: status=%q source=%q (%#v)", got.Status, got.StatusSource, got)
	}
	if got.KilledByUser {
		t.Fatalf("killed_by_user survived the repair, so the row is still frozen against every hook: %#v", got)
	}
	// (b) the event.
	assertSingleEvent(t, db, session.ID, "tmux.terminal_pane_alive")
	if !auditContains(t, logger.Path(), session.ID, "tmux.terminal_pane_alive") {
		t.Fatalf("audit lacks the repair transition for %q", session.ID)
	}
	// (c) the fake tmux client received no kill/respawn/send-keys at all.
	assertNoPaneMutation(t, calls)

	// Unleased and idempotent: a second pass repairs nothing, because the row
	// is no longer terminal.
	if err := svc.Reconcile(context.Background()); err != nil {
		t.Fatalf("repeat reconcile: %v", err)
	}
	again, err := db.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if again.Status != got.Status || again.StatusSource != got.StatusSource || again.StatusAt != got.StatusAt || again.KilledByUser {
		t.Fatalf("repeat reconcile changed the repaired row: before=%#v after=%#v", got, again)
	}
	assertSingleEvent(t, db, session.ID, "tmux.terminal_pane_alive")
	assertNoPaneMutation(t, calls)
}

// TestReconcileRepairSpendsCrashVerdictOnLivePane is the other terminal
// verdict a repaired row can carry. A stored pane_exit_status describes a pane
// that died; under a live pane it is spent, and left in place it keeps the row
// terminal for every later pass -- the row would read `starting` while being
// invisible to reconciliation and unwritable by hooks, which is §9.1's
// "spent verdict outranks everything forever" bug.
func TestReconcileRepairSpendsCrashVerdictOnLivePane(t *testing.T) {
	svc, db, logger, calls := newFakeTMuxRepairService(t, "repair-crash")

	session, err := db.CreateSession(context.Background(), store.CreateSessionInput{
		ID: "00000000-0000-4000-8000-000000000042", Name: "crashed but alive", CWD: t.TempDir(),
		Agent: "claude", CapturedPath: "/bin", Status: "running", StatusSource: "hook", StatusAt: 1, CreatedAt: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	exitStatus := 9
	if err := db.UpdateSessionStatus(context.Background(), store.StatusUpdateInput{
		SessionID: session.ID, Status: "error", Reason: "tmux pane exited with status 9", Source: "tmux",
		At: 2, EventKind: "tmux.pane_dead", PaneExitStatus: &exitStatus, CrashTail: "boom",
	}); err != nil {
		t.Fatal(err)
	}
	fakeTMuxLivePane(t, svc.TMux, session.Slug)

	if err := svc.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	got, err := db.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "starting" || got.StatusSource != "tmux" {
		t.Fatalf("row was not repaired: status=%q source=%q (%#v)", got.Status, got.StatusSource, got)
	}
	if got.PaneExitStatus != nil || got.CrashTail != "" {
		t.Fatalf("spent crash verdict survived the repair: %#v", got)
	}
	assertSingleEvent(t, db, session.ID, "tmux.terminal_pane_alive")
	if !auditContains(t, logger.Path(), session.ID, "tmux.terminal_pane_alive") {
		t.Fatalf("audit lacks the repair transition for %q", session.ID)
	}
	assertNoPaneMutation(t, calls)

	if err := svc.Reconcile(context.Background()); err != nil {
		t.Fatalf("repeat reconcile: %v", err)
	}
	again, err := db.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if again.Status != got.Status || again.StatusAt != got.StatusAt || again.PaneExitStatus != nil {
		t.Fatalf("repeat reconcile changed the repaired row: before=%#v after=%#v", got, again)
	}
	assertSingleEvent(t, db, session.ID, "tmux.terminal_pane_alive")
	assertNoPaneMutation(t, calls)
}

// fakeTMuxClient is the recording tmux double: a real executable put behind
// tmux.Client.Binary, which is the seam the product already depends on (deck
// configures the binary it runs). Every invocation is appended to a log, and
// only the two read-only liveness commands answer anything -- so a repair
// that reached for kill-session, respawn-pane or send-keys would be recorded
// verbatim instead of quietly succeeding against a real server.
type fakeTMuxClient struct {
	logPath string
	script  string
}

func newFakeTMuxClient(t *testing.T, dir, slug string) *fakeTMuxClient {
	t.Helper()
	fake := &fakeTMuxClient{logPath: filepath.Join(dir, "tmux-calls.log"), script: filepath.Join(dir, "fake-tmux")}
	body := strings.NewReplacer("@LOG@", fake.logPath, "@SLUG@", slug).Replace(`#!/bin/sh
printf '%s\n' "$*" >> '@LOG@'
for arg in "$@"; do
	case "$arg" in
	list-sessions)
		printf 'deck_@SLUG@\n'
		exit 0
		;;
	list-panes)
		# pane_id|current_path|pid|dead|dead_status|dead_signal|command|width|height
		printf '%s\n' '%1|/tmp|4242|0|||sh|80|24'
		exit 0
		;;
	esac
done
exit 0
`)
	if err := os.WriteFile(fake.script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	return fake
}

// calls returns every recorded invocation's argv, one per line.
func (f *fakeTMuxClient) calls(t *testing.T) []string {
	t.Helper()
	contents, err := os.ReadFile(f.logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	var lines []string
	for _, line := range strings.Split(strings.TrimSpace(string(contents)), "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func newFakeTMuxRepairService(t *testing.T, socket string) (svc Service, db *store.Store, logger *audit.Logger, fake *fakeTMuxClient) {
	t.Helper()
	home := t.TempDir()
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
	// The slug the fake reports live is derived the same way the store does,
	// and is rewritten by fakeTMuxLivePane once the row exists.
	fake = newFakeTMuxClient(t, home, "unset")
	svc = Service{Store: db, TMux: tmux.Client{Binary: fake.script, Socket: socket}, Audit: logger, Clock: clock}
	return svc, db, logger, fake
}

// fakeTMuxLivePane points the fake tmux client at slug, so its list-sessions
// reports exactly one live deck session with one live, non-dead pane.
func fakeTMuxLivePane(t *testing.T, client tmux.Client, slug string) {
	t.Helper()
	contents, err := os.ReadFile(client.Binary)
	if err != nil {
		t.Fatal(err)
	}
	updated := strings.Replace(string(contents), "deck_unset", "deck_"+slug, 1)
	if updated == string(contents) {
		t.Fatalf("fake tmux script has no session placeholder to bind to %q", slug)
	}
	if err := os.WriteFile(client.Binary, []byte(updated), 0o700); err != nil {
		t.Fatal(err)
	}
}

func assertNoPaneMutation(t *testing.T, fake *fakeTMuxClient) {
	t.Helper()
	calls := fake.calls(t)
	var sawList bool
	for _, call := range calls {
		for _, command := range paneMutatingCommands {
			if strings.Contains(call, command) {
				t.Fatalf("the repair touched the pane: tmux %s (all calls: %q)", call, calls)
			}
		}
		if strings.Contains(call, "list-sessions") {
			sawList = true
		}
	}
	if !sawList {
		t.Fatalf("fake tmux client was never consulted, so this proves nothing: %q", calls)
	}
}
