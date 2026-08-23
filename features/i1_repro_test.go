package features

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestI1KeystrokeDropReproduction is I-1's manual reproduction driver (task
// 004). It answers one question the three flaky @requirement-29-bulk-*
// scenarios (features/kill_delete_undo.feature) cannot answer on their own:
// when the marked-set idiom (k, m, j, m) drops one of the two marks, did the
// dropped keystroke ever reach deck's own tea.Model Update loop (task 003's
// input-count instrument, cmd/deck/inputcount_hook.go) or not? "Arrived but
// ignored" and "never arrived" point at different layers of the stack.
//
// It is deliberately NOT part of the default `go test ./...` run -- unlike
// a godog scenario it cannot be excluded by defaultTags, so it gates itself
// on an environment variable the same way ci/stability.sh's
// DECK_STABILITY_SELFTEST_FAIL knob is opt-in rather than tag-excluded:
//
//	DECK_I1_REPRO=1 ci/run.sh go test -count=1 -run TestI1KeystrokeDropReproduction -v ./features/
//
// It drives the exact idiom the scenarios use against an instrumented
// (-tags deckinputcount) binary, first at whatever load the host naturally
// has, then -- if that does not reproduce -- under synthetic CPU load from
// hog processes this test spawns and kills itself (never by pattern),
// recording /proc/loadavg and nproc for every logged attempt. See
// docs/reports/phase3d-i1-repro.log for a committed run of this test.
func TestI1KeystrokeDropReproduction(t *testing.T) {
	if os.Getenv("DECK_I1_REPRO") != "1" {
		t.Skip("opt-in reproduction driver for I-1 (task 004); set DECK_I1_REPRO=1 to run. See docs/reports/phase3d-i1-repro.log for a committed run.")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	binary := buildDeckBinaryWithTags(t, "deckinputcount")

	h, err := newScenarioHarness(binary)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := h.Close(); err != nil {
			t.Logf("harness close: %v", err)
		}
	}()

	countPath := h.Home + "/input-count"
	h.clientEnv = []string{"DECK_INPUT_COUNT_FILE=" + countPath}

	client, err := h.StartNamedClient(ctx, "A")
	if err != nil {
		t.Fatal(err)
	}
	if err := client.WaitForFrame(ctx, false, "No sessions"); err != nil {
		t.Fatal(err)
	}

	sctx := context.WithValue(ctx, scenarioHarnessKey{}, h)
	if err := clientCreatesShellSession(sctx, "A", "bk-one"); err != nil {
		t.Fatalf("create bk-one: %v", err)
	}
	if err := clientCreatesShellSession(sctx, "A", "bk-two"); err != nil {
		t.Fatalf("create bk-two: %v", err)
	}

	logf := func(format string, args ...interface{}) {
		t.Logf(format, args...)
	}

	nproc := runtime.NumCPU()
	logf("nproc=%d", nproc)

	loadavg := func() string {
		data, err := os.ReadFile("/proc/loadavg")
		if err != nil {
			return fmt.Sprintf("unreadable: %v", err)
		}
		return strings.TrimSpace(string(data))
	}
	logf("loadavg at start=%s", loadavg())

	// expected tracks exactly how many keystrokes this test has sent so
	// far (every send below is a single-rune key, weight 1 under task
	// 003's rules), so waitForInputCount(countPath, expected) answers
	// "did deck's Update loop see everything sent up to this point" at
	// any moment, independently of what the screen shows.
	expected := 0
	sendCounted := func(key string) error {
		if err := client.Send(key); err != nil {
			return err
		}
		expected++
		return nil
	}
	settle := func() { time.Sleep(120 * time.Millisecond) }

	// resetMarks returns the two-row list to "neither marked", using only
	// deterministic navigation: with exactly two rows, one "k" always
	// lands on bk-one and one "j" from bk-one always lands on bk-two,
	// regardless of where the cursor started.
	resetMarks := func() error {
		if err := sendCounted("k"); err != nil {
			return err
		}
		settle()
		if strings.Contains(client.Frame(false), "bk-one running [marked]") {
			if err := sendCounted("m"); err != nil {
				return err
			}
			settle()
		}
		if err := sendCounted("j"); err != nil {
			return err
		}
		settle()
		if strings.Contains(client.Frame(false), "bk-two running [marked]") {
			if err := sendCounted("m"); err != nil {
				return err
			}
			settle()
		}
		return nil
	}

	type reproduction struct {
		phase                 string
		attempt, nproc        int
		loadBefore, loadAfter string
		expected, delivered   int
		arrived               bool
		frame                 string
	}

	// runAttempt drives one full k/m/j/m cycle -- the exact idiom
	// @requirement-29-bulk-kill/-bulk-delete/-batch-undo use -- and
	// returns non-nil the first moment either mark fails to show up,
	// with the instrument's verdict already attached.
	runAttempt := func(phase string, i int) (*reproduction, error) {
		loadBefore := loadavg()
		if err := resetMarks(); err != nil {
			return nil, fmt.Errorf("reset marks: %w", err)
		}

		checkFailure := func(frame string) (*reproduction, error) {
			got, waitErr := waitForInputCount(countPath, expected)
			return &reproduction{
				phase: phase, attempt: i, nproc: nproc,
				loadBefore: loadBefore, loadAfter: loadavg(),
				expected: expected, delivered: got, arrived: waitErr == nil,
				frame: frame,
			}, nil
		}

		if err := sendCounted("k"); err != nil {
			return nil, err
		}
		time.Sleep(100 * time.Millisecond)
		if err := sendCounted("m"); err != nil {
			return nil, err
		}
		settle()
		frame := client.Frame(false)
		if !strings.Contains(frame, "bk-one running [marked]") {
			return checkFailure(frame)
		}

		if err := sendCounted("j"); err != nil {
			return nil, err
		}
		settle()
		frame = client.Frame(false)
		if !strings.Contains(frame, "> bk-two running") {
			return checkFailure(frame)
		}

		if err := sendCounted("m"); err != nil {
			return nil, err
		}
		settle()
		frame = client.Frame(false)
		if !strings.Contains(frame, "bk-one running [marked]") || !strings.Contains(frame, "bk-two running [marked]") {
			return checkFailure(frame)
		}
		return nil, nil
	}

	var repro *reproduction

	logf("=== phase 1: natural host load ===")
	natural := 0
	for i := 1; i <= 80 && repro == nil; i++ {
		r, err := runAttempt("natural", i)
		if err != nil {
			t.Fatalf("attempt %d: %v", i, err)
		}
		natural = i
		if i%10 == 0 {
			logf("natural attempt %d: loadavg=%s", i, loadavg())
		}
		repro = r
	}
	logf("phase 1 completed %d attempts, reproduced=%v", natural, repro != nil)

	var hogs []*exec.Cmd
	stopHogs := func() {
		for _, cmd := range hogs {
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
		}
		for _, cmd := range hogs {
			_ = cmd.Wait()
		}
		hogs = nil
	}
	defer stopHogs()

	synthetic := 0
	if repro == nil {
		logf("=== phase 2: synthetic CPU load (spawning %d hog processes, PIDs captured at spawn) ===", nproc)
		for i := 0; i < nproc; i++ {
			cmd := exec.Command("sh", "-c", "yes > /dev/null")
			if err := cmd.Start(); err != nil {
				t.Fatalf("start load hog: %v", err)
			}
			hogs = append(hogs, cmd)
		}

		deadline := time.Now().Add(3 * time.Minute)
		for time.Now().Before(deadline) && repro == nil {
			synthetic++
			r, err := runAttempt("synthetic", synthetic)
			if err != nil {
				t.Fatalf("synthetic attempt %d: %v", synthetic, err)
			}
			if synthetic%10 == 0 {
				logf("synthetic attempt %d: loadavg=%s", synthetic, loadavg())
			}
			repro = r
		}
		logf("phase 2 completed %d attempts, reproduced=%v, final loadavg=%s", synthetic, repro != nil, loadavg())
		stopHogs()
	}

	if repro == nil {
		logf("NOT REPRODUCED in this run (%d natural-load attempts, %d synthetic-load attempts).", natural, synthetic)
		return
	}

	logf("REPRODUCED: phase=%s attempt=%d nproc=%d loadavg-before=%q loadavg-after=%q",
		repro.phase, repro.attempt, repro.nproc, repro.loadBefore, repro.loadAfter)
	logf("expected keystroke total at failure = %d; instrument-reported delivered total = %d; reached-expected-within-2s-poll=%v",
		repro.expected, repro.delivered, repro.arrived)
	if repro.arrived {
		logf("VERDICT: the byte ARRIVED at deck's Update loop (instrument count reached the expected total) but the application did not act on it as expected -- an app-level drop, not a pty/tmux/bubbletea delivery drop.")
	} else {
		logf("VERDICT: the byte NEVER ARRIVED at deck's Update loop within the poll deadline (instrument count %d < expected %d) -- a delivery-layer drop below the application.", repro.delivered, repro.expected)
	}
	logf("frame at failure:\n%s", repro.frame)
}
