package features

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Requirement 36: SGR (1006) mouse reporting is enabled on start and
// disabled on every exit path, including a panic. These tests drive the
// real released deck binary through a real PTY (never through deck's own
// code or logs) and read the raw byte stream bubbletea actually wrote,
// exactly like features/mouse_control_test.go's enable-on-start proof.
const (
	sgrCellMotionDisableSequence = "\x1b[?1002l"
	sgrExtModeEnableSequence     = "\x1b[?1006h"
	sgrExtModeDisableSequence    = "\x1b[?1006l"
)

// buildDeckBinaryWithTags is buildDeckBinary with extra `go build` tags. It
// exists solely for TestMouseReportingDisabledOnPanic, which needs the
// decktestpanic-tagged deliberate-panic hook (cmd/deck/testpanic_hook.go);
// every other test in this package builds the plain, tagless, released
// binary that a user actually gets.
func buildDeckBinaryWithTags(t *testing.T, tags ...string) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "deck")
	args := []string{"build"}
	if len(tags) > 0 {
		args = append(args, "-tags", strings.Join(tags, ","))
	}
	args = append(args, "-o", binary, "github.com/n-orlov/deck/cmd/deck")
	command := exec.Command("go", args...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build deck binary (tags %v): %v\n%s", tags, err, output)
	}
	return binary
}

func TestMouseReportingEnablesSGRExtendedMode(t *testing.T) {
	// The enable sequence must include 1006 (SGR extended mode), not just
	// 1002 (cell motion): 1002 alone reports mouse coordinates X10-encoded,
	// one byte per axis (32+coordinate), which silently corrupts any column
	// past 223. 1006 reports the coordinate as decimal text instead, so it
	// is what actually makes "coordinates past column 223 are handled" true
	// (features/mouse_synthesis_test.go's TestSGRMouseEncodesColumnsPastX10Ceiling
	// proves the encoding itself has no such ceiling; this proves deck asks
	// the terminal to use that encoding).
	binary := buildDeckBinary(t)
	home := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	driver, err := StartScreenDriver(ctx, binary, []string{
		"DECK_HOME=" + home,
		"DECK_TMUX_SOCKET=deck_mouse_sgr_enable_test",
		"DECK_ASCII=1", "DECK_ANIM=0", "NO_COLOR=1",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = driver.Stop(time.Second) }()

	if err := driver.WaitForFrame(ctx, false, "No sessions"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(driver.Raw(), sgrMouseEnableSequence) {
		t.Fatalf("deck did not enable cell-motion mouse reporting: %q", driver.Raw())
	}
	if !strings.Contains(driver.Raw(), sgrExtModeEnableSequence) {
		t.Fatalf("deck enabled mouse reporting without SGR extended mode (1006), so coordinates past column 223 would be corrupted: %q", driver.Raw())
	}

	if err := driver.Send("q"); err != nil {
		t.Fatalf("send q: %v", err)
	}
	if err := driver.Stop(3 * time.Second); err != nil {
		t.Fatalf("deck did not quit cleanly: %v", err)
	}
}

func TestMouseReportingDisabledOnNormalQuit(t *testing.T) {
	binary := buildDeckBinary(t)
	home := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	driver, err := StartScreenDriver(ctx, binary, []string{
		"DECK_HOME=" + home,
		"DECK_TMUX_SOCKET=deck_mouse_quit_disable_test",
		"DECK_ASCII=1", "DECK_ANIM=0", "NO_COLOR=1",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = driver.Stop(time.Second) }()

	if err := driver.WaitForFrame(ctx, false, "No sessions"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(driver.Raw(), sgrMouseEnableSequence) {
		t.Fatalf("deck never enabled mouse reporting, so disable-on-quit proves nothing: %q", driver.Raw())
	}

	if err := driver.Send("q"); err != nil {
		t.Fatalf("send q: %v", err)
	}
	if err := driver.Stop(3 * time.Second); err != nil {
		t.Fatalf("deck did not quit cleanly: %v", err)
	}
	if !strings.Contains(driver.Raw(), sgrCellMotionDisableSequence) || !strings.Contains(driver.Raw(), sgrExtModeDisableSequence) {
		t.Fatalf("deck quit without disabling mouse reporting; a shell now prints raw escapes at every mouse move: %q", driver.Raw())
	}
}

func TestMouseReportingDisabledOnSignalledExit(t *testing.T) {
	// SIGTERM is deck's "error exit" path in the sense the PRD means:
	// terminated by something other than its own quit key, the same way a
	// supervisor, `kill`, or a closing terminal ends it. Bubble Tea's signal
	// handler cancels the run context, which still walks the ordinary
	// shutdown path (disable mouse, restore terminal) rather than leaving
	// escapes enabled because the exit wasn't user-initiated.
	binary := buildDeckBinary(t)
	home := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	driver, err := StartScreenDriver(ctx, binary, []string{
		"DECK_HOME=" + home,
		"DECK_TMUX_SOCKET=deck_mouse_signal_disable_test",
		"DECK_ASCII=1", "DECK_ANIM=0", "NO_COLOR=1",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = driver.Stop(time.Second) }()

	if err := driver.WaitForFrame(ctx, false, "No sessions"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(driver.Raw(), sgrMouseEnableSequence) {
		t.Fatalf("deck never enabled mouse reporting, so disable-on-signal proves nothing: %q", driver.Raw())
	}

	if driver.cmd.Process == nil {
		t.Fatal("deck process not started")
	}
	if err := driver.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("send SIGTERM: %v", err)
	}
	if err := driver.Stop(3 * time.Second); err != nil {
		t.Fatalf("deck did not exit after SIGTERM: %v", err)
	}
	if !strings.Contains(driver.Raw(), sgrCellMotionDisableSequence) || !strings.Contains(driver.Raw(), sgrExtModeDisableSequence) {
		t.Fatalf("deck exited on SIGTERM without disabling mouse reporting: %q", driver.Raw())
	}
}

func TestMouseReportingDisabledOnPanic(t *testing.T) {
	// Builds the deck binary with -tags decktestpanic, the one build that
	// carries cmd/deck/testpanic_hook.go's deliberate-panic wiring (a no-op
	// unless DECK_TEST_PANIC_KEY is set, and never part of a release or of
	// this package's other tests). Pressing that key panics inside the same
	// Update call chain a real bug would use, so what is actually being
	// proven is that Bubble Tea's own recover-and-restore path — which deck
	// relies on rather than reimplementing — disables mouse reporting and
	// still reports the panic before the process exits.
	binary := buildDeckBinaryWithTags(t, "decktestpanic")
	home := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	driver, err := StartScreenDriver(ctx, binary, []string{
		"DECK_HOME=" + home,
		"DECK_TMUX_SOCKET=deck_mouse_panic_disable_test",
		"DECK_ASCII=1", "DECK_ANIM=0", "NO_COLOR=1",
		"DECK_TEST_PANIC_KEY=z",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = driver.Stop(time.Second) }()

	if err := driver.WaitForFrame(ctx, false, "No sessions"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(driver.Raw(), sgrMouseEnableSequence) {
		t.Fatalf("deck never enabled mouse reporting, so disable-on-panic proves nothing: %q", driver.Raw())
	}

	if err := driver.Send("z"); err != nil {
		t.Fatalf("send the deliberate-panic key: %v", err)
	}
	if err := driver.Stop(3 * time.Second); err != nil {
		t.Fatalf("deck did not exit after the deliberate panic: %v", err)
	}
	raw := driver.Raw()
	if !strings.Contains(raw, sgrCellMotionDisableSequence) || !strings.Contains(raw, sgrExtModeDisableSequence) {
		t.Fatalf("deck panicked without disabling mouse reporting: %q", raw)
	}
	if !strings.Contains(raw, "Caught panic") {
		t.Fatalf("the panic was not reported anywhere in deck's output: %q", raw)
	}
	if !strings.Contains(raw, "deliberate test panic") {
		t.Fatalf("the reported panic is not the one this test induced: %q", raw)
	}
}
