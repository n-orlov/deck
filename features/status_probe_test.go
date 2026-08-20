package features

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/cucumber/godog"
)

const probeClock = "2025-01-02T03:04:05Z"

func registerProbeStatusSteps(sc *godog.ScenarioContext) {
	sc.Step(`^probe fixture agents and an advanceable frozen clock are configured$`, configureProbeScenario)
	sc.Step(`^fake agent session "([^"]+)" renders golden fixture "([^"]+)"$`, renderGoldenFixture)
	sc.Step(`^fake agent session "([^"]+)" renders these exact golden fixtures:$`, renderGoldenFixtures)
	sc.Step(`^deck client "([^"]+)" creates persistent shell session "([^"]+)"$`, createPersistentShell)
	sc.Step(`^the probe event count for session "([^"]+)" is ([0-9]+)$`, probeEventCount)
	sc.Step(`^the state database session "([^"]+)" has status "([^"]+)" from "([^"]+)"$`, databaseSessionStatusSource)
	sc.Step(`^the state database session "([^"]+)" has probe status "([^"]+)" with reason "([^"]+)"$`, databaseSessionProbeStatus)
	sc.Step(`^session "([^"]+)" has one losing "([^"]+)" event$`, sessionHasOneLosingProbeEvent)
	sc.Step(`^within one configured reconcile interval deck client "([^"]+)" row "([^"]+)" contains "([^"]+)"$`, clientRowContainsWithinReconcile)
	sc.Step(`^the frozen clock advances across stale_after while a fresh hook races the next probe of "([^"]+)" from "([^"]+)"$`, raceFreshHookAgainstProbe)
}

func configureProbeScenario(ctx context.Context) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	if err := installFakeClaudeOnPATH(ctx, true); err != nil {
		return err
	}
	root, err := repositoryRoot()
	if err != nil {
		return err
	}
	pi := exec.CommandContext(ctx, "go", "build", "-o", filepath.Join(h.agentPATHDir, "pi"), "./cmd/fake-pi")
	pi.Dir = root
	if output, err := pi.CombinedOutput(); err != nil {
		return fmt.Errorf("build fake pi fixture: %w\n%s", err, output)
	}
	fixtureDir := filepath.Join(root, "internal", "agent", "testdata", "probes")
	config := fmt.Sprintf("stale_after = \"45s\"\n[env]\nFAKE_PI_COMMANDS = \"1\"\nFAKE_AGENT_FIXTURE_DIR = %q\n", fixtureDir)
	if err := os.WriteFile(filepath.Join(h.Home, "config.toml"), []byte(config), 0o600); err != nil {
		return fmt.Errorf("write probe config: %w", err)
	}

	// Only the released deck process sees this wrapper. Harness tmux commands
	// still use the real binary, which lets the race step observe and release a
	// capture at the precise precedence boundary without importing service code.
	realTMux, err := exec.LookPath("tmux")
	if err != nil {
		return err
	}
	arm := filepath.Join(h.Home, "probe-capture.arm")
	started := filepath.Join(h.Home, "probe-capture.started")
	release := filepath.Join(h.Home, "probe-capture.release")
	wrapper := fmt.Sprintf(`#!/bin/sh
arm=%q
started=%q
release=%q
capture=0
match=0
target=""
[ ! -f "$arm" ] || target=$(cat "$arm")
for arg in "$@"; do
  [ "$arg" != capture-pane ] || capture=1
  [ -z "$target" ] || [ "$arg" != "$target" ] || match=1
done
if [ "$capture" = 1 ] && [ "$match" = 1 ]; then
  : > "$started"
  i=0
  while [ ! -f "$release" ] && [ "$i" -lt 300 ]; do sleep 0.01; i=$((i+1)); done
fi
exec %q "$@"
`, arm, started, release, realTMux)
	if err := os.WriteFile(filepath.Join(h.agentPATHDir, "tmux"), []byte(wrapper), 0o700); err != nil {
		return fmt.Errorf("write probe capture wrapper: %w", err)
	}
	h.clientEnv = []string{"DECK_CLOCK=" + probeClock, "DECK_CLOCK_STEP=45s"}
	return nil
}

func renderGoldenFixtures(ctx context.Context, session string, table *godog.Table) error {
	for _, row := range table.Rows {
		if len(row.Cells) != 1 {
			return fmt.Errorf("fixture row has %d cells, want one", len(row.Cells))
		}
		if err := renderGoldenFixture(ctx, session, row.Cells[0].Value); err != nil {
			return err
		}
	}
	return nil
}

func renderGoldenFixture(ctx context.Context, session, fixture string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	slug, err := sessionSlugByName(h, session)
	if err != nil {
		return err
	}
	request, _ := json.Marshal(map[string]string{"command": "fixture", "name": fixture})
	target := "deck_" + slug
	if _, err := tmuxOutput(ctx, h, "send-keys", "-t", target, "-l", string(request)); err != nil {
		return err
	}
	if _, err := tmuxOutput(ctx, h, "send-keys", "-t", target, "Enter"); err != nil {
		return err
	}
	root, err := repositoryRoot()
	if err != nil {
		return err
	}
	golden, err := os.ReadFile(filepath.Join(root, "internal", "agent", "testdata", "probes", fixture))
	if err != nil {
		return err
	}
	want := strings.ReplaceAll(string(golden), "\r\n", "\n")
	deadline := time.Now().Add(3 * time.Second)
	for {
		captured, captureErr := tmuxOutput(ctx, h, "capture-pane", "-p", "-J", "-S", "-", "-t", target)
		if captureErr == nil && strings.Contains(strings.ReplaceAll(string(captured), "\r\n", "\n"), want) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("pane %q did not render exact golden bytes for %s; capture error=%v\npane:\n%s", target, fixture, captureErr, captured)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func createPersistentShell(ctx context.Context, clientName, name string) error {
	_, client, err := positionCreateModalOnProfileField(ctx, clientName, "shell", name, "safe")
	if err != nil {
		return err
	}
	// The ordinary interactive /bin/sh fixture can legitimately consume EOF
	// under a heavily repeated PTY suite. Pin this eligibility row to an
	// explicit long-running shell command instead.
	if err := client.Send("\t[\"-c\",\"while :; do sleep 3600; done\"]\r"); err != nil {
		return err
	}
	return client.WaitForFrame(ctx, false, "starting")
}

func probeEventCount(ctx context.Context, session string, want int) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	var got int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM events WHERE session_id = (SELECT id FROM sessions WHERE name = ?) AND kind LIKE 'probe.%'`, session).Scan(&got); err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("session %q probe event count = %d, want %d", session, got, want)
	}
	return nil
}

func databaseSessionStatusSource(ctx context.Context, name, status, source string) error {
	return waitForDatabaseVerdict(ctx, name, status, source, "", false)
}

func databaseSessionProbeStatus(ctx context.Context, name, status, reason string) error {
	return waitForDatabaseVerdict(ctx, name, status, "probe", reason, true)
}

func waitForDatabaseVerdict(ctx context.Context, name, wantStatus, wantSource, wantReason string, checkReason bool) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	deadline := time.Now().Add(3 * time.Second)
	for {
		var status, source, reason string
		err := db.QueryRowContext(ctx, `SELECT status, status_source, COALESCE(status_reason, '') FROM sessions WHERE name = ?`, name).Scan(&status, &source, &reason)
		if err == nil && status == wantStatus && source == wantSource && (!checkReason || reason == wantReason) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("session %q verdict = %q/%q reason %q, want %q/%q reason %q (err=%v)", name, status, source, reason, wantStatus, wantSource, wantReason, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func sessionHasOneLosingProbeEvent(ctx context.Context, name, kind string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	deadline := time.Now().Add(3 * time.Second)
	for {
		var count int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM events WHERE session_id = (SELECT id FROM sessions WHERE name = ?) AND kind = ?`, name, kind).Scan(&count); err != nil {
			return err
		}
		var source string
		if err := db.QueryRowContext(ctx, `SELECT status_source FROM sessions WHERE name = ?`, name).Scan(&source); err != nil {
			return err
		}
		if count == 1 && source == "hook" {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("session %q has %d %q events and source %q, want one losing-probe evidence event with durable hook source", name, count, kind, source)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func clientRowContainsWithinReconcile(ctx context.Context, clientName, rowName, want string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(scenarioReconcileInterval + 250*time.Millisecond)
	for {
		for _, line := range strings.Split(client.Frame(false), "\n") {
			if strings.Contains(line, rowName) && strings.Contains(line, want) {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("client %q row %q did not contain %q within reconcile interval\nframe:\n%s", clientName, rowName, want, client.Frame(false))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func raceFreshHookAgainstProbe(ctx context.Context, victim, emitter string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	victimSlug, err := sessionSlugByName(h, victim)
	if err != nil {
		return err
	}
	paneIDRaw, err := tmuxOutput(ctx, h, "display-message", "-p", "-t", "deck_"+victimSlug, "#{pane_id}")
	if err != nil {
		return err
	}
	client, err := h.Client("A")
	if err != nil {
		return err
	}
	// The live preview (tasks 017-021) captures whichever row is currently
	// selected using the exact same capture-pane-by-pane-ID technique the
	// probe capture below uses, and the arm/release wrapper below cannot
	// distinguish the two callers -- it just pauses the next capture-pane
	// invocation naming this pane. If the victim's own row stayed selected
	// (its natural position after creation), the preview's own frequent
	// ticks -- not the reconciler's stale-probe capture this step means to
	// intercept -- would satisfy the "started" wait below prematurely, and
	// the reconciler's own probe might never run before the hook resolves
	// the race, leaving zero losing-probe evidence. Move selection onto the
	// durable "probe shell" row first so the preview targets a pane the
	// wrapper never arms, and the arm/release handshake below can only ever
	// observe the reconciler's own probe-eligible capture of the victim.
	// selectRowByName only searches downward from wherever the cursor
	// currently sits, so rewind to the top first (there is no bound "go to
	// top" key yet) rather than assume a starting position among the
	// attention-sorted rows.
	for i := 0; i < 10; i++ {
		if err := client.Send("\x1b[A"); err != nil { // up arrow
			return err
		}
		time.Sleep(25 * time.Millisecond)
	}
	if err := selectRowByName(client, "probe shell"); err != nil {
		return fmt.Errorf("move selection off probe victim %q before arming: %w", victim, err)
	}
	arm := filepath.Join(h.Home, "probe-capture.arm")
	started := filepath.Join(h.Home, "probe-capture.started")
	release := filepath.Join(h.Home, "probe-capture.release")
	_ = os.Remove(started)
	_ = os.Remove(release)
	if err := os.WriteFile(arm, []byte(strings.TrimSpace(string(paneIDRaw))), 0o600); err != nil {
		return err
	}
	// Reach stale_after through the released client's on-demand increment. This
	// deliberately does not calculate or write an absolute clock.now value.
	if err := client.cmd.Process.Signal(syscall.SIGUSR1); err != nil {
		return fmt.Errorf("signal frozen probe clock step: %w", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(started); err == nil {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("released client did not begin targeted probe capture for %q", victim)
		}
		time.Sleep(10 * time.Millisecond)
	}

	conversation, err := sessionConversationID(h, victim)
	if err != nil {
		return err
	}
	request, _ := json.Marshal(map[string]any{"command": "hook", "event": "SessionStart", "payload": map[string]string{"session_id": conversation, "source": "fresh"}})
	emitterSlug, err := sessionSlugByName(h, emitter)
	if err != nil {
		return err
	}
	if _, err := tmuxOutput(ctx, h, "send-keys", "-t", "deck_"+emitterSlug, "-l", string(request)); err != nil {
		return err
	}
	if _, err := tmuxOutput(ctx, h, "send-keys", "-t", "deck_"+emitterSlug, "Enter"); err != nil {
		return err
	}
	if err := waitForDatabaseVerdict(ctx, victim, "running", "hook", "", false); err != nil {
		return err
	}
	if err := os.WriteFile(release, []byte("release\n"), 0o600); err != nil {
		return err
	}
	return nil
}
