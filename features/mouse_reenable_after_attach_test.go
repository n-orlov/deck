package features

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// Steer 005: bubbletea's tea.ExecProcess (attachSelected, internal/tui/tui.go)
// brackets a real tmux attach with Program.ReleaseTerminal/RestoreTerminal.
// Those restore only altScreenWasActive/bpWasActive/reportFocus (bubbletea
// v1.3.10's tea.go:184-188) -- there is no mouseWasActive field anywhere in
// bubbletea v1 -- so once tmux's own detach sequence turns SGR mouse
// reporting off, nothing re-enables it for the rest of that deck process's
// life unless deck does it itself. These tests drive the real released deck
// binary through a real PTY (never through deck's own code or logs, exactly
// like features/mouse_exit_paths_test.go) and read the raw byte stream
// bubbletea actually wrote, so what is asserted is the real escape-sequence
// traffic, not whether the test harness's own synthetic mouse-click bytes
// happen to be accepted (features/mouse.feature's godog harness injects SGR
// bytes directly into deck's stdin regardless of terminal mouse-mode state,
// so it cannot distinguish a dead mouse from a live one for this bug).
func attachAndDetach(ctx context.Context, driver *ScreenDriver, name string) error {
	if err := driver.Send("n"); err != nil {
		return err
	}
	if err := driver.WaitForFrame(ctx, false, "Create shell session"); err != nil {
		return err
	}
	if err := driver.Send(name); err != nil {
		return err
	}
	time.Sleep(75 * time.Millisecond)
	if err := driver.Send("\r"); err != nil {
		return err
	}
	if err := driver.WaitForFrame(ctx, false, "starting"); err != nil {
		return err
	}
	if err := driver.Send("a"); err != nil {
		return err
	}
	// Bubble Tea must first hand the terminal to tmux before pane input is
	// meaningful (mirrors features/assertions_test.go's clientAttachesAndDetaches).
	time.Sleep(250 * time.Millisecond)
	if err := driver.Send("echo mouse-reenable-attached\r"); err != nil {
		return err
	}
	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := driver.WaitForFrame(waitCtx, false, "mouse-reenable-attached"); err != nil {
		return err
	}
	time.Sleep(100 * time.Millisecond)
	if err := driver.Send("\x02d"); err != nil {
		return err
	}
	return driver.WaitForFrame(waitCtx, false, "deck - sessions")
}

// rawAfterDetach returns the raw byte stream from tmux's own detach message
// onward, so what is asserted is only the traffic bubbletea's RestoreTerminal
// / attachFinished path emits AFTER control returns to deck -- not the
// traffic the attached tmux client itself emits for its own mouse tracking
// while it owns the terminal (tmux enables mouse tracking for its own
// pane/status-line clicks per the separate top-level tmux_mouse config key,
// regardless of [ui] mouse/DECK_MOUSE, which only gates deck's own bubbletea
// mouse capability; conflating the two periods would make either assertion
// direction vacuous).
func rawAfterDetach(driver *ScreenDriver) (string, error) {
	raw := driver.Raw()
	const marker = "[detached (from session"
	idx := strings.LastIndex(raw, marker)
	if idx < 0 {
		return "", fmt.Errorf("tmux's own detach message never appeared in the raw stream: %q", raw)
	}
	return raw[idx:], nil
}

func TestMouseReportingReenabledAfterAttachDetachCycle(t *testing.T) {
	binary := buildDeckBinary(t)
	home := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	driver, err := StartScreenDriver(ctx, binary, []string{
		"DECK_HOME=" + home,
		"DECK_TMUX_SOCKET=deck_mouse_reenable_test",
		"DECK_ASCII=1", "DECK_ANIM=0", "NO_COLOR=1",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := driver.Stop(time.Second); err != nil && !strings.Contains(err.Error(), "hung deck client") {
			t.Logf("deck exit: %v", err)
		}
	}()

	if err := driver.WaitForFrame(ctx, false, "No sessions"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(driver.Raw(), sgrMouseEnableSequence) || !strings.Contains(driver.Raw(), sgrExtModeEnableSequence) {
		t.Fatalf("deck never enabled mouse reporting at startup, so re-enable-after-attach proves nothing: %q", driver.Raw())
	}

	if err := attachAndDetach(ctx, driver, "mouse-reenable-alpha"); err != nil {
		t.Fatalf("attach and detach: %v", err)
	}

	tail, err := rawAfterDetach(driver)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(tail, sgrMouseEnableSequence) || !strings.Contains(tail, sgrExtModeEnableSequence) {
		t.Fatalf("deck did not re-enable SGR mouse reporting after a real tmux attach/detach cycle; a click after this point would be a dead no-op: %q", tail)
	}
}

func TestMouseReportingStaysOffAfterAttachDetachWithMouseDisabled(t *testing.T) {
	binary := buildDeckBinary(t)
	home := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	driver, err := StartScreenDriver(ctx, binary, []string{
		"DECK_HOME=" + home,
		"DECK_TMUX_SOCKET=deck_mouse_reenable_off_test",
		"DECK_ASCII=1", "DECK_ANIM=0", "NO_COLOR=1",
		"DECK_MOUSE=0",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := driver.Stop(time.Second); err != nil && !strings.Contains(err.Error(), "hung deck client") {
			t.Logf("deck exit: %v", err)
		}
	}()

	if err := driver.WaitForFrame(ctx, false, "No sessions"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(driver.Raw(), sgrMouseEnableSequence) || strings.Contains(driver.Raw(), sgrExtModeEnableSequence) {
		t.Fatalf("DECK_MOUSE=0 must not enable mouse reporting at startup: %q", driver.Raw())
	}

	if err := attachAndDetach(ctx, driver, "mouse-reenable-off-alpha"); err != nil {
		t.Fatalf("attach and detach: %v", err)
	}

	tail, err := rawAfterDetach(driver)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(tail, sgrMouseEnableSequence) || strings.Contains(tail, sgrExtModeEnableSequence) {
		t.Fatalf("DECK_MOUSE=0 (settings.Mouse=false) must keep mouse reporting off after an attach/detach cycle, matching the startup gate: %q", tail)
	}
}
