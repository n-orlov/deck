package features

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

// registerAttachScrollSteps backs features/attach_scroll.feature (task 116,
// requirement 48: mouse `on` must scroll an attached pane's own scrollback
// via tmux's copy-mode, never leak an Up/Down arrow into the shell's input
// line). These steps drive a real attached tmux client through the same
// PTY-level ScreenDriver the sidebar mouse steps use
// (features/mouse_synthesis_test.go) -- once attached, that PTY belongs to
// tmux's own client process, not deck's Bubble Tea program, so the wheel
// reports land on tmux directly, exactly as a real terminal's would.
func registerAttachScrollSteps(sc *godog.ScenarioContext) {
	sc.Step(`^deck client "([^"]+)" attaches to the selected session$`, clientAttachesToSelectedSession)
	sc.Step(`^deck client "([^"]+)" fills the attached pane with more than one screen of scrollback$`, clientFillsAttachedPaneWithScrollback)
	sc.Step(`^deck client "([^"]+)" scrolls the wheel up (\d+) times over the attached pane at column (\d+) row (\d+)$`, clientScrollsWheelUpNTimesAt)
	sc.Step(`^deck client "([^"]+)" attached pane shows the top of the scrollback$`, clientAttachedPaneShowsTopOfScrollback)
	sc.Step(`^deck client "([^"]+)" exits copy-mode on the attached pane$`, clientExitsCopyModeOnAttachedPane)

	// requirement 49 (task 117): capture-pane while a client is scrolled
	// back in copy-mode.
	sc.Step(`^probe fixture agents for attach-scroll are configured$`, configureAttachScrollProbeScenario)
	sc.Step(`^deck client "([^"]+)" scrolls the wheel up (\d+) times over the attached pane at column (\d+) row (\d+) until it shows "([^"]+)"$`, clientScrollsWheelUpNTimesAtUntil)
	sc.Step(`^deck client "([^"]+)" attached pane shows "([^"]+)"$`, clientAttachedPaneShowsText)
}

// configureAttachScrollProbeScenario is requirement 49's own, deliberately
// smaller cousin of features/status_probe_test.go's configureProbeScenario:
// it needs a claude fixture that can render probe golden fixtures and a
// short stale_after, but none of that scenario's frozen-clock/SIGUSR1/
// capture-race wrapper machinery -- this scenario never races a hook
// against a probe, so real wall-clock time against a short stale_after is
// simpler and just as deterministic (the wait is bounded by the fixed
// wheel-scroll and fixture-render steps that precede the probe assertion,
// which already take longer than a one-second stale_after in practice).
func configureAttachScrollProbeScenario(ctx context.Context) error {
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
	fixtureDir := filepath.Join(root, "internal", "agent", "testdata", "probes")
	config := fmt.Sprintf("stale_after = \"1s\"\n[env]\nFAKE_AGENT_FIXTURE_DIR = %q\n", fixtureDir)
	return os.WriteFile(filepath.Join(h.Home, "config.toml"), []byte(config), 0o600)
}

// clientScrollsWheelUpNTimesAtUntil is clientScrollsWheelUpNTimesAt's
// counterpart for a scenario that needs to prove a specific, previously
// rendered probe fixture's text (not attachScrollTopMarker) has scrolled
// back into view.
func clientScrollsWheelUpNTimesAtUntil(ctx context.Context, name string, times, col, row int, want string) error {
	client, err := mouseSynthesisClient(ctx, name)
	if err != nil {
		return err
	}
	for i := 0; i < times; i++ {
		if err := client.WheelUp(col, row); err != nil {
			return fmt.Errorf("wheel-up notch %d/%d: %w", i+1, times, err)
		}
	}
	return client.WaitForFrame(ctx, false, want)
}

// clientAttachedPaneShowsText is a plain, non-polling confirmation of
// whatever the immediately preceding wait step already established (it
// never itself waits) -- kept as its own Then step purely so the scenario
// states the invariant plainly rather than relying solely on a When step's
// side effect.
func clientAttachedPaneShowsText(ctx context.Context, name, want string) error {
	client, err := mouseSynthesisClient(ctx, name)
	if err != nil {
		return err
	}
	frame := client.Frame(false)
	if !strings.Contains(frame, want) {
		return fmt.Errorf("client %q pane does not show %q:\n%s", name, want, frame)
	}
	return nil
}

// attachScrollTopMarker is echoed once, before the pane is filled with more
// than a screen of numbered lines, so a later step can prove the visible
// region actually reached back that far rather than merely moved a little.
const attachScrollTopMarker = "ATTACH_SCROLL_TOP_MARKER"

// attachScrollLineCount is comfortably more than the harness's terminal
// height (30 rows, features/pty_driver_test.go) so the loop's own output
// alone pushes attachScrollTopMarker well off the live view.
const attachScrollLineCount = 120

func clientAttachesToSelectedSession(ctx context.Context, name string) error {
	client, err := mouseSynthesisClient(ctx, name)
	if err != nil {
		return err
	}
	if err := client.Send("\r"); err != nil {
		return err
	}
	// Bubble Tea must hand the terminal to tmux before pane input is
	// meaningful, mirroring features/assertions_test.go's own attach steps.
	time.Sleep(250 * time.Millisecond)
	return client.WaitForFrameGone(ctx, false, "deck - sessions")
}

func clientFillsAttachedPaneWithScrollback(ctx context.Context, name string) error {
	client, err := mouseSynthesisClient(ctx, name)
	if err != nil {
		return err
	}
	if err := client.Send("echo " + attachScrollTopMarker + "\r"); err != nil {
		return err
	}
	if err := client.WaitForFrame(ctx, false, attachScrollTopMarker); err != nil {
		return err
	}
	lastLine := fmt.Sprintf("SCROLL_LINE_%d", attachScrollLineCount)
	command := fmt.Sprintf("for i in $(seq 1 %d); do echo SCROLL_LINE_$i; done\r", attachScrollLineCount)
	if err := client.Send(command); err != nil {
		return err
	}
	if err := client.WaitForFrame(ctx, false, lastLine); err != nil {
		return err
	}
	// Let the pane settle at its live tail before the scenario captures its
	// "before" baseline, so that baseline is the pane at rest, not mid-scroll.
	time.Sleep(100 * time.Millisecond)
	return nil
}

func clientScrollsWheelUpNTimesAt(ctx context.Context, name string, times, col, row int) error {
	client, err := mouseSynthesisClient(ctx, name)
	if err != nil {
		return err
	}
	for i := 0; i < times; i++ {
		if err := client.WheelUp(col, row); err != nil {
			return fmt.Errorf("wheel-up notch %d/%d: %w", i+1, times, err)
		}
	}
	return client.WaitForFrame(ctx, false, attachScrollTopMarker)
}

func clientAttachedPaneShowsTopOfScrollback(ctx context.Context, name string) error {
	client, err := mouseSynthesisClient(ctx, name)
	if err != nil {
		return err
	}
	frame := client.Frame(false)
	if !strings.Contains(frame, attachScrollTopMarker) {
		return fmt.Errorf("client %q pane does not show %q after scrolling up, want the pane's visible region to have moved up through the scrollback:\n%s", name, attachScrollTopMarker, frame)
	}
	return nil
}

// clientExitsCopyModeOnAttachedPane sends tmux's own copy-mode cancel key
// (q, bound by default in both the emacs and vi copy-mode keytables). This
// is a deliberate test action to return to the pane's live tail for the
// byte-identical comparison the scenario's next step makes -- not a stand-in
// for anything the product itself sends.
func clientExitsCopyModeOnAttachedPane(ctx context.Context, name string) error {
	client, err := mouseSynthesisClient(ctx, name)
	if err != nil {
		return err
	}
	if err := client.Send("q"); err != nil {
		return err
	}
	if err := client.WaitForFrameGone(ctx, false, attachScrollTopMarker); err != nil {
		return err
	}
	// Give the pane's post-cancel redraw a moment to settle before the
	// scenario's byte-identical frame comparison, mirroring
	// clientCapturesFrameAs/clientFrameStillMatchesCaptured's own settle.
	time.Sleep(100 * time.Millisecond)
	return nil
}
