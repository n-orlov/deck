package features

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// readInputCount reads the plain decimal integer cmd/deck's -tags
// deckinputcount build (cmd/deck/inputcount_hook.go) overwrites path with
// after every key it receives. A missing file (the instrumented process has
// not received its first key yet, or was never started with
// DECK_INPUT_COUNT_FILE set) reads as 0, not an error, so a scenario can
// poll it before sending anything.
func readInputCount(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, fmt.Errorf("read input count %q: %w", path, err)
	}
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return 0, nil
	}
	total, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, fmt.Errorf("input count %q is not a bare integer: %q", path, trimmed)
	}
	return total, nil
}

// waitForInputCount is task 003's harness capability: "at any point in a
// scenario", assert how many keys the running program (started with
// DECK_INPUT_COUNT_FILE set against a -tags deckinputcount binary) has
// actually received, in the same spirit as waitForFixtureFullyRendered's
// byte-count poll (features/fake_agent_size_test.go) -- except this counts
// INPUT the process consumed, not output it produced, which is what I-1's
// keystroke-drop investigation needs: a scenario can send N keys and then
// ask whether the program's own Update loop ever saw all N, independently
// of whatever the screen ends up showing. It polls rather than sleeping a
// fixed amount because key delivery through a real pty is asynchronous with
// the driver.Send call that queued it.
func waitForInputCount(path string, want int) (int, error) {
	deadline := time.Now().Add(2 * time.Second)
	var last int
	for {
		total, err := readInputCount(path)
		if err != nil {
			return 0, err
		}
		last = total
		if total >= want {
			return total, nil
		}
		if time.Now().After(deadline) {
			return last, fmt.Errorf("timed out waiting for input count to reach %d (last seen %d)", want, last)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestInputCountInstrumentCountsRealKeystrokesThroughARealPTY is the
// harness-level half of task 003's "proves the counter is real": it drives
// an actual, released-shaped deck process (built with -tags deckinputcount
// so cmd/deck/inputcount_hook.go's wrapper is compiled in, otherwise
// identical to every other test in this package) through the same real PTY
// every scenario uses, sends real keystrokes with driver.Send, and asserts
// waitForInputCount sees exactly that many -- proving the file this test
// polls is fed by deck's actual Update loop over a real pty, not by
// anything the test harness itself controls or fabricates.
func TestInputCountInstrumentCountsRealKeystrokesThroughARealPTY(t *testing.T) {
	binary := buildDeckBinaryWithTags(t, "deckinputcount")
	home := t.TempDir()
	countPath := filepath.Join(home, "input-count")

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	driver, err := StartScreenDriver(ctx, binary, []string{
		"DECK_HOME=" + home,
		"DECK_TMUX_SOCKET=deck_input_count_test",
		"DECK_INPUT_COUNT_FILE=" + countPath,
		"DECK_ASCII=1", "DECK_ANIM=0", "NO_COLOR=1",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := driver.Stop(3 * time.Second); err != nil && !strings.Contains(err.Error(), "hung deck client") {
			t.Logf("deck exit: %v", err)
		}
	}()

	if err := driver.WaitForFrame(ctx, false, "No sessions"); err != nil {
		t.Fatal(err)
	}

	// Before any key is sent, the file must not already claim a count: a
	// hard-coded or stale counter would pass every later assertion in this
	// test by accident.
	if got, err := readInputCount(countPath); err != nil {
		t.Fatal(err)
	} else if got != 0 {
		t.Fatalf("input count before any key = %d, want 0", got)
	}

	// "j" on an empty list is a real, harmless no-op keystroke (list
	// navigation with nothing to navigate). Three of them, individually
	// paced exactly like sendClientKeys elsewhere in this package, must
	// each be counted once.
	for i := 0; i < 3; i++ {
		if err := driver.Send("j"); err != nil {
			t.Fatalf("send key %d: %v", i, err)
		}
		time.Sleep(25 * time.Millisecond)
	}
	if got, err := waitForInputCount(countPath, 3); err != nil {
		t.Fatalf("after 3 separately paced keys: %v (last seen %d)", err, got)
	} else if got != 3 {
		t.Fatalf("after 3 separately paced keys: count = %d, want exactly 3", got)
	}

	// Two more keys sent back to back with NO pacing -- exactly task 118's
	// coalescing shape -- must still add exactly 2 to the total, proving
	// this instrument counts keystrokes bubbletea's PTY reader coalesced
	// into one KeyMsg, not one per Update call.
	if err := driver.Send("j"); err != nil {
		t.Fatalf("send unpaced key 1: %v", err)
	}
	if err := driver.Send("j"); err != nil {
		t.Fatalf("send unpaced key 2: %v", err)
	}
	if got, err := waitForInputCount(countPath, 5); err != nil {
		t.Fatalf("after 2 more unpaced keys: %v (last seen %d)", err, got)
	} else if got != 5 {
		t.Fatalf("after 5 total keys: count = %d, want exactly 5", got)
	}

	if err := driver.Send("q"); err != nil {
		t.Fatal(err)
	}
}
