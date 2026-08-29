package features

import (
	"context"
	"fmt"
	"os"
	"os/exec"
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

// attachScrollProbeStaleAfter is the stale_after configureAttachScrollProbeScenario
// writes below, named so a caller that needs to reason about the resulting
// probe/repair oscillation period (features/interactive_scroll_test.go's
// clientRowContainsAcrossSeveralProbeCycles) shares the one source of truth
// instead of re-typing the duration.
const attachScrollProbeStaleAfter = time.Second

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
	config := fmt.Sprintf("stale_after = %q\n[env]\nFAKE_AGENT_FIXTURE_DIR = %q\n", attachScrollProbeStaleAfter.String(), fixtureDir)
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
	// a is the full-attach key (task 061, PRD Part II §11.9: Enter now
	// enters interactive mode instead).
	if err := client.Send("a"); err != nil {
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
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	if err := waitForAutomaticRenameToRender(ctx, name, h.Socket); err != nil {
		return err
	}
	// Let the pane settle at its live tail before the scenario captures its
	// "before" baseline, so that baseline is the pane at rest, not mid-scroll.
	time.Sleep(100 * time.Millisecond)
	return nil
}

// waitForAutomaticRenameToRender closes task 503's race (docs/reports/
// phase3g-503-attach-scroll-sync/README.md): tmux's window_name (the field
// the status line actually renders) is a CACHED value that tmux's own
// automatic-rename hook only refreshes on specific internal events (a job
// or process-table change) -- it is not recomputed merely because a client
// left copy-mode. #{pane_current_command}, by contrast, is read live from
// /proc at query time (proven in this task's own diagnostic,
// docs/reports/phase3g-503-attach-scroll-sync/README.md: right after
// cancelling copy-mode, #{pane_current_command} already reported "sh" while
// #{window_name} was still stably reporting the copy-mode-only placeholder
// "[tmux]" -- stable for a full 300ms poll window, not merely lagging by a
// few milliseconds, because nothing was scheduled to re-check it). Waiting
// LONGER for window_name to "settle" on ground truth cannot fix that --
// there is nothing pending to settle, the hook simply never re-ran -- which
// is why this task's own first attempt at exactly that (a quiet-window
// settle check on window_name, superseded by this function) still failed
// under load. This function sidesteps automatic-rename's own timing
// entirely: it reads the live, authoritative pane_current_command and
// explicitly renames the window to match, which (proven experimentally,
// same report) pushes the change to the client's pty immediately, the same
// way any other tmux command that changes window state does -- never a
// sleep, never hoping some unrelated later event forces the flush as a
// side effect (which is exactly how this raced before: the scenario's own
// wheel-scroll gesture happened to be that unrelated forcing event, making
// the failure depend on scheduling, not on anything the scenario itself
// controls).
func waitForAutomaticRenameToRender(ctx context.Context, name, socket string) error {
	command, windowID, err := paneCurrentCommandAndWindow(ctx, socket)
	if err != nil {
		return err
	}
	if out, err := exec.CommandContext(ctx, "tmux", "-L", socket, "rename-window", "-t", windowID, command).CombinedOutput(); err != nil {
		return fmt.Errorf("tmux -L %s rename-window -t %s %q: %w: %s", socket, windowID, command, err, strings.TrimSpace(string(out)))
	}
	client, err := mouseSynthesisClient(ctx, name)
	if err != nil {
		return err
	}
	_, err = client.WaitForFrameFunc(ctx, false, func(frame string) bool {
		return strings.Contains(frame, command+"*")
	})
	if err != nil {
		return fmt.Errorf("waiting for window name %q to reach client %q's own frame: %w", command, name, err)
	}
	return nil
}

// paneCurrentCommandAndWindow reads the real tmux server's own live
// #{pane_current_command} together with the id of the window it belongs
// to, for this scenario's single window -- never the harness's own screen
// emulator, same ground-truth discipline as waitForCopyModeQueueToDrain/
// copyCursorLine below.
func paneCurrentCommandAndWindow(ctx context.Context, socket string) (command, windowID string, err error) {
	out, err := exec.CommandContext(ctx, "tmux", "-L", socket, "list-panes", "-a", "-F", "#{pane_current_command}|#{window_id}").CombinedOutput()
	if err != nil {
		return "", "", fmt.Errorf("tmux -L %s list-panes -F #{pane_current_command}|#{window_id}: %w: %s", socket, err, strings.TrimSpace(string(out)))
	}
	fields := strings.SplitN(strings.TrimSpace(string(out)), "|", 2)
	if len(fields) != 2 {
		return "", "", fmt.Errorf("tmux -L %s list-panes -F #{pane_current_command}|#{window_id}: unexpected output %q", socket, string(out))
	}
	return fields[0], fields[1], nil
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
//
// Root-caused (task 206, docs/reports/phase3d-206-attach-scroll-hang/): a
// burst of 30 unpaced wheel-up SGR reports followed immediately (within a
// few ms) by "q" occasionally (~13% isolated, loadavg uncorrelated) leaves
// the REAL tmux server's pane still in copy-mode (confirmed against the
// tmux socket directly, bypassing the harness's own screen emulator
// entirely -- #{pane_in_mode}=1 at the moment of failure -- so this is not
// an emulator desync, the cancel keystroke genuinely never registered
// against the server). The scenario's own wait for the top-of-scrollback
// marker (clientScrollsWheelUpNTimesAt's WaitForFrame) proves tmux rendered
// A frame with the marker, but not that it has finished executing every
// queued WheelUp command -- "q" can still race the tail of that queue.
//
// Any FIXED delay before "q" makes that race worse, not better: 40/40
// isolated runs with a 300ms settle inserted here still failed, on a
// different symptom (a post-cancel frame mismatch, not a hang) --
// idle dwell in copy-mode lets tmux's own position/clock indicator
// ("HH:MM:SS [line/total]", never normalized by NormalizeFrame, which only
// strips deck's own ISO timestamps) tick, and a resend-on-timeout retry
// that must itself wait out a whole WaitForFrameGone bound before checking
// ground truth inherits the same problem (3/40 in an earlier attempt at
// this fix, docs/reports/phase3d-206-attach-scroll-hang/diag3/). So this
// polls the SAME ground truth as fast as tmux itself allows, entirely
// event-driven (no sleep): before sending "q" it waits for the real
// server's own #{copy_cursor_line} to stop moving for two consecutive polls
// 5ms apart, proving the queued WheelUp commands are fully drained, then
// sends "q" into a genuinely idle copy-mode instead of a still-draining one.
func clientExitsCopyModeOnAttachedPane(ctx context.Context, name string) error {
	client, err := mouseSynthesisClient(ctx, name)
	if err != nil {
		return err
	}
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	if err := waitForCopyModeQueueToDrain(ctx, h.Socket); err != nil {
		return err
	}
	if err := client.Send("q"); err != nil {
		return err
	}
	if err := client.WaitForFrameGone(ctx, false, attachScrollTopMarker); err != nil {
		stillInMode, checkErr := paneStillInCopyMode(ctx, h.Socket)
		return fmt.Errorf("%w\nDIAGNOSTIC real tmux pane_in_mode after cancel: stillInMode=%v checkErr=%v", err, stillInMode, checkErr)
	}
	// Cancelling copy-mode is itself an automatic-rename trigger, but
	// tmux's window_name is a CACHED field (see waitForAutomaticRenameToRender
	// above): #{pane_current_command} already reports the shell's name the
	// instant copy-mode is gone, while #{window_name} -- what the status
	// line actually renders -- can go on reporting copy-mode's own
	// placeholder name indefinitely, because nothing is scheduled to
	// re-check it. Close that same gap here, before the byte-identical
	// comparison that follows.
	if err := waitForAutomaticRenameToRender(ctx, name, h.Socket); err != nil {
		return err
	}
	// Give the pane's post-cancel redraw a moment to settle before the
	// scenario's byte-identical frame comparison, mirroring
	// clientCapturesFrameAs/clientFrameStillMatchesCaptured's own settle.
	time.Sleep(100 * time.Millisecond)
	return nil
}

// copyModeDrainPollInterval is the gap between the two #{copy_cursor_line}
// reads waitForCopyModeQueueToDrain compares -- short enough that it adds no
// meaningful dwell in copy-mode of its own (task 206: any dwell of
// hundreds of ms desyncs the post-cancel frame via tmux's own ticking
// position/clock indicator), long enough that two genuinely back-to-back
// tmux commands don't look stable by accident.
const copyModeDrainPollInterval = 5 * time.Millisecond

// copyModeDrainTimeout bounds waitForCopyModeQueueToDrain. 2s is generous
// for draining a queue of 30 already-sent mouse events against a local
// tmux server; it is not a mitigation for the race itself (nothing here
// waits out a hang), only a diagnostic-carrying ceiling matching this
// package's other checkpoints.
const copyModeDrainTimeout = 2 * time.Second

// waitForCopyModeQueueToDrain polls the real tmux server (never the
// harness's screen emulator) until #{copy_cursor_line} reports the same
// value on two consecutive reads copyModeDrainPollInterval apart, proving
// the pane has stopped moving -- i.e. every WheelUp command queued ahead of
// the caller's next keystroke has actually finished executing, not merely
// that a frame containing the expected text was rendered at some point
// during the burst.
func waitForCopyModeQueueToDrain(ctx context.Context, socket string) error {
	deadline := time.Now().Add(copyModeDrainTimeout)
	var previous string
	for {
		current, err := copyCursorLine(ctx, socket)
		if err != nil {
			return err
		}
		if current == previous {
			return nil
		}
		previous = current
		if time.Now().After(deadline) {
			return fmt.Errorf("tmux -L %s pane's copy_cursor_line never stopped moving within %s (last two reads: %q then unchanged never observed)", socket, copyModeDrainTimeout, current)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(copyModeDrainPollInterval):
		}
	}
}

// copyCursorLine reads the real tmux server's own #{copy_cursor_line} for
// this scenario's single pane.
func copyCursorLine(ctx context.Context, socket string) (string, error) {
	out, err := exec.CommandContext(ctx, "tmux", "-L", socket, "list-panes", "-a", "-F", "#{copy_cursor_line}").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("tmux -L %s list-panes -F #{copy_cursor_line}: %w: %s", socket, err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// paneStillInCopyMode asks the real tmux server directly (never the
// harness's own screen emulator) whether any pane on this scenario's socket
// is still in a mode (copy-mode included) -- the ground truth task 206's
// root-cause needs to tell a genuinely dropped cancel keystroke apart from
// the emulator merely lagging behind a server that already left it.
func paneStillInCopyMode(ctx context.Context, socket string) (bool, error) {
	out, err := exec.CommandContext(ctx, "tmux", "-L", socket, "list-panes", "-a", "-F", "#{pane_in_mode}").CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("tmux -L %s list-panes -F #{pane_in_mode}: %w: %s", socket, err, strings.TrimSpace(string(out)))
	}
	return strings.Contains(string(out), "1"), nil
}
