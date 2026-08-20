package features

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/cucumber/godog"
)

const frozenClock = "2025-01-02T03:04:05Z"

func registerDeterminismSteps(sc *godog.ScenarioContext) {
	sc.Step(`^deck frames are byte-stable with DECK_ASCII and NO_COLOR$`, deterministicFrames)
	sc.Step(`^a stepped frozen-clock shell session is created and killed$`, frozenClockSessionIsCreatedAndKilled)
	sc.Step(`^both running clients and a later hook subprocess share the stepped wall clock$`, steppedClockIsSharedWithHook)
	sc.Step(`^its audit wall clock steps on demand while monotonic durations advance$`, frozenAuditIsSteppedAndAdvances)
	sc.Step(`^repeating DECK_ID_SEED reproduces generated ids$`, repeatingSeedReproducesID)
}

func deterministicEnvironment() []string {
	// The deliberately unequal short cadences exercise both released TUI timers:
	// reconciliation reloads rows every 25ms while the preview wake-up is 65ms.
	// Animation is disabled, so neither wake-up can change a frame.
	return []string{"DECK_ASCII=1", "DECK_ANIM=0", "NO_COLOR=1", "DECK_RECONCILE_MS=25", "DECK_PREVIEW_MS=65", "DECK_CLOCK=" + frozenClock, "DECK_CLOCK_STEP=2m", "DECK_ID_SEED=determinism-seed"}
}

// deterministicFrames compares emulator-derived, padding-normalized frames
// from two separate launches. This observes the released binary rather than
// rendering a product model in-process.
func deterministicFrames(ctx context.Context) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	first, err := h.StartNamedClient(ctx, "deterministic-first", deterministicEnvironment()...)
	if err != nil {
		return err
	}
	if err := first.WaitForFrame(ctx, true, "No sessions yet"); err != nil {
		return err
	}
	frame := first.Frame(true)
	if strings.Contains(first.Raw(), "\x1b[1;36m") {
		return fmt.Errorf("NO_COLOR emitted product styling escape: %q", first.Raw())
	}
	// Wait across both unequal runtime tick intervals. With DECK_ANIM=0,
	// timer wake-ups must not alter the emulator-derived frame.
	time.Sleep(150 * time.Millisecond)
	if got := first.Frame(true); got != frame {
		return fmt.Errorf("animation-disabled frame changed across configured timer cadences\nbefore:\n%s\nafter:\n%s", frame, got)
	}
	if err := first.Send("q"); err != nil {
		return err
	}
	if err := first.Stop(5 * time.Second); err != nil {
		return fmt.Errorf("stop first deterministic client: %w", err)
	}

	second, err := h.StartNamedClient(ctx, "deterministic-second", deterministicEnvironment()...)
	if err != nil {
		return err
	}
	if err := second.WaitForFrame(ctx, true, "No sessions yet"); err != nil {
		return err
	}
	if got := second.Frame(true); got != frame {
		return fmt.Errorf("normalized deterministic frames differ\nfirst:\n%s\nsecond:\n%s", frame, got)
	}
	if strings.Contains(second.Raw(), "\x1b[1;36m") {
		return fmt.Errorf("NO_COLOR emitted product styling escape on repeat: %q", second.Raw())
	}
	if err := second.Send("q"); err != nil {
		return err
	}
	return second.Stop(5 * time.Second)
}

func frozenClockSessionIsCreatedAndKilled(ctx context.Context) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	// Keep the deterministic controls on the scenario harness, not only on
	// individual clients: the later released _hook process must join the same
	// shared clock after SIGUSR1 advances it.
	h.clientEnv = deterministicEnvironment()
	client, err := h.StartNamedClient(ctx, "frozen-clock")
	if err != nil {
		return err
	}
	if err := client.WaitForFrame(ctx, true, "No sessions yet"); err != nil {
		return err
	}
	observer, err := h.StartNamedClient(ctx, "frozen-clock-observer")
	if err != nil {
		return err
	}
	if err := observer.WaitForFrame(ctx, true, "No sessions yet"); err != nil {
		return err
	}
	cwd := filepath.Join(h.Home, "frozen-clock-cwd")
	if err := os.MkdirAll(cwd, 0o700); err != nil {
		return err
	}
	if err := client.Send("n"); err != nil {
		return err
	}
	if err := client.WaitForFrame(ctx, true, "Create shell session"); err != nil {
		return err
	}
	if err := client.Send("frozen clock"); err != nil {
		return err
	}
	time.Sleep(75 * time.Millisecond)
	if err := client.Send("\t" + cwd + "\r"); err != nil {
		return err
	}
	if err := waitForPrivateSession(ctx, "deck_frozen-clock"); err != nil {
		return err
	}
	if err := client.WaitForFrame(ctx, true, "created just now"); err != nil {
		return fmt.Errorf("creation unexpectedly stepped frozen time: %w", err)
	}
	// Exercise the production trigger while both released clients are already
	// running. The signalled client advances by exactly DECK_CLOCK_STEP; neither
	// the scenario nor an external caller calculates or writes an absolute time.
	if err := client.cmd.Process.Signal(syscall.SIGUSR1); err != nil {
		return fmt.Errorf("signal shared frozen-clock step: %w", err)
	}
	if err := client.WaitForFrame(ctx, true, "created 2m ago"); err != nil {
		return fmt.Errorf("running client did not read shared frozen now: %w", err)
	}
	if err := observer.WaitForFrame(ctx, true, "created 2m ago"); err != nil {
		return fmt.Errorf("already-running observer did not read shared frozen now: %w", err)
	}
	// Start a later released deck _hook subprocess only after both clients have
	// rendered the stepped age. Hooks intentionally reject shell rows, so create
	// an external Claude target in the already-bootstrapped scenario store.
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	// Seed the row at the unstepped time. The subsequent assertion reads the
	// hook's durable event, so it cannot pass merely because fixture status_at
	// already happened to equal the expected stepped timestamp.
	fixtureAt := time.Date(2025, time.January, 2, 3, 4, 5, 0, time.UTC).UnixMilli()
	_, insertErr := db.ExecContext(ctx, `INSERT INTO sessions
		(id, name, slug, cwd, agent, captured_path, conversation_id,
		 status, status_source, status_at, created_at)
		VALUES ('clock-step-hook', 'clock step hook', 'clock-step-hook', ?, 'claude', ?,
		 'clock-step-conversation', 'starting', 'user', ?, ?)`, h.Home, os.Getenv("PATH"), fixtureAt, fixtureAt)
	closeErr := db.Close()
	if insertErr != nil {
		return fmt.Errorf("create later hook target: %w", insertErr)
	}
	if closeErr != nil {
		return closeErr
	}
	if err := releasedHookForSession(ctx, "clock step hook", `{"hook_event_name":"SessionStart","source":"clock-step-proof"}`); err != nil {
		return err
	}
	// The delay makes the externally logged monotonic duration observably
	// advance even though every wall-clock timestamp remains frozen.
	time.Sleep(30 * time.Millisecond)
	if err := observer.Send("x"); err != nil {
		return err
	}
	// "resumable" now renders on the footer for whichever row client has
	// selected (task 012): client's selection never moved off index 0, and
	// the row observer just killed with "x" (its own selected row, also
	// index 0) is the oldest of the two sessions, so it still lands at
	// index 0 in client's list too once the reload lands.
	if err := client.WaitForFrame(ctx, true, "resumable"); err != nil {
		return err
	}
	for _, running := range []*ScreenDriver{client, observer} {
		if err := running.Send("q"); err != nil {
			return err
		}
		if err := running.Stop(5 * time.Second); err != nil {
			return err
		}
	}
	return nil
}

func steppedClockIsSharedWithHook(ctx context.Context) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	var hookAt int64
	if err := db.QueryRowContext(ctx, `SELECT at FROM events
		WHERE session_id = 'clock-step-hook' AND kind = 'session_start'`).Scan(&hookAt); err != nil {
		return fmt.Errorf("read later hook event timestamp: %w", err)
	}
	want := time.Date(2025, time.January, 2, 3, 6, 5, 0, time.UTC).UnixMilli()
	if hookAt != want {
		return fmt.Errorf("later hook persisted event timestamp=%d, want stepped shared time %d", hookAt, want)
	}
	return nil
}

func frozenAuditIsSteppedAndAdvances(ctx context.Context) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	records, err := readAudit(h)
	if err != nil {
		return err
	}
	var launch, killed struct {
		timestamp string
		duration  int64
	}
	for _, record := range records {
		var event, timestamp string
		var duration int64
		_ = json.Unmarshal(record["event"], &event)
		_ = json.Unmarshal(record["timestamp"], &timestamp)
		_ = json.Unmarshal(record["duration_ms"], &duration)
		if event == "launch" {
			launch.timestamp, launch.duration = timestamp, duration
		}
		if event == "killed" {
			killed.timestamp, killed.duration = timestamp, duration
		}
	}
	const steppedClock = "2025-01-02T03:06:05Z"
	if launch.timestamp != frozenClock || killed.timestamp != steppedClock {
		return fmt.Errorf("stepped audit timestamps = launch %q, killed %q; want %q then %q", launch.timestamp, killed.timestamp, frozenClock, steppedClock)
	}
	if killed.duration <= launch.duration {
		return fmt.Errorf("monotonic audit duration did not advance: launch=%dms killed=%dms", launch.duration, killed.duration)
	}
	return nil
}

func repeatingSeedReproducesID(ctx context.Context) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	first, err := generatedIDForSeed(ctx, h.Binary, "repeatable-seed")
	if err != nil {
		return err
	}
	second, err := generatedIDForSeed(ctx, h.Binary, "repeatable-seed")
	if err != nil {
		return err
	}
	if first != second {
		return fmt.Errorf("DECK_ID_SEED generated ids differ: %q != %q", first, second)
	}
	return nil
}

// generatedIDForSeed deliberately uses a fresh home and private socket for
// each execution: equal IDs here prove repeatability across process runs, not
// merely repeated access to one in-memory generator.
func generatedIDForSeed(ctx context.Context, binary, seed string) (string, error) {
	h, err := newScenarioHarness(binary)
	if err != nil {
		return "", err
	}
	defer h.Close()
	client, err := h.StartNamedClient(ctx, "seed", "DECK_ID_SEED="+seed)
	if err != nil {
		return "", err
	}
	if err := client.WaitForFrame(ctx, false, "No sessions yet"); err != nil {
		return "", err
	}
	cwd := filepath.Join(h.Home, "seed-cwd")
	if err := os.MkdirAll(cwd, 0o700); err != nil {
		return "", err
	}
	if err := client.Send("n"); err != nil {
		return "", err
	}
	if err := client.WaitForFrame(ctx, false, "Create shell session"); err != nil {
		return "", err
	}
	if err := client.Send("seed session"); err != nil {
		return "", err
	}
	time.Sleep(75 * time.Millisecond)
	if err := client.Send("\t" + cwd + "\r"); err != nil {
		return "", err
	}
	if err := waitForPrivateSession(context.WithValue(ctx, scenarioHarnessKey{}, h), "deck_seed-session"); err != nil {
		return "", err
	}
	db, err := sql.Open("sqlite", filepath.Join(h.Home, "state.db"))
	if err != nil {
		return "", err
	}
	defer db.Close()
	var id string
	if err := db.QueryRowContext(ctx, `SELECT id FROM sessions WHERE name = 'seed session'`).Scan(&id); err != nil {
		return "", err
	}
	// tmux can exist before Bubble Tea has consumed the modal submit. Wait for
	// the list footer so q is unambiguously a quit key rather than text typed
	// into a still-closing form field.
	if err := client.WaitForFrame(ctx, false, "up/down - Enter attach"); err != nil {
		return "", err
	}
	if err := client.Send("q"); err != nil {
		return "", err
	}
	if err := client.Stop(5 * time.Second); err != nil {
		return "", err
	}
	return id, nil
}
