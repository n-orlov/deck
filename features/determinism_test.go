package features

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

const frozenClock = "2025-01-02T03:04:05Z"

func registerDeterminismSteps(sc *godog.ScenarioContext) {
	sc.Step(`^deck frames are byte-stable with DECK_ASCII and NO_COLOR$`, deterministicFrames)
	sc.Step(`^a stepped frozen-clock shell session is created and killed$`, frozenClockSessionIsCreatedAndKilled)
	sc.Step(`^its wall clock steps while monotonic durations advance$`, frozenAuditIsSteppedAndAdvances)
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
	client, err := h.StartNamedClient(ctx, "frozen-clock", deterministicEnvironment()...)
	if err != nil {
		return err
	}
	if err := client.WaitForFrame(ctx, true, "No sessions yet"); err != nil {
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
	// Successful creation is the normal released-binary interaction that steps
	// the frozen wall clock. The persisted row predates that step, so this
	// external frame observes the exact two-minute relative age.
	if err := client.WaitForFrame(ctx, true, "created 2m ago"); err != nil {
		return fmt.Errorf("successful creation did not step frozen relative time: %w", err)
	}
	// The delay makes the externally logged monotonic duration observably
	// advance even though every wall-clock timestamp remains frozen.
	time.Sleep(30 * time.Millisecond)
	if err := client.Send("x"); err != nil {
		return err
	}
	if err := client.WaitForFrame(ctx, true, "resumable"); err != nil {
		return err
	}
	if err := client.Send("q"); err != nil {
		return err
	}
	return client.Stop(5 * time.Second)
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
	if err := client.Send("q"); err != nil {
		return "", err
	}
	if err := client.Stop(5 * time.Second); err != nil {
		return "", err
	}
	return id, nil
}
